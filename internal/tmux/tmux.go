// Package tmux is a thin wrapper over the tmux CLI. grove hosts its interactive
// PTY sessions inside tmux so a session's child process survives daemon
// restarts: the daemon attaches a client to stream the terminal, and when the
// daemon dies the child keeps running detached, ready to be re-attached.
//
// The runner is an injectable seam so tests drive argument construction with a
// fake instead of a real tmux binary. Only NewSession/HasSession/ListGrove/
// KillSession/Version go through it; AttachCommand and ReadExitCode are pure.
package tmux

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// sessionPrefix namespaces every grove-managed tmux session so grove can find
// its own sessions among any others on the shared tmux server.
const sessionPrefix = "grove-"

// RunnerFunc executes `tmux <args...>` and returns stdout. It is the seam tests
// replace with a fake.
type RunnerFunc func(ctx context.Context, args ...string) ([]byte, error)

// Client wraps the tmux CLI. Build one with New.
type Client struct {
	run     RunnerFunc
	baseEnv func() []string
}

// Option configures a Client.
type Option func(*Client)

// WithRunner injects a custom runner, replacing the default tmux binary
// shell-out. Tests use it to capture argument construction.
func WithRunner(run RunnerFunc) Option {
	return func(c *Client) { c.run = run }
}

// WithBaseEnv sets the environment for tmux binary invocations, and thus for
// the tmux server the first new-session starts. grove passes its scrubbed
// environment so hosted children never inherit the daemon's secrets from the
// shared server's global environment.
func WithBaseEnv(env func() []string) Option {
	return func(c *Client) { c.baseEnv = env }
}

// New builds a Client. Without options it runs the real tmux binary on PATH
// with the current process environment.
func New(opts ...Option) *Client {
	c := &Client{}
	for _, opt := range opts {
		opt(c)
	}
	if c.run == nil {
		c.run = c.execRun
	}
	return c
}

// Available reports whether a tmux binary is on PATH.
func Available() bool {
	_, err := exec.LookPath("tmux")
	return err == nil
}

// SessionName returns the tmux session name hosting a grove session id. tmux
// session names cannot contain "." or ":"; grove's UUIDv7 ids contain neither,
// but any such runes are replaced defensively so the name is always valid.
func SessionName(id string) string {
	return sessionPrefix + sanitize(id)
}

// SessionID extracts the grove session id from a tmux session name, reporting
// false when name is not a grove-managed session.
func SessionID(name string) (string, bool) {
	if !strings.HasPrefix(name, sessionPrefix) {
		return "", false
	}
	return strings.TrimPrefix(name, sessionPrefix), true
}

func sanitize(s string) string {
	return strings.NewReplacer(".", "-", ":", "-").Replace(s)
}

// NewSpec describes an interactive session to launch under tmux.
type NewSpec struct {
	Name     string   // tmux session name (see SessionName)
	Argv     []string // the real command to run
	Env      []string // KEY=VAL pairs set in the session environment
	Dir      string   // working directory
	Cols     uint16
	Rows     uint16
	ExitFile string // file the wrapper writes the child's exit code to
}

// NewSession creates a detached tmux session running spec.Argv, wrapped so the
// child's exit status is recorded to spec.ExitFile when it exits. The wrapper
// runs the real command then writes $? to the exit file; because that write
// happens before the wrapping shell exits (and thus before tmux tears the
// session down), the code is durably captured for a clean child exit.
func (c *Client) NewSession(ctx context.Context, spec NewSpec) error {
	wrapped := shellJoin(spec.Argv) + `; printf %d "$?" > ` + shellQuote(spec.ExitFile)
	args := []string{
		"new-session", "-d", "-s", spec.Name,
		"-x", strconv.Itoa(int(spec.Cols)), "-y", strconv.Itoa(int(spec.Rows)),
	}
	for _, kv := range spec.Env {
		args = append(args, "-e", kv)
	}
	if spec.Dir != "" {
		args = append(args, "-c", spec.Dir)
	}
	args = append(args, "sh", "-c", wrapped)
	if _, err := c.run(ctx, args...); err != nil {
		return fmt.Errorf("tmux new-session %s: %w", spec.Name, err)
	}
	// remain-on-exit off: the session ends when the child exits (so an exited
	// child is detected via HasSession). status off: no status bar stealing a
	// row. aggressive-resize on: the pane follows the attached client's size.
	// history-limit: a deep scrollback survives re-attaches.
	for _, opt := range [][]string{
		{"set-option", "-t", spec.Name, "remain-on-exit", "off"},
		{"set-option", "-t", spec.Name, "status", "off"},
		{"set-option", "-t", spec.Name, "aggressive-resize", "on"},
		{"set-option", "-t", spec.Name, "history-limit", "50000"},
	} {
		if _, err := c.run(ctx, opt...); err != nil {
			// Don't leave a half-configured session behind.
			_ = c.KillSession(ctx, spec.Name)
			return fmt.Errorf("tmux %s: %w", strings.Join(opt, " "), err)
		}
	}
	return nil
}

