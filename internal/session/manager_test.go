package session

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/driver"
	"github.com/AnkushinDaniil/grove/internal/testutil/fakeagent"
	"github.com/AnkushinDaniil/grove/internal/tree"
)

// recordStore is a no-op tree.Store that counts persisted rows, mirroring the
// pattern in internal/tree/tree_test.go without touching the tree package.
type recordStore struct {
	mu       sync.Mutex
	nodes    int
	sessions int
	events   int
}

func (s *recordStore) SaveNodes(_ context.Context, ns []core.Node) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nodes += len(ns)
	return nil
}

func (s *recordStore) SaveSessions(_ context.Context, ss []core.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions += len(ss)
	return nil
}

func (s *recordStore) AppendEvents(_ context.Context, es []core.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events += len(es)
	return nil
}

func (s *recordStore) AckNodeEvents(_ context.Context, _ core.NodeID, _ time.Time) ([]core.Event, error) {
	return nil, nil
}

// newFixture builds a manager over a fake driver and a tree with a
// root→project(driver=fake)→task hierarchy, returning the task node id.
func newFixture(t *testing.T, cfg Config, script []fakeagent.Step) (*Manager, *tree.Tree, core.NodeID) {
	t.Helper()
	bin := fakeagent.Build(t)
	return newFixtureWithDriver(t, cfg, fakeagent.NewDriver(bin, fakeagent.WriteScript(t, script)))
}

func newFixtureWithDriver(t *testing.T, cfg Config, drv driver.Driver) (*Manager, *tree.Tree, core.NodeID) {
	t.Helper()
	reg, err := driver.NewRegistry(drv)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	tr := tree.New(&recordStore{})
	ctx := t.Context()
	root, err := tr.Bootstrap(ctx, "ws")
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	proj, err := tr.CreateNode(ctx, tree.CreateSpec{
		ParentID: root.ID, Kind: core.KindProject, Title: "P", Driver: "fake",
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

	if cfg.ScrollbackDir == "" {
		cfg.ScrollbackDir = t.TempDir()
	}
	m := NewManager(reg, tr, cfg)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = m.Shutdown(ctx)
	})
	return m, tr, task.ID
}

// settleTimeout bounds how long tests wait for an out-of-band process/goroutine
// to drive tree state to an expected value.
const settleTimeout = 10 * time.Second

