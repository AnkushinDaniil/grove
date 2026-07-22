package github

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

// prListJSON is a canned `gh pr list --json …` payload authored from the real
// gh object shape, exercising every field ListPRs projects.
const prListJSON = `[
  {
    "number": 12540,
    "title": "Add fast sync",
    "author": {"login": "kamilchodola", "is_bot": false},
    "isDraft": false,
    "updatedAt": "2026-07-22T09:01:12Z",
    "reviewDecision": "REVIEW_REQUIRED",
    "reviewRequests": [{"login": "svlachakis"}, {}],
    "latestReviews": [{"author": {"login": "octocat"}, "state": "COMMENTED"}],
    "statusCheckRollup": [{"__typename": "CheckRun", "status": "COMPLETED", "conclusion": "SUCCESS"}],
    "additions": 120,
    "deletions": 8,
    "url": "https://github.com/NethermindEth/nethermind/pull/12540",
    "headRefOid": "abc123"
  },
  {
    "number": 12541,
    "title": "Draft work",
    "author": {"login": "svlachakis", "is_bot": false},
    "isDraft": true,
    "updatedAt": "2026-07-22T09:08:03Z",
    "reviewDecision": "REVIEW_REQUIRED",
    "reviewRequests": [],
    "latestReviews": [],
    "statusCheckRollup": [],
    "additions": 0,
    "deletions": 0,
    "url": "https://github.com/NethermindEth/nethermind/pull/12541",
    "headRefOid": "def456"
  }
]`

func TestListPRsParses(t *testing.T) {
	c := New(WithRunner(func(_ context.Context, dir string, args ...string) ([]byte, error) {
		if dir != "/repo" {
			t.Errorf("dir = %q, want /repo", dir)
		}
		if args[0] != "pr" || args[1] != "list" {
			t.Errorf("args = %v, want to start with pr list", args)
		}
		return []byte(prListJSON), nil
	}))

	prs, err := c.ListPRs(context.Background(), "/repo")
	if err != nil {
		t.Fatalf("ListPRs() error = %v", err)
	}
	if len(prs) != 2 {
		t.Fatalf("len(prs) = %d, want 2", len(prs))
	}

	got := prs[0]
	want := PR{
		Number:         12540,
		Title:          "Add fast sync",
		Author:         "kamilchodola",
		URL:            "https://github.com/NethermindEth/nethermind/pull/12540",
		IsDraft:        false,
		UpdatedAt:      "2026-07-22T09:01:12Z",
		ReviewDecision: "REVIEW_REQUIRED",
		Checks:         "passing",
		Additions:      120,
		Deletions:      8,
		HeadRefOid:     "abc123",
		ReviewRequests: []string{"svlachakis"}, // the team entry with no login is dropped
		LatestReviews:  []Review{{Author: "octocat", State: "COMMENTED"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("prs[0] =\n %+v\nwant\n %+v", got, want)
	}

	if prs[1].Checks != "none" {
		t.Errorf("empty rollup checks = %q, want none", prs[1].Checks)
	}
	if !prs[1].IsDraft {
		t.Error("prs[1].IsDraft = false, want true")
	}
}

func TestListPRsFallsBackWithoutChecks(t *testing.T) {
	const noChecksJSON = `[{"number":1,"title":"t","author":{"login":"alice"},"headRefOid":"h1"}]`
	var sawChecksQuery, sawFallbackQuery bool
	c := New(WithRunner(func(_ context.Context, _ string, args ...string) ([]byte, error) {
		fields := args[len(args)-1]
		if strings.Contains(fields, "statusCheckRollup") {
			sawChecksQuery = true
			// Simulate GitHub timing out computing the rollup on a large repo.
			return nil, &GHError{Args: args, ExitCode: 1, Stderr: "HTTP 502: 502 Bad Gateway"}
		}
		sawFallbackQuery = true
		return []byte(noChecksJSON), nil
	}))

	prs, err := c.ListPRs(context.Background(), "/repo")
	if err != nil {
		t.Fatalf("ListPRs() error = %v", err)
	}
	if !sawChecksQuery || !sawFallbackQuery {
		t.Fatalf("query path: checks=%v fallback=%v, want both true", sawChecksQuery, sawFallbackQuery)
	}
	if len(prs) != 1 {
		t.Fatalf("len(prs) = %d, want 1", len(prs))
	}
	if prs[0].Checks != "none" {
		t.Errorf("checks = %q, want none (degraded when rollup unavailable)", prs[0].Checks)
	}
}

func TestListPRsBothQueriesFail(t *testing.T) {
	// A non-timeout failure (e.g. auth) fails both queries; the original error
	// is surfaced rather than masked by the fallback.
	c := New(WithRunner(func(_ context.Context, _ string, args ...string) ([]byte, error) {
		return nil, &GHError{Args: args, ExitCode: 1, Stderr: "gh: not authenticated"}
	}))
	if _, err := c.ListPRs(context.Background(), "/repo"); err == nil {
		t.Fatal("ListPRs() error = nil, want error when both queries fail")
	}
}

func TestListPRsInvalidJSON(t *testing.T) {
	c := New(WithRunner(func(context.Context, string, ...string) ([]byte, error) {
		return []byte("not json"), nil
	}))
	if _, err := c.ListPRs(context.Background(), "/repo"); err == nil {
		t.Fatal("ListPRs() with bad JSON error = nil, want error")
	}
}

func TestListPRsRunnerError(t *testing.T) {
	c := New(WithRunner(func(context.Context, string, ...string) ([]byte, error) {
		return nil, &GHError{Args: []string{"pr", "list"}, ExitCode: 1, Stderr: "gh: not found"}
	}))
	if _, err := c.ListPRs(context.Background(), "/repo"); err == nil {
		t.Fatal("ListPRs() error = nil, want error")
	}
}

func TestSummarizeChecks(t *testing.T) {
	cases := []struct {
		name    string
		entries []checkRollupEntry
		want    string
	}{
		{"empty", nil, "none"},
		{"all success", []checkRollupEntry{
			{Status: "COMPLETED", Conclusion: "SUCCESS"},
			{State: "SUCCESS"},
		}, "passing"},
		{"neutral and skipped pass", []checkRollupEntry{
			{Status: "COMPLETED", Conclusion: "NEUTRAL"},
			{Status: "COMPLETED", Conclusion: "SKIPPED"},
		}, "passing"},
		{"a failure wins over success", []checkRollupEntry{
			{Status: "COMPLETED", Conclusion: "SUCCESS"},
			{Status: "COMPLETED", Conclusion: "FAILURE"},
		}, "failing"},
		{"statuscontext error is failing", []checkRollupEntry{
			{State: "ERROR"},
		}, "failing"},
		{"failure wins over pending", []checkRollupEntry{
			{Status: "IN_PROGRESS"},
			{Status: "COMPLETED", Conclusion: "FAILURE"},
		}, "failing"},
		{"pending when in progress and no failure", []checkRollupEntry{
			{Status: "COMPLETED", Conclusion: "SUCCESS"},
			{Status: "IN_PROGRESS"},
		}, "pending"},
		{"queued is pending", []checkRollupEntry{
			{Status: "QUEUED"},
		}, "pending"},
		{"statuscontext pending", []checkRollupEntry{
			{State: "PENDING"},
		}, "pending"},
		{"timed out is failing", []checkRollupEntry{
			{Status: "COMPLETED", Conclusion: "TIMED_OUT"},
		}, "failing"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := summarizeChecks(tc.entries); got != tc.want {
				t.Errorf("summarizeChecks() = %q, want %q", got, tc.want)
			}
		})
	}
}
