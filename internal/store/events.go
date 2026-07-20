package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/AnkushinDaniil/grove/internal/core"
)

const insertEventSQL = `
INSERT INTO events (id, node_id, session_id, type, payload, requires_attention, acked_at, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
`

// AppendEvents inserts events in a single transaction (tree.Store). Events
// are append-only: this is always a plain INSERT, never an upsert.
func (s *Store) AppendEvents(ctx context.Context, events []core.Event) error {
	if len(events) == 0 {
		return nil
	}
	return s.inTx(ctx, func(tx *sql.Tx) error {
		for _, ev := range events {
			_, err := tx.ExecContext(ctx, insertEventSQL,
				string(ev.ID), string(ev.NodeID), string(ev.SessionID), string(ev.Type),
				ev.Payload, ev.RequiresAttention, nullMS(ev.AckedAt), msFromTime(ev.CreatedAt),
			)
			if err != nil {
				return fmt.Errorf("insert event %s: %w", ev.ID, err)
			}
		}
		return nil
	})
}

const (
	defaultEventsLimit = 500
	maxEventsLimit     = 500
)

// clampEventsLimit applies ListEvents' default/max page size of 500: a
// non-positive limit becomes the default, anything larger is capped.
func clampEventsLimit(limit int) int {
	if limit <= 0 {
		return defaultEventsLimit
	}
	if limit > maxEventsLimit {
		return maxEventsLimit
	}
	return limit
}

const selectEventsSQL = `
SELECT id, node_id, session_id, type, payload, requires_attention, acked_at, created_at
FROM events
WHERE node_id = ? AND id > ?
ORDER BY id ASC
LIMIT ?
`

// ListEvents returns up to limit events for nodeID with id > afterID,
// ordered oldest first. Event IDs are UUIDv7 (time-sortable), so this is a
// simple keyset-paginated feed: pass the last returned event's ID as
// afterID to fetch the next page. limit <= 0 defaults to 500; values above
// 500 are capped at 500.
func (s *Store) ListEvents(ctx context.Context, nodeID core.NodeID, afterID core.EventID, limit int) ([]core.Event, error) {
	rows, err := s.db.QueryContext(ctx, selectEventsSQL, string(nodeID), string(afterID), clampEventsLimit(limit))
	if err != nil {
		return nil, fmt.Errorf("list events for node %s: %w", nodeID, err)
	}
	events, err := collect(rows, scanEvent)
	if err != nil {
		return nil, fmt.Errorf("list events for node %s: %w", nodeID, err)
	}
	return events, nil
}

const selectInboxSQL = `
SELECT id, node_id, session_id, type, payload, requires_attention, acked_at, created_at
FROM events
WHERE requires_attention = 1 AND acked_at IS NULL
ORDER BY id DESC
`

// ListInbox returns every unacknowledged attention-requiring event across
// all nodes, newest first.
func (s *Store) ListInbox(ctx context.Context) ([]core.Event, error) {
	rows, err := s.db.QueryContext(ctx, selectInboxSQL)
	if err != nil {
		return nil, fmt.Errorf("list inbox: %w", err)
	}
	events, err := collect(rows, scanEvent)
	if err != nil {
		return nil, fmt.Errorf("list inbox: %w", err)
	}
	return events, nil
}

const ackNodeEventsSQL = `
UPDATE events
SET acked_at = ?
WHERE node_id = ? AND requires_attention = 1 AND acked_at IS NULL
`

// AckNodeEvents marks every unacknowledged attention-requiring event for
// nodeID as acknowledged at at. Returns the number of events affected.
func (s *Store) AckNodeEvents(ctx context.Context, nodeID core.NodeID, at time.Time) (int64, error) {
	var affected int64
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, ackNodeEventsSQL, msFromTime(at), string(nodeID))
		if err != nil {
			return fmt.Errorf("ack events for node %s: %w", nodeID, err)
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