func waitNodeStatus(t *testing.T, tr *tree.Tree, id core.NodeID, want core.NodeStatus) {
	t.Helper()
	deadline := time.Now().Add(settleTimeout)
	for time.Now().Before(deadline) {
		if n, ok := tr.Get(id); ok && n.Status == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	n, _ := tr.Get(id)
	t.Fatalf("node %s status = %s, want %s", id, n.Status, want)
}

func waitSession(t *testing.T, tr *tree.Tree, node core.NodeID, pred func(core.Session) bool) core.Session {
	t.Helper()
	deadline := time.Now().Add(settleTimeout)
	for time.Now().Before(deadline) {
		if s, ok := tr.SessionFor(node); ok && pred(s) {
			return s
		}
		time.Sleep(5 * time.Millisecond)
	}
	s, _ := tr.SessionFor(node)
	t.Fatalf("session for node %s never matched: %+v", node, s)
	return core.Session{}
}

func drainEventTypes(ch <-chan tree.Delta, quiet time.Duration) map[core.EventType]int {
	out := map[core.EventType]int{}
	timer := time.NewTimer(quiet)
	defer timer.Stop()
	for {
		select {
		case d, ok := <-ch:
			if !ok {
				return out
			}
			for _, e := range d.Events {
				out[e.Type]++
			}
			if !timer.Stop() {
				<-timer.C
			}
			timer.Reset(quiet)
		case <-timer.C:
			return out
		}
	}
}

const sessionStartedLine = `{"event":"session_started","payload":{"driver_session_id":"sess-1","transcript_path":"/tmp/t.jsonl"}}`

func TestHeadlessHappyPath(t *testing.T) {
	m, tr, node := newFixture(t, Config{}, []fakeagent.Step{
		{Emit: sessionStartedLine},
		{Emit: "some assistant text"},
		{Emit: `{"event":"turn_done","payload":{"result_text":"finished"}}`},
		{ExitCode: new(0)},
	})
	_, ch, cancel := tr.Subscribe()
	defer cancel()

	sess, err := m.Start(t.Context(), node, core.ModeHeadless, "do it", "")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if sess.Status != core.SessionRunning {
		t.Fatalf("start status = %s, want running", sess.Status)
	}

	waitNodeStatus(t, tr, node, core.StatusDone)

	got, ok := tr.SessionFor(node)
	if !ok {
		t.Fatal("no session for node")
	}
	if got.DriverSessionID != "sess-1" {
		t.Errorf("DriverSessionID = %q, want sess-1", got.DriverSessionID)
	}
	if got.TranscriptPath != "/tmp/t.jsonl" {
		t.Errorf("TranscriptPath = %q, want /tmp/t.jsonl", got.TranscriptPath)
	}
	if got.Status != core.SessionExited || got.ExitCode == nil || *got.ExitCode != 0 {
		t.Errorf("final session = %s, code %v; want exited 0", got.Status, got.ExitCode)
	}

	types := drainEventTypes(ch, 300*time.Millisecond)
	for _, want := range []core.EventType{core.EventSessionStarted, core.EventText, core.EventTurnDone} {
		if types[want] == 0 {
			t.Errorf("missing %s event in delta feed (%v)", want, types)
		}
	}
}

func TestHeadlessFailureExit(t *testing.T) {
	m, tr, node := newFixture(t, Config{}, []fakeagent.Step{
		{Emit: `{"event":"session_started","payload":{"driver_session_id":"s2"}}`},
		{Emit: "working then failing"},
		{ExitCode: new(2)},
	})
	if _, err := m.Start(t.Context(), node, core.ModeHeadless, "", ""); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitNodeStatus(t, tr, node, core.StatusFailed)

	got, _ := tr.SessionFor(node)
	if got.Status != core.SessionExited {
		t.Errorf("session status = %s, want exited", got.Status)
	}
	if got.ExitCode == nil || *got.ExitCode != 2 {
		t.Errorf("exit code = %v, want 2", got.ExitCode)
	}
}

// TestHeadlessStopMidRun documents the Stop decision: an explicit Stop signals
// the process, which exits via signal, so the session finalizes as
// SessionExited with code 128+signum and the node resolves to Failed. There is
// deliberately no dedicated "stopped" status.
func TestHeadlessStopMidRun(t *testing.T) {
	m, tr, node := newFixture(t, Config{}, []fakeagent.Step{
		{Emit: `{"event":"session_started","payload":{"driver_session_id":"s3"}}`},
		{WaitStdinLine: true},
		{ExitCode: new(0)},
	})
	sess, err := m.Start(t.Context(), node, core.ModeHeadless, "", "")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitSession(t, tr, node, func(s core.Session) bool { return s.DriverSessionID == "s3" })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := m.Stop(ctx, sess.ID); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	got, _ := tr.SessionFor(node)
	if got.Status != core.SessionExited {
		t.Errorf("session status = %s, want exited", got.Status)
	}
	if got.ExitCode == nil || *got.ExitCode < 128 {
		t.Errorf("exit code = %v, want signalled (>=128)", got.ExitCode)
	}
	if n, _ := tr.Get(node); n.Status != core.StatusFailed {
		t.Errorf("node status = %s, want failed", n.Status)
	}
}

// TestHeadlessStderrReported points the fake agent at an unparseable script so
// it fails and writes to stderr; the manager should surface that tail as an
// EventError and fail the node.
func TestHeadlessStderrReported(t *testing.T) {
	bin := fakeagent.Build(t)
	badScript := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(badScript, []byte("this is not valid json"), 0o600); err != nil {
		t.Fatalf("write bad script: %v", err)
	}
	m, tr, node := newFixtureWithDriver(t, Config{}, fakeagent.NewDriver(bin, badScript))

	_, ch, cancel := tr.Subscribe()
	defer cancel()

	if _, err := m.Start(t.Context(), node, core.ModeHeadless, "", ""); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitNodeStatus(t, tr, node, core.StatusFailed)
	if !sawEvent(ch, core.EventError, 2*time.Second) {
		t.Fatal("no EventError surfaced for a failed process with stderr output")
	}
}

func TestHeadlessPrompt(t *testing.T) {
	m, tr, node := newFixture(t, Config{}, []fakeagent.Step{
		{WaitStdinLine: true},
		{Emit: `{"event":"turn_done","payload":{}}`},
		{ExitCode: new(0)},
	})
	sess, err := m.Start(t.Context(), node, core.ModeHeadless, "", "")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := m.Prompt(t.Context(), sess.ID, "continue please"); err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	waitNodeStatus(t, tr, node, core.StatusDone)
}

// TestWorkingDirInheritsUserSetWorkDir sets a user work dir on the project and
// starts a session on that project (which has no machine-managed WorkspaceDir),
// so the session must launch in the inherited work dir.
func TestWorkingDirInheritsUserSetWorkDir(t *testing.T) {
	m, tr, node := newFixture(t, Config{}, []fakeagent.Step{
		{WaitStdinLine: true},
		{ExitCode: new(0)},
	})
	// The task fixture carries a WorkspaceDir that always wins; use its project
	// ancestor (no WorkspaceDir) to exercise the inherited user work-dir path.
	task, ok := tr.Get(node)
	if !ok {
		t.Fatal("task node not found")
	}
	project := task.ParentID

	workDir := t.TempDir()
	if _, err := tr.UpdateNode(t.Context(), project, tree.Patch{WorkDir: &workDir}); err != nil {
		t.Fatalf("UpdateNode: %v", err)
	}

	sess, err := m.Start(t.Context(), project, core.ModeHeadless, "", "")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if sess.CWD != workDir {
		t.Errorf("started session CWD = %q, want inherited work dir %q", sess.CWD, workDir)
	}
	got := waitSession(t, tr, project, func(s core.Session) bool { return s.ID == sess.ID })
	if got.CWD != workDir {
		t.Errorf("tree session CWD = %q, want %q", got.CWD, workDir)
	}
}

func TestBudgetExhausted(t *testing.T) {
	m, _, node := newFixture(t, Config{MaxRunning: 1}, []fakeagent.Step{
		{WaitStdinLine: true},
		{ExitCode: new(0)},
	})
	if _, err := m.Start(t.Context(), node, core.ModeHeadless, "", ""); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	_, err := m.Start(t.Context(), node, core.ModeHeadless, "", "")
	if !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("second Start = %v, want ErrBudgetExhausted", err)
	}
}

