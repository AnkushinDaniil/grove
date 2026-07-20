package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/AnkushinDaniil/grove/internal/core"
)

// hookPost posts a hook payload for a node with the given event and token.
func (h *harness) hookPost(node, event, token string, payload map[string]any) response {
	h.t.Helper()
	buf, err := json.Marshal(payload)
	if err != nil {
		h.t.Fatalf("marshal hook payload: %v", err)
	}
	url := h.ts.URL + PathHook + "?node=" + node + "&driver=fake&event=" + event
	req, err := http.NewRequestWithContext(h.t.Context(), http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		h.t.Fatalf("new hook request: %v", err)
	}
	req.Header.Set(hookTokenHeader, token)
	return h.doReq(req)
}

func TestHookRejectsBadToken(t *testing.T) {
	h := newHarness(t, nil)
	project := h.createNode(h.root.ID, core.KindProject, "P", "fake")
	task := h.createNode(core.NodeID(project.ID), core.KindTask, "T", "")
	token := h.hookTokens.Mint(core.NodeID(task.ID))

	resp := h.hookPost(task.ID, "Notification", token+"bad", map[string]any{
		"notification_type": "permission", "message": "allow?",
	})
	h.decode(resp, http.StatusUnauthorized, nil)
}

func TestHookNotificationRaisesAttention(t *testing.T) {
	h := newHarness(t, nil)
	project := h.createNode(h.root.ID, core.KindProject, "P", "fake")
	task := h.createNode(core.NodeID(project.ID), core.KindTask, "T", "")
	token := h.hookTokens.Mint(core.NodeID(task.ID))

	resp := h.hookPost(task.ID, "Notification", token, map[string]any{
		"notification_type": "permission",
		"message":           "allow tool run?",
	})
	h.decode(resp, http.StatusNoContent, nil)

	n := h.waitNode(core.NodeID(task.ID), func(n core.Node) bool { return n.Attention == core.AttentionPermission })
	if n.AttentionReason != "allow tool run?" {
		t.Errorf("attention reason = %q, want the notification message", n.AttentionReason)
	}
	h.waitEvents(task.ID, func(e []EventDTO) bool { return hasEventType(e, core.EventAwaitingInput) })
}

func TestHookAcceptsDaemonTokenFallback(t *testing.T) {
	h := newHarness(t, nil)
	project := h.createNode(h.root.ID, core.KindProject, "P", "fake")
	task := h.createNode(core.NodeID(project.ID), core.KindTask, "T", "")

	// The daemon token is accepted even without a minted per-node token.
	resp := h.hookPost(task.ID, "Stop", testToken, map[string]any{})
	h.decode(resp, http.StatusNoContent, nil)

	h.waitEvents(task.ID, func(e []EventDTO) bool { return hasEventType(e, core.EventTurnDone) })
}

func TestHookMissingEvent(t *testing.T) {
	h := newHarness(t, nil)
	project := h.createNode(h.root.ID, core.KindProject, "P", "fake")
	task := h.createNode(core.NodeID(project.ID), core.KindTask, "T", "")
	token := h.hookTokens.Mint(core.NodeID(task.ID))

	// No ?event= and no hook_event_name in the payload → 400.
	resp := h.hookPost(task.ID, "", token, map[string]any{})
	h.decode(resp, http.StatusBadRequest, nil)
}
