package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/AnkushinDaniil/grove/internal/core"
)

const upsertWorktreeSQL = `
INSERT INTO worktrees (id, node_id, repo_id, path, branch, base_ref, status, created_at, removed_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	node_id = excluded.node_id,
	repo_id = excluded.repo_id,
	path = excluded.path,
	branch = excluded.branch,
	base_ref = excluded.base_ref,
	status = excluded.status,
	removed_at = excluded.removed_at
`

// SaveWorktree upserts one worktree.
func (s *Store) SaveWorktree(ctx context.Context, w core.Worktree) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, upsertWorktreeSQL,
			string(w.ID), string(w.NodeID), string(w.RepoID), w.Path, w.Branch, w.BaseRef,
			string(w.Status), msFromTime(w.CreatedAt), nullMS(w.RemovedAt),
		)
		if err != nil {
			return fmt.Errorf("upsert worktree %s: %w", w.ID, err)
		}
		return nil
	})
}

const selectWorktreesByNodeSQL = `
SELECT id, node_id, repo_id, path, branch, base_ref, status, created_at, removed_at
FROM worktrees
WHERE node_id = ?
ORDER BY created_at ASC
`

// ListWorktrees returns every worktree bound to nodeID, oldest first.
func (s *Store) ListWorktrees(ctx context.Context, nodeID core.NodeID) ([]core.Worktree, error) {
	rows, err := s.db.QueryContext(ctx, selectWorktreesByNodeSQL, string(nodeID))
	if err != nil {
		return nil, fmt.Errorf("list worktrees for node %s: %w", nodeID, err)
	}
	worktrees, err := collect(rows, scanWorktree)
	if err != nil {
		return nil, fmt.Errorf("list worktrees for node %s: %w", nodeID, err)
	}
	return worktrees, nil
}

const selectWorktreesByStatusSQL = `
SELECT id, node_id, repo_id, path, branch, base_ref, status, created_at, removed_at
FROM worktrees
WHERE status = ?
ORDER BY created_at ASC
`

// ListWorktreesByStatus returns every worktree in the given status, oldest
// first.
func (s *Store) ListWorktreesByStatus(ctx context.Context, status core.WorktreeStatus) ([]core.Worktree, error) {
	rows, err := s.db.QueryContext(ctx, selectWorktreesByStatusSQL, string(status))
	if err != nil {
		return nil, fmt.Errorf("list worktrees with status %s: %w", status, err)
	}
	worktrees, err := collect(rows, scanWorktree)
	if err != nil {
		return nil, fmt.Errorf("list worktrees with status %s: %w", status, err)
	}
	return worktrees, nil
}
