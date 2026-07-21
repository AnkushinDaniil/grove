package usage

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/store"
	"github.com/AnkushinDaniil/grove/internal/tree"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "grove.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// seedSession persists a node and a session bound to it with the given
// attribution, so usage events on that session resolve to driver/profile.
func seedSession(t *testing.T, st *store.Store, driver, profileID string) core.Session {
	t.Helper()
	now := time.UnixMilli(1_700_000_000_000).UTC()
	node := core.Node{
		ID: core.NewNodeID(), Kind: core.KindTask, Title: "n", Status: core.StatusIdle,
		Attention: core.AttentionNone, Meta: "{}", CreatedAt: now, UpdatedAt: now,
	}
	if err := st.SaveNodes(t.Context(), []core.Node{node}); err != nil {
		t.Fatalf("SaveNodes: %v", err)
	}
	sess := core.Session{
		ID: core.NewSessionID(), NodeID: node.ID, Driver: driver, ProfileID: core.ProfileID(profileID),
		Mode: core.ModeHeadless, Status: core.SessionRunning, CWD: "/tmp", StartedAt: now,
	}
	if err := st.SaveSessions(t.Context(), []core.Session{sess}); err != nil {
		t.Fatalf("SaveSessions: %v", err)
	}
	return sess
}

func usagePayload(t *testing.T, in, out int64, cost float64, model string) string {
	t.Helper()
	p, err := core.MarshalPayload(core.UsagePayload{InputTokens: in, OutputTokens: out, CostUSD: cost, Model: model})
	if err != nil {
		t.Fatalf("MarshalPayload: %v", err)
	}
	return p
}

// appendUsageEvent inserts one usage event straight into the store at ts.
func appendUsageEvent(t *testing.T, st *store.Store, sess core.Session, payload string, ts time.Time) core.Event {
	t.Helper()
	ev := core.Event{
		ID: core.NewEventID(), NodeID: sess.NodeID, SessionID: sess.ID,
		Type: core.EventUsage, Payload: payload, CreatedAt: ts,
	}
	if err := st.AppendEvents(t.Context(), []core.Event{ev}); err != nil {
		t.Fatalf("AppendEvents: %v", err)
	}
	return ev
}

func windowTotals(t *testing.T, st *store.Store) []store.UsageWindowRow {
	t.Helper()
	rows, err := st.QueryUsageWindow(t.Context(), time.UnixMilli(0), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("QueryUsageWindow: %v", err)
	}
	return rows
}

func TestBucketStartMS(t *testing.T) {
	tests := []struct {
		ms, want int64
	}{
		{0, 0},
		{1, 0},
		{bucketMS - 1, 0},
		{bucketMS, bucketMS},
		{bucketMS + 1, bucketMS},
		{2*bucketMS + 123, 2 * bucketMS},
	}
	for _, tt := range tests {
		if got := bucketStartMS(tt.ms); got != tt.want {
			t.Errorf("bucketStartMS(%d) = %d, want %d", tt.ms, got, tt.want)
		}
	}
}

