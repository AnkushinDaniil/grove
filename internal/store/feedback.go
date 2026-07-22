package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/AnkushinDaniil/grove/internal/core"
)

// Feedback status filters for ListFeedback. An unrecognized value is treated as
// FeedbackAll by ListFeedback; the API validates the query parameter separately.
const (
	FeedbackOpen     = "open"
	FeedbackResolved = "resolved"
	FeedbackAll      = "all"
)

// Feedback is one user-recorded quality signal attached to a node (docs/API.md
// "Feedback loop"). It is open until ResolvedAt is set; FixNodeID then links the
// task node created to address it. Kind is one of skill|tool|model|agent|other,
// validated at the API boundary and stored verbatim here.
type Feedback struct {
	ID         string
	NodeID     string
	SessionID  string
	EventID    string
	Kind       string
	Subject    string
	Comment    string
	CreatedAt  time.Time
	ResolvedAt time.Time // zero = open
	FixNodeID  string
}

const insertFeedbackSQL = `
INSERT INTO feedback (id, node_id, session_id, event_id, kind, subject, comment, created_at, resolved_at, fix_node_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`

// SaveFeedback inserts a new feedback row. Feedback is created fresh with a
// unique id, so this is always a plain INSERT.
func (s *Store) SaveFeedback(ctx context.Context, f Feedback) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, insertFeedbackSQL,
			f.ID, f.NodeID, f.SessionID, f.EventID, f.Kind, f.Subject, f.Comment,
			msFromTime(f.CreatedAt), nullMS(f.ResolvedAt), f.FixNodeID,
		)
		if err != nil {
			return fmt.Errorf("insert feedback %s: %w", f.ID, err)
		}
		return nil
	})
}

const selectFeedbackColumnsSQL = `
SELECT id, node_id, session_id, event_id, kind, subject, comment, created_at, resolved_at, fix_node_id
FROM feedback
`

// ListFeedback returns feedback filtered by status, newest first. status
// FeedbackOpen keeps only unresolved rows, FeedbackResolved only resolved rows,
// and anything else (FeedbackAll) returns every row.
func (s *Store) ListFeedback(ctx context.Context, status string) ([]Feedback, error) {
	query := selectFeedbackColumnsSQL
	switch status {
	case FeedbackOpen:
		query += "WHERE resolved_at IS NULL\n"
	case FeedbackResolved:
		query += "WHERE resolved_at IS NOT NULL\n"
	}
	query += "ORDER BY created_at DESC, id DESC"

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list feedback (%s): %w", status, err)
	}
	out, err := collect(rows, scanFeedback)
	if err != nil {
		return nil, fmt.Errorf("list feedback (%s): %w", status, err)
	}
	return out, nil
}

const resolveFeedbackSQL = `
UPDATE feedback
SET resolved_at = COALESCE(resolved_at, ?), fix_node_id = ?
WHERE id = ?
`

// ResolveFeedback marks feedback id resolved at at and links fixNodeID, returning
// the updated row. Re-resolving keeps the original resolved_at (COALESCE) while
// still updating fix_node_id, so a fix task can be linked after the fact. An
// unknown id yields core.ErrInvalid with a "not found" message (→ 404).
func (s *Store) ResolveFeedback(ctx context.Context, id, fixNodeID string, at time.Time) (Feedback, error) {
	var out Feedback
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, resolveFeedbackSQL, msFromTime(at), fixNodeID, id)
		if err != nil {
			return fmt.Errorf("resolve feedback %s: %w", id, err)
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("resolve feedback %s rows affected: %w", id, err)
		}
		if affected == 0 {
			return fmt.Errorf("%w: feedback %q not found", core.ErrInvalid, id)
		}
		row := tx.QueryRowContext(ctx, selectFeedbackColumnsSQL+"WHERE id = ?", id)
		out, err = scanFeedback(row)
		if err != nil {
			return fmt.Errorf("load resolved feedback %s: %w", id, err)
		}
		return nil
	})
	if err != nil {
		return Feedback{}, err
	}
	return out, nil
}

// FeedbackStat is one (kind, subject) feedback group with its open and total
// counts, for the stats feedback[] breakdown.
type FeedbackStat struct {
	Kind    string
	Subject string
	Open    int
	Total   int
}

// FeedbackBreakdown groups feedback for the nodes in scope by (kind, subject),
// counting open (unresolved) and total rows per group, ordered by total
// descending. An empty scope returns no groups without touching the database.
func (s *Store) FeedbackBreakdown(ctx context.Context, nodeIDs []core.NodeID) ([]FeedbackStat, error) {
	placeholders, args, ok := nodeIDPlaceholders(nodeIDs)
	if !ok {
		return nil, nil
	}
	//nolint:gosec // G202: placeholders is only "?," separators; every id is a bound parameter passed via args.
	query := `
SELECT kind, subject,
	COALESCE(SUM(CASE WHEN resolved_at IS NULL THEN 1 ELSE 0 END), 0) AS open,
	COUNT(*) AS total
FROM feedback
WHERE node_id IN (` + placeholders + `)
GROUP BY kind, subject
ORDER BY total DESC, kind ASC, subject ASC`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("feedback breakdown: %w", err)
	}
	out, err := collect(rows, scanFeedbackStat)
	if err != nil {
		return nil, fmt.Errorf("feedback breakdown: %w", err)
	}
	return out, nil
}

// scanFeedback scans one row shaped like selectFeedbackColumnsSQL into a Feedback.
func scanFeedback(row rowScanner) (Feedback, error) {
	var (
		f          Feedback
		createdAt  int64
		resolvedAt sql.NullInt64
	)
	if err := row.Scan(
		&f.ID, &f.NodeID, &f.SessionID, &f.EventID, &f.Kind, &f.Subject, &f.Comment,
		&createdAt, &resolvedAt, &f.FixNodeID,
	); err != nil {
		return Feedback{}, fmt.Errorf("scan feedback row: %w", err)
	}
	f.CreatedAt = timeFromMS(createdAt)
	f.ResolvedAt = timeFromNullMS(resolvedAt)
	return f, nil
}

// scanFeedbackStat scans one grouped row from FeedbackBreakdown.
func scanFeedbackStat(row rowScanner) (FeedbackStat, error) {
	var fs FeedbackStat
	if err := row.Scan(&fs.Kind, &fs.Subject, &fs.Open, &fs.Total); err != nil {
		return FeedbackStat{}, fmt.Errorf("scan feedback stat row: %w", err)
	}
	return fs, nil
}
