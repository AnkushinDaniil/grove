package api

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
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

// claudeTranscriptExists reports whether a conversation id has a persisted,
// NON-EMPTY transcript under the default claude config dir (any project
// slug). A file alone is not enough: interrupted sessions can leave a
// few-hundred-byte "last-prompt" stub with no conversation in it, and
// resuming a stub reproduces "No conversation found".
func (h *Handlers) claudeTranscriptExists(id string) bool {
	home, err := h.home()
	if err != nil || home == "" || id == "" {
		return true // cannot verify — do not block the attempt
	}
	matches, err := filepath.Glob(filepath.Join(home, ".claude", "projects", "*", id+".jsonl"))
	if err != nil || len(matches) == 0 {
		return false
	}
	return transcriptHasConversation(matches[0])
}

// transcriptHasConversation scans the head of a transcript for an actual
// message entry (stubs carry only bookkeeping lines).
func transcriptHasConversation(path string) bool {
	//nolint:gosec // G703: path comes from a home-rooted glob over the user's own transcripts (single-user loopback daemon; see fs.go trust model).
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	head := make([]byte, 32<<10)
	n, _ := io.ReadFull(f, head)
	return bytes.Contains(head[:n], []byte(`"message"`))
}
