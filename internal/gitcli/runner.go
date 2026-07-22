package gitcli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Runner executes git commands via the git binary on PATH. The zero value is
// ready to use.
type Runner struct{}

// NewRunner constructs a Runner.
func NewRunner() *Runner { return &Runner{} }

// Run executes `git <args...>` with dir as the process's working directory
// and returns trimmed stdout. On failure it returns a *GitError carrying the
// exit code and stderr.
func (r *Runner) Run(ctx context.Context, dir string, args ...string) (string, error) {
	out, err := r.output(ctx, dir, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// output executes `git <args...>` in dir and returns raw, untrimmed stdout
// bytes. It is the byte-preserving core behind Run, used directly by callers
// like ShowFile that must not have file contents trimmed. On failure it returns
// a *GitError carrying the exit code and stderr.
func (r *Runner) output(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...) //nolint:gosec // G204: binary is the fixed literal "git"; args are internal command construction, not raw external input
	cmd.Dir = dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, &GitError{
			Args:     args,
			Dir:      dir,
			ExitCode: exitCode(err),
			Stderr:   strings.TrimSpace(stderr.String()),
		}
	}
	return stdout.Bytes(), nil
}

// GitError reports a failed git invocation.
type GitError struct {
	Args     []string
	Dir      string
	ExitCode int
	Stderr   string
}

func (e *GitError) Error() string {
	msg := fmt.Sprintf("git %s (dir=%s): exit %d", strings.Join(e.Args, " "), e.Dir, e.ExitCode)
	if e.Stderr != "" {
		msg += ": " + e.Stderr
	}
	return msg
}

// exitCode extracts the process exit code from a *exec.ExitError, or -1 if
// the command never produced one (e.g. it could not be started, or the
// context was cancelled first).
func exitCode(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}
