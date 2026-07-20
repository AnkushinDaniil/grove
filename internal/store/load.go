package store

import (
	"context"
	"fmt"

	"github.com/AnkushinDaniil/grove/internal/core"
)

const selectLiveNodesSQL = `
SELECT id, parent_id, kind, title, brief, status, attention, attention_reason,
	attention_since, driver, profile_id, current_session_id, workspace_dir,
	work_dir, meta, position, created_at, updated_at, archived_at
FROM nodes
WHERE archived_at IS NULL
`

// selectLatestLiveSessionsSQL picks, for every live (non-archived) node, its
// single most recent session by started_at, ties broken by id (UUIDv7 is
// time-sortable, so higher id also means more recent). id and node_id are
// qualified in the window clause because the join brings in nodes.id too.
const selectLatestLiveSessionsSQL = `
SELECT id, node_id, driver, profile_id, mode, driver_session_id,
	parent_driver_session_id, status, exit_code, transcript_path, cwd,
	started_at, ended_at
FROM (
	SELECT sessions.*,
		ROW_NUMBER() OVER (
			PARTITION BY sessions.node_id ORDER BY sessions.started_at DESC, sessions.id DESC
		) AS rn
	FROM sessions
	JOIN nodes ON nodes.id = sessions.node_id
	WHERE nodes.archived_at IS NULL
)
WHERE rn = 1
`

// LoadLive returns every non-archived node plus, for each such node, its
// most recent session if it has one. It is meant to feed tree.Tree.Load at
// daemon startup.
func (s *Store) LoadLive(ctx context.Context) ([]core.Node, []core.Session, error) {
	nodeRows, err := s.db.QueryContext(ctx, selectLiveNodesSQL)
	if err != nil {
		return nil, nil, fmt.Errorf("load live nodes: %w", err)
	}
	nodes, err := collect(nodeRows, scanNode)
	if err != nil {
		return nil, nil, fmt.Errorf("load live nodes: %w", err)
	}

	sessionRows, err := s.db.QueryContext(ctx, selectLatestLiveSessionsSQL)
	if err != nil {
		return nil, nil, fmt.Errorf("load latest sessions: %w", err)
	}
	sessions, err := collect(sessionRows, scanSession)
	if err != nil {
		return nil, nil, fmt.Errorf("load latest sessions: %w", err)
	}

	return nodes, sessions, nil
}
