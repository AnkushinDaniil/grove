package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/AnkushinDaniil/grove/internal/core"
)

const upsertSessionSQL = `
INSERT INTO sessions (
	id, node_id, driver, profile_id, mode, driver_session_id,
	parent_driver_session_id, status, exit_code, transcript_path, cwd,
	started_at, ended_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	node_id = excluded.node_id,
	driver = excluded.driver,
	profile_id = excluded.profile_id,
	mode = excluded.mode,
	driver_session_id = excluded.driver_session_id,
	parent_driver_session_id = excluded.parent_driver_session_id,
	status = excluded.status,
	exit_code = excluded.exit_code,
	transcript_path = excluded.transcript_path,
	cwd = excluded.cwd,
	started_at = excluded.started_at,
	ended_at = excluded.ended_at
`

// SaveSessions upserts sessions in a single transaction (tree.Store).
func (s *Store) SaveSessions(ctx context.Context, sessions []core.Session) error {
	if len(sessions) == 0 {
		return nil
	}
	return s.inTx(ctx, func(tx *sql.Tx) error {
		for _, sess := range sessions {
			if err := upsertSession(ctx, tx, sess); err != nil {
				return err
			}
		}
		return nil
	})
}

func upsertSession(ctx context.Context, tx *sql.Tx, sess core.Session) error {
	_, err := tx.ExecContext(ctx, upsertSessionSQL,
		string(sess.ID), string(sess.NodeID), sess.Driver, string(sess.ProfileID), string(sess.Mode),
		sess.DriverSessionID, sess.ParentDriverSessionID, string(sess.Status), nullIntFromPtr(sess.ExitCode),
		sess.TranscriptPath, sess.CWD, msFromTime(sess.StartedAt), nullMS(sess.EndedAt),
	)
	if err != nil {
		return fmt.Errorf("upsert session %s: %w", sess.ID, err)
	}
	return nil
}

const markInterruptedSQL = `
UPDATE sessions
SET status = ?, ended_at = ?
WHERE status IN (?, ?, ?)
`

// MarkInterrupted transitions every session still in a non-terminal status
// (starting, running, awaiting_input) to interrupted and stamps ended_at at
// at. Intended to be called once at daemon startup to recover from an
// unclean shutdown. Returns the number of sessions affected.
func (s *Store) MarkInterrupted(ctx context.Context, at time.Time) (int64, error) {
	var affected int64
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, markInterruptedSQL,
			string(core.SessionInterrupted), msFromTime(at),
			string(core.SessionStarting), string(core.SessionRunning), string(core.SessionAwaitingInput),
		)
		if err != nil {
			return fmt.Errorf("mark interrupted sessions: %w", err)
		}
		affected, err = res.RowsAffected()
		if err != nil {
			return fmt.Errorf("rows affected: %w", err)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return affected, nil
}

const sessionsForNodeSQL = `
SELECT id, node_id, driver, profile_id, mode, driver_session_id,
	parent_driver_session_id, status, exit_code, transcript_path, cwd,
	started_at, ended_at
FROM sessions
WHERE node_id = ?
ORDER BY started_at DESC, id DESC
`

// SessionsForNode returns every session ever bound to a node, newest first —
// the resume fallback walks this history for a conversation that still exists.
func (s *Store) SessionsForNode(ctx context.Context, nodeID core.NodeID) ([]core.Session, error) {
	rows, err := s.db.QueryContext(ctx, sessionsForNodeSQL, string(nodeID))
	if err != nil {
		return nil, fmt.Errorf("query sessions for node %s: %w", nodeID, err)
	}
	return collect(rows, scanSession)
}

const sessionsSansResumeSQL = `
SELECT id, node_id, driver, profile_id, mode, driver_session_id,
	parent_driver_session_id, status, exit_code, transcript_path, cwd,
	started_at, ended_at
FROM sessions
WHERE driver_session_id = '' AND mode = 'pty'
`

// SessionsWithoutResumeID returns PTY sessions persisted without a
// conversation id (pre-hook-wiring history) so startup can backfill them from
// scrollback farewells.
func (s *Store) SessionsWithoutResumeID(ctx context.Context) ([]core.Session, error) {
	rows, err := s.db.QueryContext(ctx, sessionsSansResumeSQL)
	if err != nil {
		return nil, fmt.Errorf("query sessions without resume id: %w", err)
	}
	return collect(rows, scanSession)
}
