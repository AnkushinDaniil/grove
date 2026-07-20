package gitcli

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRunSuccess(t *testing.T) {
	repo := newRepo(t)
	r := NewRunner()

	out, err := r.Run(context.Background(), repo, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if out != "main" {
		t.Fatalf("Run() = %q, want %q", out, "main")
	}
}

func TestRunTrimsOutput(t *testing.T) {
	repo := newRepo(t)
	r := NewRunner()

	// `git status --porcelain` on a clean tree prints nothing at all, so
	// exercise trimming via a command that emits a trailing newline.
	out, err := r.Run(context.Background(), repo, "rev-list", "--count", "HEAD")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if strings.ContainsAny(out, " \t\n\r") {
		t.Fatalf("Run() output not trimmed: %q", out)
	}
}

func TestRunFailureReturnsGitError(t *testing.T) {
	repo := newRepo(t)
	r := NewRunner()

	_, err := r.Run(context.Background(), repo, "not-a-real-subcommand")
	if err == nil {
		t.Fatal("Run() error = nil, want error")
	}

	var gitErr *GitError
	if !errors.As(err, &gitErr) {
		t.Fatalf("Run() error = %T, want *GitError", err)
	}
	if len(gitErr.Args) == 0 || gitErr.Args[0] != "not-a-real-subcommand" {
		t.Errorf("GitError.Args = %v, want to start with %q", gitErr.Args, "not-a-real-subcommand")
	}
	if gitErr.Dir != repo {
		t.Errorf("GitError.Dir = %q, want %q", gitErr.Dir, repo)
	}
	if gitErr.ExitCode == 0 {
		t.Error("GitError.ExitCode = 0, want non-zero")
	}
	if gitErr.Stderr == "" {
		t.Error("GitError.Stderr is empty, want git's error message")
	}
	if !strings.Contains(gitErr.Error(), "not-a-real-subcommand") {
		t.Errorf("GitError.Error() = %q, want it to mention the failing args", gitErr.Error())
	}
}

func TestRunContextCancelled(t *testing.T) {
	repo := newRepo(t)
	r := NewRunner()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := r.Run(ctx, repo, "status")
	if err == nil {
		t.Fatal("Run() with cancelled context error = nil, want error")
	}
}

func TestRunNonexistentDir(t *testing.T) {
	r := NewRunner()

	_, err := r.Run(context.Background(), "/no/such/directory/grove-test", "status")
	if err == nil {
		t.Fatal("Run() in nonexistent dir error = nil, want error")
	}
	var gitErr *GitError
	if !errors.As(err, &gitErr) {
		t.Fatalf("Run() error = %T, want *GitError", err)
	}
}
