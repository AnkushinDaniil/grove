package github

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
)

// baseRefOID and headRefOID are the two commit oids the canned pr-view payload
// targets; the fake contents runner keys its base/head bodies off them.
const (
	baseRefOID = "base456"
	headRefOID = "abc123"
)

// prViewJSON is a canned `gh pr view --json …` payload.
const prViewJSON = `{
  "number": 12540,
  "title": "Add fast sync",
  "author": {"login": "kamilchodola"},
  "url": "https://github.com/NethermindEth/nethermind/pull/12540",
  "state": "OPEN",
  "headRefOid": "abc123",
  "baseRefOid": "base456",
  "baseRefName": "master",
  "reviewDecision": "REVIEW_REQUIRED",
  "body": "This adds fast sync.",
  "statusCheckRollup": [{"__typename":"CheckRun","status":"COMPLETED","conclusion":"SUCCESS"}]
}`

// prFilesJSON is a canned files-endpoint page: a modified file with a two-hunk
// patch, a renamed file, and a binary file with no patch.
const prFilesJSON = `[
  {
    "filename": "src/main.go",
    "status": "modified",
    "additions": 2,
    "deletions": 1,
    "patch": "@@ -1,2 +1,3 @@\n a\n-b\n+B\n+C\n@@ -10,1 +11,1 @@\n-x\n+y"
  },
  {
    "filename": "src/new.go",
    "previous_filename": "src/old.go",
    "status": "renamed",
    "additions": 1,
    "deletions": 0,
    "patch": "@@ -1 +1,2 @@\n keep\n+extra"
  },
  {
    "filename": "assets/logo.png",
    "status": "added",
    "additions": 0,
    "deletions": 0,
    "patch": ""
  }
]`

// threadsJSON is a canned GraphQL reviewThreads payload: one open thread by me
// and one resolved, outdated thread (null line) by someone else.
const threadsJSON = `{
  "data": {"repository": {"pullRequest": {"reviewThreads": {"nodes": [
    {
      "id": "PRRT_1", "isResolved": false, "line": 42, "path": "src/main.go", "diffSide": "RIGHT",
      "comments": {"nodes": [
        {"id": "C1", "author": {"login": "me"}, "body": "nit", "createdAt": "2026-07-22T09:00:00Z", "diffHunk": "@@ -1 +1 @@"}
      ]}
    },
    {
      "id": "PRRT_2", "isResolved": true, "line": null, "path": "src/old.go", "diffSide": "LEFT",
      "comments": {"nodes": [
        {"id": "C2", "author": {"login": "bob"}, "body": "outdated", "createdAt": "2026-07-21T09:00:00Z", "diffHunk": ""}
      ]}
    }
  ]}}}}
}`

// cannedContent base64-encodes a distinguishable body for the fake contents
// endpoint: "<SIDE>:<path>" where SIDE is BASE or HEAD, so tests can assert
// which ref and path each side was read from.
func cannedContent(endpoint string) []byte {
	inner := strings.TrimPrefix(endpoint, "repos/NethermindEth/nethermind/contents/")
	path, ref, _ := strings.Cut(inner, "?ref=")
	side := "HEAD"
	if ref == baseRefOID {
		side = "BASE"
	}
	return []byte(base64.StdEncoding.EncodeToString([]byte(side+":"+path)) + "\n")
}

// prDetailRunner dispatches a fake gh call to the matching canned payload,
// covering every gh invocation PRDetail makes.
func prDetailRunner(t *testing.T) RunnerFunc {
	t.Helper()
	return func(_ context.Context, _ string, args ...string) ([]byte, error) {
		switch {
		case args[0] == "repo" && args[1] == "view":
			return []byte("NethermindEth/nethermind\n"), nil
		case args[0] == "pr" && args[1] == "view":
			return []byte(prViewJSON), nil
		case args[0] == "api" && args[1] == "user":
			return []byte("me\n"), nil
		case args[0] == "api" && args[1] == "graphql":
			return []byte(threadsJSON), nil
		case args[0] == "api" && strings.Contains(args[1], "/contents/"):
			return cannedContent(args[1]), nil
		case args[0] == "api" && strings.Contains(args[1], "/files"):
			return []byte(prFilesJSON), nil
		default:
			t.Fatalf("unexpected gh call: %v", args)
			return nil, nil
		}
	}
}

