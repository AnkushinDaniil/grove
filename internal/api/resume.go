package api

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
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
	id, ok := h.resumableTarget(ctx, nodeID, requested)
	if !ok {
		return "", fmt.Errorf(
			"conversation %s not found and no earlier resumable conversation exists for this node — start a new session",
			requested,
		)
	}
	return id, nil
}

// resumableTarget returns the best conversation id to resume for a node: the
// preferred id if its transcript has real content, else the node's most recent
// earlier session whose conversation still exists. ok is false when nothing is
// resumable (e.g. cmux-era sessions whose conversation was never written as a
// standard transcript). Non-claude drivers always pass the preferred id
// through (their layout is unknown).
func (h *Handlers) resumableTarget(ctx context.Context, nodeID core.NodeID, preferred string) (string, bool) {
	resolved, ok := h.tree.Resolve(nodeID)
	if !ok || resolved.Driver != "claude" {
		return preferred, preferred != ""
	}
	if h.claudeTranscriptExists(preferred) {
		return preferred, true
	}
	history, err := h.store.SessionsForNode(ctx, nodeID)
	if err != nil {
		h.logger.Warn("resume target: session history unavailable", "node", nodeID, "err", err)
		return preferred, preferred != "" // let the CLI report the failure rather than block
	}
	for _, sess := range history {
		if sess.DriverSessionID != "" && sess.DriverSessionID != preferred &&
			h.claudeTranscriptExists(sess.DriverSessionID) {
			return sess.DriverSessionID, true
		}
	}
	return "", false
}

// resumeTargetResponse tells the UI whether a node's latest session can be
// resumed, and with which conversation id.
type resumeTargetResponse struct {
	Resumable       bool   `json:"resumable"`
	DriverSessionID string `json:"driver_session_id"`
	Reason          string `json:"reason"`
}

// handleResumeTarget answers whether the node's latest session is resumable,
// so the terminal view can present an honest (enabled/disabled) Resume control
// instead of a button that always looks active and then errors.
func (h *Handlers) handleResumeTarget(w http.ResponseWriter, r *http.Request) {
	nodeID := pathID(r)
	sess, ok := h.tree.SessionFor(nodeID)
	if !ok {
		writeJSON(w, h.logger, http.StatusOK, resumeTargetResponse{
			Reason: "this node has no session yet",
		})
		return
	}
	id, resumable := h.resumableTarget(r.Context(), nodeID, sess.DriverSessionID)
	resp := resumeTargetResponse{Resumable: resumable, DriverSessionID: id}
	if !resumable {
		resp.Reason = "the previous conversation was not saved as a resumable transcript " +
			"(sessions run through a third-party wrapper like cmux keep their history internally) — start a new session"
	}
	writeJSON(w, h.logger, http.StatusOK, resp)
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