func TestBackfillAttributesAdvancesHWMAndIsIdempotent(t *testing.T) {
	st := newStore(t)
	claude := seedSession(t, st, "claude", "profile-1")
	codex := seedSession(t, st, "codex", "profile-2")

	base := time.UnixMilli(1_700_000_000_000).UTC()
	appendUsageEvent(t, st, claude, usagePayload(t, 10, 2, 0.10, "sonnet"), base)
	appendUsageEvent(t, st, codex, usagePayload(t, 20, 4, 0.20, "gpt"), base.Add(time.Minute))
	last := appendUsageEvent(t, st, claude, usagePayload(t, 5, 1, 0.05, "sonnet"), base.Add(2*time.Minute))

	agg := New(st, nil, testLogger())
	hwm, err := agg.backfill(t.Context())
	if err != nil {
		t.Fatalf("backfill: %v", err)
	}
	if hwm != string(last.ID) {
		t.Errorf("hwm = %q, want last event id %q", hwm, last.ID)
	}

	rows := windowTotals(t, st)
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2 (claude, codex)", len(rows))
	}
	// Ordered by profile_id: profile-1 (claude) then profile-2 (codex).
	if rows[0].ProfileID != "profile-1" || rows[0].Driver != "claude" ||
		rows[0].InputTokens != 15 || rows[0].OutputTokens != 3 {
		t.Errorf("claude row = %+v, want profile-1/claude/15/3", rows[0])
	}
	if rows[1].ProfileID != "profile-2" || rows[1].Driver != "codex" || rows[1].InputTokens != 20 {
		t.Errorf("codex row = %+v, want profile-2/codex/20", rows[1])
	}

	// Idempotent: a second backfill folds nothing new in.
	hwm2, err := agg.backfill(t.Context())
	if err != nil {
		t.Fatalf("backfill re-run: %v", err)
	}
	if hwm2 != hwm {
		t.Errorf("re-run hwm = %q, want unchanged %q", hwm2, hwm)
	}
	rows = windowTotals(t, st)
	if rows[0].InputTokens != 15 || rows[1].InputTokens != 20 {
		t.Errorf("re-run changed totals: %+v", rows)
	}
}

// TestBackfillEmptyStore keeps the high-water mark empty when there is nothing
// to fold in.
func TestBackfillEmptyStore(t *testing.T) {
	st := newStore(t)
	agg := New(st, nil, testLogger())
	hwm, err := agg.backfill(t.Context())
	if err != nil {
		t.Fatalf("backfill: %v", err)
	}
	if hwm != "" {
		t.Errorf("hwm = %q, want empty", hwm)
	}
	if rows := windowTotals(t, st); len(rows) != 0 {
		t.Errorf("rows = %d, want 0", len(rows))
	}
}

// TestBackfillSkipsBadPayloadButAdvances folds valid events and steps the mark
// past an unparseable one rather than reprocessing it forever.
func TestBackfillSkipsBadPayload(t *testing.T) {
	st := newStore(t)
	sess := seedSession(t, st, "claude", "p1")
	base := time.UnixMilli(1_700_000_000_000).UTC()
	appendUsageEvent(t, st, sess, "not json", base)
	last := appendUsageEvent(t, st, sess, usagePayload(t, 7, 3, 0, "sonnet"), base.Add(time.Minute))

	agg := New(st, nil, testLogger())
	hwm, err := agg.backfill(t.Context())
	if err != nil {
		t.Fatalf("backfill: %v", err)
	}
	if hwm != string(last.ID) {
		t.Errorf("hwm = %q, want %q (advanced past bad payload)", hwm, last.ID)
	}
	rows := windowTotals(t, st)
	if len(rows) != 1 || rows[0].InputTokens != 7 {
		t.Fatalf("rows = %+v, want one row with 7 input", rows)
	}
}

func TestFoldDeltaDedupsBackfilledEvents(t *testing.T) {
	st := newStore(t)
	sess := seedSession(t, st, "claude", "p1")
	agg := New(st, nil, testLogger())

	base := time.UnixMilli(1_700_000_000_000).UTC()
	oldEv := core.Event{
		ID: core.NewEventID(), NodeID: sess.NodeID, SessionID: sess.ID,
		Type: core.EventUsage, Payload: usagePayload(t, 100, 100, 0, "sonnet"), CreatedAt: base,
	}
	newEv := core.Event{
		ID: core.NewEventID(), NodeID: sess.NodeID, SessionID: sess.ID,
		Type: core.EventUsage, Payload: usagePayload(t, 5, 6, 0, "sonnet"), CreatedAt: base.Add(time.Minute),
	}
	// hwm sits at oldEv: oldEv was already counted by backfill, newEv was not.
	hwm := string(oldEv.ID)

	acc := make(map[bucketKey]*store.UsageBucket)
	agg.foldDelta(t.Context(), acc, tree.Delta{Events: []core.Event{oldEv, newEv}}, hwm)

	if len(acc) != 1 {
		t.Fatalf("acc has %d cells, want 1 (only newEv)", len(acc))
	}
	for _, b := range acc {
		if b.InputTokens != 5 || b.OutputTokens != 6 {
			t.Errorf("folded = %d/%d, want 5/6 (oldEv deduped)", b.InputTokens, b.OutputTokens)
		}
	}
}

