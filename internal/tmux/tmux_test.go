package tmux_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/AnkushinDaniil/grove/internal/tmux"
)

// fakeRunner records every tmux invocation and returns a programmed response,
// standing in for the real tmux binary so argument construction is asserted
// without a tmux server (mirrors internal/github's injected RunnerFunc).
type fakeRunner struct {
	calls   [][]string
	respond func(args []string) ([]byte, error)
}

func (f *fakeRunner) run(_ context.Context, args ...string) ([]byte, error) {
	f.calls = append(f.calls, slices.Clone(args))
	if f.respond != nil {
		return f.respond(args)
	}
	return nil, nil
}

func TestNewSessionArgs(t *testing.T) {
	f := &fakeRunner{}
	c := tmux.New(tmux.WithRunner(f.run))

	err := c.NewSession(t.Context(), tmux.NewSpec{
		Name:     "grove-abc",
		Argv:     []string{"claude", "--flag", "a b"},
		Env:      []string{"FOO=bar", "BAZ=1"},
		Dir:      "/work",
		Cols:     120,
		Rows:     32,
		ExitFile: "/s/abc.exit",
	})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if len(f.calls) != 5 {
		t.Fatalf("call count = %d, want 5 (new-session + 4 set-option)", len(f.calls))
	}

	wantNew := []string{
		"new-session", "-d", "-s", "grove-abc",
		"-x", "120", "-y", "32",
		"-e", "FOO=bar", "-e", "BAZ=1",
		"-c", "/work",
		"sh", "-c", `'claude' '--flag' 'a b'; printf %d "$?" > '/s/abc.exit'`,
	}
	if !slices.Equal(f.calls[0], wantNew) {
		t.Errorf("new-session args =\n  %q\nwant\n  %q", f.calls[0], wantNew)
	}

	wantOpts := [][]string{
		{"set-option", "-t", "grove-abc", "remain-on-exit", "off"},
		{"set-option", "-t", "grove-abc", "status", "off"},
		{"set-option", "-t", "grove-abc", "aggressive-resize", "on"},
		{"set-option", "-t", "grove-abc", "history-limit", "50000"},
	}
	for i, want := range wantOpts {
		if !slices.Equal(f.calls[i+1], want) {
			t.Errorf("option call %d = %q, want %q", i, f.calls[i+1], want)
		}
	}
}

func TestNewSessionOmitsDirWhenEmpty(t *testing.T) {
	f := &fakeRunner{}
	c := tmux.New(tmux.WithRunner(f.run))
	if err := c.NewSession(t.Context(), tmux.NewSpec{Name: "grove-x", Argv: []string{"cmd"}, ExitFile: "/e", Cols: 1, Rows: 1}); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	// With no Dir there is no `-c <dir>`; the only `-c` is the trailing `sh -c`.
	want := []string{
		"new-session", "-d", "-s", "grove-x", "-x", "1", "-y", "1",
		"sh", "-c", `'cmd'; printf %d "$?" > '/e'`,
	}
	if !slices.Equal(f.calls[0], want) {
		t.Errorf("new-session args =\n  %q\nwant\n  %q", f.calls[0], want)
	}
}

// TestWrapperArgvRoundTripsThroughSh runs the generated `sh -c` wrapper for real
// and checks that argv elements with spaces and quotes survive intact, and that
// the wrapper records the child's exit code. This exercises the shell quoting
// and ReadExitCode together without needing tmux.
func TestWrapperArgvRoundTripsThroughSh(t *testing.T) {
	f := &fakeRunner{}
	c := tmux.New(tmux.WithRunner(f.run))
	exit := filepath.Join(t.TempDir(), "code")

	argv := []string{"printf", "[%s]", "a b", "c'd", `e"f`}
	if err := c.NewSession(t.Context(), tmux.NewSpec{Name: "grove-q", Argv: argv, ExitFile: exit, Cols: 1, Rows: 1}); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	wrapped := f.calls[0][len(f.calls[0])-1]

	out, err := exec.CommandContext(t.Context(), "sh", "-c", wrapped).Output()
	if err != nil {
		t.Fatalf("run wrapped: %v", err)
	}
	if got, want := string(out), `[a b][c'd][e"f]`; got != want {
		t.Errorf("argv mangled through sh: got %q, want %q", got, want)
	}
	if code, ok := tmux.ReadExitCode(exit); !ok || code != 0 {
		t.Errorf("ReadExitCode = %d, %v; want 0, true", code, ok)
	}
}

