// Package github wraps the `gh` CLI for grove's Review Radar. The daemon stores
// no GitHub tokens; every call shells out to the user's existing `gh` login. The
// runner is an injectable seam so tests drive the wrapper with canned JSON
// instead of a real gh binary.
package github

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// defaultTimeout bounds a single gh invocation when the caller's context
// carries no deadline of its own.
const defaultTimeout = 20 * time.Second

// RunnerFunc executes `gh <args...>` with dir as the working directory and
// returns stdout. It is the seam tests replace with a fake.
type RunnerFunc func(ctx context.Context, dir string, args ...string) ([]byte, error)

// Client wraps the gh CLI. The zero value is not usable; build one with New.
type Client struct {
	run    RunnerFunc
	logger *slog.Logger

	mu        sync.Mutex
	login     string            // cached gh login, resolved once per process on first success
	repoNames map[string]string // cached dir -> owner/repo, stable for a daemon run
}

// Option configures a Client.
type Option func(*Client)

// WithRunner injects a custom runner, replacing the default gh binary shell-out.
func WithRunner(run RunnerFunc) Option {
	return func(c *Client) { c.run = run }
}

// WithLogger sets the logger used for best-effort warnings (e.g. a per-file
// content fetch that degraded rather than failed the whole review).
func WithLogger(logger *slog.Logger) Option {
	return func(c *Client) { c.logger = logger }
}

// New builds a Client. Without options it runs the real gh binary on PATH and
// logs to slog.Default().
func New(opts ...Option) *Client {
	c := &Client{run: execRunner, logger: slog.Default(), repoNames: map[string]string{}}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// call applies the default timeout when ctx has no deadline, then runs the
// command through the (possibly injected) runner.
func (c *Client) call(ctx context.Context, dir string, args ...string) ([]byte, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultTimeout)
		defer cancel()
	}
	return c.run(ctx, dir, args...)
}

// Login returns the authenticated gh user's login, caching it on the Client:
// the login does not change over a daemon run. Only successful lookups are
// cached, so a transient failure (e.g. gh not yet authenticated) can be retried.
func (c *Client) Login(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.login != "" {
		return c.login, nil
	}
	out, err := c.call(ctx, "", "api", "user", "--jq", ".login")
	if err != nil {
		return "", fmt.Errorf("gh api user: %w", err)
	}
	login := strings.TrimSpace(string(out))
	if login == "" {
		return "", fmt.Errorf("gh api user: empty login")
	}
	c.login = login
	return login, nil
}

// RepoName returns the "owner/repo" name of the repository rooted at dir. The
// result is cached per dir: it does not change over a daemon run, so every
// review flow (PRDetail, submit, the radar poll) reuses one lookup instead of
// spawning a fresh gh each time. That both speeds things up and shrinks the
// window for a transient gh failure under load. Only successful lookups are
// cached, so a genuine failure (e.g. gh not yet authenticated) can be retried.
func (c *Client) RepoName(ctx context.Context, dir string) (string, error) {
	c.mu.Lock()
	cached, ok := c.repoNames[dir]
	c.mu.Unlock()
	if ok {
		return cached, nil
	}

	// Deliberately not holding the lock across the gh call, so concurrent
	// lookups for different dirs don't serialize (a rare duplicate lookup for
	// the same dir is harmless).
	out, err := c.call(ctx, dir, "repo", "view", "--json", "nameWithOwner", "--jq", ".nameWithOwner")
	if err != nil {
		return "", fmt.Errorf("gh repo view: %w", err)
	}
	name := strings.TrimSpace(string(out))
	if name == "" {
		return "", fmt.Errorf("gh repo view: empty nameWithOwner")
	}
	c.mu.Lock()
	c.repoNames[dir] = name
	c.mu.Unlock()
	return name, nil
}

// execRunner is the production RunnerFunc: it shells out to the gh binary.
func execRunner(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gh", args...) //nolint:gosec // G204: binary is the fixed literal "gh"; args are internal command construction, not raw external input
	cmd.Dir = dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// A dead context means gh was SIGKILLed because our context ended, not
		// because gh itself failed (it exits -1 with empty stderr, which reads as
		// a cryptic error). Surface why: a canceled request (the client/tab went
		// away mid-call) versus a real timeout.
		reason := strings.TrimSpace(stderr.String())
		switch {
		case errors.Is(ctx.Err(), context.Canceled):
			reason = "canceled (the request ended before gh finished, e.g. the tab reloaded)"
		case errors.Is(ctx.Err(), context.DeadlineExceeded):
			reason = "timed out (gh was slow, possibly under machine load)"
		}
		return nil, &GHError{Args: args, Dir: dir, ExitCode: exitCode(err), Stderr: reason}
	}
	return stdout.Bytes(), nil
}

// GHError reports a failed gh invocation.
type GHError struct {
	Args     []string
	Dir      string
	Stderr   string
	ExitCode int
}

func (e *GHError) Error() string {
	msg := fmt.Sprintf("gh %s (dir=%s): exit %d", strings.Join(e.Args, " "), e.Dir, e.ExitCode)
	if e.Stderr != "" {
		msg += ": " + e.Stderr
	}
	return msg
}

// exitCode extracts the process exit code from a *exec.ExitError, or -1 if the
// command never produced one (failed to start, or the context ended first).
func exitCode(err error) int {
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		return exitErr.ExitCode()
	}
	return -1
}
