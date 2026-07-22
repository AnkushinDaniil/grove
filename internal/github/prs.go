package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// prListFields is the gh --json field set requested for each open PR. It carries
// everything Classify and the wire PR shape need in a single gh call.
const prListFields = "number,title,author,isDraft,updatedAt,reviewDecision," +
	"reviewRequests,latestReviews,statusCheckRollup,additions,deletions,url,headRefOid"

// prListFieldsNoChecks is prListFields without statusCheckRollup. That field is
// expensive for GitHub to compute server-side: on very large/active repos the
// GraphQL query for up to 100 PRs' rollups times out (HTTP 502/504). Dropping it
// keeps the Review Radar usable there — PRs still list and classify — at the
// cost of an unknown checks summary (reported as "none").
const prListFieldsNoChecks = "number,title,author,isDraft,updatedAt,reviewDecision," +
	"reviewRequests,latestReviews,additions,deletions,url,headRefOid"

// PR is grove's internal view of one open pull request, parsed from gh's JSON.
// The wire shape (docs/API.md) is a projection of this; ReviewRequests,
// LatestReviews and HeadRefOid drive classification and are not exposed there.
type PR struct {
	Number         int
	Title          string
	Author         string // author.login
	URL            string
	IsDraft        bool
	UpdatedAt      string // RFC 3339, as returned by gh
	ReviewDecision string // REVIEW_REQUIRED|APPROVED|CHANGES_REQUESTED|""
	Checks         string // passing|failing|pending|none
	Additions      int
	Deletions      int
	HeadRefOid     string
	ReviewRequests []string // requested reviewer logins (teams without a login are dropped)
	LatestReviews  []Review // latest review state per reviewer
}

// Review is one reviewer's latest review on a PR.
type Review struct {
	Author string // author.login
	State  string // APPROVED|CHANGES_REQUESTED|COMMENTED|DISMISSED|PENDING
}

// rawPR mirrors the gh `pr list --json` object shape before projection to PR.
type rawPR struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Author struct {
		Login string `json:"login"`
	} `json:"author"`
	IsDraft        bool   `json:"isDraft"`
	UpdatedAt      string `json:"updatedAt"`
	ReviewDecision string `json:"reviewDecision"`
	ReviewRequests []struct {
		Login string `json:"login"`
	} `json:"reviewRequests"`
	LatestReviews []struct {
		Author struct {
			Login string `json:"login"`
		} `json:"author"`
		State string `json:"state"`
	} `json:"latestReviews"`
	StatusCheckRollup []checkRollupEntry `json:"statusCheckRollup"`
	Additions         int                `json:"additions"`
	Deletions         int                `json:"deletions"`
	URL               string             `json:"url"`
	HeadRefOid        string             `json:"headRefOid"`
}

// checkRollupEntry is one entry of statusCheckRollup. gh returns two shapes:
// CheckRun (status + conclusion) and StatusContext (state); we read whichever
// fields are present.
type checkRollupEntry struct {
	Status     string `json:"status"`     // CheckRun: QUEUED|IN_PROGRESS|COMPLETED
	Conclusion string `json:"conclusion"` // CheckRun terminal outcome
	State      string `json:"state"`      // StatusContext: SUCCESS|FAILURE|PENDING|ERROR|EXPECTED
}

// ListPRs returns the open pull requests of the repository rooted at dir. When
// the full query fails at the gh level (typically statusCheckRollup timing out
// on a large repo), it retries once without that field so classification still
// works; the checks summary then degrades to "none". A non-gh failure (e.g. a
// parse error) is returned as-is, and if the fallback also fails the original
// error is surfaced.
func (c *Client) ListPRs(ctx context.Context, dir string) ([]PR, error) {
	prs, err := c.listPRs(ctx, dir, prListFields)
	if err == nil {
		return prs, nil
	}
	var ghErr *GHError
	if !errors.As(err, &ghErr) {
		return nil, err
	}
	fallback, fallbackErr := c.listPRs(ctx, dir, prListFieldsNoChecks)
	if fallbackErr != nil {
		return nil, err
	}
	return fallback, nil
}

// listPRs runs `gh pr list` for the given --json field set and parses the result.
func (c *Client) listPRs(ctx context.Context, dir, fields string) ([]PR, error) {
	out, err := c.call(ctx, dir, "pr", "list", "--state", "open", "--limit", "100", "--json", fields)
	if err != nil {
		return nil, fmt.Errorf("gh pr list: %w", err)
	}
	var raw []rawPR
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parse gh pr list output: %w", err)
	}
	prs := make([]PR, 0, len(raw))
	for _, r := range raw {
		prs = append(prs, r.toPR())
	}
	return prs, nil
}

// toPR projects a raw gh object into a PR, dropping requested-reviewer teams
// (which carry no login) and summarizing the checks rollup.
func (r rawPR) toPR() PR {
	reqs := make([]string, 0, len(r.ReviewRequests))
	for _, rr := range r.ReviewRequests {
		if rr.Login != "" {
			reqs = append(reqs, rr.Login)
		}
	}
	reviews := make([]Review, 0, len(r.LatestReviews))
	for _, lr := range r.LatestReviews {
		reviews = append(reviews, Review{Author: lr.Author.Login, State: lr.State})
	}
	return PR{
		Number:         r.Number,
		Title:          r.Title,
		Author:         r.Author.Login,
		URL:            r.URL,
		IsDraft:        r.IsDraft,
		UpdatedAt:      r.UpdatedAt,
		ReviewDecision: r.ReviewDecision,
		Checks:         summarizeChecks(r.StatusCheckRollup),
		Additions:      r.Additions,
		Deletions:      r.Deletions,
		HeadRefOid:     r.HeadRefOid,
		ReviewRequests: reqs,
		LatestReviews:  reviews,
	}
}

// summarizeChecks folds a status-check rollup into one summary token: any
// failure wins, then any pending, then passing when non-empty, else none.
func summarizeChecks(entries []checkRollupEntry) string {
	if len(entries) == 0 {
		return "none"
	}
	anyPending := false
	for _, e := range entries {
		switch rollupState(e) {
		case "failing":
			return "failing"
		case "pending":
			anyPending = true
		}
	}
	if anyPending {
		return "pending"
	}
	return "passing"
}

// rollupState reduces one rollup entry to failing|pending|passing. A
// StatusContext exposes State directly; a CheckRun reports Conclusion once
// COMPLETED and is otherwise still in flight. Terminal non-success outcomes
// (timeouts, startup failures, required actions) are treated as failing so a
// problem is never rendered green.
func rollupState(e checkRollupEntry) string {
	token := e.State
	if token == "" {
		if e.Status == "COMPLETED" {
			token = e.Conclusion
		} else {
			token = e.Status
		}
	}
	switch token {
	case "FAILURE", "ERROR", "TIMED_OUT", "STARTUP_FAILURE", "ACTION_REQUIRED":
		return "failing"
	case "PENDING", "IN_PROGRESS", "QUEUED", "EXPECTED", "":
		return "pending"
	default: // SUCCESS, NEUTRAL, SKIPPED, CANCELLED
		return "passing"
	}
}
