package github

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLiveNethermind exercises the real gh CLI against a local checkout of
// NethermindEth/nethermind. It is skipped unless GROVE_LIVE=1 so the default
// `go test` run stays hermetic and offline.
//
//	GROVE_LIVE=1 go test -run TestLiveNethermind -v ./internal/github/
func TestLiveNethermind(t *testing.T) {
	if os.Getenv("GROVE_LIVE") != "1" {
		t.Skip("set GROVE_LIVE=1 to run the live gh test")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	dir := filepath.Join(home, "RiderProjects", "nethermind")

	c := New()
	ctx := t.Context()

	login, err := c.Login(ctx)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	name, err := c.RepoName(ctx, dir)
	if err != nil {
		t.Fatalf("RepoName: %v", err)
	}
	prs, err := c.ListPRs(ctx, dir)
	if err != nil {
		t.Fatalf("ListPRs: %v", err)
	}
	b := Classify(prs, login, map[int]string{})

	t.Logf("login=%s repo=%s open_prs=%d", login, name, len(prs))
	t.Logf("buckets: needs_review=%d re_review=%d reviewed=%d mine=%d",
		len(b.NeedsReview), len(b.ReReview), len(b.Reviewed), len(b.Mine))
}

// TestLivePRDetail exercises the real gh CLI assembling one PR's full review
// view (metadata + diff + threads) against a local nethermind checkout. Skipped
// unless GROVE_LIVE=1.
//
//	GROVE_LIVE=1 go test -run TestLivePRDetail -v ./internal/github/
func TestLivePRDetail(t *testing.T) {
	if os.Getenv("GROVE_LIVE") != "1" {
		t.Skip("set GROVE_LIVE=1 to run the live gh test")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	dir := filepath.Join(home, "RiderProjects", "nethermind")

	const prNumber = 12540
	pr, err := New().PRDetail(t.Context(), dir, prNumber)
	if err != nil {
		t.Fatalf("PRDetail(#%d): %v", prNumber, err)
	}

	firstHunks := 0
	firstPath := ""
	if len(pr.Files) > 0 {
		firstPath = pr.Files[0].Path
		firstHunks = len(pr.Files[0].Hunks)
	}
	t.Logf("pr #%d %q state=%s checks=%s base=%s head=%s",
		pr.Number, pr.Title, pr.State, pr.Checks, pr.BaseRef, pr.HeadSHA)
	t.Logf("files=%d first_file=%q first_file_hunks=%d threads=%d",
		len(pr.Files), firstPath, firstHunks, len(pr.Threads))
}