// TestFoldDeltaIgnoresNonUsageAndMissingAttribution checks non-usage events are
// skipped and a usage event whose session is unknown still counts, attributed
// to the empty/default profile.
func TestFoldDeltaIgnoresNonUsageAndMissingAttribution(t *testing.T) {
	st := newStore(t)
	agg := New(st, nil, testLogger())
	base := time.UnixMilli(1_700_000_000_000).UTC()

	textEv := core.Event{ID: core.NewEventID(), Type: core.EventText, Payload: `{"text":"hi"}`, CreatedAt: base}
	// Usage event with a session id that does not exist in the store.
	orphan := core.Event{
		ID: core.NewEventID(), SessionID: core.SessionID("ghost"),
		Type: core.EventUsage, Payload: usagePayload(t, 9, 1, 0, ""), CreatedAt: base,
	}

	acc := make(map[bucketKey]*store.UsageBucket)
	agg.foldDelta(t.Context(), acc, tree.Delta{Events: []core.Event{textEv, orphan}}, "")

	if len(acc) != 1 {
		t.Fatalf("acc has %d cells, want 1 (text ignored, orphan counted)", len(acc))
	}
	for k, b := range acc {
		if k.profileID != "" || k.driver != "" {
			t.Errorf("orphan attribution = %q/%q, want empty/empty", k.profileID, k.driver)
		}
		if b.InputTokens != 9 {
			t.Errorf("orphan input = %d, want 9", b.InputTokens)
		}
	}
}

func TestFlushPersistsAndClears(t *testing.T) {
	st := newStore(t)
	agg := New(st, nil, testLogger())
	acc := map[bucketKey]*store.UsageBucket{
		{bucketStart: 0, driver: "claude"}: {BucketStart: 0, Driver: "claude", InputTokens: 4},
	}
	agg.flush(t.Context(), acc)
	if len(acc) != 0 {
		t.Errorf("acc not cleared after flush: %d cells", len(acc))
	}
	if rows := windowTotals(t, st); len(rows) != 1 || rows[0].InputTokens != 4 {
		t.Fatalf("rows = %+v, want one row with 4", rows)
	}
}

// TestFlushKeepsBucketsOnError retains the accumulator when the store write
// fails so the next flush retries.
func TestFlushKeepsBucketsOnError(t *testing.T) {
	fs := &fakeStore{upsertErr: errors.New("boom")}
	agg := New(fs, nil, testLogger())
	acc := map[bucketKey]*store.UsageBucket{
		{driver: "claude"}: {Driver: "claude", InputTokens: 1},
	}
	agg.flush(t.Context(), acc)
	if len(acc) != 1 {
		t.Errorf("acc cleared despite flush error: %d cells, want 1", len(acc))
	}
}

