// Package usage rolls grove's normalized `usage` events up into the pre-summed
// 5-minute buckets that back GET /usage.
//
// The Aggregator does a one-time backfill from the events table at startup
// (attributing each usage event to its session's driver/profile and advancing a
// high-water mark so restarts are idempotent), then folds live usage events off
// the tree's delta feed into in-memory buckets that are flushed to usage_rollup
// every 30s and once more on shutdown. Aggregation is best-effort: the meter it
// powers tolerates losing the last unflushed window on a hard crash.
package usage

import (
	"context"
	"log/slog"
	"time"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/store"
	"github.com/AnkushinDaniil/grove/internal/tree"
)

const (
	// bucketMS is the rollup bucket width: 5 minutes in milliseconds.
	bucketMS = int64(5 * 60 * 1000)
	// flushInterval bounds how long folded usage sits in memory before it is
	// persisted, and so the worst-case loss on an unclean shutdown.
	flushInterval = 30 * time.Second
	// backfillPage is the keyset page size when scanning historical usage events.
	backfillPage = 500
	// finalFlushTimeout bounds the best-effort flush when the loop stops.
	finalFlushTimeout = 5 * time.Second
)

// usageStore is the persistence surface the aggregator needs, satisfied by
// *store.Store.
type usageStore interface {
	GetSetting(ctx context.Context, key string) (string, bool, error)
	UsageEventsAfter(ctx context.Context, afterID string, limit int) ([]store.UsageEventRow, error)
	SaveUsageBackfill(ctx context.Context, buckets []store.UsageBucket, hwm string) error
	UpsertUsageBuckets(ctx context.Context, buckets []store.UsageBucket) error
	SessionAttribution(ctx context.Context, sessionID core.SessionID) (driver, profileID string, found bool, err error)
}

// deltaSource is the tree's live delta feed, satisfied by *tree.Tree.
type deltaSource interface {
	Subscribe() (tree.Snapshot, <-chan tree.Delta, func())
}

// bucketKey identifies one rollup cell in the in-memory accumulator.
type bucketKey struct {
	bucketStart int64
	profileID   string
	driver      string
	model       string
}

// Aggregator folds usage events into usage_rollup. Start it once; it runs until
// its context is canceled.
type Aggregator struct {
	store  usageStore
	source deltaSource
	logger *slog.Logger

	// flushInterval bounds how long folded usage sits in memory before it is
	// persisted; New sets the default and tests may shorten it.
	flushInterval time.Duration

	done chan struct{}
}

// New builds an Aggregator over the store and the tree's delta feed.
func New(st usageStore, source deltaSource, logger *slog.Logger) *Aggregator {
	if logger == nil {
		logger = slog.Default()
	}
	return &Aggregator{
		store:         st,
		source:        source,
		logger:        logger,
		flushInterval: flushInterval,
		done:          make(chan struct{}),
	}
}

// Start launches the aggregation loop in the background and returns at once. The
// loop backfills, consumes live deltas, and does a best-effort final flush when
// ctx is canceled. Wait blocks for that final flush.
func (a *Aggregator) Start(ctx context.Context) {
	go a.run(ctx)
}

// Wait blocks until the background loop has exited (after its final flush), or
// waitCtx is canceled first.
func (a *Aggregator) Wait(waitCtx context.Context) {
	select {
	case <-a.done:
	case <-waitCtx.Done():
	}
}

// run subscribes first (so nothing created during backfill is missed), backfills
// once, then consumes live deltas. A dropped subscription (extreme lag) is
// resubscribed for continued live counting; the rare gap is not re-counted.
func (a *Aggregator) run(ctx context.Context) {
	defer close(a.done)

	_, deltas, cancel := a.source.Subscribe()
	hwm, err := a.backfill(ctx)
	if err != nil {
		a.logger.Warn("usage backfill", "err", err)
	}
	for {
		closed := a.consume(ctx, deltas, hwm)
		cancel()
		if !closed {
			return
		}
		a.logger.Warn("usage delta subscription dropped; resubscribing")
		_, deltas, cancel = a.source.Subscribe()
	}
}

// backfill folds every usage event newer than the stored high-water mark into
// rollup buckets and advances the mark, all persisted in one transaction so a
// re-run counts nothing twice. It returns the mark used as the live-dedup
// boundary (the stored value when nothing new was found or on error).
func (a *Aggregator) backfill(ctx context.Context) (string, error) {
	hwm, _, err := a.store.GetSetting(ctx, store.UsageBackfillHWMKey)
	if err != nil {
		return "", err
	}
	acc := make(map[bucketKey]*store.UsageBucket)
	last := hwm
	for {
		rows, err := a.store.UsageEventsAfter(ctx, last, backfillPage)
		if err != nil {
			return hwm, err
		}
		for i := range rows {
			r := rows[i]
			last = r.EventID
			p, perr := core.UnmarshalPayload[core.UsagePayload](r.Payload)
			if perr != nil {
				a.logger.Warn("usage backfill payload", "event", r.EventID, "err", perr)
				continue
			}
			addUsage(acc, bucketStartMS(r.CreatedAt), r.ProfileID, r.Driver, p)
		}
		if len(rows) < backfillPage {
			break
		}
	}
	if last == hwm {
		return hwm, nil // nothing new to fold in
	}
	if err := a.store.SaveUsageBackfill(ctx, flatten(acc), last); err != nil {
		return hwm, err
	}
	return last, nil
}

