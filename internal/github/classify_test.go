package github

import (
	"testing"
)

// numbers extracts PR numbers from a bucket for compact assertions.
func numbers(prs []PR) []int {
	out := make([]int, 0, len(prs))
	for _, pr := range prs {
		out = append(out, pr.Number)
	}
	return out
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestClassifyBuckets(t *testing.T) {
	const me = "me"
	prs := []PR{
		// 1: someone else's PR, no review from me, not draft → needs_review.
		{Number: 1, Author: "alice", HeadRefOid: "h1"},
		// 2: my review requested even though I already reviewed → needs_review.
		{
			Number: 2, Author: "bob", HeadRefOid: "h2",
			ReviewRequests: []string{"me"},
			LatestReviews:  []Review{{Author: "me", State: "APPROVED"}},
		},
		// 3: I reviewed, head unchanged since last seen → reviewed.
		{
			Number: 3, Author: "carol", HeadRefOid: "h3",
			LatestReviews: []Review{{Author: "me", State: "APPROVED"}},
		},
		// 4: I reviewed, head moved since last seen → re_review.
		{
			Number: 4, Author: "dave", HeadRefOid: "h4-new",
			LatestReviews: []Review{{Author: "me", State: "CHANGES_REQUESTED"}},
		},
		// 5: authored by me → mine.
		{Number: 5, Author: "me", HeadRefOid: "h5"},
		// 6: draft by someone else, no review → excluded from needs_review, no bucket.
		{Number: 6, Author: "erin", HeadRefOid: "h6", IsDraft: true},
		// 7: my own draft → still mine.
		{Number: 7, Author: "me", HeadRefOid: "h7", IsDraft: true},
	}
	seenHeads := map[int]string{
		3: "h3",     // unchanged
		4: "h4-old", // changed vs current h4-new
	}

	b := Classify(prs, me, seenHeads)

	if got := numbers(b.NeedsReview); !equalInts(got, []int{1, 2}) {
		t.Errorf("needs_review = %v, want [1 2]", got)
	}
	if got := numbers(b.ReReview); !equalInts(got, []int{4}) {
		t.Errorf("re_review = %v, want [4]", got)
	}
	if got := numbers(b.Reviewed); !equalInts(got, []int{3}) {
		t.Errorf("reviewed = %v, want [3]", got)
	}
	if got := numbers(b.Mine); !equalInts(got, []int{5, 7}) {
		t.Errorf("mine = %v, want [5 7]", got)
	}
}

func TestClassifyReReviewNeedsPriorSighting(t *testing.T) {
	const me = "me"
	prs := []PR{
		{
			Number: 1, Author: "alice", HeadRefOid: "current",
			LatestReviews: []Review{{Author: "me", State: "APPROVED"}},
		},
	}
	// With no seenHeads entry the commits-since state is unknown: the PR stays
	// in reviewed, not re_review, until it has been seen at least once.
	b := Classify(prs, me, map[int]string{})
	if len(b.ReReview) != 0 {
		t.Errorf("re_review = %v, want empty (never seen before)", numbers(b.ReReview))
	}
	if got := numbers(b.Reviewed); !equalInts(got, []int{1}) {
		t.Errorf("reviewed = %v, want [1]", got)
	}

	// An empty recorded head is likewise treated as unknown, not a change.
	b = Classify(prs, me, map[int]string{1: ""})
	if len(b.ReReview) != 0 {
		t.Errorf("re_review with empty recorded head = %v, want empty", numbers(b.ReReview))
	}
}

func TestClassifyEmptyLogin(t *testing.T) {
	prs := []PR{
		{Number: 1, Author: "alice", HeadRefOid: "h1"},
		{
			Number: 2, Author: "bob", HeadRefOid: "h2",
			LatestReviews: []Review{{Author: "someone", State: "APPROVED"}},
		},
	}
	// An unknown login (gh login failed) leaves everything unclassified rather
	// than dumping every PR into needs_review.
	b := Classify(prs, "", map[int]string{})
	if len(b.NeedsReview)+len(b.ReReview)+len(b.Reviewed)+len(b.Mine) != 0 {
		t.Errorf("empty-login buckets not all empty: %+v", b)
	}
}

func TestClassifyReviewRequestedFromMeWhenDraft(t *testing.T) {
	const me = "me"
	// A draft is never needs_review even if my review is somehow requested.
	prs := []PR{{Number: 1, Author: "alice", IsDraft: true, ReviewRequests: []string{"me"}}}
	b := Classify(prs, me, map[int]string{})
	if len(b.NeedsReview) != 0 {
		t.Errorf("draft in needs_review = %v, want empty", numbers(b.NeedsReview))
	}
}
