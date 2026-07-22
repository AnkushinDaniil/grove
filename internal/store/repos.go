package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/AnkushinDaniil/grove/internal/core"
)

const upsertRepoSQL = `
INSERT INTO repos (id, project_id, name, source_path, default_base, created_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	project_id = excluded.project_id,
	name = excluded.name,
	source_path = excluded.source_path,
	default_base = excluded.default_base
`

// SaveRepo upserts one repo.
func (s *Store) SaveRepo(ctx context.Context, r core.Repo) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, upsertRepoSQL,
			string(r.ID), string(r.ProjectID), r.Name, r.SourcePath, r.DefaultBase, msFromTime(r.CreatedAt),
		)
		if err != nil {
			return fmt.Errorf("upsert repo %s: %w", r.ID, err)
		}
		return nil
	})
}

const selectReposByProjectSQL = `
SELECT id, project_id, name, source_path, default_base, created_at
FROM repos
WHERE project_id = ? AND deleted_at IS NULL
ORDER BY created_at ASC
`

// ListRepos returns every non-deleted repo registered on projectID, oldest
// first.
func (s *Store) ListRepos(ctx context.Context, projectID core.NodeID) ([]core.Repo, error) {
	rows, err := s.db.QueryContext(ctx, selectReposByProjectSQL, string(projectID))
	if err != nil {
		return nil, fmt.Errorf("list repos for project %s: %w", projectID, err)
	}
	repos, err := collect(rows, scanRepo)
	if err != nil {
		return nil, fmt.Errorf("list repos for project %s: %w", projectID, err)
	}
	return repos, nil
}

const softDeleteRepoSQL = `
UPDATE repos
SET deleted_at = ?, name = name || '#deleted-' || id
WHERE id = ? AND deleted_at IS NULL
`

// DeleteRepo soft-deletes a repo: it stops appearing in ListRepos and its
// (project_id, name) slot is freed for reuse (the stored name is tombstoned
// with the repo's own id, so it can never collide with a future repo), but
// the row itself is kept. worktrees.repo_id is a NOT NULL foreign key into
// repos, and worktree rows must survive repo deletion for review, so a hard
// delete here would permanently fail once a repo has ever provisioned a
// worktree. Deleting an ID that does not exist, or is already deleted, is
// not an error.
func (s *Store) DeleteRepo(ctx context.Context, id core.RepoID) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, softDeleteRepoSQL, msFromTime(time.Now()), string(id)); err != nil {
			return fmt.Errorf("delete repo %s: %w", id, err)
		}
		return nil
	})
}
