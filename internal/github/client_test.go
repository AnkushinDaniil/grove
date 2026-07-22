package github

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// recordingRunner is a fake RunnerFunc that records every call and delegates to
// a per-test response function.
type recordingRunner struct {
	mu    sync.Mutex
	calls [][]string // args of each call, in order
	fn    func(ctx context.Context, dir string, args ...string) ([]byte, error)
}

func (r *recordingRunner) run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	r.mu.Lock()
	r.calls = append(r.calls, args)
	r.mu.Unlock()
	return r.fn(ctx, dir, args...)
}

func (r *recordingRunner) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.calls)
}

func TestLoginTrimsAndCaches(t *testing.T) {
	rr := &recordingRunner{fn: func(context.Context, string, ...string) ([]byte, error) {
		return []byte("svlachakis\n"), nil
	}}
	c := New(WithRunner(rr.run))

	got, err := c.Login(context.Background())
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if got != "svlachakis" {
		t.Errorf("Login() = %q, want %q", got, "svlachakis")
	}

	// Second call is served from cache: the runner is not invoked again.
	if _, err := c.Login(context.Background()); err != nil {
		t.Fatalf("second Login() error = %v", err)
	}
	if n := rr.count(); n != 1 {
		t.Errorf("runner called %d times, want 1 (login cached)", n)
	}
}

func TestLoginErrorNotCached(t *testing.T) {
	var calls int
	rr := &recordingRunner{fn: func(context.Context, string, ...string) ([]byte, error) {
		calls++
		if calls == 1 {
			return nil, &GHError{Args: []string{"api", "user"}, ExitCode: 1, Stderr: "not logged in"}
		}
		return []byte("octocat"), nil
	}}
	c := New(WithRunner(rr.run))

	if _, err := c.Login(context.Background()); err == nil {
		t.Fatal("first Login() error = nil, want error")
	}
	// A prior failure must not poison the cache: a retry succeeds.
	got, err := c.Login(context.Background())
	if err != nil {
		t.Fatalf("retry Login() error = %v", err)
	}
	if got != "octocat" {
		t.Errorf("Login() = %q, want %q", got, "octocat")
	}
	if rr.count() != 2 {
		t.Errorf("runner called %d times, want 2 (error retried)", rr.count())
	}
}

func TestLoginEmptyOutput(t *testing.T) {
	c := New(WithRunner(func(context.Context, string, ...string) ([]byte, error) {
		return []byte("  \n"), nil
	}))
	if _, err := c.Login(context.Background()); err == nil {
		t.Fatal("Login() with empty output error = nil, want error")
	}
}

func TestRepoName(t *testing.T) {
	rr := &recordingRunner{fn: func(_ context.Context, dir string, _ ...string) ([]byte, error) {
		if dir != "/repo" {
			t.Errorf("dir = %q, want /repo", dir)
		}
		return []byte("NethermindEth/nethermind\n"), nil
	}}
	c := New(WithRunner(rr.run))

	got, err := c.RepoName(context.Background(), "/repo")
	if err != nil {
		t.Fatalf("RepoName() error = %v", err)
	}
	if got != "NethermindEth/nethermind" {
		t.Errorf("RepoName() = %q, want NethermindEth/nethermind", got)
	}
	// Sanity-check the argv shape gh is invoked with.
	want := []string{"repo", "view", "--json", "nameWithOwner", "--jq", ".nameWithOwner"}
	if strings.Join(rr.calls[0], " ") != strings.Join(want, " ") {
		t.Errorf("args = %v, want %v", rr.calls[0], want)
	}
}

func TestRepoNameError(t *testing.T) {
	c := New(WithRunner(func(context.Context, string, ...string) ([]byte, error) {
		return nil, &GHError{Args: []string{"repo", "view"}, ExitCode: 1, Stderr: "no repo"}
	}))
	if _, err := c.RepoName(context.Background(), "/repo"); err == nil {
		t.Fatal("RepoName() error = nil, want error")
	}
}

func TestGHErrorFormat(t *testing.T) {
	withStderr := &GHError{Args: []string{"pr", "list"}, Dir: "/x", ExitCode: 2, Stderr: "boom"}
	if got := withStderr.Error(); got != "gh pr list (dir=/x): exit 2: boom" {
		t.Errorf("Error() = %q", got)
	}
	noStderr := &GHError{Args: []string{"api", "user"}, Dir: "/y", ExitCode: 1}
	if got := noStderr.Error(); got != "gh api user (dir=/y): exit 1" {
		t.Errorf("Error() = %q", got)
	}
	// It threads through errors.As from a wrapped error.
	var target *GHError
	if !errors.As(errors.Join(withStderr), &target) {
		t.Error("errors.As did not match *GHError")
	}
}

func TestCallInjectsDefaultTimeout(t *testing.T) {
	var gotDeadline bool
	c := New(WithRunner(func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
		_, gotDeadline = ctx.Deadline()
		return []byte("x"), nil
	}))
	// A context with no deadline gets the default timeout applied.
	if _, err := c.RepoName(context.Background(), "/repo"); err != nil {
		t.Fatalf("RepoName() error = %v", err)
	}
	if !gotDeadline {
		t.Error("runner ctx had no deadline, want the default timeout applied")
	}
}

func TestCallPreservesCallerDeadline(t *testing.T) {
	caller, cancel := context.WithDeadline(context.Background(), time.Now().Add(2*time.Hour))
	defer cancel()
	wantDeadline, _ := caller.Deadline()

	var got time.Time
	c := New(WithRunner(func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
		got, _ = ctx.Deadline()
		return []byte("x"), nil
	}))
	if _, err := c.RepoName(caller, "/repo"); err != nil {
		t.Fatalf("RepoName() error = %v", err)
	}
	if !got.Equal(wantDeadline) {
		t.Errorf("runner deadline = %v, want caller's %v (not overridden)", got, wantDeadline)
	}
}
