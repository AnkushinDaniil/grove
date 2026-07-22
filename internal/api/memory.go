package api

import (
	"context"
	"net/http"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/memory"
)

// Memory reads a node's MemPalace-backed memory for GET /nodes/{id}/memory. It
// is the seam the daemon fills with *memory.Client; a nil Memory leaves the
// endpoint reporting an unavailable backend (healthy:false), which is exactly
// how the UI degrades when MemPalace is not installed.
type Memory interface {
	NodeMemory(ctx context.Context, nodeID core.NodeID, scope memory.Scope) memory.Result
}

// MemoryEntryDTO is the wire shape of one memory item (docs/API.md, Node
// memory). created_at is passed through as MemPalace's ISO 8601 string.
type MemoryEntryDTO struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Content   string `json:"content"`
	Source    string `json:"source"`
	CreatedAt string `json:"created_at"`
}

// memoryResponse is the GET /nodes/{id}/memory body. healthy is false with an
// empty backend when MemPalace is unavailable.
type memoryResponse struct {
	Entries []MemoryEntryDTO `json:"entries"`
	Backend string           `json:"backend"`
	Healthy bool             `json:"healthy"`
}

// handleNodeMemory returns a node's in-scope memory. An unknown node is a 404;
// an unavailable backend is a healthy 200 with an empty result (never a 5xx),
// so the tab shows a "run `grove memory install`" hint instead of an error.
func (h *Handlers) handleNodeMemory(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	if _, ok := h.tree.Get(id); !ok {
		writeErrorStatus(w, h.logger, http.StatusNotFound, "node not found")
		return
	}
	scope := memory.ParseScope(r.URL.Query().Get("scope"))
	var res memory.Result
	if h.memory != nil {
		res = h.memory.NodeMemory(r.Context(), id, scope)
	}
	writeJSON(w, h.logger, http.StatusOK, memoryResponse{
		Entries: memoryEntriesToDTO(res.Entries),
		Backend: res.Backend,
		Healthy: res.Healthy,
	})
}

// memoryEntriesToDTO maps entries to their wire shape, always returning a
// non-nil slice so the contract's "entries:[]" holds when there are none.
func memoryEntriesToDTO(entries []memory.Entry) []MemoryEntryDTO {
	out := make([]MemoryEntryDTO, 0, len(entries))
	for _, e := range entries {
		out = append(out, MemoryEntryDTO{
			ID:        e.ID,
			Kind:      e.Kind,
			Content:   e.Content,
			Source:    e.Source,
			CreatedAt: e.CreatedAt,
		})
	}
	return out
}
