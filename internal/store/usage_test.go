package store

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/AnkushinDaniil/grove/internal/core"
)

// seedDBUpToMigration builds a database at path with every migration up to and
// including upTo applied and recorded, reproducing an older on-disk schema.
func seedDBUpToMigration(t *testing.T, path string, upTo int64) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	defer func() { _ = db.Close() }()

	migrations, err := loadMigrations()
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	if _, err := db.ExecContext(t.Context(), createMigrationsTableSQL); err != nil {
		t.Fatalf("create schema_migrations table: %v", err)
	}
	for _, m := range migrations {
		if m.version > upTo {
			continue
		}
		if _, err := db.ExecContext(t.Context(), m.sql); err != nil {
			t.Fatalf("apply migration %d: %v", m.version, err)
		}
		if _, err := db.ExecContext(t.Context(),
			"INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)",
			m.version, time.Now().UnixMilli()); err != nil {
			t.Fatalf("record migration %d: %v", m.version, err)
		}
	}
}

// TestMigrateUsageRollupFromZero opens a fresh database and verifies 0003 lands,
// giving a usable usage_rollup table.
func TestMigrateUsageRollupFromZero(t *testing.T) {
	s := newTestStore(t)
	assertMigrationApplied(t, s, 3)

	b := UsageBucket{BucketStart: 300_000, Driver: "claude", InputTokens: 5, OutputTokens: 7, CostUSD: 0.5}
	if err := s.UpsertUsageBuckets(t.Context(), []UsageBucket{b}); err != nil {
		t.Fatalf("UpsertUsageBuckets: %v", err)
	}
	rows, err := s.QueryUsageWindow(t.Context(), msTime(0), msTime(600_000))
	if err != nil {
		t.Fatalf("QueryUsageWindow: %v", err)
	}
	if len(rows) != 1 || rows[0].InputTokens != 5 || rows[0].OutputTokens != 7 {
		t.Fatalf("rows = %+v, want one row 5/7", rows)
	}
}

// TestMigrateUsageRollupFromExisting0002DB simulates a database created before
// 0003 (only 0001+0002 applied) and verifies opening it applies 0003 without
// disturbing existing data.
func TestMigrateUsageRollupFromExisting0002DB(t *testing.T) {
	path := filepath.Join(t.TempDir(), "grove.db")
	seedDBUpToMigration(t, path, 2)

	s, err := Open(t.Context(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})

	assertMigrationApplied(t, s, 3)

	// 0001 data (a node) still round-trips after the upgrade.
	n := testNode(core.NewNodeID(), "")
	mustSaveNode(t, s, n)
	if got := loadNodeDirect(t, s, n.ID); got.ID != n.ID {
		t.Fatalf("node id = %q, want %q", got.ID, n.ID)
	}
	// And the new table is usable.
	if err := s.UpsertUsageBuckets(t.Context(),
		[]UsageBucket{{BucketStart: 0, Driver: "claude", InputTokens: 3}}); err != nil {
		t.Fatalf("UpsertUsageBuckets after upgrade: %v", err)
	}
}

func TestUpsertUsageBucketsAccumulates(t *testing.T) {
	s := newTestStore(t)
	key := UsageBucket{BucketStart: 300_000, ProfileID: "p1", Driver: "claude", Model: "sonnet"}

	first := key
	first.InputTokens, first.OutputTokens, first.CostUSD = 10, 4, 0.10
	second := key
	second.InputTokens, second.OutputTokens, second.CostUSD = 5, 6, 0.05

	for _, b := range []UsageBucket{first, second} {
		if err := s.UpsertUsageBuckets(t.Context(), []UsageBucket{b}); err != nil {
			t.Fatalf("UpsertUsageBuckets: %v", err)
		}
	}

	rows, err := s.QueryUsageWindow(t.Context(), msTime(0), msTime(600_000))
	if err != nil {
		t.Fatalf("QueryUsageWindow: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1 (same key sums)", len(rows))
	}
	if rows[0].InputTokens != 15 || rows[0].OutputTokens != 10 {
		t.Errorf("summed tokens = %d/%d, want 15/10", rows[0].InputTokens, rows[0].OutputTokens)
	}
	if rows[0].CostUSD < 0.1499 || rows[0].CostUSD > 0.1501 {
		t.Errorf("summed cost = %v, want ~0.15", rows[0].CostUSD)
	}
}

