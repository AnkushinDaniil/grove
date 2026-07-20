package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/testutil/fakeagent"
)

// settleTimeout bounds waits for out-of-band process/goroutine progress.
const settleTimeout = 10 * time.Second

// waitNode polls the tree until node id satisfies pred.
func (h *harness) waitNode(id core.NodeID, pred func(core.Node) bool) core.Node {
	h.t.Helper()
	deadline := time.Now().Add(settleTimeout)
	for time.Now().Before(deadline) {
		if n, ok := h.tree.Get(id); ok && pred(n) {
			return n
		}
		time.Sleep(10 * time.Millisecond)
	}
	n, _ := h.tree.Get(id)
	h.t.Fatalf("node %s never matched predicate (last: %+v)", id, n)
	return core.Node{}
}

// waitEvents polls GET /events for a node until pred holds over its events.
func (h *harness) waitEvents(nodeID string, pred func([]EventDTO) bool) {
	h.t.Helper()
	deadline := time.Now().Add(settleTimeout)
	for time.Now().Before(deadline) {
		var events []EventDTO
		h.decode(h.do(http.MethodGet, "/api/v1/nodes/"+nodeID+"/events", nil), http.StatusOK, &events)
		if pred(events) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	h.t.Fatalf("events for node %s never matched predicate", nodeID)
}

func hasEventType(events []EventDTO, typ core.EventType) bool {
	for _, e := range events {
		if e.Type == string(typ) {
			return true
		}
	}
	return false
}

func TestSessionStartPromptExit(t *testing.T) {
	h := newHarness(t, []fakeagent.Step{
		{Emit: `{"event":"session_started","payload":{"driver_session_id":"sess-1"}}`},
		{Emit: "working"},
		{WaitStdinLine: true}, // block until the follow-up prompt arrives
		{Emit: `{"event":"turn_done","payload":{"result_text":"finished"}}`},
		{ExitCode: intPtr(0)},
	})
	project := h.createNode(h.root.ID, core.KindProject, "P", "fake")
	task := h.createNode(core.NodeID(project.ID), core.KindTask, "T", "")

	var sess SessionDTO
	h.decode(h.do(http.MethodPost, "/api/v1/nodes/"+task.ID+"/sessions", map[string]string{
		"mode":   "headless",
		"prompt": "do it",
	}), http.StatusCreated, &sess)
	if sess.NodeID != task.ID || sess.Driver != "fake" || sess.Mode != "headless" {
		t.Fatalf("session = %+v, want fake/headless bound to task", sess)
	}

	// Events flow through and are visible over REST.
	h.waitEvents(task.ID, func(e []EventDTO) bool {
		return hasEventType(e, core.EventSessionStarted) && hasEventType(e, core.EventText)
	})

	// The follow-up prompt unblocks the agent, which finishes and exits 0.
	h.decode(h.do(http.MethodPost, "/api/v1/nodes/"+task.ID+"/prompt", map[string]string{
		"text": "continue",
	}), http.StatusNoContent, nil)

	// The prompt is echoed into history as a user-role text event.
	h.waitEvents(task.ID, func(e []EventDTO) bool { return hasUserPrompt(e, "continue") })

	h.waitNode(core.NodeID(task.ID), func(n core.Node) bool { return n.Status == core.StatusDone })
	h.waitEvents(task.ID, func(e []EventDTO) bool { return hasEventType(e, core.EventTurnDone) })
}

// hasUserPrompt reports whether events contain a user-role text event with text.
func hasUserPrompt(events []EventDTO, text string) bool {
	for _, e := range events {
		if e.Type != string(core.EventText) {
			continue
		}
		var p core.TextPayload
		if json.Unmarshal(e.Payload, &p) == nil && p.Role == "user" && p.Text == text {
			return true
		}
	}
	return false
}

func TestSessionStop(t *testing.T) {
	h := newHarness(t, []fakeagent.Step{
		{Emit: `{"event":"session_started","payload":{"driver_session_id":"sess-2"}}`},
		{WaitStdinLine: true}, // block forever (no prompt is sent)
		{ExitCode: intPtr(0)},
	})
	project := h.createNode(h.root.ID, core.KindProject, "P", "fake")
	task := h.createNode(core.NodeID(project.ID), core.KindTask, "T", "")

	var sess SessionDTO
	h.decode(h.do(http.MethodPost, "/api/v1/nodes/"+task.ID+"/sessions", map[string]string{
		"mode": "headless",
	}), http.StatusCreated, &sess)
	h.waitEvents(task.ID, func(e []EventDTO) bool { return hasEventType(e, core.EventSessionStarted) })

	h.decode(h.do(http.MethodPost, "/api/v1/sessions/"+sess.ID+"/stop", nil), http.StatusNoContent, nil)

	// After stop the session reaches a terminal status.
	got := h.waitNode(core.NodeID(task.ID), func(n core.Node) bool { return n.Status.Terminal() })
	if got.Status != core.StatusFailed && got.Status != core.StatusDone {
		t.Errorf("node status after stop = %s, want a terminal status", got.Status)
	}
}

func TestPromptNoSession(t *testing.T) {
	h := newHarness(t, nil)
	project := h.createNode(h.root.ID, core.KindProject, "P", "fake")
	task := h.createNode(core.NodeID(project.ID), core.KindTask, "T", "")

	resp := h.do(http.MethodPost, "/api/v1/nodes/"+task.ID+"/prompt", map[string]string{"text": "hi"})
	h.decode(resp, http.StatusNotFound, nil)
}

func TestStopUnknownSession(t *testing.T) {
	h := newHarness(t, nil)
	resp := h.do(http.MethodPost, "/api/v1/sessions/does-not-exist/stop", nil)
	h.decode(resp, http.StatusNotFound, nil)
}