// HasSession reports whether a tmux session named name exists. A "no such
// session" or "no server running" (exit 1) is a normal negative, not an error.
func (c *Client) HasSession(ctx context.Context, name string) (bool, error) {
	if _, err := c.run(ctx, "has-session", "-t", name); err != nil {
		if isExit1(err) {
			return false, nil
		}
		return false, fmt.Errorf("tmux has-session %s: %w", name, err)
	}
	return true, nil
}

// ListGroveSessions returns the names of all grove-managed tmux sessions. When
// no tmux server is running there are none, which is not an error.
func (c *Client) ListGroveSessions(ctx context.Context) ([]string, error) {
	out, err := c.run(ctx, "list-sessions", "-F", "#{session_name}")
	if err != nil {
		if isExit1(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("tmux list-sessions: %w", err)
	}
	var names []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, sessionPrefix) {
			names = append(names, line)
		}
	}
	return names, nil
}

// AttachCommand returns the argv that attaches a client to session name. The
// caller runs it under a PTY (see term.Start); the resulting client is what
// grove streams through, exactly as it would a directly-spawned child.
func AttachCommand(name string) []string {
	return []string{"tmux", "attach-session", "-t", name}
}

// KillSession destroys a tmux session, SIGHUPing its child. A missing session
// (exit 1) is treated as success.
func (c *Client) KillSession(ctx context.Context, name string) error {
	if _, err := c.run(ctx, "kill-session", "-t", name); err != nil {
		if isExit1(err) {
			return nil
		}
		return fmt.Errorf("tmux kill-session %s: %w", name, err)
	}
	return nil
}

// Version returns the tmux version string (e.g. "tmux 3.6a").
func (c *Client) Version(ctx context.Context) (string, error) {
	out, err := c.run(ctx, "-V")
	if err != nil {
		return "", fmt.Errorf("tmux -V: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// ReadExitCode reads the child exit code recorded by the NewSession wrapper.
// ok is false when the file is absent or unparseable (e.g. the child was
// SIGKILLed before the wrapper could record it).
func ReadExitCode(exitFile string) (code int, ok bool) {
	if exitFile == "" {
		return 0, false
	}
	data, err := os.ReadFile(exitFile)
	if err != nil {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, false
	}
	return n, true
}

// shellJoin shell-quotes each argv element and joins them, so a command with
// spaces or quotes survives being passed through `sh -c`.
func shellJoin(argv []string) string {
	parts := make([]string, len(argv))
	for i, a := range argv {
		parts[i] = shellQuote(a)
	}
	return strings.Join(parts, " ")
}

// shellQuote single-quotes s for POSIX sh, escaping embedded single quotes via
// the standard '\” idiom.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// isExit1 reports whether err is a tmux invocation that exited with status 1,
// which tmux uses for benign negatives ("no such session", "no server running").
func isExit1(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.ExitCode == 1
}

// Error reports a failed tmux invocation.
type Error struct {
	Args     []string
	Stderr   string
	ExitCode int
}

func (e *Error) Error() string {
	msg := fmt.Sprintf("tmux %s: exit %d", strings.Join(e.Args, " "), e.ExitCode)
	if e.Stderr != "" {
		msg += ": " + e.Stderr
	}
	return msg
}

// execRun is the production RunnerFunc: it shells out to the tmux binary,
// setting the configured base environment so the server it may start does not
// inherit the daemon's secrets.
func (c *Client) execRun(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "tmux", args...) //nolint:gosec // G204: binary is the fixed literal "tmux"; args are internal command construction, not raw external input.
	if c.baseEnv != nil {
		cmd.Env = c.baseEnv()
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.Bytes(), &Error{
			Args:     args,
			ExitCode: exitCode(err),
			Stderr:   strings.TrimSpace(stderr.String()),
		}
	}
	return stdout.Bytes(), nil
}

// exitCode extracts the process exit code from a *exec.ExitError, or -1 when the
// command never produced one (failed to start, or the context ended first).
func exitCode(err error) int {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return -1
}
