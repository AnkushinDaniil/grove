package store

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/AnkushinDaniil/grove/internal/core"
)

// testFeedback returns a feedback fixture on nodeID, open by default.
func testFeedback(nodeID core.NodeID, kind, subject, comment string, createdMS int64) Feedback {
	return Feedback{
		ID:        string(core.NewEventID()), // any UUIDv7 works as a feedback id
		NodeID:    string(nodeID),
		Kind:      kind,
		Subject:   subject,
		Comment:   comment,
		CreatedAt: msTime(createdMS),
	}
}

func mustSaveFeedback(t *testing.T, s *Store, f Feedback) {
	t.Helper()
	if err := s.SaveFeedback(t.Context(), f); err != nil {
		t.Fatalf("SaveFeedback(%s): %v", f.ID, err)
	}
}

func TestSaveAndListFeedbackStatusFilter(t *testing.T) {
	s := newTestStore(t)
	node := testNode(core.NewNodeID(), "")
	mustSaveNode(t, s, node)

	open := testFeedback(node.ID, "skill", "code-review", "misfired", 1_000)
	resolved := testFeedback(node.ID, "tool", "Bash", "flaky", 2_000)
	resolved.ResolvedAt = msTime(3_000)
	resolved.FixNodeID = "fix-1"
	mustSaveFeedback(t, s, open)
	mustSaveFeedback(t, s, resolved)

	// all → newest first (created_at DESC).
	all, err := s.ListFeedback(t.Context(), FeedbackAll)
	if err != nil {
		t.Fatalf("ListFeedback(all): %v", err)
	}
	if len(all) != 2 || all[0].ID != resolved.ID || all[1].ID != open.ID {
		t.Fatalf("all order = %v, want [%s %s]", feedbackIDs(all), resolved.ID, open.ID)
	}

	openList, err := s.ListFeedback(t.Context(), FeedbackOpen)
	if err != nil {
		t.Fatalf("ListFeedback(open): %v", err)
	}
	if len(openList) != 1 || openList[0].ID != open.ID {
		t.Errorf("open = %v, want [%s]", feedbackIDs(openList), open.ID)
	}
	if !openList[0].ResolvedAt.IsZero() {
		t.Errorf("open item ResolvedAt = %v, want zero", openList[0].ResolvedAt)
	}

	resolvedList, err := s.ListFeedback(t.Context(), FeedbackResolved)
	if err != nil {
		t.Fatalf("ListFeedback(resolved): %v", err)
	}
	if len(resolvedList) != 1 || resolvedList[0].ID != resolved.ID {
		t.Errorf("resolved = %v, want [%s]", feedbackIDs(resolvedList), resolved.ID)
	}
	if resolvedList[0].FixNodeID != "fix-1" || resolvedList[0].ResolvedAt.IsZero() {
		t.Errorf("resolved item = %+v, want fix-1 and non-zero ResolvedAt", resolvedList[0])
	}
}

func TestResolveFeedback(t *testing.T) {
	s := newTestStore(t)
	node := testNode(core.NewNodeID(), "")
	mustSaveNode(t, s, node)
	f := testFeedback(node.ID, "model", "claude-sonnet-5", "wrong answer", 1_000)
	mustSaveFeedback(t, s, f)

	at := msTime(5_000)
	got, err := s.ResolveFeedback(t.Context(), f.ID, "fix-node", at)
	if err != nil {
		t.Fatalf("ResolveFeedback: %v", err)
	}
	if !got.ResolvedAt.Equal(at) || got.FixNodeID != "fix-node" {
		t.Errorf("resolved = %+v, want ResolvedAt=%v fix-node", got, at)
	}

	// Re-resolving keeps the original resolved_at (COALESCE) but can relink the fix.
	got2, err := s.ResolveFeedback(t.Context(), f.ID, "fix-node-2", msTime(9_000))
	if err != nil {
		t.Fatalf("ResolveFeedback (second): %v", err)
	}
	if !got2.ResolvedAt.Equal(at) {
		t.Errorf("re-resolve ResolvedAt = %v, want unchanged %v", got2.ResolvedAt, at)
	}
	if got2.FixNodeID != "fix-node-2" {
		t.Errorf("re-resolve FixNodeID = %q, want fix-node-2", got2.FixNodeID)
	}

	// Unknown id → ErrInvalid ("not found" flavored → 404 at the API).
	_, err = s.ResolveFeedback(t.Context(), "missing", "", at)
	if !errors.Is(err, core.ErrInvalid) {
		t.Errorf("ResolveFeedback(missing) err = %v, want core.ErrInvalid", err)
	}
}

