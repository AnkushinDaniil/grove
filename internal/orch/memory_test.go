package orch

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/mcpserv"
)

// captureCall records one Memory.Capture invocation.
type captureCall struct {
	node    core.NodeID
	kind    string
	content string
	source  string
}

// fakeMemory is a recording Memory: Recall returns a fixed block, Capture
// publishes on a channel so async captures can be awaited.
type fakeMemory struct {
	recall   string
	captures chan captureCall
}

func newFakeMemory(recall string) *fakeMemory {
	return &fakeMemory{recall: recall, captures: make(chan captureCall, 8)}
}

func (m *fakeMemory) Recall(context.Context, core.NodeID, int) string { return m.recall }

func (m *fakeMemory) Capture(_ context.Context, node core.NodeID, kind, content, source string) {
	m.captures <- captureCall{node: node, kind: kind, content: content, source: source}
}

func (m *fakeMemory) awaitCapture(t *testing.T, node core.NodeID, timeout time.Duration) captureCall {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case c := <-m.captures:
			if c.node == node {
				return c
			}
		case <-deadline:
			t.Fatalf("no Capture for node %s within %s", node, timeout)
		}
	}
}

// TestSpawnInjectsRecalledMemory verifies recall injection: a spawned child's
// briefing carries the memory block the scheduler recalled for it.
func TestSpawnInjectsRecalledMemory(t *testing.T) {
	tr, root := newTree(t)
	reg := mcpserv.NewRegistry()
	starter := newFakeStarter()
	mem := newFakeMemory("## Memory\n\n- **decision**: Chose Postgres for durability\n")
	s := runScheduler(t, tr, starter, reg, func(d *Deps) { d.Memory = mem })

	p := mkProject(t, tr, reg, root, "")
	childID, err := s.Spawn(context.Background(), p.ID, mcpserv.SpawnRequest{
		Title: "Add auth", Prompt: "implement login", Role: mcpserv.RoleWorker,
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	spawnStart := starter.await(t, childID, 2*time.Second)
	if !strings.Contains(spawnStart.prompt, "## Memory") || !strings.Contains(spawnStart.prompt, "Chose Postgres for durability") {
		t.Errorf("child briefing should carry the recalled memory block, got: %q", spawnStart.prompt)
	}
	// The task prompt and node-context header are still present.
	if !strings.Contains(spawnStart.prompt, "implement login") || !strings.Contains(spawnStart.prompt, "grove node context") {
		t.Errorf("child briefing lost its task/context, got: %q", spawnStart.prompt)
	}
}

// TestAutoCaptureOnCompletion verifies auto-capture: a done child files a fact
// carrying its summary, and a failed child files a gotcha.
func TestAutoCaptureOnCompletion(t *testing.T) {
	tr, root := newTree(t)
	reg := mcpserv.NewRegistry()
	starter := newFakeStarter()
	mem := newFakeMemory("")
	s := runScheduler(t, tr, starter, reg, func(d *Deps) { d.Memory = mem })

	p := mkProject(t, tr, reg, root, "p-conv-1")

	done, err := s.Spawn(context.Background(), p.ID, mcpserv.SpawnRequest{Title: "auth", Prompt: "x", Role: mcpserv.RoleWorker})
	if err != nil {
		t.Fatalf("spawn done child: %v", err)
	}
	starter.await(t, done, 2*time.Second)
	completeChild(t, tr, done, "done", "login works end to end")

	got := mem.awaitCapture(t, done, 2*time.Second)
	if got.kind != memoryKindFact || got.source != memorySourceAuto {
		t.Errorf("done capture = kind %q source %q, want fact/auto", got.kind, got.source)
	}
	if !strings.Contains(got.content, "login works end to end") {
		t.Errorf("done capture content = %q, want the completion summary", got.content)
	}

	failed, err := s.Spawn(context.Background(), p.ID, mcpserv.SpawnRequest{Title: "cache", Prompt: "y", Role: mcpserv.RoleWorker})
	if err != nil {
		t.Fatalf("spawn failed child: %v", err)
	}
	starter.await(t, failed, 2*time.Second)
	completeChild(t, tr, failed, "failed", "broke the build")

	gotFail := mem.awaitCapture(t, failed, 2*time.Second)
	if gotFail.kind != memoryKindGotcha || gotFail.source != memorySourceAuto {
		t.Errorf("failed capture = kind %q source %q, want gotcha/auto", gotFail.kind, gotFail.source)
	}
}

// TestAutoCaptureDisabledWithoutMemory ensures completions are harmless when no
// memory backend is wired (the default).
func TestAutoCaptureDisabledWithoutMemory(t *testing.T) {
	tr, root := newTree(t)
	reg := mcpserv.NewRegistry()
	starter := newFakeStarter()
	s := runScheduler(t, tr, starter, reg, nil) // no Memory

	p := mkProject(t, tr, reg, root, "")
	child, err := s.Spawn(context.Background(), p.ID, mcpserv.SpawnRequest{Title: "x", Prompt: "y", Role: mcpserv.RoleWorker})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	starter.await(t, child, 2*time.Second)
	completeChild(t, tr, child, "done", "fine") // must not panic
	// Give any (absent) capture goroutine a beat; absence of a panic is the assertion.
	time.Sleep(50 * time.Millisecond)
}
