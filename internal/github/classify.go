package github

import "slices"

// Buckets holds a repository's open PRs grouped by what they need from the user.
// A PR lands in exactly one bucket; see Classify for the precedence order.
type Buckets struct {
	NeedsReview []PR
	ReReview    []PR
	Reviewed    []PR
	Mine        []PR
}

// Classify sorts prs into buckets for the given user login. seenHeads maps a PR
// number to the head-oid observed on a previous read, driving re-review
// detection; a PR absent from it has an unknown commits-since state.
//
// Precedence (first match wins), per docs/API.md:
//  1. needs_review: open, not draft, not mine, and either my review is requested
//     or I have not reviewed it yet.
//  2. re_review: I have reviewed it, its head changed since it was last seen.
//  3. reviewed: I have reviewed it (kept for revisiting).
//  4. mine: I authored it.
//
// With an empty login (gh login unavailable) no PR is user-classifiable, so all
// buckets come back empty.
func Classify(prs []PR, login string, seenHeads map[int]string) Buckets {
	var b Buckets
	for _, pr := range prs {
		switch {
		case isNeedsReview(pr, login):
			b.NeedsReview = append(b.NeedsReview, pr)
		case isReReview(pr, login, seenHeads):
			b.ReReview = append(b.ReReview, pr)
		case hasMyReview(pr, login):
			b.Reviewed = append(b.Reviewed, pr)
		case login != "" && pr.Author == login:
			b.Mine = append(b.Mine, pr)
		}
	}
	return b
}

// isNeedsReview reports whether pr awaits the user's first (or re-requested)
// review: open and not draft, authored by someone else, and either my review is
// requested or I have not reviewed it.
func isNeedsReview(pr PR, login string) bool {
	if login == "" || pr.IsDraft || pr.Author == login {
		return false
	}
	return slices.Contains(pr.ReviewRequests, login) || !hasMyReview(pr, login)
}

// isReReview reports whether the user reviewed pr and new commits have landed
// since it was last seen (its recorded head differs from the current one).
func isReReview(pr PR, login string, seenHeads map[int]string) bool {
	if !hasMyReview(pr, login) {
		return false
	}
	seen, ok := seenHeads[pr.Number]
	return ok && seen != "" && seen != pr.HeadRefOid
}

// hasMyReview reports whether login has a latest review on pr.
func hasMyReview(pr PR, login string) bool {
	if login == "" {
		return false
	}
	for _, r := range pr.LatestReviews {
		if r.Author == login {
			return true
		}
	}
	return false
}
