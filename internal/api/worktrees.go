package api

import (
	"context"
	"fmt"
	"time"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/worktree"
)

// provisionTask creates one worktree per repo owned by the task's nearest
// project ancestor, persists them, and records the workspace dir on the node.
// Any failure is reported as an error event on the node (raising attention) and
// the node is returned unchanged rather than lost.
func (h *Handlers) provisionTask(ctx context.Context, node core.Node) core.Node {
	repos, ok := h.projectRepos(ctx, node)
	if !ok || len(repos) == 0 {
		return node
	}

	parentByRepo := h.activeParentWorktrees(ctx, node.ParentID)
	ws, wts, err := h.worktrees.Create(ctx, node, repos, parentByRepo)
	if err != nil {
		h.ingestNodeError(ctx, node.ID, fmt.Sprintf("worktree setup failed: %v", err))
		return h.reload(ctx, node)
	}
	for _, wt := range wts {
		if err := h.store.SaveWorktree(ctx, wt); err != nil {
			h.ingestNodeError(ctx, node.ID, fmt.Sprintf("record worktree failed: %v", err))
			return h.reload(ctx, node)
		}
	}
	updated, err := h.tree.SetWorkspaceDir(ctx, node.ID, ws)
	if err != nil {
		h.ingestNodeError(ctx, node.ID, fmt.Sprintf("record workspace dir failed: %v", err))
		return h.reload(ctx, node)
	}
	return updated
}

// projectRepos returns the repos registered on node's nearest project ancestor.
// The bool is false when there is no project ancestor or the lookup failed.
func (h *Handlers) projectRepos(ctx context.Context, node core.Node) ([]core.Repo, bool) {
	projectID, ok := h.nearestProject(node)
	if !ok {
		return nil, false
	}
	repos, err := h.store.ListRepos(ctx, projectID)
	if err != nil {
		h.logger.Error("list project repos", "project", projectID, "err", err)
		return nil, false
	}
	return repos, true
}

// nearestProject walks up the parent chain from node to the closest project
// node, which owns the repos a task's worktrees are cut from.
func (h *Handlers) nearestProject(node core.Node) (core.NodeID, bool) {
	cur := node
	for {
		parent, ok := h.tree.Get(cur.ParentID)
		if !ok {
			return "", false
		}
		switch parent.Kind {
		case core.KindProject:
			return parent.ID, true
		case core.KindTask:
			cur = parent
		case core.KindWorkspace:
			return "", false
		default:
			return "", false
		}
	}
}

// activeParentWorktrees maps the parent node's active worktrees by repo, so a
// stacked subtask branches from its parent's branch instead of the repo base.
func (h *Handlers) activeParentWorktrees(ctx context.Context, parentID core.NodeID) map[core.RepoID]core.Worktree {
	wts, err := h.store.ListWorktrees(ctx, parentID)
	if err != nil {
		h.logger.Error("list parent worktrees", "parent", parentID, "err", err)
		return nil
	}
	out := make(map[core.RepoID]core.Worktree, len(wts))
	for _, wt := range wts {
		if wt.Status == core.WorktreeActive {
			out[wt.RepoID] = wt
		}
	}
	return out
}

// cleanupWorktrees removes or orphans the active worktrees of an archived node.
// Clean worktrees are removed and marked removed; dirty or unmerged ones are
// kept on disk, marked orphaned, and flagged with an error event so the
// uncommitted work is never silently discarded.
//
// TODO(review-attention): raising EventError yields AttentionError; a dedicated
// AttentionReview signal (docs/API.md attention=review) would read better once
// the tree exposes a review-flavored ingest.
func (h *Handlers) cleanupWorktrees(ctx context.Context, nodeID core.NodeID) {
	wts, err := h.store.ListWorktrees(ctx, nodeID)
	if err != nil {
		h.logger.Error("list worktrees for cleanup", "node", nodeID, "err", err)
		return
	}
	repoByID := h.repoIndex(ctx, nodeID)
	for _, wt := range wts {
		if wt.Status != core.WorktreeActive {
			continue
		}
		h.cleanupWorktree(ctx, nodeID, wt, repoByID)
	}
}

// cleanupWorktree handles one active worktree of an archived node.
func (h *Handlers) cleanupWorktree(
	ctx context.Context,
	nodeID core.NodeID,
	wt core.Worktree,
	repoByID map[core.RepoID]core.Repo,
) {
	repo, ok := repoByID[wt.RepoID]
	if !ok {
		h.orphanWorktree(ctx, nodeID, wt, "repo metadata missing")
		return
	}
	state, err := h.worktrees.Check(ctx, wt)
	if err != nil {
		h.orphanWorktree(ctx, nodeID, wt, fmt.Sprintf("status check failed: %v", err))
		return
	}
	if state != worktree.Clean {
		h.orphanWorktree(ctx, nodeID, wt, fmt.Sprintf("uncommitted work in %s — worktree kept", repo.Name))
		return
	}
	if err := h.worktrees.Remove(ctx, repo, wt, false); err != nil {
		h.orphanWorktree(ctx, nodeID, wt, fmt.Sprintf("remove failed: %v", err))
		return
	}
	removed := wt
	removed.Status = core.WorktreeRemoved
	removed.RemovedAt = time.Now()
	if err := h.store.SaveWorktree(ctx, removed); err != nil {
		h.logger.Error("record removed worktree", "worktree", wt.ID, "err", err)
	}
}

// orphanWorktree marks a worktree orphaned (kept on disk) and raises attention
// so the retained work is surfaced for review.
func (h *Handlers) orphanWorktree(ctx context.Context, nodeID core.NodeID, wt core.Worktree, reason string) {
	orphaned := wt
	orphaned.Status = core.WorktreeOrphaned
	if err := h.store.SaveWorktree(ctx, orphaned); err != nil {
		h.logger.Error("record orphaned worktree", "worktree", wt.ID, "err", err)
	}
	h.ingestNodeError(ctx, nodeID, reason)
}

// repoIndex builds a repo-by-id lookup for a node from its nearest project.
func (h *Handlers) repoIndex(ctx context.Context, nodeID core.NodeID) map[core.RepoID]core.Repo {
	node, ok := h.tree.Get(nodeID)
	if !ok {
		return nil
	}
	repos, ok := h.projectRepos(ctx, node)
	if !ok {
		return nil
	}
	out := make(map[core.RepoID]core.Repo, len(repos))
	for _, repo := range repos {
		out[repo.ID] = repo
	}
	return out
}

// ingestNodeError appends a node-level error event, raising attention.
func (h *Handlers) ingestNodeError(ctx context.Context, nodeID core.NodeID, msg string) {
	payload, err := core.MarshalPayload(core.ErrorPayload{Message: msg})
	if err != nil {
		h.logger.Error("marshal error payload", "err", err)
		return
	}
	if _, err := h.tree.IngestEvents(ctx, nodeID, "", []core.EventInput{{
		Type:    core.EventError,
		Payload: payload,
		Detail:  msg,
	}}); err != nil {
		h.logger.Error("ingest node error event", "node", nodeID, "err", err)
	}
}

// reload returns the node's latest snapshot, falling back to the passed value.
func (h *Handlers) reload(_ context.Context, node core.Node) core.Node {
	if latest, ok := h.tree.Get(node.ID); ok {
		return latest
	}
	return node
}
