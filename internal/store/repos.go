package store

import (
	"context"
	"database/sql"
	"fmt"

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
WHERE project_id = ?
ORDER BY created_at ASC
`

// ListRepos returns every repo registered on projectID, oldest first.
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

// DeleteRepo removes a repo by ID. Deleting an ID that does not exist is not
// an error.
func (s *Store) DeleteRepo(ctx context.Context, id core.RepoID) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "DELETE FROM repos WHERE id = ?", string(id)); err != nil {
			return fmt.Errorf("delete repo %s: %w", id, err)
		}
		return nil
	})
}