// TestUpsertUsageBucketsDistinctModel verifies model is part of the key: two
// models under the same (bucket, profile, driver) are distinct rows that still
// group together in the window query.
func TestUpsertUsageBucketsDistinctModel(t *testing.T) {
	s := newTestStore(t)
	buckets := []UsageBucket{
		{BucketStart: 300_000, ProfileID: "p1", Driver: "claude", Model: "sonnet", InputTokens: 10},
		{BucketStart: 300_000, ProfileID: "p1", Driver: "claude", Model: "opus", InputTokens: 20},
	}
	if err := s.UpsertUsageBuckets(t.Context(), buckets); err != nil {
		t.Fatalf("UpsertUsageBuckets: %v", err)
	}
	rows, err := s.QueryUsageWindow(t.Context(), msTime(0), msTime(600_000))
	if err != nil {
		t.Fatalf("QueryUsageWindow: %v", err)
	}
	// The window query groups by (profile, driver), summing across models.
	if len(rows) != 1 || rows[0].InputTokens != 30 {
		t.Fatalf("rows = %+v, want one row summing to 30", rows)
	}
}

func TestQueryUsageWindowBoundaries(t *testing.T) {
	s := newTestStore(t)
	from, to := msTime(1_000_000), msTime(2_000_000)
	buckets := []UsageBucket{
		{BucketStart: 1_000_000, Driver: "claude", InputTokens: 10}, // == from → in
		{BucketStart: 900_000, Driver: "claude", InputTokens: 20},   // < from → out
		{BucketStart: 1_500_000, Driver: "claude", InputTokens: 40}, // in
		{BucketStart: 2_000_000, Driver: "claude", InputTokens: 80}, // == to → out (half-open)
	}
	if err := s.UpsertUsageBuckets(t.Context(), buckets); err != nil {
		t.Fatalf("UpsertUsageBuckets: %v", err)
	}
	rows, err := s.QueryUsageWindow(t.Context(), from, to)
	if err != nil {
		t.Fatalf("QueryUsageWindow: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if rows[0].InputTokens != 50 {
		t.Errorf("in-window input = %d, want 50 (10+40)", rows[0].InputTokens)
	}
}

func TestQueryUsageWindowGroupsAndEmpty(t *testing.T) {
	s := newTestStore(t)

	// Empty table → no rows.
	rows, err := s.QueryUsageWindow(t.Context(), msTime(0), msTime(600_000))
	if err != nil {
		t.Fatalf("QueryUsageWindow empty: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("empty window rows = %d, want 0", len(rows))
	}

	buckets := []UsageBucket{
		{BucketStart: 0, ProfileID: "p1", Driver: "claude", InputTokens: 10},
		{BucketStart: 300_000, ProfileID: "p1", Driver: "claude", InputTokens: 5},
		{BucketStart: 0, ProfileID: "p2", Driver: "codex", InputTokens: 7},
	}
	if err := s.UpsertUsageBuckets(t.Context(), buckets); err != nil {
		t.Fatalf("UpsertUsageBuckets: %v", err)
	}
	rows, err = s.QueryUsageWindow(t.Context(), msTime(0), msTime(600_000))
	if err != nil {
		t.Fatalf("QueryUsageWindow: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2 groups", len(rows))
	}
	// Ordered by profile_id, driver: p1/claude then p2/codex.
	if rows[0].ProfileID != "p1" || rows[0].Driver != "claude" || rows[0].InputTokens != 15 {
		t.Errorf("row0 = %+v, want p1/claude/15", rows[0])
	}
	if rows[1].ProfileID != "p2" || rows[1].Driver != "codex" || rows[1].InputTokens != 7 {
		t.Errorf("row1 = %+v, want p2/codex/7", rows[1])
	}
}

func TestSaveUsageBackfillAdvancesHWM(t *testing.T) {
	s := newTestStore(t)
	buckets := []UsageBucket{{BucketStart: 0, Driver: "claude", InputTokens: 9}}
	if err := s.SaveUsageBackfill(t.Context(), buckets, "event-42"); err != nil {
		t.Fatalf("SaveUsageBackfill: %v", err)
	}

	hwm, ok, err := s.GetSetting(t.Context(), UsageBackfillHWMKey)
	if err != nil {
		t.Fatalf("GetSetting: %v", err)
	}
	if !ok || hwm != "event-42" {
		t.Errorf("hwm = %q (present=%v), want event-42", hwm, ok)
	}
	rows, err := s.QueryUsageWindow(t.Context(), msTime(0), msTime(600_000))
	if err != nil {
		t.Fatalf("QueryUsageWindow: %v", err)
	}
	if len(rows) != 1 || rows[0].InputTokens != 9 {
		t.Fatalf("rows = %+v, want one row 9", rows)
	}
}

func TestUsageEventsAfterAttributionAndPaging(t *testing.T) {
	s := newTestStore(t)

	// A node + two sessions with distinct attribution.
	node := testNode(core.NewNodeID(), "")
	mustSaveNode(t, s, node)
	claudeSess := testSession(core.NewSessionID(), node.ID) // claude / profile-1
	mustSaveSession(t, s, claudeSess)
	codexSess := testSession(core.NewSessionID(), node.ID)
	codexSess.Driver = "codex"
	codexSess.ProfileID = core.ProfileID("profile-2")
	mustSaveSession(t, s, codexSess)

	usagePayload := func(in, out int64, model string) string {
		p, err := core.MarshalPayload(core.UsagePayload{InputTokens: in, OutputTokens: out, Model: model})
		if err != nil {
			t.Fatalf("MarshalPayload: %v", err)
		}
		return p
	}

	base := time.UnixMilli(1_700_000_000_000).UTC()
	// Ascending ids: NewEventID is UUIDv7 (time-sortable) and we create them in order.
	ev1 := core.Event{ID: core.NewEventID(), NodeID: node.ID, SessionID: claudeSess.ID, Type: core.EventUsage, Payload: usagePayload(10, 2, "sonnet"), CreatedAt: base}
	ev2 := core.Event{ID: core.NewEventID(), NodeID: node.ID, SessionID: codexSess.ID, Type: core.EventUsage, Payload: usagePayload(20, 4, "gpt"), CreatedAt: base.Add(time.Minute)}
	// A non-usage event must be excluded.
	textEv := core.Event{ID: core.NewEventID(), NodeID: node.ID, SessionID: claudeSess.ID, Type: core.EventText, Payload: `{"text":"hi"}`, CreatedAt: base}
	if err := s.AppendEvents(t.Context(), []core.Event{ev1, ev2, textEv}); err != nil {
		t.Fatalf("AppendEvents: %v", err)
	}

	// First page from the beginning.
	rows, err := s.UsageEventsAfter(t.Context(), "", 1)
	if err != nil {
		t.Fatalf("UsageEventsAfter: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("page1 rows = %d, want 1", len(rows))
	}
	if rows[0].EventID != string(ev1.ID) || rows[0].Driver != "claude" || rows[0].ProfileID != "profile-1" {
		t.Errorf("page1[0] = %+v, want ev1 claude/profile-1", rows[0])
	}
	if rows[0].CreatedAt != base.UnixMilli() {
		t.Errorf("page1[0].CreatedAt = %d, want %d", rows[0].CreatedAt, base.UnixMilli())
	}

	// Next page after ev1 → ev2 (codex), text event excluded.
	rows, err = s.UsageEventsAfter(t.Context(), string(ev1.ID), 10)
	if err != nil {
		t.Fatalf("UsageEventsAfter page2: %v", err)
	}
	if len(rows) != 1 || rows[0].EventID != string(ev2.ID) || rows[0].Driver != "codex" {
		t.Fatalf("page2 = %+v, want only ev2 (codex)", rows)
	}
}

func TestSessionAttribution(t *testing.T) {
	s := newTestStore(t)
	node := testNode(core.NewNodeID(), "")
	mustSaveNode(t, s, node)
	sess := testSession(core.NewSessionID(), node.ID)
	mustSaveSession(t, s, sess)

	driver, profileID, found, err := s.SessionAttribution(t.Context(), sess.ID)
	if err != nil {
		t.Fatalf("SessionAttribution: %v", err)
	}
	if !found || driver != "claude" || profileID != "profile-1" {
		t.Errorf("attribution = %q/%q found=%v, want claude/profile-1", driver, profileID, found)
	}

	_, _, found, err = s.SessionAttribution(t.Context(), core.SessionID("missing"))
	if err != nil {
		t.Fatalf("SessionAttribution missing: %v", err)
	}
	if found {
		t.Error("found = true for missing session, want false")
	}
}
