package mcpserv

import (
	"strings"
	"testing"

	"github.com/AnkushinDaniil/grove/internal/core"
)

// orchestratorSubtree builds root(workspace) → project P (orchestrator) with two
// worker task children A and B, returning P and its children.
func orchestratorSubtree(t *testing.T, ts *testServer, root core.Node) (core.Node, core.Node, core.Node) {
	t.Helper()
	p := mkNode(t, ts.tr, root.ID, core.KindProject, "API")
	a := mkNode(t, ts.tr, p.ID, core.KindTask, "Add auth")
	b := mkNode(t, ts.tr, p.ID, core.KindTask, "Add cache")
	return p, a, b
}

func TestGetContext(t *testing.T) {
	ts, root := newTestServer(t)
	p, _, _ := orchestratorSubtree(t, ts, root)
	sess := ts.session(p.ID, RoleOrchestrator)

	out, isErr, rerr := ts.call(t, sess, toolGetContext, nil)
	if rerr != nil || isErr {
		t.Fatalf("get_context failed: rerr=%v isErr=%v", rerr, isErr)
	}
	if out["node_id"] != string(p.ID) {
		t.Errorf("node_id = %v, want %s", out["node_id"], p.ID)
	}
	if path, _ := out["tree_path"].(string); path != "Workspace / API" {
		t.Errorf("tree_path = %q, want %q", path, "Workspace / API")
	}
	if out["role"] != string(RoleOrchestrator) {
		t.Errorf("role = %v, want orchestrator", out["role"])
	}
	limits, ok := out["limits"].(map[string]any)
	if !ok {
		t.Fatalf("limits missing: %v", out)
	}
	// P has two children; remaining = 12 - 2 = 10.
	if got := limits["children_remaining"]; got != float64(10) {
		t.Errorf("children_remaining = %v, want 10", got)
	}
}

func TestReportProgressMutatesNode(t *testing.T) {
	ts, root := newTestServer(t)
	_, a, _ := orchestratorSubtree(t, ts, root)
	sess := ts.session(a.ID, RoleWorker)

	out, isErr, rerr := ts.call(t, sess, toolReportProgress, map[string]any{
		"summary":   "wired the handler",
		"checklist": []string{"[x] design", "[ ] tests"},
		"percent":   40,
	})
	if rerr != nil || isErr {
		t.Fatalf("report_progress failed: rerr=%v isErr=%v", rerr, isErr)
	}
	if out["ok"] != true {
		t.Errorf("ok = %v, want true", out["ok"])
	}
	node, _ := ts.tr.Get(a.ID)
	if !strings.Contains(node.Meta, "wired the handler") {
		t.Errorf("progress not recorded in meta: %s", node.Meta)
	}
}

func TestReportProgressRequiresSummary(t *testing.T) {
	ts, root := newTestServer(t)
	_, a, _ := orchestratorSubtree(t, ts, root)
	sess := ts.session(a.ID, RoleWorker)
	_, isErr, rerr := ts.call(t, sess, toolReportProgress, map[string]any{"summary": "  "})
	if rerr != nil {
		t.Fatalf("unexpected protocol error: %v", rerr)
	}
	if !isErr {
		t.Fatal("empty summary should be a visible tool error")
	}
}

func TestRaiseAttention(t *testing.T) {
	ts, root := newTestServer(t)
	_, a, _ := orchestratorSubtree(t, ts, root)
	sess := ts.session(a.ID, RoleWorker)

	_, isErr, rerr := ts.call(t, sess, toolRaiseAttention, map[string]any{
		"kind":    "question",
		"message": "which database?",
		"options": []string{"postgres", "sqlite"},
	})
	if rerr != nil || isErr {
		t.Fatalf("raise_attention failed: rerr=%v isErr=%v", rerr, isErr)
	}
	node, _ := ts.tr.Get(a.ID)
	if node.Attention != core.AttentionQuestion {
		t.Errorf("attention = %q, want question", node.Attention)
	}
	if !strings.Contains(node.AttentionReason, "which database?") {
		t.Errorf("attention reason = %q", node.AttentionReason)
	}
}

