package api

import (
	"net/http"
	"strconv"

	"github.com/AnkushinDaniil/grove/internal/core"
)

// handleListEvents returns a node's events ascending by id, keyset-paginated by
// the optional after (last seen event id) and limit query parameters.
func (h *Handlers) handleListEvents(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	after := core.EventID(r.URL.Query().Get("after"))
	limit := 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			limit = n // the store clamps non-positive and oversized values
		}
	}
	events, err := h.store.ListEvents(r.Context(), id, after, limit)
	if err != nil {
		writeError(w, h.logger, err)
		return
	}
	writeJSON(w, h.logger, http.StatusOK, EventsToDTO(events))
}

// handleInbox returns every unacknowledged attention-requiring event, newest
// first.
func (h *Handlers) handleInbox(w http.ResponseWriter, r *http.Request) {
	events, err := h.store.ListInbox(r.Context())
	if err != nil {
		writeError(w, h.logger, err)
		return
	}
	writeJSON(w, h.logger, http.StatusOK, EventsToDTO(events))
}
