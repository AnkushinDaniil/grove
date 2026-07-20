package store

import (
	"reflect"
	"testing"

	"github.com/AnkushinDaniil/grove/internal/core"
)

func TestSaveNodesRoundTrip(t *testing.T) {
	s := newTestStore(t)

	root := testNode(core.NewNodeID(), "") // root: empty ParentID -> NULL parent_id
	child := testNode(core.NewNodeID(), root.ID)
	child.Attention = core.AttentionQuestion
	child.AttentionReason = "waiting on user"
	child.AttentionSince = msTime(1_700_000_001_000)

	if err := s.SaveNodes(t.Context(), []core.Node{root, child}); err != nil {
		t.Fatalf("SaveNodes: %v", err)
	}

	if got := loadNodeDirect(t, s, root.ID); !reflect.DeepEqual(got, root) {
		t.Errorf("root round trip mismatch:\ngot  %+v\nwant %+v", got, root)
	}
	if got := loadNodeDirect(t, s, child.ID); !reflect.DeepEqual(got, child) {
		t.Errorf("child round trip mismatch:\ngot  %+v\nwant %+v", got, child)
	}
}

func TestSaveNodesUpsertUpdatesInPlace(t *testing.T) {
	s := newTestStore(t)
	n := testNode(core.NewNodeID(), "")
	mustSaveNode(t, s, n)

	n.Title = "Updated title"
	n.Status = core.StatusRunning
	n.UpdatedAt = msTime(1_700_000_002_000)
	mustSaveNode(t, s, n)

	got := loadNodeDirect(t, s, n.ID)
	if !reflect.DeepEqual(got, n) {
		t.Errorf("updated node mismatch:\ngot  %+v\nwant %+v", got, n)
	}

	var count int
	if err := s.db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM nodes WHERE id = ?", string(n.ID)).
		Scan(&count); err != nil {
		t.Fatalf("count nodes: %v", err)
	}
	if count != 1 {
		t.Errorf("row count for node %s = %d, want 1 (upsert must not duplicate)", n.ID, count)
	}
}

func TestSaveNodesArchivedAtRoundTrip(t *testing.T) {
	s := newTestStore(t)
	n := testNode(core.NewNodeID(), "")
	n.ArchivedAt = msTime(1_700_000_003_000)
	mustSaveNode(t, s, n)

	got := loadNodeDirect(t, s, n.ID)
	if !got.ArchivedAt.Equal(n.ArchivedAt) {
		t.Errorf("ArchivedAt = %v, want %v", got.ArchivedAt, n.ArchivedAt)
	}
}

func TestSaveNodesEmptyIsNoop(t *testing.T) {
	s := newTestStore(t)
	if err := s.SaveNodes(t.Context(), nil); err != nil {
		t.Errorf("SaveNodes(nil): %v", err)
	}
}
