package store

import (
	"reflect"
	"testing"
)

// testDraft returns a fully populated draft fixture for (dir, pr).
func testDraft(id, dir string, pr int) ReviewDraft {
	return ReviewDraft{
		ID:        id,
		Dir:       dir,
		PR:        pr,
		Path:      "src/main.go",
		Line:      42,
		Side:      "RIGHT",
		Body:      "consider renaming this",
		CreatedAt: msTime(1_700_000_000_000),
	}
}

func TestReviewDraftRoundTrip(t *testing.T) {
	s := newTestStore(t)
	want := testDraft("d1", "/repo", 12540)
	if err := s.SaveReviewDraft(t.Context(), want); err != nil {
		t.Fatalf("SaveReviewDraft: %v", err)
	}

	got, err := s.ListReviewDrafts(t.Context(), "/repo", 12540)
	if err != nil {
		t.Fatalf("ListReviewDrafts: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(drafts) = %d, want 1", len(got))
	}
	if !reflect.DeepEqual(got[0], want) {
		t.Errorf("draft =\n %+v\nwant\n %+v", got[0], want)
	}
}

func TestReviewDraftsListedByDirAndPR(t *testing.T) {
	s := newTestStore(t)
	// Two drafts on the target workspace, plus decoys on other (dir, pr) pairs.
	drafts := []ReviewDraft{
		testDraft("a", "/repo", 1),
		testDraft("b", "/repo", 1),
		testDraft("c", "/repo", 2),  // same dir, different pr
		testDraft("d", "/other", 1), // different dir, same pr
	}
	for _, d := range drafts {
		if err := s.SaveReviewDraft(t.Context(), d); err != nil {
			t.Fatalf("SaveReviewDraft(%s): %v", d.ID, err)
		}
	}

	got, err := s.ListReviewDrafts(t.Context(), "/repo", 1)
	if err != nil {
		t.Fatalf("ListReviewDrafts: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(drafts) = %d, want 2", len(got))
	}
	if got[0].ID != "a" || got[1].ID != "b" {
		t.Errorf("draft ids = [%s %s], want [a b] (created_at, id order)", got[0].ID, got[1].ID)
	}
}

func TestReviewDraftDelete(t *testing.T) {
	s := newTestStore(t)
	if err := s.SaveReviewDraft(t.Context(), testDraft("d1", "/repo", 1)); err != nil {
		t.Fatalf("SaveReviewDraft: %v", err)
	}
	if err := s.DeleteReviewDraft(t.Context(), "d1"); err != nil {
		t.Fatalf("DeleteReviewDraft: %v", err)
	}
	got, err := s.ListReviewDrafts(t.Context(), "/repo", 1)
	if err != nil {
		t.Fatalf("ListReviewDrafts: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len(drafts) = %d after delete, want 0", len(got))
	}

	// Deleting an unknown id is not an error.
	if err := s.DeleteReviewDraft(t.Context(), "missing"); err != nil {
		t.Errorf("DeleteReviewDraft(missing) = %v, want nil (idempotent)", err)
	}
}

func TestReviewDraftsByIDs(t *testing.T) {
	s := newTestStore(t)
	for _, id := range []string{"a", "b", "c"} {
		if err := s.SaveReviewDraft(t.Context(), testDraft(id, "/repo", 1)); err != nil {
			t.Fatalf("SaveReviewDraft(%s): %v", id, err)
		}
	}

	got, err := s.ListReviewDraftsByIDs(t.Context(), []string{"a", "c", "missing"})
	if err != nil {
		t.Fatalf("ListReviewDraftsByIDs: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(drafts) = %d, want 2 (missing id skipped)", len(got))
	}
	if got[0].ID != "a" || got[1].ID != "c" {
		t.Errorf("draft ids = [%s %s], want [a c]", got[0].ID, got[1].ID)
	}

	// Empty id set never queries and returns nothing.
	empty, err := s.ListReviewDraftsByIDs(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListReviewDraftsByIDs(nil): %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("len(drafts) = %d for empty ids, want 0", len(empty))
	}
}