func TestFeedbackBreakdownGroupsAndScope(t *testing.T) {
	s := newTestStore(t)
	inScope := testNode(core.NewNodeID(), "")
	outScope := testNode(core.NewNodeID(), "")
	mustSaveNode(t, s, inScope)
	mustSaveNode(t, s, outScope)

	// Two skill/code-review (one resolved), one tool/Bash, all in scope.
	fr := testFeedback(inScope.ID, "skill", "code-review", "a", 1_000)
	fr.ResolvedAt = msTime(1_500)
	mustSaveFeedback(t, s, fr)
	mustSaveFeedback(t, s, testFeedback(inScope.ID, "skill", "code-review", "b", 2_000))
	mustSaveFeedback(t, s, testFeedback(inScope.ID, "tool", "Bash", "c", 3_000))
	// Out of scope: must be excluded.
	mustSaveFeedback(t, s, testFeedback(outScope.ID, "skill", "code-review", "d", 4_000))

	stats, err := s.FeedbackBreakdown(t.Context(), []core.NodeID{inScope.ID})
	if err != nil {
		t.Fatalf("FeedbackBreakdown: %v", err)
	}
	// Ordered by total DESC: skill/code-review (2) then tool/Bash (1).
	if len(stats) != 2 {
		t.Fatalf("groups = %+v, want 2", stats)
	}
	if stats[0].Kind != "skill" || stats[0].Subject != "code-review" || stats[0].Total != 2 || stats[0].Open != 1 {
		t.Errorf("group0 = %+v, want skill/code-review total=2 open=1", stats[0])
	}
	if stats[1].Kind != "tool" || stats[1].Subject != "Bash" || stats[1].Total != 1 || stats[1].Open != 1 {
		t.Errorf("group1 = %+v, want tool/Bash total=1 open=1", stats[1])
	}

	// Empty scope → no groups, no query.
	empty, err := s.FeedbackBreakdown(t.Context(), nil)
	if err != nil {
		t.Fatalf("FeedbackBreakdown(nil): %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("empty scope groups = %+v, want none", empty)
	}
}

// TestMigrateFeedbackFromExisting simulates a database created before 0006 (only
// 0001..0005 applied) and verifies opening it applies 0006 and gives a usable
// feedback table.
func TestMigrateFeedbackFromExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "grove.db")
	seedDBUpToMigration(t, path, 5)

	s, err := Open(t.Context(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})

	assertMigrationApplied(t, s, 6)

	node := testNode(core.NewNodeID(), "")
	mustSaveNode(t, s, node)
	f := testFeedback(node.ID, "other", "", "note", 1_000)
	mustSaveFeedback(t, s, f)
	got, err := s.ListFeedback(t.Context(), FeedbackAll)
	if err != nil {
		t.Fatalf("ListFeedback after upgrade: %v", err)
	}
	if len(got) != 1 || got[0].ID != f.ID {
		t.Fatalf("feedback after upgrade = %v, want [%s]", feedbackIDs(got), f.ID)
	}
}

func feedbackIDs(fs []Feedback) []string {
	out := make([]string, len(fs))
	for i, f := range fs {
		out[i] = f.ID
	}
	return out
}