func TestHasSession(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		want    bool
		wantErr bool
	}{
		{"exists", nil, true, false},
		{"missing_or_no_server", &tmux.Error{ExitCode: 1}, false, false},
		{"real_error", &tmux.Error{ExitCode: 2, Stderr: "boom"}, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := tmux.New(tmux.WithRunner(func(_ context.Context, _ ...string) ([]byte, error) { return nil, tt.err }))
			got, err := c.HasSession(t.Context(), "grove-x")
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("HasSession = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestListGroveSessions(t *testing.T) {
	tests := []struct {
		name    string
		out     string
		err     error
		want    []string
		wantErr bool
	}{
		{"filters_prefix", "grove-a\nother\ngrove-b\n", nil, []string{"grove-a", "grove-b"}, false},
		{"no_server", "", &tmux.Error{ExitCode: 1}, nil, false},
		{"empty_output", "", nil, nil, false},
		{"real_error", "", &tmux.Error{ExitCode: 2}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := tmux.New(tmux.WithRunner(func(_ context.Context, _ ...string) ([]byte, error) {
				return []byte(tt.out), tt.err
			}))
			got, err := c.ListGroveSessions(t.Context())
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("ListGroveSessions = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestKillSession(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantErr bool
	}{
		{"ok", nil, false},
		{"already_gone", &tmux.Error{ExitCode: 1}, false},
		{"real_error", &tmux.Error{ExitCode: 2}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := tmux.New(tmux.WithRunner(func(_ context.Context, _ ...string) ([]byte, error) { return nil, tt.err }))
			if err := c.KillSession(t.Context(), "grove-x"); (err != nil) != tt.wantErr {
				t.Errorf("KillSession err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSessionNameRoundTrip(t *testing.T) {
	id := "0190abcd-1234-7abc-8def-001122334455"
	name := tmux.SessionName(id)
	if name != "grove-"+id {
		t.Errorf("SessionName = %q, want grove-%s", name, id)
	}
	got, ok := tmux.SessionID(name)
	if !ok || got != id {
		t.Errorf("SessionID(%q) = %q, %v; want %q, true", name, got, ok, id)
	}
	if _, ok := tmux.SessionID("unrelated"); ok {
		t.Error("SessionID parsed a non-grove name")
	}
	if got := tmux.SessionName("a.b:c"); got != "grove-a-b-c" {
		t.Errorf("SessionName sanitize = %q, want grove-a-b-c", got)
	}
}

func TestAttachCommand(t *testing.T) {
	want := []string{"tmux", "attach-session", "-t", "grove-x"}
	if got := tmux.AttachCommand("grove-x"); !slices.Equal(got, want) {
		t.Errorf("AttachCommand = %q, want %q", got, want)
	}
}

func TestReadExitCode(t *testing.T) {
	dir := t.TempDir()
	present := filepath.Join(dir, "code")
	if err := os.WriteFile(present, []byte("42\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, ok := tmux.ReadExitCode(present); !ok || code != 42 {
		t.Errorf("present = %d, %v; want 42, true", code, ok)
	}
	if _, ok := tmux.ReadExitCode(filepath.Join(dir, "absent")); ok {
		t.Error("absent file reported ok")
	}
	bad := filepath.Join(dir, "bad")
	if err := os.WriteFile(bad, []byte("not-a-number"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := tmux.ReadExitCode(bad); ok {
		t.Error("unparseable file reported ok")
	}
	if _, ok := tmux.ReadExitCode(""); ok {
		t.Error("empty path reported ok")
	}
}

// TestIntegrationLifecycle exercises the real tmux binary end to end: create a
// grove session, observe it via HasSession/ListGroveSessions, kill it, and
// confirm it is gone. Guarded by Available so CI without tmux skips it.
func TestIntegrationLifecycle(t *testing.T) {
	if !tmux.Available() {
		t.Skip("tmux not available")
	}
	// Private socket so the test never touches the developer's real tmux server.
	// The dir must be short: a long t.TempDir() path exceeds the ~104-char Unix
	// socket path limit.
	socketDir, err := os.MkdirTemp("/tmp", "gtmux")
	if err != nil {
		t.Fatalf("tmux tmpdir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(socketDir) })
	t.Setenv("TMUX_TMPDIR", socketDir)
	t.Cleanup(func() { _ = exec.CommandContext(context.Background(), "tmux", "kill-server").Run() })

	c := tmux.New()
	ctx := t.Context()
	name := "grove-test-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	t.Cleanup(func() { _ = c.KillSession(context.Background(), name) })

	if err := c.NewSession(ctx, tmux.NewSpec{
		Name: name, Argv: []string{"sh", "-c", "sleep 30"},
		Cols: 80, Rows: 24, ExitFile: filepath.Join(t.TempDir(), "code"),
	}); err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	if ok, err := c.HasSession(ctx, name); err != nil || !ok {
		t.Fatalf("HasSession = %v, %v; want true, nil", ok, err)
	}
	names, err := c.ListGroveSessions(ctx)
	if err != nil {
		t.Fatalf("ListGroveSessions: %v", err)
	}
	if !slices.Contains(names, name) {
		t.Fatalf("ListGroveSessions = %v, want to contain %s", names, name)
	}

	if err := c.KillSession(ctx, name); err != nil {
		t.Fatalf("KillSession: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		ok, err := c.HasSession(ctx, name)
		if err != nil {
			t.Fatalf("HasSession post-kill: %v", err)
		}
		if !ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("session still present 5s after kill")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
