package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/gitcli"
	"github.com/AnkushinDaniil/grove/internal/github"
	"github.com/AnkushinDaniil/grove/internal/store"
	"github.com/AnkushinDaniil/grove/internal/worktree"
)

// worktreeCommentPR is the sentinel pr value under which a node's worktree
// review notes are stored in the shared review_drafts table (docs/API.md
// "Worktree review": ai-draft is reused with pr=0). A worktree comment is a
// review_drafts row keyed by (dir=worktree path, pr=0); node_id and repo are not
// persisted — the request supplies them, and a worktree path maps to exactly one
// (node, repo). PR drafts use pr>0, so the two never collide.
const worktreeCommentPR = 0

// --- wire DTOs (docs/API.md "Worktree review") ---

type worktreeReviewDTO struct {
	NodeID         string            `json:"node_id"`
	Repo           string            `json:"repo"`
	WorktreePath   string            `json:"worktree_path"`
	Branch         string            `json:"branch"`
	BaseRef        string            `json:"base_ref"`
	HasUncommitted bool              `json:"has_uncommitted"`
	Files          []worktreeFileDTO `json:"files"`
}

// worktreeFileDTO is the content-bearing file shape shared with PR review
// (docs/API.md "Diff content for rich rendering"). Hunks is kept for shape
// parity — Pierre computes its own diff from the full contents.
type worktreeFileDTO struct {
	Path            string    `json:"path"`
	OldPath         string    `json:"old_path"`
	Status          string    `json:"status"`
	Additions       int       `json:"additions"`
	Deletions       int       `json:"deletions"`
	Binary          bool      `json:"binary"`
	OriginalContent string    `json:"original_content"`
	ModifiedContent string    `json:"modified_content"`
	ContentOmitted  string    `json:"content_omitted"`
	Hunks           []hunkDTO `json:"hunks"`
}

