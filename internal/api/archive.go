package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/session"
)

// archiveTimeout bounds the whole archive operation, including waiting for live
// sessions in the subtree to stop.
const archiveTimeout = 30 * time.Second

// archiveResponse is the POST /nodes/{id}/archive body.
type archiveResponse struct {
	Archived []string `json:"archived"`
}

// handleArchiveNode archives a node and its live subtree: it stops every live
// session in the subtree, archives the nodes, then removes clean worktrees and
// orphans dirty ones (keeping uncommitted work on disk).
func (h *Handlers) handleArchiveNode(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)

	// Capture the subtree before archiving; afterwards these nodes are archived
	// and excluded from the live subtree query. Detach from the request context
	// so the operation completes even if the client disconnects mid-archive.
	subtree := h.tree.SubtreeIDs(id)
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), archiveTimeout)
	defer cancel()

	h.stopSessions(ctx, subtree)

	archived, err := h.tree.Archive(ctx, id)
	if err != nil {
		writeError(w, h.logger, err)
		return
	}
	for _, nid := range archived {
		h.cleanupWorktrees(ctx, nid)
	}

	ids := make([]string, 0, len(archived))
	for _, nid := range archived {
		ids = append(ids, string(nid))
	}
	writeJSON(w, h.logger, http.StatusOK, archiveResponse{Archived: ids})
}

// stopSessions stops the live session (if any) of each node id, ignoring the
// already-exited case.
func (h *Handlers) stopSessions(ctx context.Context, ids []core.NodeID) {
	for _, nid := range ids {
		sess, ok := h.tree.SessionFor(nid)
		if !ok || sess.Status.Terminal() {
			continue
		}
		if err := h.sessions.Stop(ctx, sess.ID); err != nil && !errors.Is(err, session.ErrSessionNotFound) {
			h.logger.Warn("stop session during archive", "session", sess.ID, "err", err)
		}
	}
}
