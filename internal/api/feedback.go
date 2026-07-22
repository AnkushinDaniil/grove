package api

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/store"
)

// feedbackKinds is the closed set of kinds accepted by POST /feedback
// (docs/API.md "Feedback loop").
var feedbackKinds = map[string]bool{
	"skill": true,
	"tool":  true,
	"model": true,
	"agent": true,
	"other": true,
}

// feedbackDTO is the wire representation of a store.Feedback. resolved_at is
// omitted while the item is still open (per the contract's "omitted when zero").
type feedbackDTO struct {
	ID         string  `json:"id"`
	NodeID     string  `json:"node_id"`
	SessionID  string  `json:"session_id"`
	EventID    string  `json:"event_id"`
	Kind       string  `json:"kind"`
	Subject    string  `json:"subject"`
	Comment    string  `json:"comment"`
	CreatedAt  string  `json:"created_at"`
	ResolvedAt *string `json:"resolved_at,omitempty"`
	FixNodeID  string  `json:"fix_node_id"`
}

func feedbackToDTO(f store.Feedback) feedbackDTO {
	return feedbackDTO{
		ID:         f.ID,
		NodeID:     f.NodeID,
		SessionID:  f.SessionID,
		EventID:    f.EventID,
		Kind:       f.Kind,
		Subject:    f.Subject,
		Comment:    f.Comment,
		CreatedAt:  rfc3339(f.CreatedAt),
		ResolvedAt: rfc3339Ptr(f.ResolvedAt),
		FixNodeID:  f.FixNodeID,
	}
}

func feedbacksToDTO(fs []store.Feedback) []feedbackDTO {
	out := make([]feedbackDTO, 0, len(fs))
	for _, f := range fs {
		out = append(out, feedbackToDTO(f))
	}
	return out
}

// createFeedbackRequest is the POST /feedback body.
type createFeedbackRequest struct {
	NodeID    string `json:"node_id"`
	SessionID string `json:"session_id"`
	EventID   string `json:"event_id"`
	Kind      string `json:"kind"`
	Subject   string `json:"subject"`
	Comment   string `json:"comment"`
}

// handleCreateFeedback records a new quality signal: kind must be in the closed
// set, the node must exist, and the comment must be non-empty (all → 400).
func (h *Handlers) handleCreateFeedback(w http.ResponseWriter, r *http.Request) {
	var req createFeedbackRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "invalid request body")
		return
	}
	if !feedbackKinds[req.Kind] {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "kind must be one of skill, tool, model, agent, other")
		return
	}
	if req.Comment == "" {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "comment must not be empty")
		return
	}
	if _, ok := h.tree.Get(core.NodeID(req.NodeID)); !ok {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "unknown node_id")
		return
	}

	f := store.Feedback{
		ID:        uuid.Must(uuid.NewV7()).String(),
		NodeID:    req.NodeID,
		SessionID: req.SessionID,
		EventID:   req.EventID,
		Kind:      req.Kind,
		Subject:   req.Subject,
		Comment:   req.Comment,
		CreatedAt: time.Now(),
	}
	if err := h.store.SaveFeedback(r.Context(), f); err != nil {
		writeError(w, h.logger, err)
		return
	}
	writeJSON(w, h.logger, http.StatusCreated, feedbackToDTO(f))
}

// handleListFeedback lists feedback filtered by status. An omitted status
// defaults to "all"; any other value outside open|resolved|all → 400.
func (h *Handlers) handleListFeedback(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	if status == "" {
		status = store.FeedbackAll
	}
	switch status {
	case store.FeedbackOpen, store.FeedbackResolved, store.FeedbackAll:
	default:
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "status must be open, resolved, or all")
		return
	}
	items, err := h.store.ListFeedback(r.Context(), status)
	if err != nil {
		writeError(w, h.logger, err)
		return
	}
	writeJSON(w, h.logger, http.StatusOK, feedbacksToDTO(items))
}

// resolveFeedbackRequest is the POST /feedback/{id}/resolve body; fix_node_id is
// optional (links the task node created to address the feedback).
type resolveFeedbackRequest struct {
	FixNodeID string `json:"fix_node_id"`
}

// handleResolveFeedback marks a feedback item resolved, optionally linking the
// fix node. An unknown id → 404.
func (h *Handlers) handleResolveFeedback(w http.ResponseWriter, r *http.Request) {
	var req resolveFeedbackRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "invalid request body")
		return
	}
	f, err := h.store.ResolveFeedback(r.Context(), r.PathValue("id"), req.FixNodeID, time.Now())
	if err != nil {
		writeError(w, h.logger, err)
		return
	}
	writeJSON(w, h.logger, http.StatusOK, feedbackToDTO(f))
}
