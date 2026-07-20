package api

import (
	"net/http"

	"github.com/AnkushinDaniil/grove/internal/core"
)

// hookTokenHeader carries the per-node hook token on agent callbacks.
//
//nolint:gosec // G101: this is an HTTP header name, not a credential value.
const hookTokenHeader = "X-Grove-Hook-Token"

// handleHook receives an agent's hook callback and dispatches it onto tree
// state via the session manager. It authenticates the per-node hook token (with
// the daemon token accepted as a fallback so hooks survive a registry reset),
// then resolves the hook event name from the query override or the payload.
func (h *Handlers) handleHook(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	node := core.NodeID(q.Get("node"))
	if node == "" {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "missing node")
		return
	}
	token := r.Header.Get(hookTokenHeader)
	if !h.hookTokens.Valid(node, token) && !h.auth.Matches(token) {
		writeErrorStatus(w, h.logger, http.StatusUnauthorized, "invalid hook token")
		return
	}

	var payload map[string]any
	if err := decodeJSON(w, r, &payload); err != nil {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "invalid hook payload")
		return
	}

	hookEvent := q.Get("event")
	if hookEvent == "" {
		hookEvent, _ = payload["hook_event_name"].(string)
	}
	if hookEvent == "" {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "missing hook event")
		return
	}

	if err := h.sessions.ApplyHook(r.Context(), node, hookEvent, payload); err != nil {
		writeError(w, h.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