func TestCompleteMarksAndRevokes(t *testing.T) {
	ts, root := newTestServer(t)
	_, a, _ := orchestratorSubtree(t, ts, root)
	sess := ts.session(a.ID, RoleWorker)

	_, isErr, rerr := ts.call(t, sess, toolComplete, map[string]any{
		"result":    "done",
		"summary":   "shipped it",
		"artifacts": []string{"/pr/42"},
	})
	if rerr != nil || isErr {
		t.Fatalf("complete failed: rerr=%v isErr=%v", rerr, isErr)
	}
	node, _ := ts.tr.Get(a.ID)
	if node.Attention != core.AttentionDone {
		t.Errorf("attention = %q, want done", node.Attention)
	}
	if !strings.Contains(node.Meta, "shipped it") {
		t.Errorf("completion not recorded in meta: %s", node.Meta)
	}
	if _, ok := ts.reg.Resolve(a.ID, sess.token); ok {
		t.Error("token should be revoked after complete")
	}
}

func TestCompleteFailedRaisesError(t *testing.T) {
	ts, root := newTestServer(t)
	_, a, _ := orchestratorSubtree(t, ts, root)
	sess := ts.session(a.ID, RoleWorker)
	_, _, rerr := ts.call(t, sess, toolComplete, map[string]any{"result": "failed", "summary": "blocked on deps"})
	if rerr != nil {
		t.Fatalf("complete failed: %v", rerr)
	}
	node, _ := ts.tr.Get(a.ID)
	if node.Attention != core.AttentionError {
		t.Errorf("attention = %q, want error", node.Attention)
	}
}

func TestSpawnChildDelegates(t *testing.T) {
	ts, root := newTestServer(t)
	p, _, _ := orchestratorSubtree(t, ts, root)
	child := core.NewNodeID()
	ts.fake.nextID = child
	sess := ts.session(p.ID, RoleOrchestrator)

	out, isErr, rerr := ts.call(t, sess, toolSpawnChild, map[string]any{
		"title":  "Write docs",
		"prompt": "document the API",
		"role":   "worker",
	})
	if rerr != nil || isErr {
		t.Fatalf("spawn_child failed: rerr=%v isErr=%v", rerr, isErr)
	}
	if out["node_id"] != string(child) {
		t.Errorf("node_id = %v, want %s", out["node_id"], child)
	}
	if out["status"] != "spawning" {
		t.Errorf("status = %v, want spawning", out["status"])
	}
	if ts.fake.spawnCount() != 1 {
		t.Errorf("spawn calls = %d, want 1", ts.fake.spawnCount())
	}
	if ts.fake.spawns[0].parent != p.ID {
		t.Errorf("spawn parent = %s, want %s", ts.fake.spawns[0].parent, p.ID)
	}
}

func TestSpawnRequiresOrchestrator(t *testing.T) {
	ts, root := newTestServer(t)
	_, a, _ := orchestratorSubtree(t, ts, root)
	sess := ts.session(a.ID, RoleWorker)

	_, _, rerr := ts.call(t, sess, toolSpawnChild, map[string]any{"title": "x", "prompt": "y"})
	if rerr == nil {
		t.Fatal("worker spawn_child should be a capability error")
	}
	if rerr.Code != codeInvalidRequest {
		t.Errorf("code = %d, want %d", rerr.Code, codeInvalidRequest)
	}
	if ts.fake.spawnCount() != 0 {
		t.Error("worker spawn should not reach the spawner")
	}
}

