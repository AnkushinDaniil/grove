package orch

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/mcpserv"
	"github.com/AnkushinDaniil/grove/internal/tree"
)

// mkProject creates an orchestrator project P under the workspace root and mints
// it an orchestrator token, plus a session it can be resumed from.
func mkProject(t *testing.T, tr *tree.Tree, reg *mcpserv.Registry, root core.Node, resumeID string) core.Node {
	t.Helper()
	p, err := tr.CreateNode(context.Background(), tree.CreateSpec{
		ParentID: root.ID, Kind: core.KindProject, Title: "API", Driver: "fake",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	reg.Mint(p.ID, mcpserv.RoleOrchestrator)
	if resumeID != "" {
		exit := 0
		if _, err := tr.ApplySession(context.Background(), core.Session{
			ID: core.NewSessionID(), NodeID: p.ID, Driver: "fake", Mode: core.ModeHeadless,
			DriverSessionID: resumeID, Status: core.SessionExited, ExitCode: &exit,
			CWD: "/tmp", StartedAt: time.Now(),
		}); err != nil {
			t.Fatalf("apply project session: %v", err)
		}
	}
	return p
}

// TestSpawnCompleteWake is the core loop: an orchestrator spawns a worker, the
// worker completes, and the orchestrator gets one headless wake turn carrying a
// child_completed digest that resumes its own conversation.
func TestSpawnCompleteWake(t *testing.T) {
	tr, root := newTree(t)
	reg := mcpserv.NewRegistry()
	starter := newFakeStarter()
	s := runScheduler(t, tr, starter, reg, nil)

	p := mkProject(t, tr, reg, root, "p-conv-1")

	childID, err := s.Spawn(context.Background(), p.ID, mcpserv.SpawnRequest{
		Title: "Add auth", Prompt: "implement login", Role: mcpserv.RoleWorker,
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	// The child's session is launched asynchronously with the grove MCP mounted
	// and its briefing prepended to the task prompt.
	spawnStart := starter.await(t, childID, 2*time.Second)
	if !spawnStart.mcpMounted {
		t.Error("spawned child should have the grove MCP mounted")
	}
	if spawnStart.mode != core.ModeHeadless {
		t.Errorf("child mode = %q, want headless", spawnStart.mode)
	}
	if !strings.Contains(spawnStart.prompt, "implement login") || !strings.Contains(spawnStart.prompt, "grove node context") {
		t.Errorf("child prompt should carry briefing + task, got: %q", spawnStart.prompt)
	}

	// The worker completes.
	completeChild(t, tr, childID, "done", "login works")

	// The orchestrator is woken with a child_completed digest, resuming its own
	// conversation.
	wake := starter.await(t, p.ID, 2*time.Second)
	if !wake.mcpMounted {
		t.Error("wake turn should have the grove MCP mounted")
	}
	if wake.resumeID != "p-conv-1" {
		t.Errorf("wake resumeID = %q, want p-conv-1 (resume the orchestrator's own conversation)", wake.resumeID)
	}
	if !strings.Contains(wake.prompt, "child_completed") {
		t.Errorf("wake digest should report child_completed, got: %q", wake.prompt)
	}
	if !strings.Contains(wake.prompt, "login works") {
		t.Errorf("wake digest should carry the completion summary, got: %q", wake.prompt)
	}
	if !strings.Contains(wake.prompt, "<grove-events") {
		t.Errorf("wake should use the grove-events envelope, got: %q", wake.prompt)
	}
}

// TestWakeOnChildFailure wakes the parent with child_failed when a child crashes.
func TestWakeOnChildFailure(t *testing.T) {
	tr, root := newTree(t)
	reg := mcpserv.NewRegistry()
	starter := newFakeStarter()
	s := runScheduler(t, tr, starter, reg, nil)
	p := mkProject(t, tr, reg, root, "p-conv")

	childID, err := s.Spawn(context.Background(), p.ID, mcpserv.SpawnRequest{Title: "x", Prompt: "y"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	starter.await(t, childID, 2*time.Second)

	completeChild(t, tr, childID, "failed", "compile error")

	wake := starter.await(t, p.ID, 2*time.Second)
	if !strings.Contains(wake.prompt, "child_failed") {
		t.Errorf("wake should report child_failed, got: %q", wake.prompt)
	}
}

// TestWakeOnChildAttention wakes the parent when a child raises a question.
func TestWakeOnChildAttention(t *testing.T) {
	tr, root := newTree(t)
	reg := mcpserv.NewRegistry()
	starter := newFakeStarter()
	s := runScheduler(t, tr, starter, reg, nil)
	p := mkProject(t, tr, reg, root, "p-conv")

	childID, err := s.Spawn(context.Background(), p.ID, mcpserv.SpawnRequest{Title: "x", Prompt: "y"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	starter.await(t, childID, 2*time.Second)

	// The worker raises a question (non-terminal attention).
	payload, _ := core.MarshalPayload(core.AwaitingPayload{Reason: core.AwaitQuestion, Detail: "which db?"})
	if _, err := tr.IngestEvents(context.Background(), childID, "", []core.EventInput{
		{Type: core.EventAwaitingInput, Payload: payload, Reason: core.AwaitQuestion, Detail: "which db?"},
	}); err != nil {
		t.Fatalf("ingest attention: %v", err)
	}

	wake := starter.await(t, p.ID, 2*time.Second)
	if !strings.Contains(wake.prompt, "child_attention") {
		t.Errorf("wake should report child_attention, got: %q", wake.prompt)
	}
}

// TestSpawnDeniedByMaxChildren denies a spawn once the direct-children cap is hit.
func TestSpawnDeniedByMaxChildren(t *testing.T) {
	tr, root := newTree(t)
	reg := mcpserv.NewRegistry()
	starter := newFakeStarter()
	s := runScheduler(t, tr, starter, reg, func(d *Deps) {
		d.Limits = mcpserv.Limits{MaxDepth: 5, MaxChildren: 1, MaxDescendants: 40}
	})
	p := mkProject(t, tr, reg, root, "")

	if _, err := s.Spawn(context.Background(), p.ID, mcpserv.SpawnRequest{Title: "a", Prompt: "a"}); err != nil {
		t.Fatalf("first spawn should succeed: %v", err)
	}
	_, err := s.Spawn(context.Background(), p.ID, mcpserv.SpawnRequest{Title: "b", Prompt: "b"})
	if !errors.Is(err, mcpserv.ErrLimit) {
		t.Fatalf("second spawn error = %v, want ErrLimit", err)
	}
}

// TestSpawnDeniedByMaxDepth denies a spawn that would exceed the depth cap.
func TestSpawnDeniedByMaxDepth(t *testing.T) {
	tr, root := newTree(t)
	reg := mcpserv.NewRegistry()
	starter := newFakeStarter()
	// Depth: root=0, project=1. MaxDepth 1 means a child of the project (depth 2)
	// is denied, but the project itself is allowed under root.
	s := runScheduler(t, tr, starter, reg, func(d *Deps) {
		d.Limits = mcpserv.Limits{MaxDepth: 1, MaxChildren: 12, MaxDescendants: 40}
	})

	p, err := s.Spawn(context.Background(), root.ID, mcpserv.SpawnRequest{Title: "proj", Prompt: "p", Role: mcpserv.RoleOrchestrator})
	if err != nil {
		t.Fatalf("spawn project under root should succeed: %v", err)
	}
	reg.Mint(p, mcpserv.RoleOrchestrator)
	_, err = s.Spawn(context.Background(), p, mcpserv.SpawnRequest{Title: "task", Prompt: "t"})
	if !errors.Is(err, mcpserv.ErrLimit) {
		t.Fatalf("spawn beyond max depth error = %v, want ErrLimit", err)
	}
}

// TestSendMessageWakesTarget delivers a queued message to the target node.
func TestSendMessageWakesTarget(t *testing.T) {
	tr, root := newTree(t)
	reg := mcpserv.NewRegistry()
	starter := newFakeStarter()
	// Messages use the debounce window; shorten it so the test is fast.
	s := runScheduler(t, tr, starter, reg, func(d *Deps) { d.Debounce = 10 * time.Millisecond })
	p := mkProject(t, tr, reg, root, "p-conv")

	if err := s.SendMessage(context.Background(), root.ID, p.ID, "please status"); err != nil {
		t.Fatalf("send message: %v", err)
	}
	wake := starter.await(t, p.ID, 2*time.Second)
	if !strings.Contains(wake.prompt, "please status") || !strings.Contains(wake.prompt, "message") {
		t.Errorf("wake should carry the message, got: %q", wake.prompt)
	}
}

// TestNoWakeWhileParentBusy defers a wake until the orchestrator is idle so two
// turns never run at once on one node.
func TestNoWakeWhileParentBusy(t *testing.T) {
	tr, root := newTree(t)
	reg := mcpserv.NewRegistry()
	starter := newFakeStarter()
	s := runScheduler(t, tr, starter, reg, nil)
	p := mkProject(t, tr, reg, root, "p-conv")

	// Mark the orchestrator busy by giving it a running session.
	if _, err := tr.ApplySession(context.Background(), core.Session{
		ID: core.NewSessionID(), NodeID: p.ID, Driver: "fake", Mode: core.ModeHeadless,
		DriverSessionID: "p-conv", Status: core.SessionRunning, CWD: "/tmp", StartedAt: time.Now(),
	}); err != nil {
		t.Fatalf("mark busy: %v", err)
	}

	childID, err := s.Spawn(context.Background(), p.ID, mcpserv.SpawnRequest{Title: "x", Prompt: "y"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	starter.await(t, childID, 2*time.Second)
	completeChild(t, tr, childID, "done", "ok")

	// While the parent is running, no wake should fire.
	select {
	case c := <-starter.ch:
		if c.node == p.ID {
			t.Fatal("parent was woken while still running")
		}
	case <-time.After(200 * time.Millisecond):
	}

	// Once the parent goes idle, the buffered wake fires.
	exit := 0
	if _, err := tr.ApplySession(context.Background(), core.Session{
		ID: core.NewSessionID(), NodeID: p.ID, Driver: "fake", Mode: core.ModeHeadless,
		DriverSessionID: "p-conv", Status: core.SessionExited, ExitCode: &exit, CWD: "/tmp", StartedAt: time.Now(),
	}); err != nil {
		t.Fatalf("mark idle: %v", err)
	}
	wake := starter.await(t, p.ID, 3*time.Second)
	if !strings.Contains(wake.prompt, "child_completed") {
		t.Errorf("deferred wake should report child_completed, got: %q", wake.prompt)
	}
}