// worktreeCommentDTO is a local review note keyed to (node, repo, path, line).
type worktreeCommentDTO struct {
	ID        string `json:"id"`
	NodeID    string `json:"node_id"`
	Repo      string `json:"repo"`
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Side      string `json:"side"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
}

// mergeResponse is the POST /reviews/worktree/merge body.
type mergeResponse struct {
	Merged  bool   `json:"merged"`
	Message string `json:"message"`
}

// --- handlers ---

// handleWorktreeReview returns the local diff of a task node's worktree: its
// working tree (committed + uncommitted) against its merge-base with the base
// ref, with each file's full base/head contents for rich rendering.
func (h *Handlers) handleWorktreeReview(w http.ResponseWriter, r *http.Request) {
	nodeID, repo, ok := h.queryNodeRepo(w, r)
	if !ok {
		return
	}
	wt, ok := h.worktreeFor(r.Context(), nodeID, repo)
	if !ok {
		writeErrorStatus(w, h.logger, http.StatusNotFound, "no worktree for node and repo")
		return
	}
	review, err := h.buildWorktreeReview(r.Context(), nodeID, repo, wt)
	if err != nil {
		h.writeGitError(w, err)
		return
	}
	writeJSON(w, h.logger, http.StatusOK, review)
}

// handleListWorktreeComments returns the local review notes on a node's worktree.
func (h *Handlers) handleListWorktreeComments(w http.ResponseWriter, r *http.Request) {
	nodeID, repo, ok := h.queryNodeRepo(w, r)
	if !ok {
		return
	}
	wt, ok := h.worktreeFor(r.Context(), nodeID, repo)
	if !ok {
		writeErrorStatus(w, h.logger, http.StatusNotFound, "no worktree for node and repo")
		return
	}
	drafts, err := h.store.ListReviewDrafts(r.Context(), wt.Path, worktreeCommentPR)
	if err != nil {
		writeError(w, h.logger, err)
		return
	}
	out := make([]worktreeCommentDTO, 0, len(drafts))
	for _, d := range drafts {
		out = append(out, worktreeCommentToDTO(d, nodeID, repo))
	}
	writeJSON(w, h.logger, http.StatusOK, map[string][]worktreeCommentDTO{"comments": out})
}

type createWorktreeCommentRequest struct {
	Node string `json:"node"`
	Repo string `json:"repo"`
	Path string `json:"path"`
	Line int    `json:"line"`
	Side string `json:"side"`
	Body string `json:"body"`
}

// handleCreateWorktreeComment validates and stores one local review note against
// a node's worktree.
func (h *Handlers) handleCreateWorktreeComment(w http.ResponseWriter, r *http.Request) {
	var req createWorktreeCommentRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Node == "" {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "node is required")
		return
	}
	if strings.TrimSpace(req.Body) == "" {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "body must not be empty")
		return
	}
	if req.Path == "" {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "path must not be empty")
		return
	}
	side, ok := normalizeSide(req.Side)
	if !ok {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "side must be RIGHT or LEFT")
		return
	}
	nodeID := core.NodeID(req.Node)
	wt, ok := h.worktreeFor(r.Context(), nodeID, req.Repo)
	if !ok {
		writeErrorStatus(w, h.logger, http.StatusNotFound, "no worktree for node and repo")
		return
	}
	draft := store.ReviewDraft{
		ID:        uuid.Must(uuid.NewV7()).String(),
		Dir:       wt.Path,
		PR:        worktreeCommentPR,
		Path:      req.Path,
		Line:      req.Line,
		Side:      side,
		Body:      req.Body,
		CreatedAt: time.Now(),
	}
	if err := h.store.SaveReviewDraft(r.Context(), draft); err != nil {
		writeError(w, h.logger, err)
		return
	}
	writeJSON(w, h.logger, http.StatusCreated, worktreeCommentToDTO(draft, nodeID, req.Repo))
}

// handleDeleteWorktreeComment removes one local review note by id. Deletion is
// idempotent: an unknown id still returns 204.
func (h *Handlers) handleDeleteWorktreeComment(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "comment id is required")
		return
	}
	if err := h.store.DeleteReviewDraft(r.Context(), id); err != nil {
		writeError(w, h.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleWorktreeMerge squash-merges a node's worktree into its parent node's
// worktree for the same repo. A dirty parent yields 409; the merge is otherwise
// finalized as a commit on the parent.
func (h *Handlers) handleWorktreeMerge(w http.ResponseWriter, r *http.Request) {
	req, ok := h.decodeWorktreeAction(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	nodeID := core.NodeID(req.Node)
	child, ok := h.worktreeFor(ctx, nodeID, req.Repo)
	if !ok {
		writeErrorStatus(w, h.logger, http.StatusNotFound, "no worktree for node and repo")
		return
	}
	node, ok := h.tree.Get(nodeID)
	if !ok {
		writeErrorStatus(w, h.logger, http.StatusNotFound, "node not found")
		return
	}
	parent, ok := h.worktreeFor(ctx, node.ParentID, req.Repo)
	if !ok {
		writeErrorStatus(w, h.logger, http.StatusNotFound, "parent node has no worktree for repo")
		return
	}
	if err := h.worktrees.MergeToParent(ctx, child, parent); err != nil {
		if errors.Is(err, worktree.ErrDirtyParent) {
			writeJSON(w, h.logger, http.StatusConflict, mergeResponse{Merged: false, Message: err.Error()})
			return
		}
		h.writeGitError(w, err)
		return
	}
	writeJSON(w, h.logger, http.StatusOK, mergeResponse{
		Merged:  true,
		Message: fmt.Sprintf("merged %s into %s", child.Branch, parent.Branch),
	})
}

// handleWorktreeAddress starts a PTY session on the node, prompted with its
// worktree review comments so the agent fixes them. It is a 400 when there are
// no comments to address.
func (h *Handlers) handleWorktreeAddress(w http.ResponseWriter, r *http.Request) {
	req, ok := h.decodeWorktreeAction(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	nodeID := core.NodeID(req.Node)
	wt, ok := h.worktreeFor(ctx, nodeID, req.Repo)
	if !ok {
		writeErrorStatus(w, h.logger, http.StatusNotFound, "no worktree for node and repo")
		return
	}
	comments, err := h.store.ListReviewDrafts(ctx, wt.Path, worktreeCommentPR)
	if err != nil {
		writeError(w, h.logger, err)
		return
	}
	if len(comments) == 0 {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "no review comments to address")
		return
	}
	// Detach from the request: the launched session outlives this HTTP call.
	sess, err := h.sessions.Start(
		context.WithoutCancel(ctx), nodeID, core.ModePTY, composeAddressPrompt(comments), "",
	)
	if err != nil {
		writeError(w, h.logger, err)
		return
	}
	writeJSON(w, h.logger, http.StatusCreated, SessionToDTO(sess))
}

// --- helpers ---

type worktreeActionRequest struct {
	Node string `json:"node"`
	Repo string `json:"repo"`
}

// decodeWorktreeAction decodes and validates the shared {node, repo} body used
// by merge and address, writing a 400 and returning ok=false on failure.
func (h *Handlers) decodeWorktreeAction(w http.ResponseWriter, r *http.Request) (worktreeActionRequest, bool) {
	var req worktreeActionRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "invalid request body")
		return req, false
	}
	if req.Node == "" {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "node is required")
		return req, false
	}
	return req, true
}

// queryNodeRepo parses the shared ?node=&repo= query for the worktree read
// endpoints. Only node is required; an empty repo means "the node's single
// worktree" (the common single-repo task, where the UI has no repo to pass),
// resolved in worktreeFor.
func (h *Handlers) queryNodeRepo(w http.ResponseWriter, r *http.Request) (core.NodeID, string, bool) {
	node := r.URL.Query().Get("node")
	if node == "" {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "node query parameter is required")
		return "", "", false
	}
	return core.NodeID(node), r.URL.Query().Get("repo"), true
}

// worktreeFor resolves nodeID's non-removed worktree for the named repo. ok is
// false when the node has no matching worktree.
func (h *Handlers) worktreeFor(ctx context.Context, nodeID core.NodeID, repoName string) (core.Worktree, bool) {
	wts, err := h.store.ListWorktrees(ctx, nodeID)
	if err != nil {
		h.logger.Error("list worktrees", "node", nodeID, "err", err)
		return core.Worktree{}, false
	}
	active := make([]core.Worktree, 0, len(wts))
	for _, wt := range wts {
		if wt.Status != core.WorktreeRemoved {
			active = append(active, wt)
		}
	}
	// An empty repo name means "the node's single worktree" — the common
	// single-repo task case, where the UI has no repo to disambiguate by.
	if repoName == "" {
		if len(active) == 1 {
			return active[0], true
		}
		return core.Worktree{}, false
	}
	repoByID := h.repoIndex(ctx, nodeID)
	for _, wt := range active {
		if repo, ok := repoByID[wt.RepoID]; ok && repo.Name == repoName {
			return wt, true
		}
	}
	return core.Worktree{}, false
}

// buildWorktreeReview assembles the WorktreeReview for a resolved worktree.
func (h *Handlers) buildWorktreeReview(ctx context.Context, nodeID core.NodeID, repo string, wt core.Worktree) (worktreeReviewDTO, error) {
	files, err := h.worktreeFiles(ctx, wt)
	if err != nil {
		return worktreeReviewDTO{}, err
	}
	dirty, err := h.git.IsDirty(ctx, wt.Path)
	if err != nil {
		return worktreeReviewDTO{}, fmt.Errorf("status: %w", err)
	}
	return worktreeReviewDTO{
		NodeID:         string(nodeID),
		Repo:           repo,
		WorktreePath:   wt.Path,
		Branch:         wt.Branch,
		BaseRef:        wt.BaseRef,
		HasUncommitted: dirty,
		Files:          files,
	}, nil
}

// worktreeFiles diffs the worktree's working tree against its merge-base with
// the base ref and returns one content-bearing file per change. Untracked files
// (which a base-tree diff misses) are folded in as additions.
func (h *Handlers) worktreeFiles(ctx context.Context, wt core.Worktree) ([]worktreeFileDTO, error) {
	mergeBase, err := h.git.MergeBase(ctx, wt.Path, wt.BaseRef, "HEAD")
	if err != nil {
		return nil, err
	}
	changes, err := h.git.DiffNameStatus(ctx, wt.Path, mergeBase)
	if err != nil {
		return nil, err
	}
	counts, err := h.git.NumStat(ctx, wt.Path, mergeBase)
	if err != nil {
		return nil, err
	}
	untracked, err := h.git.UntrackedFiles(ctx, wt.Path)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool, len(changes))
	files := make([]worktreeFileDTO, 0, len(changes)+len(untracked))
	for _, ch := range changes {
		seen[ch.Path] = true
		files = append(files, h.worktreeFile(ctx, wt.Path, mergeBase, ch, counts[ch.Path]))
	}
	for _, path := range untracked {
		if seen[path] {
			continue
		}
		files = append(files, h.worktreeFile(ctx, wt.Path, mergeBase, gitcli.NameStatus{Status: "A", Path: path}, counts[path]))
	}
	return files, nil
}

// worktreeFile resolves one changed file's base/head contents and omission
// reason. The base side is read from the merge-base tree (its old path for
// renames), the head side from the working file on disk.
func (h *Handlers) worktreeFile(ctx context.Context, wtPath, mergeBase string, ch gitcli.NameStatus, count gitcli.NumStatEntry) worktreeFileDTO {
	status := mapWorktreeStatus(ch.Status)
	var original, modified []byte
	if status != "added" {
		basePath := ch.Path
		if ch.OldPath != "" {
			basePath = ch.OldPath
		}
		if b, err := h.git.ShowFile(ctx, wtPath, mergeBase, basePath); err == nil {
			original = b
		}
	}
	if status != "removed" {
		if b, err := os.ReadFile(filepath.Join(wtPath, ch.Path)); err == nil {
			modified = b
		}
	}
	dec := github.DecideContent(original, modified, false)
	return worktreeFileDTO{
		Path:            ch.Path,
		OldPath:         ch.OldPath,
		Status:          status,
		Additions:       max(0, count.Additions),
		Deletions:       max(0, count.Deletions),
		Binary:          dec.Omitted == "binary",
		OriginalContent: dec.Original,
		ModifiedContent: dec.Modified,
		ContentOmitted:  dec.Omitted,
		Hunks:           []hunkDTO{},
	}
}

// mapWorktreeStatus maps a git name-status letter to the wire status enum
// (modified|added|removed|renamed).
func mapWorktreeStatus(gitStatus string) string {
	if gitStatus == "" {
		return "modified"
	}
	switch gitStatus[0] {
	case 'A':
		return "added"
	case 'D':
		return "removed"
	case 'R':
		return "renamed"
	case 'C':
		return "added"
	default: // M, T, and anything else
		return "modified"
	}
}

// composeAddressPrompt renders a node's worktree comments into a fix prompt for
// the address session.
func composeAddressPrompt(comments []store.ReviewDraft) string {
	var b strings.Builder
	b.WriteString("Address these review comments on your changes:\n")
	for _, c := range comments {
		fmt.Fprintf(&b, "- %s:%d — %s\n", c.Path, c.Line, c.Body)
	}
	return b.String()
}

// worktreeCommentToDTO maps a stored review_drafts row (reused as a worktree
// comment) to its wire shape. node and repo come from the request, not storage.
func worktreeCommentToDTO(d store.ReviewDraft, nodeID core.NodeID, repo string) worktreeCommentDTO {
	return worktreeCommentDTO{
		ID:        d.ID,
		NodeID:    string(nodeID),
		Repo:      repo,
		Path:      d.Path,
		Line:      d.Line,
		Side:      d.Side,
		Body:      d.Body,
		CreatedAt: rfc3339(d.CreatedAt),
	}
}

// localDiffForPath renders the local diff of one path in a worktree for the
// ai-draft endpoint (pr=0). It prefers the change set since the branch point
// (merge-base with the detected default base), falling back to the working diff
// when no base resolves. Best-effort: an empty result still yields a usable
// prompt.
func (h *Handlers) localDiffForPath(ctx context.Context, dir, path string) string {
	if base, err := h.git.DetectDefaultBase(ctx, dir); err == nil {
		if mergeBase, err := h.git.MergeBase(ctx, dir, base, "HEAD"); err == nil {
			if out, err := h.git.Run(ctx, dir, "diff", mergeBase, "--", path); err == nil && out != "" {
				return out
			}
		}
	}
	if out, err := h.git.Run(ctx, dir, "diff", "HEAD", "--", path); err == nil && out != "" {
		return out
	}
	out, _ := h.git.Run(ctx, dir, "diff", "--", path)
	return out
}

// buildWorktreeDraftPrompt builds the ai-draft prompt for a worktree comment
// (pr=0), grounding it in the local diff instead of a PR's assembled diff.
func buildWorktreeDraftPrompt(req aiDraftRequest, diff string) string {
	var b strings.Builder
	b.WriteString("You are helping review local worktree changes before they become a PR.\n")
	fmt.Fprintf(&b, "Write a concise, specific code review comment for %s", req.Path)
	if req.Line > 0 {
		fmt.Fprintf(&b, ":%d", req.Line)
	}
	b.WriteString(".\n\nThe diff under review:\n")
	b.WriteString(capText(diff, maxPromptDiffBytes))
	b.WriteString("\n")
	if strings.TrimSpace(req.Instruction) != "" {
		fmt.Fprintf(&b, "\nAdditional instruction: %s\n", req.Instruction)
	}
	b.WriteString("\nRespond with only the comment text, no preamble.")
	return b.String()
}

// writeGitError reports a local git failure as 502 with its message, mirroring
// how the PR review handlers surface upstream tooling failures.
func (h *Handlers) writeGitError(w http.ResponseWriter, err error) {
	h.logger.Warn("git command failed", "err", err)
	writeErrorStatus(w, h.logger, http.StatusBadGateway, err.Error())
}