// TestRunEndToEnd exercises the whole loop over real store + tree: Start,
// backfill, live fold off the delta feed, periodic flush, and a final flush on
// ctx cancel.
func TestRunEndToEnd(t *testing.T) {
	st := newStore(t)
	tr := tree.New(st)
	root, err := tr.Bootstrap(t.Context(), "ws")
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	// A session bound to the root supplies attribution for its usage events.
	now := time.Now()
	sess := core.Session{
		ID: core.NewSessionID(), NodeID: root.ID, Driver: "claude", ProfileID: "profile-1",
		Mode: core.ModeHeadless, Status: core.SessionRunning, CWD: "/tmp", StartedAt: now,
	}
	if _, err := tr.ApplySession(t.Context(), sess); err != nil {
		t.Fatalf("ApplySession: %v", err)
	}

	agg := New(st, tr, testLogger())
	agg.flushInterval = 5 * time.Millisecond

	ctx, cancel := context.WithCancel(t.Context())
	agg.Start(ctx)

	// Emit a live usage event through the tree; the aggregator folds it off the
	// delta feed and the short-interval flush persists it.
	if _, err := tr.IngestEvents(ctx, root.ID, sess.ID, []core.EventInput{
		{Type: core.EventUsage, Payload: usagePayload(t, 30, 12, 0.3, "sonnet")},
	}); err != nil {
		t.Fatalf("IngestEvents: %v", err)
	}

	waitFor(t, time.Second, func() bool {
		rows := windowTotals(t, st)
		return len(rows) == 1 && rows[0].OutputTokens == 12
	})

	cancel()
	waitCtx, waitCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer waitCancel()
	agg.Wait(waitCtx)

	rows := windowTotals(t, st)
	if len(rows) != 1 || rows[0].Driver != "claude" || rows[0].ProfileID != "profile-1" ||
		rows[0].InputTokens != 30 || rows[0].OutputTokens != 12 {
		t.Fatalf("rows = %+v, want one claude/profile-1 row 30/12", rows)
	}
}

// TestFlushFinalPersists covers the shutdown flush, which persists the last
// buckets using its own fresh context.
func TestFlushFinalPersists(t *testing.T) {
	st := newStore(t)
	agg := New(st, nil, testLogger())

	agg.flushFinal(nil) // empty accumulator is a no-op
	if rows := windowTotals(t, st); len(rows) != 0 {
		t.Fatalf("rows = %d after empty final flush, want 0", len(rows))
	}

	acc := map[bucketKey]*store.UsageBucket{
		{driver: "claude"}: {Driver: "claude", OutputTokens: 9},
	}
	agg.flushFinal(acc)
	if rows := windowTotals(t, st); len(rows) != 1 || rows[0].OutputTokens != 9 {
		t.Fatalf("rows = %+v, want one row with 9 output", rows)
	}
}

// waitFor polls cond until true or the deadline elapses.
func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}

// fakeStore is an injectable usageStore for exercising error paths.
type fakeStore struct {
	getErr      error
	eventsErr   error
	backfillErr error
	upsertErr   error
	hwm         string
	events      []store.UsageEventRow
}

func (f *fakeStore) GetSetting(_ context.Context, _ string) (string, bool, error) {
	if f.getErr != nil {
		return "", false, f.getErr
	}
	return f.hwm, f.hwm != "", nil
}

func (f *fakeStore) UsageEventsAfter(_ context.Context, _ string, _ int) ([]store.UsageEventRow, error) {
	if f.eventsErr != nil {
		return nil, f.eventsErr
	}
	return f.events, nil
}

func (f *fakeStore) SaveUsageBackfill(_ context.Context, _ []store.UsageBucket, _ string) error {
	return f.backfillErr
}

func (f *fakeStore) UpsertUsageBuckets(_ context.Context, _ []store.UsageBucket) error {
	return f.upsertErr
}

func (f *fakeStore) SessionAttribution(_ context.Context, _ core.SessionID) (string, string, bool, error) {
	return "", "", false, nil
}

func TestBackfillReturnsStoredHWMOnError(t *testing.T) {
	fs := &fakeStore{getErr: errors.New("get boom")}
	agg := New(fs, nil, testLogger())
	if _, err := agg.backfill(t.Context()); err == nil {
		t.Fatal("backfill: want error from GetSetting, got nil")
	}

	fs = &fakeStore{hwm: "hwm-1", eventsErr: errors.New("events boom")}
	agg = New(fs, nil, testLogger())
	hwm, err := agg.backfill(t.Context())
	if err == nil {
		t.Fatal("backfill: want error from UsageEventsAfter, got nil")
	}
	if hwm != "hwm-1" {
		t.Errorf("hwm = %q, want the stored hwm-1 on error", hwm)
	}
}
