package api

import (
	"context"
	"net/http"
	"time"

	"github.com/AnkushinDaniil/grove/internal/core"
)

// stopTimeout bounds a stop request so it returns even if a process ignores the
// initial SIGTERM (the manager escalates to SIGKILL within this window).
const stopTimeout = 10 * time.Second

// createSessionRequest is the POST /nodes/{id}/sessions body.
type createSessionRequest struct {
	Mode     string `json:"mode"`
	Prompt   string `json:"prompt"`
	ResumeID string `json:"resume_id"`
}

// handleCreateSession starts a session for a node. Native-hook wiring (minting
// the per-node token and embedding the hook command) is owned by the session
// manager, which gates it on the resolved driver's capabilities.
func (h *Handlers) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	var req createSessionRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "invalid request body")
		return
	}
	id := pathID(r)
	if req.ResumeID != "" {
		resumeID, err := h.resolveResumeID(r.Context(), id, req.ResumeID)
		if err != nil {
			writeErrorStatus(w, h.logger, http.StatusBadRequest, err.Error())
			return
		}
		req.ResumeID = resumeID
	}
	// Detach from the request: a launched session outlives the HTTP request and
	// must not be torn down if the client disconnects.
	ctx := context.WithoutCancel(r.Context())
	// Headless sessions run as orchestrators (grove MCP tools mounted) so any
	// autonomous agent can drive the tree — spawn children, report, complete.
	// Interactive PTY sessions stay plain workers the user drives by hand.
	var sess core.Session
	var err error
	if core.SessionMode(req.Mode) == core.ModeHeadless && h.orchestrator != nil {
		sess, err = h.orchestrator.LaunchOrchestrator(ctx, id, req.Prompt, req.ResumeID)
	} else {
		sess, err = h.sessions.Start(ctx, id, core.SessionMode(req.Mode), req.Prompt, req.ResumeID)
	}
	if err != nil {
		writeError(w, h.logger, err)
		return
	}
	writeJSON(w, h.logger, http.StatusCreated, SessionToDTO(sess))
}

// promptRequest is the POST /nodes/{id}/prompt body.
type promptRequest struct {
	Text string `json:"text"`
}

// handlePrompt delivers a follow-up prompt to a node's current session. The
// prompt is echoed into the node's history as a user-role text event before it
// is forwarded, so headless conversation views show the user's side of the turn.
func (h *Handlers) handlePrompt(w http.ResponseWriter, r *http.Request) {
	var req promptRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "invalid request body")
		return
	}
	nodeID := pathID(r)
	sess, ok := h.tree.SessionFor(nodeID)
	if !ok {
		writeErrorStatus(w, h.logger, http.StatusNotFound, "no session for node")
		return
	}
	h.ingestUserPrompt(r.Context(), nodeID, sess.ID, req.Text)
	if err := h.sessions.Prompt(r.Context(), sess.ID, req.Text); err != nil {
		writeError(w, h.logger, err)
		return
	}
	// Answering IS acknowledging: the user just responded to whatever the
	// node was waiting on, so the attention flag auto-resolves.
	h.autoAck(r.Context(), nodeID)
	w.WriteHeader(http.StatusNoContent)
}

// autoAck clears a node's attention (and its unacked events) as a side effect
// of the user responding; failures only log — the primary action succeeded.
func (h *Handlers) autoAck(ctx context.Context, nodeID core.NodeID) {
	if _, err := h.tree.Ack(ctx, nodeID); err != nil {
		h.logger.Warn("auto-ack node", "node", nodeID, "err", err)
	}
}

// ingestUserPrompt records the user's prompt as a user-role text event on the
// node so it appears in the conversation history alongside agent output.
func (h *Handlers) ingestUserPrompt(ctx context.Context, nodeID core.NodeID, sessionID core.SessionID, text string) {
	payload, err := core.MarshalPayload(core.TextPayload{Text: text, Role: "user"})
	if err != nil {
		h.logger.Error("marshal user prompt payload", "err", err)
		return
	}
	if _, err := h.tree.IngestEvents(ctx, nodeID, sessionID, []core.EventInput{{
		Type:    core.EventText,
		Payload: payload,
	}}); err != nil {
		h.logger.Error("ingest user prompt event", "node", nodeID, "err", err)
	}
}

// handleStopSession stops a live session by id.
func (h *Handlers) handleStopSession(w http.ResponseWriter, r *http.Request) {
	sid := core.SessionID(r.PathValue("id"))
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), stopTimeout)
	defer cancel()
	if err := h.sessions.Stop(ctx, sid); err != nil {
		writeError(w, h.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
