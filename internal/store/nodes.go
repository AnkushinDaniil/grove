package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/AnkushinDaniil/grove/internal/core"
)

const upsertNodeSQL = `
INSERT INTO nodes (
	id, parent_id, kind, title, brief, status, attention, attention_reason,
	attention_since, driver, profile_id, current_session_id, workspace_dir,
	meta, position, created_at, updated_at, archived_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	parent_id = excluded.parent_id,
	kind = excluded.kind,
	title = excluded.title,
	brief = excluded.brief,
	status = excluded.status,
	attention = excluded.attention,
	attention_reason = excluded.attention_reason,
	attention_since = excluded.attention_since,
	driver = excluded.driver,
	profile_id = excluded.profile_id,
	current_session_id = excluded.current_session_id,
	workspace_dir = excluded.workspace_dir,
	meta = excluded.meta,
	position = excluded.position,
	updated_at = excluded.updated_at,
	archived_at = excluded.archived_at
`

// SaveNodes upserts nodes in a single transaction (tree.Store).
func (s *Store) SaveNodes(ctx context.Context, nodes []core.Node) error {
	if len(nodes) == 0 {
		return nil
	}
	return s.inTx(ctx, func(tx *sql.Tx) error {
		for _, n := range nodes {
			if err := upsertNode(ctx, tx, n); err != nil {
				return err
			}
		}
		return nil
	})
}

func upsertNode(ctx context.Context, tx *sql.Tx, n core.Node) error {
	_, err := tx.ExecContext(ctx, upsertNodeSQL,
		string(n.ID), nullStr(string(n.ParentID)), string(n.Kind), n.Title, n.Brief,
		string(n.Status), string(n.Attention), n.AttentionReason, nullMS(n.AttentionSince),
		n.Driver, string(n.ProfileID), string(n.CurrentSessionID), n.WorkspaceDir,
		n.Meta, n.Position, msFromTime(n.CreatedAt), msFromTime(n.UpdatedAt), nullMS(n.ArchivedAt),
	)
	if err != nil {
		return fmt.Errorf("upsert node %s: %w", n.ID, err)
	}
	return nil
}
