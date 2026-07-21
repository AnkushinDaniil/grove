package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/AnkushinDaniil/grove/internal/core"
)

// UsageBackfillHWMKey is the settings key holding the id of the last `usage`
// event folded into usage_rollup by the aggregator's one-time backfill. The
// aggregator reads it to resume where it left off and only counts newer events.
const UsageBackfillHWMKey = "usage_backfill_hwm"

// UsageBucket is one 5-minute rollup cell: token and cost totals attributed to
// a (profile, driver, model) at a bucket start (unix milliseconds truncated to
// a 5-minute boundary). Upserts accumulate onto the matching row.
type UsageBucket struct {
	BucketStart  int64
	ProfileID    string
	Driver       string
	Model        string
	InputTokens  int64
	OutputTokens int64
	CostUSD      float64
}

// UsageWindowRow is the summed usage for one (profile, driver) over a window,
// as returned by QueryUsageWindow.
type UsageWindowRow struct {
	ProfileID    string
	Driver       string
	InputTokens  int64
	OutputTokens int64
	CostUSD      float64
}

// UsageEventRow is one `usage` event with its owning session's driver/profile
// attribution, as returned by UsageEventsAfter for backfill. Payload is the raw
// UsagePayload JSON, parsed by the caller (not in SQL).
type UsageEventRow struct {
	EventID   string
	CreatedAt int64 // unix milliseconds
	Driver    string
	ProfileID string
	Payload   string
}

const upsertUsageBucketSQL = `
INSERT INTO usage_rollup (bucket_start, profile_id, driver, model, input_tokens, output_tokens, cost_usd)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(bucket_start, profile_id, driver, model) DO UPDATE SET
	input_tokens = input_tokens + excluded.input_tokens,
	output_tokens = output_tokens + excluded.output_tokens,
	cost_usd = cost_usd + excluded.cost_usd
`

// upsertUsageBucketsTx accumulates each bucket onto its rollup row within tx.
func upsertUsageBucketsTx(ctx context.Context, tx *sql.Tx, buckets []UsageBucket) error {
	for _, b := range buckets {
		_, err := tx.ExecContext(ctx, upsertUsageBucketSQL,
			b.BucketStart, b.ProfileID, b.Driver, b.Model,
			b.InputTokens, b.OutputTokens, b.CostUSD,
		)
		if err != nil {
			return fmt.Errorf("upsert usage bucket %d/%s/%s/%s: %w",
				b.BucketStart, b.ProfileID, b.Driver, b.Model, err)
		}
	}
	return nil
}

// UpsertUsageBuckets accumulates a batch of buckets into usage_rollup in a
// single transaction. Re-applying the same increment sums it onto the row, so
// callers must not replay the same usage twice.
func (s *Store) UpsertUsageBuckets(ctx context.Context, buckets []UsageBucket) error {
	if len(buckets) == 0 {
		return nil
	}
	return s.inTx(ctx, func(tx *sql.Tx) error {
		return upsertUsageBucketsTx(ctx, tx, buckets)
	})
}

// SaveUsageBackfill accumulates buckets and advances the backfill high-water
// mark to hwm in one transaction, so a crash mid-backfill never leaves the
// counted buckets and the watermark out of step (which would double-count on
// the next run).
func (s *Store) SaveUsageBackfill(ctx context.Context, buckets []UsageBucket, hwm string) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		if err := upsertUsageBucketsTx(ctx, tx, buckets); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, upsertSettingSQL, UsageBackfillHWMKey, hwm); err != nil {
			return fmt.Errorf("advance usage backfill hwm: %w", err)
		}
		return nil
	})
}

const selectUsageWindowSQL = `
SELECT profile_id, driver,
	COALESCE(SUM(input_tokens), 0),
	COALESCE(SUM(output_tokens), 0),
	COALESCE(SUM(cost_usd), 0)
FROM usage_rollup
WHERE bucket_start >= ? AND bucket_start < ?
GROUP BY profile_id, driver
ORDER BY profile_id, driver
`

// QueryUsageWindow sums usage_rollup over the half-open bucket range
// [from, to), grouped by (profile, driver). Only groups with usage in the
// window appear. Bucket membership is by bucket_start, so a bucket counts when
// its start falls in the range.
func (s *Store) QueryUsageWindow(ctx context.Context, from, to time.Time) ([]UsageWindowRow, error) {
	rows, err := s.db.QueryContext(ctx, selectUsageWindowSQL, from.UnixMilli(), to.UnixMilli())
	if err != nil {
		return nil, fmt.Errorf("query usage window: %w", err)
	}
	out, err := collect(rows, scanUsageWindowRow)
	if err != nil {
		return nil, fmt.Errorf("query usage window: %w", err)
	}
	return out, nil
}

func scanUsageWindowRow(row rowScanner) (UsageWindowRow, error) {
	var r UsageWindowRow
	if err := row.Scan(&r.ProfileID, &r.Driver, &r.InputTokens, &r.OutputTokens, &r.CostUSD); err != nil {
		return UsageWindowRow{}, fmt.Errorf("scan usage window row: %w", err)
	}
	return r, nil
}

const selectUsageEventsAfterSQL = `
SELECT e.id, e.created_at, COALESCE(s.driver, ''), COALESCE(s.profile_id, ''), e.payload
FROM events e
LEFT JOIN sessions s ON s.id = e.session_id
WHERE e.type = ? AND e.id > ?
ORDER BY e.id ASC
LIMIT ?
`

// UsageEventsAfter returns up to limit `usage` events with id > afterID
// (ascending), each joined to its session's driver/profile for attribution.
// Event ids are UUIDv7, so afterID keyset-paginates the backfill. A usage
// event whose session is missing yields empty driver/profile.
func (s *Store) UsageEventsAfter(ctx context.Context, afterID string, limit int) ([]UsageEventRow, error) {
	rows, err := s.db.QueryContext(ctx, selectUsageEventsAfterSQL, string(core.EventUsage), afterID, limit)
	if err != nil {
		return nil, fmt.Errorf("query usage events after %q: %w", afterID, err)
	}
	out, err := collect(rows, scanUsageEventRow)
	if err != nil {
		return nil, fmt.Errorf("query usage events after %q: %w", afterID, err)
	}
	return out, nil
}

func scanUsageEventRow(row rowScanner) (UsageEventRow, error) {
	var r UsageEventRow
	if err := row.Scan(&r.EventID, &r.CreatedAt, &r.Driver, &r.ProfileID, &r.Payload); err != nil {
		return UsageEventRow{}, fmt.Errorf("scan usage event row: %w", err)
	}
	return r, nil
}

// SessionAttribution returns the driver and profile a session was launched
// under, for attributing that session's live usage events. found is false when
// no such session exists yet.
func (s *Store) SessionAttribution(ctx context.Context, sessionID core.SessionID) (driver, profileID string, found bool, err error) {
	row := s.db.QueryRowContext(ctx,
		"SELECT driver, profile_id FROM sessions WHERE id = ?", string(sessionID))
	err = row.Scan(&driver, &profileID)
	switch {
	case err == nil:
		return driver, profileID, true, nil
	case errors.Is(err, sql.ErrNoRows):
		return "", "", false, nil
	default:
		return "", "", false, fmt.Errorf("session attribution %s: %w", sessionID, err)
	}
}