func TestPRDetailAssembles(t *testing.T) {
	c := New(WithRunner(prDetailRunner(t)))
	pr, err := c.PRDetail(context.Background(), "/repo", 12540)
	if err != nil {
		t.Fatalf("PRDetail() error = %v", err)
	}

	// Metadata.
	if pr.Number != 12540 || pr.Title != "Add fast sync" || pr.Author != "kamilchodola" {
		t.Errorf("metadata = %+v, want number/title/author populated", pr)
	}
	if pr.State != "OPEN" || pr.HeadSHA != "abc123" || pr.BaseRef != "master" {
		t.Errorf("state/head/base = %q/%q/%q", pr.State, pr.HeadSHA, pr.BaseRef)
	}
	if pr.Checks != "passing" {
		t.Errorf("checks = %q, want passing", pr.Checks)
	}
	if pr.ReviewDecision != "REVIEW_REQUIRED" || pr.Body != "This adds fast sync." {
		t.Errorf("review_decision/body = %q/%q", pr.ReviewDecision, pr.Body)
	}

	// Files.
	if len(pr.Files) != 3 {
		t.Fatalf("files = %d, want 3", len(pr.Files))
	}
	main := pr.Files[0]
	if main.Path != "src/main.go" || main.Status != "modified" || main.Binary {
		t.Errorf("main file = %+v, want modified non-binary src/main.go", main)
	}
	if len(main.Hunks) != 2 {
		t.Errorf("main hunks = %d, want 2", len(main.Hunks))
	}
	renamed := pr.Files[1]
	if renamed.OldPath != "src/old.go" || renamed.Status != "renamed" {
		t.Errorf("renamed file = %+v, want old_path src/old.go", renamed)
	}
	binary := pr.Files[2]
	if !binary.Binary || len(binary.Hunks) != 0 {
		t.Errorf("binary file = %+v, want binary=true no hunks", binary)
	}

	// Rich-diff contents: a modified file carries both sides read from the base
	// and head refs; a renamed file reads its base side from the old path; an
	// added binary file omits its contents.
	if main.ContentOmitted != "" || main.OriginalContent != "BASE:src/main.go" || main.ModifiedContent != "HEAD:src/main.go" {
		t.Errorf("main contents = %q/%q omit=%q, want BASE/HEAD of src/main.go", main.OriginalContent, main.ModifiedContent, main.ContentOmitted)
	}
	if renamed.OriginalContent != "BASE:src/old.go" || renamed.ModifiedContent != "HEAD:src/new.go" {
		t.Errorf("renamed contents = %q/%q, want base at old path, head at new path", renamed.OriginalContent, renamed.ModifiedContent)
	}
	if binary.ContentOmitted != "binary" || binary.OriginalContent != "" || binary.ModifiedContent != "" {
		t.Errorf("binary file contents = %q/%q omit=%q, want empty omit=binary", binary.OriginalContent, binary.ModifiedContent, binary.ContentOmitted)
	}

	// Threads.
	if len(pr.Threads) != 2 {
		t.Fatalf("threads = %d, want 2", len(pr.Threads))
	}
	open := pr.Threads[0]
	if open.ID != "PRRT_1" || open.Line != 42 || open.Side != "RIGHT" || open.IsResolved {
		t.Errorf("open thread = %+v, want line 42 RIGHT unresolved", open)
	}
	if open.DiffHunk != "@@ -1 +1 @@" {
		t.Errorf("thread diff_hunk = %q, want it from the first comment", open.DiffHunk)
	}
	if len(open.Comments) != 1 || !open.Comments[0].IsMine {
		t.Errorf("open thread comment = %+v, want is_mine true for login me", open.Comments)
	}
	outdated := pr.Threads[1]
	if outdated.Line != 0 || !outdated.IsResolved || outdated.Comments[0].IsMine {
		t.Errorf("outdated thread = %+v, want line 0 resolved not-mine", outdated)
	}
}

func TestPRDetailRepoNameError(t *testing.T) {
	c := New(WithRunner(func(context.Context, string, ...string) ([]byte, error) {
		return nil, &GHError{Args: []string{"repo", "view"}, ExitCode: 1, Stderr: "no remote"}
	}))
	if _, err := c.PRDetail(context.Background(), "/repo", 1); err == nil {
		t.Fatal("PRDetail() error = nil, want error when repo name fails")
	}
}

