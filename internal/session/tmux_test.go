package session

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/driver"
	"github.com/AnkushinDaniil/grove/internal/tmux"
	"github.com/AnkushinDaniil/grove/internal/tree"
)

// echoSleepDriver runs a shell that prints a marker then sleeps, standing in
// for a long-running interactive CLI in the tmux integration test.
type echoSleepDriver struct{ marker string }

func (echoSleepDriver) ID() string                { return "echosleep" }
func (echoSleepDriver) Capabilities() driver.Caps { return driver.Caps{Interactive: true} }
func (echoSleepDriver) NewParser() driver.Parser  { return nil }
func (echoSleepDriver) FormatPrompt(string) ([]byte, error) {
	return nil, driver.ErrUnsupported
}

func (echoSleepDriver) RecoverSessionID(context.Context, driver.SessionInfo) (string, error) {
	return "", driver.ErrUnsupported
}

func (d echoSleepDriver) NewCommand(spec driver.LaunchSpec) (driver.ExecSpec, error) {
	return driver.ExecSpec{
		Argv: []string{"sh", "-c", "echo " + d.marker + "; sleep 30"},
		Dir:  spec.CWD,
	}, nil
}

// newTmuxTree builds a root→project(echosleep)→task tree over a no-op store and
// returns the task node. The in-memory tree is the source of truth shared
// between the two managers in the restart test.
func newTmuxTree(t *testing.T) (*tree.Tree, core.NodeID) {
	t.Helper()
	tr := tree.New(&recordStore{})
	ctx := t.Context()
	root, err := tr.Bootstrap(ctx, "ws")
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	proj, err := tr.CreateNode(ctx, tree.CreateSpec{
		ParentID: root.ID, Kind: core.KindProject, Title: "P", Driver: "echosleep",
	})
	if err != nil {
		t.Fatalf("CreateNode project: %v", err)
	}
	task, err := tr.CreateNode(ctx, tree.CreateSpec{ParentID: proj.ID, Kind: core.KindTask, Title: "t"})
	if err != nil {
		t.Fatalf("CreateNode task: %v", err)
	}
	if _, err := tr.SetWorkspaceDir(ctx, task.ID, t.TempDir()); err != nil {
		t.Fatalf("SetWorkspaceDir: %v", err)
	}
	return tr, task.ID
}

// TestTmuxSurvivesRestart is the end-to-end proof of the feature: an interactive
// PTY session hosted in tmux keeps its child alive across a daemon "restart"
// (a fresh Manager over the same tree/scrollback) and is revived by Reattach,
// then torn down by Stop. It runs on a private tmux socket (TMUX_TMPDIR) so it
// never touches the developer's real tmux server, and is skipped without tmux.
func TestTmuxSurvivesRestart(t *testing.T) {
	if !tmux.Available() {
		t.Skip("tmux not available")
	}
	// Private socket: isolates this test's tmux server (and Reattach's orphan
	// killing) from any real grove sessions. t.Setenv also forbids t.Parallel.
	// The dir must be short: the tmux socket path has an ~104-char OS limit that
	// a long t.TempDir() blows past.
	t.Setenv("TMUX_TMPDIR", shortTmuxDir(t))
	t.Cleanup(func() { _ = exec.Command("tmux", "kill-server").Run() })

	const marker = "TMUX-READY-XYZ"
	reg, err := driver.NewRegistry(echoSleepDriver{marker: marker})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	tr, node := newTmuxTree(t)
	cfg := Config{ScrollbackDir: t.TempDir(), UseTmux: true}

	// --- daemon #1: start the interactive session ---
	m1 := NewManager(reg, tr, cfg)
	sess, err := m1.Start(t.Context(), node, core.ModePTY, "", "")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if sess.Status != core.SessionRunning {
		t.Fatalf("start status = %s, want running", sess.Status)
	}
	name := tmux.SessionName(string(sess.ID))
	t.Cleanup(func() { _ = exec.Command("tmux", "kill-session", "-t", name).Run() })

	waitTmuxSession(t, name, true)
	assertMarkerVisible(t, m1, sess.ID, marker)

	// --- simulate a daemon restart ---
	// Shutdown detaches (leaves the tmux child alive); the session in the tree
	// is then marked interrupted, exactly as store.MarkInterrupted would at the
	// next startup.
	shutdown(t, m1)
	waitTmuxSession(t, name, true) // the child SURVIVED the daemon going away

	interrupted := sess
	interrupted.Status = core.SessionInterrupted
	interrupted.EndedAt = time.Now()
	if _, err := tr.ApplySession(t.Context(), interrupted); err != nil {
		t.Fatalf("mark interrupted: %v", err)
	}
	if n, _ := tr.Get(node); n.Status != core.StatusInterrupted {
		t.Fatalf("node status = %s, want interrupted", n.Status)
	}

	// --- daemon #2: reattach ---
	m2 := NewManager(reg, tr, cfg)
	t.Cleanup(func() { shutdown(t, m2) })

	reattached, err := m2.Reattach(t.Context())
	if err != nil {
		t.Fatalf("Reattach: %v", err)
	}
	if reattached != 1 {
		t.Fatalf("reattached = %d, want 1", reattached)
	}
	waitNodeStatus(t, tr, node, core.StatusRunning)
	waitTmuxSession(t, name, true)
	assertMarkerVisible(t, m2, sess.ID, marker) // stream survives the reattach

	// --- stop tears the tmux session down ---
	stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := m2.Stop(stopCtx, sess.ID); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	waitTmuxSession(t, name, false)
	got, _ := tr.SessionFor(node)
	if got.Status != core.SessionExited {
		t.Errorf("final session status = %s, want exited", got.Status)
	}
	if n, _ := tr.Get(node); n.Status.Active() {
		t.Errorf("node still active after stop: %s", n.Status)
	}
}

// assertMarkerVisible attaches to a live session's terminal and fails unless the
// marker appears (tmux repaints the pane content, including earlier output, on
// attach).
func assertMarkerVisible(t *testing.T, m *Manager, sid core.SessionID, marker string) {
	t.Helper()
	h, ok := m.Terminal(sid)
	if !ok {
		t.Fatal("Terminal returned no handle for a live tmux session")
	}
	replay, ch, cancel := h.Attach()
	defer cancel()
	if !seesMarker(replay, ch, marker, 5*time.Second) {
		t.Fatalf("did not observe %q in tmux terminal output", marker)
	}
}

// shutdown gracefully winds a manager down under a bounded context.
func shutdown(t *testing.T, m *Manager) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := m.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

// waitTmuxSession polls until the named tmux session's presence matches want.
func waitTmuxSession(t *testing.T, name string, want bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if tmuxHasSession(name) == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("tmux session %s present=%v, want %v", name, !want, want)
}

func tmuxHasSession(name string) bool {
	return exec.Command("tmux", "has-session", "-t", name).Run() == nil
}

// shortTmuxDir returns a short-lived directory under /tmp for a private tmux
// socket. It must be short because a Unix socket path is capped near 104 bytes,
// which a nested t.TempDir() path exceeds.
func shortTmuxDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "gtmux")
	if err != nil {
		t.Fatalf("tmux tmpdir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}
