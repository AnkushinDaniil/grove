package api

import (
	"net/http"
	"testing"

	"github.com/AnkushinDaniil/grove/internal/core"
)

// postFeedback POSTs a valid feedback item and returns the created DTO.
func (h *harness) postFeedback(nodeID, kind, subject, comment string) feedbackDTO {
	h.t.Helper()
	var f feedbackDTO
	h.decode(h.do(http.MethodPost, "/api/v1/feedback", map[string]any{
		"node_id": nodeID,
		"kind":    kind,
		"subject": subject,
		"comment": comment,
	}), http.StatusCreated, &f)
	return f
}

func TestFeedbackCreateListResolve(t *testing.T) {
	h := newHarness(t, nil)
	node := h.createNode(h.root.ID, core.KindProject, "Proj", "fake")

	created := h.postFeedback(node.ID, "skill", "code-review", "misfired")
	if created.ID == "" || created.Kind != "skill" || created.Subject != "code-review" {
		t.Fatalf("created = %+v, want a skill/code-review item with an id", created)
	}
	if created.ResolvedAt != nil {
		t.Errorf("new feedback resolved_at = %v, want omitted", created.ResolvedAt)
	}

	// Open list contains it.
	var open []feedbackDTO
	h.decode(h.do(http.MethodGet, "/api/v1/feedback?status=open", nil), http.StatusOK, &open)
	if len(open) != 1 || open[0].ID != created.ID {
		t.Fatalf("open list = %+v, want [%s]", open, created.ID)
	}

	// Resolve with a fix node.
	var resolved feedbackDTO
	h.decode(h.do(http.MethodPost, "/api/v1/feedback/"+created.ID+"/resolve", map[string]any{
		"fix_node_id": "fix-node-1",
	}), http.StatusOK, &resolved)
	if resolved.ResolvedAt == nil || resolved.FixNodeID != "fix-node-1" {
		t.Errorf("resolved = %+v, want resolved_at set and fix-node-1", resolved)
	}

	// Now open is empty and resolved has the item.
	var openAfter, resolvedList []feedbackDTO
	h.decode(h.do(http.MethodGet, "/api/v1/feedback?status=open", nil), http.StatusOK, &openAfter)
	if len(openAfter) != 0 {
		t.Errorf("open after resolve = %+v, want empty", openAfter)
	}
	h.decode(h.do(http.MethodGet, "/api/v1/feedback?status=resolved", nil), http.StatusOK, &resolvedList)
	if len(resolvedList) != 1 || resolvedList[0].ID != created.ID {
		t.Errorf("resolved list = %+v, want [%s]", resolvedList, created.ID)
	}

	// Default (no status) lists all.
	var all []feedbackDTO
	h.decode(h.do(http.MethodGet, "/api/v1/feedback", nil), http.StatusOK, &all)
	if len(all) != 1 {
		t.Errorf("all list = %+v, want 1", all)
	}
}

func TestFeedbackValidation(t *testing.T) {
	h := newHarness(t, nil)
	node := h.createNode(h.root.ID, core.KindProject, "Proj", "fake")

	// Invalid kind → 400.
	h.decode(h.do(http.MethodPost, "/api/v1/feedback", map[string]any{
		"node_id": node.ID, "kind": "bogus", "comment": "x",
	}), http.StatusBadRequest, nil)

	// Empty comment → 400.
	h.decode(h.do(http.MethodPost, "/api/v1/feedback", map[string]any{
		"node_id": node.ID, "kind": "tool", "comment": "",
	}), http.StatusBadRequest, nil)

	// Unknown node → 400.
	h.decode(h.do(http.MethodPost, "/api/v1/feedback", map[string]any{
		"node_id": "does-not-exist", "kind": "tool", "comment": "x",
	}), http.StatusBadRequest, nil)

	// Invalid status filter → 400.
	h.decode(h.do(http.MethodGet, "/api/v1/feedback?status=bogus", nil), http.StatusBadRequest, nil)

	// Resolve unknown id → 404.
	h.decode(h.do(http.MethodPost, "/api/v1/feedback/missing/resolve", map[string]any{}), http.StatusNotFound, nil)
}