func TestPRDetailMetaParseError(t *testing.T) {
	c := New(WithRunner(func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if args[0] == "repo" {
			return []byte("o/r"), nil
		}
		return []byte("not json"), nil // pr view returns garbage
	}))
	if _, err := c.PRDetail(context.Background(), "/repo", 1); err == nil {
		t.Fatal("PRDetail() error = nil, want a metadata parse error")
	}
}

func TestPRDetailFilesError(t *testing.T) {
	c := New(WithRunner(func(_ context.Context, _ string, args ...string) ([]byte, error) {
		switch {
		case args[0] == "repo":
			return []byte("o/r"), nil
		case args[0] == "pr":
			return []byte(prViewJSON), nil
		case args[0] == "api" && strings.Contains(args[1], "/files"):
			return nil, &GHError{Args: args, ExitCode: 1, Stderr: "404"}
		default:
			return []byte("{}"), nil
		}
	}))
	if _, err := c.PRDetail(context.Background(), "/repo", 1); err == nil {
		t.Fatal("PRDetail() error = nil, want a files error")
	}
}

func TestPRDetailFilesBadJSON(t *testing.T) {
	c := New(WithRunner(func(_ context.Context, _ string, args ...string) ([]byte, error) {
		switch {
		case args[0] == "repo":
			return []byte("o/r"), nil
		case args[0] == "pr":
			return []byte(prViewJSON), nil
		case args[0] == "api" && strings.Contains(args[1], "/files"):
			return []byte("{bad"), nil
		default:
			return []byte("{}"), nil
		}
	}))
	if _, err := c.PRDetail(context.Background(), "/repo", 1); err == nil {
		t.Fatal("PRDetail() error = nil, want a files parse error")
	}
}

func TestPRDetailThreadsError(t *testing.T) {
	c := New(WithRunner(func(_ context.Context, _ string, args ...string) ([]byte, error) {
		switch {
		case args[0] == "repo":
			return []byte("o/r"), nil
		case args[0] == "pr":
			return []byte(prViewJSON), nil
		case args[0] == "api" && args[1] == "user":
			return []byte("me"), nil
		case args[0] == "api" && args[1] == "graphql":
			return nil, &GHError{Args: args, ExitCode: 1, Stderr: "graphql error"}
		default:
			return []byte(prFilesJSON), nil
		}
	}))
	if _, err := c.PRDetail(context.Background(), "/repo", 1); err == nil {
		t.Fatal("PRDetail() error = nil, want a threads error")
	}
}

func TestPRThreadsBadRepoName(t *testing.T) {
	// A repo name lacking the owner/repo slash cannot seed the GraphQL query.
	c := New(WithRunner(func(_ context.Context, _ string, args ...string) ([]byte, error) {
		switch {
		case args[0] == "repo":
			return []byte("noslash"), nil
		case args[0] == "pr":
			return []byte(prViewJSON), nil
		case args[0] == "api" && args[1] == "user":
			return []byte("me"), nil
		default:
			return []byte(prFilesJSON), nil
		}
	}))
	if _, err := c.PRDetail(context.Background(), "/repo", 1); err == nil {
		t.Fatal("PRDetail() error = nil, want an owner/repo split error")
	}
}

// omissionFilesJSON lists one file per content-omission outcome, each with a
// non-empty patch so GitHub is not the one flagging them binary.
const omissionFilesJSON = `[
  {"filename": "added.txt", "status": "added", "additions": 1, "patch": "@@ -0,0 +1 @@\n+new"},
  {"filename": "removed.txt", "status": "removed", "deletions": 1, "patch": "@@ -1 +0,0 @@\n-old"},
  {"filename": "big.txt", "status": "modified", "patch": "@@ -1 +1 @@\n-a\n+b"},
  {"filename": "data.bin", "status": "modified", "patch": "@@ -1 +1 @@\n-a\n+b"},
  {"filename": "weird.txt", "status": "modified", "patch": "@@ -1 +1 @@\n-a\n+b"},
  {"filename": "gone.txt", "status": "modified", "patch": "@@ -1 +1 @@\n-a\n+b"}
]`

