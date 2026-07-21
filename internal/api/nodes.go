package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/tree"
)

// treeResponse is the GET /tree body: a revision plus flat node and session
// lists, mirroring tree.Snapshot.
type treeResponse struct {
	Rev      uint64       `json:"rev"`
	Nodes    []NodeDTO    `json:"nodes"`
	Sessions []SessionDTO `json:"sessions"`
}

// handleTree returns the whole live tree at the current revision.
func (h *Handlers) handleTree(w http.ResponseWriter, _ *http.Request) {
	snap := h.tree.Snapshot()
	writeJSON(w, h.logger, http.StatusOK, treeResponse{
		Rev:      snap.Rev,
		Nodes:    NodesToDTO(snap.Nodes),
		Sessions: SessionsToDTO(snap.Sessions),
	})
}

// createNodeRequest is the POST /nodes body.
type createNodeRequest struct {
	ParentID  string `json:"parent_id"`
	Kind      string `json:"kind"`
	Title     string `json:"title"`
	Brief     string `json:"brief"`
	Driver    string `json:"driver"`
	ProfileID string `json:"profile_id"`
	WorkDir   string `json:"work_dir"`
}

// handleCreateNode creates a node and, for task nodes, provisions per-repo
// worktrees against the nearest project ancestor. A worktree failure never
// loses the node: the node is created and the failure is surfaced as an error
// event raising attention.
func (h *Handlers) handleCreateNode(w http.ResponseWriter, r *http.Request) {
	var req createNodeRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.WorkDir != "" {
		normalized, err := h.resolveWorkDir(req.WorkDir)
		if err != nil {
			writeErrorStatus(w, h.logger, http.StatusBadRequest, workDirErrMsg(err))
			return
		}
		req.WorkDir = normalized
	}
	node, err := h.tree.CreateNode(r.Context(), tree.CreateSpec{
		ParentID:  core.NodeID(req.ParentID),
		Kind:      core.Kind(req.Kind),
		Title:     req.Title,
		Brief:     req.Brief,
		Driver:    req.Driver,
		ProfileID: core.ProfileID(req.ProfileID),
		WorkDir:   req.WorkDir,
	})
	if err != nil {
		writeError(w, h.logger, err)
		return
	}
	if node.Kind == core.KindTask {
		// Detach from the request context so worktree provisioning (git
		// operations) completes and records its result even if the client
		// disconnects; the node already exists.
		node = h.provisionTask(context.WithoutCancel(r.Context()), node)
	}
	writeJSON(w, h.logger, http.StatusCreated, NodeToDTO(node))
}

// patchNodeRequest is the PATCH /nodes/{id} body; nil fields are left unchanged.
type patchNodeRequest struct {
	Title     *string         `json:"title"`
	Brief     *string         `json:"brief"`
	Driver    *string         `json:"driver"`
	ProfileID *string         `json:"profile_id"`
	WorkDir   *string         `json:"work_dir"`
	Meta      json.RawMessage `json:"meta"`
}

// handlePatchNode applies a partial update to a node.
func (h *Handlers) handlePatchNode(w http.ResponseWriter, r *http.Request) {
	var req patchNodeRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "invalid request body")
		return
	}
	patch := tree.Patch{Title: req.Title, Brief: req.Brief, Driver: req.Driver}
	if req.ProfileID != nil {
		pid := core.ProfileID(*req.ProfileID)
		patch.ProfileID = &pid
	}
	if req.WorkDir != nil {
		// An explicit empty string clears the override (fall back to inheritance),
		// so it skips the existence check; a non-empty value must resolve.
		if *req.WorkDir != "" {
			normalized, err := h.resolveWorkDir(*req.WorkDir)
			if err != nil {
				writeErrorStatus(w, h.logger, http.StatusBadRequest, workDirErrMsg(err))
				return
			}
			req.WorkDir = &normalized
		}
		patch.WorkDir = req.WorkDir
	}
	if len(req.Meta) > 0 {
		if !isJSONObject(req.Meta) {
			writeErrorStatus(w, h.logger, http.StatusBadRequest, "meta must be a JSON object")
			return
		}
		meta := string(req.Meta)
		patch.Meta = &meta
	}
	node, err := h.tree.UpdateNode(r.Context(), pathID(r), patch)
	if err != nil {
		writeError(w, h.logger, err)
		return
	}
	writeJSON(w, h.logger, http.StatusOK, NodeToDTO(node))
}

// handleAckNode clears a node's attention flag and marks its unacknowledged
// attention events as read.
func (h *Handlers) handleAckNode(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	if _, err := h.store.AckNodeEvents(r.Context(), id, time.Now()); err != nil {
		writeError(w, h.logger, err)
		return
	}
	node, err := h.tree.Ack(r.Context(), id)
	if err != nil {
		writeError(w, h.logger, err)
		return
	}
	writeJSON(w, h.logger, http.StatusOK, NodeToDTO(node))
}

// pathID reads the {id} path wildcard as a NodeID (also used for session ids).
func pathID(r *http.Request) core.NodeID { return core.NodeID(r.PathValue("id")) }

// isJSONObject reports whether raw is a JSON object (the shape node meta takes).
func isJSONObject(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && trimmed[0] == '{' && json.Valid(trimmed)
}

// normalizeWorkDir expands the shorthands the completion UI produces into an
// absolute path: "~" and "~/x" expand against home, a bare relative path is
// treated as home-relative, and absolute paths are cleaned. Empty stays empty
// (it means "inherit"). An empty home leaves relative input untouched so
// validation reports it instead of fabricating a path.
func normalizeWorkDir(home, dir string) string {
	switch {
	case dir == "" || home == "":
		return dir
	case dir == "~":
		return home
	case strings.HasPrefix(dir, "~/"):
		return filepath.Join(home, dir[2:])
	case !filepath.IsAbs(dir):
		return filepath.Join(home, dir)
	default:
		return filepath.Clean(dir)
	}
}

// resolveWorkDir normalizes a non-empty work dir against the daemon user's
// home and enforces that it exists as a directory, returning the absolute path
// to store.
func (h *Handlers) resolveWorkDir(dir string) (string, error) {
	home, err := h.home()
	if err != nil {
		home = ""
	}
	normalized := normalizeWorkDir(home, dir)
	if err := validateWorkDir(normalized); err != nil {
		return "", err
	}
	return normalized, nil
}

// validateWorkDir enforces the API-layer rule for a set (non-empty) work dir: it
// must be an absolute path to a directory that exists on the daemon host, so a
// session started there does not fail at spawn time. The returned error's text
// is the actionable detail surfaced to the client.
func validateWorkDir(dir string) error {
	if !filepath.IsAbs(dir) {
		return fmt.Errorf("path is not absolute: %q", dir)
	}
	info, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("not a directory: %q", dir)
	}
	return nil
}

// workDirErrMsg wraps a validateWorkDir failure in the client-facing 400 message.
func workDirErrMsg(err error) string {
	return "work_dir must be an existing directory (absolute, ~/…, or home-relative): " + err.Error()
}
