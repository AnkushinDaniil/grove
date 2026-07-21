package api

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/AnkushinDaniil/grove/internal/core"
)

// resolveResumeID validates a requested claude conversation id against the
// transcripts that actually exist on disk, falling back to the node's most
// recent session whose conversation is still resumable.
//
// Why: a failed resume attempt mints a fresh session id via its own
// SessionStart hook and dies without persisting a conversation — resuming
// that id again just reproduces "No conversation found" forever. Grounding
// the id in an existing transcript breaks the poison loop.
func (h *Handlers) resolveResumeID(ctx context.Context, nodeID core.NodeID, requested string) (string, error) {
	resolved, ok := h.tree.Resolve(nodeID)
	if !ok || resolved.Driver != "claude" {
		// Only claude's transcript layout is known; other drivers pass through.
		return requested, nil
	}
	if h.claudeTranscriptExists(requested) {
		return requested, nil
	}
	history, err := h.store.SessionsForNode(ctx, nodeID)
	if err != nil {
		h.logger.Warn("resume fallback: session history unavailable", "node", nodeID, "err", err)
		return requested, nil // let the CLI report the failure rather than block
	}
	for _, sess := range history {
		if sess.DriverSessionID != "" && sess.DriverSessionID != requested &&
			h.claudeTranscriptExists(sess.DriverSessionID) {
			h.logger.Info("resume fallback: replacing unknown conversation id",
				"node", nodeID, "requested", requested, "using", sess.DriverSessionID)
			return sess.DriverSessionID, nil
		}
	}
	return "", fmt.Errorf(
		"conversation %s not found and no earlier resumable conversation exists for this node — start a new session",
		requested,
	)
}

// claudeTranscriptExists reports whether a conversation id has a persisted
// transcript under the default claude config dir (any project slug).
func (h *Handlers) claudeTranscriptExists(id string) bool {
	home, err := h.home()
	if err != nil || home == "" || id == "" {
		return true // cannot verify — do not block the attempt
	}
	matches, err := filepath.Glob(filepath.Join(home, ".claude", "projects", "*", id+".jsonl"))
	return err == nil && len(matches) > 0
}