func TestPRDetailContentOmission(t *testing.T) {
	// b64 encodes a small text body; headFor returns the per-path head side.
	b64 := func(s string) []byte { return []byte(base64.StdEncoding.EncodeToString([]byte(s)) + "\n") }
	runner := func(_ context.Context, _ string, args ...string) ([]byte, error) {
		switch {
		case args[0] == "repo":
			return []byte("o/r"), nil
		case args[0] == "pr":
			return []byte(prViewJSON), nil
		case args[0] == "api" && args[1] == "user":
			return []byte("me"), nil
		case args[0] == "api" && args[1] == "graphql":
			return []byte(`{"data":{"repository":{"pullRequest":{"reviewThreads":{"nodes":[]}}}}}`), nil
		case args[0] == "api" && strings.Contains(args[1], "/contents/"):
			inner := strings.TrimPrefix(args[1], "repos/o/r/contents/")
			path, ref, _ := strings.Cut(inner, "?ref=")
			if ref == baseRefOID {
				return b64("small base"), nil // every base side is small text
			}
			switch path {
			case "big.txt":
				return b64(strings.Repeat("x", MaxContentBytes+1)), nil
			case "data.bin":
				return b64("has\x00nul"), nil
			case "weird.txt":
				return []byte("!!! not base64 !!!\n"), nil
			case "gone.txt":
				return nil, &GHError{Args: args, ExitCode: 1, Stderr: "404"}
			default:
				return b64("HEAD " + path), nil
			}
		case args[0] == "api" && strings.Contains(args[1], "/files"):
			return []byte(omissionFilesJSON), nil
		default:
			t.Fatalf("unexpected gh call: %v", args)
			return nil, nil
		}
	}
	pr, err := New(WithRunner(runner)).PRDetail(context.Background(), "/repo", 1)
	if err != nil {
		t.Fatalf("PRDetail() error = %v", err)
	}
	byPath := make(map[string]PRFile, len(pr.Files))
	for _, f := range pr.Files {
		byPath[f.Path] = f
	}

	if f := byPath["added.txt"]; f.OriginalContent != "" || f.ModifiedContent != "HEAD added.txt" || f.ContentOmitted != "" {
		t.Errorf("added.txt = %q/%q omit=%q, want empty original, head modified", f.OriginalContent, f.ModifiedContent, f.ContentOmitted)
	}
	if f := byPath["removed.txt"]; f.OriginalContent != "small base" || f.ModifiedContent != "" || f.ContentOmitted != "" {
		t.Errorf("removed.txt = %q/%q omit=%q, want base original, empty modified", f.OriginalContent, f.ModifiedContent, f.ContentOmitted)
	}
	for _, path := range []string{"big.txt", "gone.txt"} {
		if f := byPath[path]; f.ContentOmitted != "too_large" || f.OriginalContent != "" || f.ModifiedContent != "" {
			t.Errorf("%s omit = %q (%q/%q), want too_large with empty contents", path, f.ContentOmitted, f.OriginalContent, f.ModifiedContent)
		}
	}
	for _, path := range []string{"data.bin", "weird.txt"} {
		if f := byPath[path]; f.ContentOmitted != "binary" || f.OriginalContent != "" || f.ModifiedContent != "" {
			t.Errorf("%s omit = %q (%q/%q), want binary with empty contents", path, f.ContentOmitted, f.OriginalContent, f.ModifiedContent)
		}
	}
}

func TestPRDetailPaginatedFiles(t *testing.T) {
	// gh --paginate may emit multiple JSON arrays back to back; decodeFilePages
	// must concatenate them.
	pages := `[{"filename":"a","status":"modified","patch":"@@ -1 +1 @@\n+a"}]` +
		`[{"filename":"b","status":"added","patch":"@@ -0,0 +1 @@\n+b"}]`
	c := New(WithRunner(func(_ context.Context, _ string, args ...string) ([]byte, error) {
		switch {
		case args[0] == "repo":
			return []byte("o/r"), nil
		case args[0] == "pr":
			return []byte(prViewJSON), nil
		case args[0] == "api" && args[1] == "user":
			return []byte("me"), nil
		case args[0] == "api" && args[1] == "graphql":
			return []byte(`{"data":{"repository":{"pullRequest":{"reviewThreads":{"nodes":[]}}}}}`), nil
		default:
			return []byte(pages), nil
		}
	}))
	pr, err := c.PRDetail(context.Background(), "/repo", 1)
	if err != nil {
		t.Fatalf("PRDetail() error = %v", err)
	}
	if len(pr.Files) != 2 || pr.Files[0].Path != "a" || pr.Files[1].Path != "b" {
		t.Errorf("files = %+v, want [a b] merged across pages", pr.Files)
	}
}