func TestListChildrenReflectsTree(t *testing.T) {
	ts, root := newTestServer(t)
	p, a, b := orchestratorSubtree(t, ts, root)
	// Give A some progress so it surfaces in the view.
	aSess := ts.session(a.ID, RoleWorker)
	if _, _, rerr := ts.call(t, aSess, toolReportProgress, map[string]any{"summary": "half done"}); rerr != nil {
		t.Fatalf("seed progress: %v", rerr)
	}

	sess := ts.session(p.ID, RoleOrchestrator)
	out, _, rerr := ts.call(t, sess, toolListChildren, nil)
	if rerr != nil {
		t.Fatalf("list_children: %v", rerr)
	}
	kids, ok := out["children"].([]any)
	if !ok || len(kids) != 2 {
		t.Fatalf("children = %v, want 2", out["children"])
	}
	ids := map[string]bool{}
	for _, k := range kids {
		m := k.(map[string]any)
		ids[m["node_id"].(string)] = true
	}
	if !ids[string(a.ID)] || !ids[string(b.ID)] {
		t.Errorf("children ids = %v, want %s and %s", ids, a.ID, b.ID)
	}
}

func TestNodeStatusSubtreeContainment(t *testing.T) {
	ts, root := newTestServer(t)
	p, a, _ := orchestratorSubtree(t, ts, root)
	// A sibling project the orchestrator P must not be able to inspect.
	other := mkNode(t, ts.tr, root.ID, core.KindProject, "Other")

	sess := ts.session(p.ID, RoleOrchestrator)

	// Own subtree: allowed.
	if _, isErr, rerr := ts.call(t, sess, toolNodeStatus, map[string]any{"node_id": string(a.ID)}); rerr != nil || isErr {
		t.Fatalf("node_status on own child failed: rerr=%v isErr=%v", rerr, isErr)
	}
	// Outside subtree: visible error, not a crash.
	_, isErr, rerr := ts.call(t, sess, toolNodeStatus, map[string]any{"node_id": string(other.ID)})
	if rerr != nil {
		t.Fatalf("unexpected protocol error: %v", rerr)
	}
	if !isErr {
		t.Fatal("node_status outside subtree should be a visible error")
	}
}

func TestSendMessageAdjacency(t *testing.T) {
	ts, root := newTestServer(t)
	p, a, _ := orchestratorSubtree(t, ts, root)
	other := mkNode(t, ts.tr, root.ID, core.KindProject, "Other")

	// Parent → child: allowed.
	pSess := ts.session(p.ID, RoleOrchestrator)
	if _, isErr, rerr := ts.call(t, pSess, toolSendMessage, map[string]any{"node_id": string(a.ID), "text": "hi"}); rerr != nil || isErr {
		t.Fatalf("parent→child message failed: rerr=%v isErr=%v", rerr, isErr)
	}
	// Child → parent: allowed.
	aSess := ts.session(a.ID, RoleWorker)
	if _, isErr, rerr := ts.call(t, aSess, toolSendMessage, map[string]any{"node_id": string(p.ID), "text": "ok"}); rerr != nil || isErr {
		t.Fatalf("child→parent message failed: rerr=%v isErr=%v", rerr, isErr)
	}
	if ts.fake.messageCount() != 2 {
		t.Errorf("message calls = %d, want 2", ts.fake.messageCount())
	}
	// Non-adjacent: rejected before reaching the spawner.
	_, isErr, rerr := ts.call(t, pSess, toolSendMessage, map[string]any{"node_id": string(other.ID), "text": "no"})
	if rerr != nil {
		t.Fatalf("unexpected protocol error: %v", rerr)
	}
	if !isErr {
		t.Fatal("message to non-adjacent node should be a visible error")
	}
	if ts.fake.messageCount() != 2 {
		t.Error("non-adjacent message should not reach the spawner")
	}
}

func TestAntiPollHint(t *testing.T) {
	ts, root := newTestServer(t)
	p, _, _ := orchestratorSubtree(t, ts, root)
	sess := ts.session(p.ID, RoleOrchestrator)

	var lastHint any
	for range pollThreshold + 1 {
		out, _, rerr := ts.call(t, sess, toolNodeStatus, nil)
		if rerr != nil {
			t.Fatalf("node_status: %v", rerr)
		}
		lastHint = out["hint"]
	}
	hint, _ := lastHint.(string)
	if !strings.Contains(hint, "Stop polling") {
		t.Errorf("expected anti-poll hint after %d calls, got %q", pollThreshold+1, hint)
	}
}