func TestPTYAttachAndPrompt(t *testing.T) {
	const marker = "PTY-MARKER-XYZ"
	m, tr, node := newFixture(t, Config{}, []fakeagent.Step{
		{Emit: marker},
		{WaitStdinLine: true},
		{ExitCode: new(0)},
	})
	sess, err := m.Start(t.Context(), node, core.ModePTY, "", "")
	if err != nil {
		t.Fatalf("Start pty: %v", err)
	}
	if sess.Status != core.SessionRunning {
		t.Fatalf("pty start status = %s, want running", sess.Status)
	}

	h, ok := m.Terminal(sess.ID)
	if !ok {
		t.Fatal("Terminal returned no handle for a live PTY session")
	}
	replay, ch, cancel := h.Attach()
	defer cancel()
	if !seesMarker(replay, ch, marker, 5*time.Second) {
		t.Fatalf("did not observe %q in PTY output", marker)
	}

	// Bracketed-paste prompt to the PTY; the trailing CR ends the fake's line.
	if err := m.Prompt(t.Context(), sess.ID, "go on"); err != nil {
		t.Fatalf("PTY Prompt: %v", err)
	}
	waitNodeStatus(t, tr, node, core.StatusDone)
}

func seesMarker(replay []byte, ch <-chan []byte, marker string, timeout time.Duration) bool {
	if bytes.Contains(replay, []byte(marker)) {
		return true
	}
	var buf []byte
	deadline := time.After(timeout)
	for {
		select {
		case chunk, ok := <-ch:
			if !ok {
				return bytes.Contains(buf, []byte(marker))
			}
			buf = append(buf, chunk...)
			if bytes.Contains(buf, []byte(marker)) {
				return true
			}
		case <-deadline:
			return false
		}
	}
}

func TestStartRejectsBadInputs(t *testing.T) {
	m, tr, node := newFixture(t, Config{}, []fakeagent.Step{{ExitCode: new(0)}})

	if _, err := m.Start(t.Context(), node, "tmux", "", ""); !errors.Is(err, core.ErrInvalid) {
		t.Errorf("bad mode = %v, want ErrInvalid", err)
	}
	if _, err := m.Start(t.Context(), "missing", core.ModeHeadless, "", ""); !errors.Is(err, core.ErrInvalid) {
		t.Errorf("missing node = %v, want ErrInvalid", err)
	}

	// A node whose resolved driver is empty yields ErrNoDriver.
	root, _ := tr.Root()
	orphan, err := tr.CreateNode(t.Context(), tree.CreateSpec{ParentID: root.ID, Kind: core.KindProject, Title: "no-driver"})
	if err != nil {
		t.Fatalf("CreateNode: %v", err)
	}
	if _, err := m.Start(t.Context(), orphan.ID, core.ModeHeadless, "", ""); !errors.Is(err, ErrNoDriver) {
		t.Errorf("no-driver node = %v, want ErrNoDriver", err)
	}
}

func TestStopAndPromptUnknownSession(t *testing.T) {
	m, _, _ := newFixture(t, Config{}, []fakeagent.Step{{ExitCode: new(0)}})
	if err := m.Stop(t.Context(), "nope"); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("Stop unknown = %v, want ErrSessionNotFound", err)
	}
	if err := m.Prompt(t.Context(), "nope", "x"); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("Prompt unknown = %v, want ErrSessionNotFound", err)
	}
	if _, ok := m.Terminal("nope"); ok {
		t.Error("Terminal unknown = ok, want not found")
	}
}
