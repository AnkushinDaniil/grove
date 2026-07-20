package api

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/AnkushinDaniil/grove/internal/core"
)

func TestAckClearsAttention(t *testing.T) {
	h := newHarness(t, nil)
	project := h.createNode(h.root.ID, core.KindProject, "P", "fake")
	task := h.createNode(core.NodeID(project.ID), core.KindTask, "T", "")
	token := h.hookTokens.Mint(core.NodeID(task.ID))

	// Raise attention via a hook, then acknowledge it.
	h.decode(h.hookPost(task.ID, "Notification", token, map[string]any{
		"notification_type": "permission", "message": "allow?",
	}), http.StatusNoContent, nil)
	h.waitNode(core.NodeID(task.ID), func(n core.Node) bool { return n.Attention == core.AttentionPermission })

	var acked NodeDTO
	h.decode(h.do(http.MethodPost, "/api/v1/nodes/"+task.ID+"/ack", nil), http.StatusOK, &acked)
	if acked.Attention != string(core.AttentionNone) {
		t.Errorf("attention after ack = %s, want none", acked.Attention)
	}

	// The acked event no longer appears in the inbox.
	var inbox []EventDTO
	h.decode(h.do(http.MethodGet, "/api/v1/inbox", nil), http.StatusOK, &inbox)
	if slices.ContainsFunc(inbox, func(e EventDTO) bool { return e.NodeID == task.ID }) {
		t.Error("acked event still present in inbox")
	}
}

func TestAckUnknownNode(t *testing.T) {
	h := newHarness(t, nil)
	h.decode(h.do(http.MethodPost, "/api/v1/nodes/nope/ack", nil), http.StatusNotFound, nil)
}

func TestPatchUnknownNode(t *testing.T) {
	h := newHarness(t, nil)
	resp := h.do(http.MethodPatch, "/api/v1/nodes/nope", map[string]any{"title": "x"})
	h.decode(resp, http.StatusNotFound, nil)
}

func TestListEventsWithLimit(t *testing.T) {
	h := newHarness(t, nil)
	project := h.createNode(h.root.ID, core.KindProject, "P", "fake")
	task := h.createNode(core.NodeID(project.ID), core.KindTask, "T", "")
	token := h.hookTokens.Mint(core.NodeID(task.ID))
	for range 3 {
		h.decode(h.hookPost(task.ID, "Stop", token, map[string]any{}), http.StatusNoContent, nil)
	}

	var events []EventDTO
	h.decode(h.do(http.MethodGet, "/api/v1/nodes/"+task.ID+"/events?limit=2", nil), http.StatusOK, &events)
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2 (limit honored)", len(events))
	}
	// Ascending by id: paging after the first returns later events.
	var page []EventDTO
	h.decode(h.do(http.MethodGet, "/api/v1/nodes/"+task.ID+"/events?after="+events[0].ID, nil), http.StatusOK, &page)
	if len(page) == 0 || page[0].ID <= events[0].ID {
		t.Errorf("paging after %s returned %d events starting %v", events[0].ID, len(page), page)
	}
}

func TestAuthorizedModes(t *testing.T) {
	auth := NewAuth(testToken)

	tests := []struct {
		name  string
		build func(*http.Request)
		want  bool
	}{
		{"no credentials", func(*http.Request) {}, false},
		{"valid cookie", func(r *http.Request) {
			r.AddCookie(&http.Cookie{Name: authCookie, Value: testToken})
		}, true},
		{"wrong cookie", func(r *http.Request) {
			r.AddCookie(&http.Cookie{Name: authCookie, Value: "nope"})
		}, false},
		{"valid bearer", func(r *http.Request) {
			r.Header.Set("Authorization", "Bearer "+testToken)
		}, true},
		{"wrong bearer", func(r *http.Request) {
			r.Header.Set("Authorization", "Bearer nope")
		}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/api/v1/tree", nil)
			tc.build(r)
			if got := auth.Authorized(r); got != tc.want {
				t.Errorf("Authorized = %v, want %v", got, tc.want)
			}
		})
	}
}
