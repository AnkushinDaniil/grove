package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// ReviewDraft is one pending inline review comment held in grove until the user
// submits a batch review for a pull request. It is keyed by (Dir, PR) — the
// interactive review workspace groups drafts by the repository directory and PR
// number.
type ReviewDraft struct {
	ID        string
	Dir       string
	PR        int
	Path      string
	Line      int
	Side      string
	Body      string
	CreatedAt time.Time
}

const insertReviewDraftSQL = `
INSERT INTO review_drafts (id, dir, pr, path, line, side, body, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
`

// SaveReviewDraft inserts a new draft. Drafts are created fresh with a unique
// id, so this is always a plain INSERT.
func (s *Store) SaveReviewDraft(ctx context.Context, d ReviewDraft) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, insertReviewDraftSQL,
			d.ID, d.Dir, d.PR, d.Path, d.Line, d.Side, d.Body, msFromTime(d.CreatedAt),
		)
		if err != nil {
			return fmt.Errorf("insert review draft %s: %w", d.ID, err)
		}
		return nil
	})
}

const selectReviewDraftsSQL = `
SELECT id, dir, pr, path, line, side, body, created_at
FROM review_drafts
WHERE dir = ? AND pr = ?
ORDER BY created_at ASC, id ASC
`

// ListReviewDrafts returns every draft for one review workspace (dir, pr),
// oldest first.
func (s *Store) ListReviewDrafts(ctx context.Context, dir string, pr int) ([]ReviewDraft, error) {
	rows, err := s.db.QueryContext(ctx, selectReviewDraftsSQL, dir, pr)
	if err != nil {
		return nil, fmt.Errorf("list review drafts for %s#%d: %w", dir, pr, err)
	}
	drafts, err := collect(rows, scanReviewDraft)
	if err != nil {
		return nil, fmt.Errorf("list review drafts for %s#%d: %w", dir, pr, err)
	}
	return drafts, nil
}

const deleteReviewDraftSQL = `DELETE FROM review_drafts WHERE id = ?`

// DeleteReviewDraft removes one draft by id. Deleting a draft that does not
// exist is not an error (the delete simply affects no rows).
func (s *Store) DeleteReviewDraft(ctx context.Context, id string) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, deleteReviewDraftSQL, id); err != nil {
			return fmt.Errorf("delete review draft %s: %w", id, err)
		}
		return nil
	})
}

// ListReviewDraftsByIDs returns the drafts whose ids are in ids, oldest first.
// An empty id set returns no drafts without touching the database. Unknown ids
// are silently skipped, so the result may be shorter than the input.
func (s *Store) ListReviewDraftsByIDs(ctx context.Context, ids []string) ([]ReviewDraft, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := strings.Repeat("?,", len(ids)-1) + "?"
	//nolint:gosec // G202: placeholders is only "?," separators; every id is a bound parameter passed via args.
	query := `
SELECT id, dir, pr, path, line, side, body, created_at
FROM review_drafts
WHERE id IN (` + placeholders + `)
ORDER BY created_at ASC, id ASC`
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list review drafts by ids: %w", err)
	}
	drafts, err := collect(rows, scanReviewDraft)
	if err != nil {
		return nil, fmt.Errorf("list review drafts by ids: %w", err)
	}
	return drafts, nil
}

// scanReviewDraft scans one row shaped like the review_drafts table's full
// column list into a ReviewDraft.
func scanReviewDraft(row rowScanner) (ReviewDraft, error) {
	var (
		d         ReviewDraft
		createdAt int64
	)
	if err := row.Scan(&d.ID, &d.Dir, &d.PR, &d.Path, &d.Line, &d.Side, &d.Body, &createdAt); err != nil {
		return ReviewDraft{}, fmt.Errorf("scan review draft row: %w", err)
	}
	d.CreatedAt = timeFromMS(createdAt)
	return d, nil
}