// consume folds live usage deltas into in-memory buckets, flushing every
// flushInterval. It returns true when the delta channel closed (caller should
// resubscribe) and false when ctx was canceled (after a final flush).
func (a *Aggregator) consume(ctx context.Context, deltas <-chan tree.Delta, hwm string) bool {
	ticker := time.NewTicker(a.flushInterval)
	defer ticker.Stop()
	acc := make(map[bucketKey]*store.UsageBucket)
	for {
		select {
		case <-ctx.Done():
			//nolint:contextcheck // ctx is already canceled at shutdown; the final flush needs a fresh context.
			a.flushFinal(acc)
			return false
		case d, ok := <-deltas:
			if !ok {
				a.flush(ctx, acc)
				return true
			}
			a.foldDelta(ctx, acc, d, hwm)
		case <-ticker.C:
			a.flush(ctx, acc)
		}
	}
}

// foldDelta accumulates the usage events in one delta, skipping any already
// counted by the backfill (id at or below the dedup boundary — they overlap the
// window between Subscribe and the backfill query).
func (a *Aggregator) foldDelta(ctx context.Context, acc map[bucketKey]*store.UsageBucket, d tree.Delta, hwm string) {
	for _, ev := range d.Events {
		if ev.Type != core.EventUsage || string(ev.ID) <= hwm {
			continue
		}
		p, err := core.UnmarshalPayload[core.UsagePayload](ev.Payload)
		if err != nil {
			a.logger.Warn("usage delta payload", "event", ev.ID, "err", err)
			continue
		}
		driver, profileID := a.attribution(ctx, ev.SessionID)
		addUsage(acc, bucketStartMS(ev.CreatedAt.UnixMilli()), profileID, driver, p)
	}
}

// attribution resolves a session's driver/profile, defaulting to empty on a
// missing session or lookup error (the usage still counts, attributed to the
// default profile).
func (a *Aggregator) attribution(ctx context.Context, sessionID core.SessionID) (driver, profileID string) {
	if sessionID == "" {
		return "", ""
	}
	driver, profileID, found, err := a.store.SessionAttribution(ctx, sessionID)
	if err != nil {
		a.logger.Warn("usage attribution", "session", sessionID, "err", err)
		return "", ""
	}
	if !found {
		return "", ""
	}
	return driver, profileID
}

// flush persists the accumulated buckets and clears them on success; on failure
// they are kept for the next flush to retry.
func (a *Aggregator) flush(ctx context.Context, acc map[bucketKey]*store.UsageBucket) {
	if len(acc) == 0 {
		return
	}
	if err := a.store.UpsertUsageBuckets(ctx, flatten(acc)); err != nil {
		a.logger.Warn("usage flush", "err", err)
		return
	}
	clear(acc)
}

// flushFinal persists the last buckets on shutdown using a fresh, bounded
// context, since the loop's context is already canceled.
func (a *Aggregator) flushFinal(acc map[bucketKey]*store.UsageBucket) {
	if len(acc) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), finalFlushTimeout)
	defer cancel()
	if err := a.store.UpsertUsageBuckets(ctx, flatten(acc)); err != nil {
		a.logger.Warn("usage final flush", "err", err)
	}
}

// addUsage accumulates one usage payload onto its bucket cell.
func addUsage(acc map[bucketKey]*store.UsageBucket, bucketStart int64, profileID, driver string, p core.UsagePayload) {
	k := bucketKey{bucketStart: bucketStart, profileID: profileID, driver: driver, model: p.Model}
	b := acc[k]
	if b == nil {
		b = &store.UsageBucket{BucketStart: bucketStart, ProfileID: profileID, Driver: driver, Model: p.Model}
		acc[k] = b
	}
	b.InputTokens += p.InputTokens
	b.OutputTokens += p.OutputTokens
	b.CostUSD += p.CostUSD
}

// flatten materializes the accumulator as a bucket slice for persistence.
func flatten(acc map[bucketKey]*store.UsageBucket) []store.UsageBucket {
	out := make([]store.UsageBucket, 0, len(acc))
	for _, b := range acc {
		out = append(out, *b)
	}
	return out
}

// bucketStartMS truncates a unix-millisecond timestamp down to its 5-minute
// bucket start.
func bucketStartMS(ms int64) int64 {
	return ms - (ms % bucketMS)
}
