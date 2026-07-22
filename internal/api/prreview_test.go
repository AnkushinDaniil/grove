package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AnkushinDaniil/grove/internal/github"
	"github.com/AnkushinDaniil/grove/internal/store"
	"github.com/AnkushinDaniil/grove/internal/tree"
)

// fakePRGH implements both GitHubClient and prReviewGitHub, recording the inputs
// submit and reply are called with so tests can assert on them.
type fakePRGH struct {
	detail    github.PRReview
	detailErr error
	submitURL string
	submitErr error
	submitIn  *github.SubmitInput
	replyErr  error
	replyIn   *github.ReplyInput
}

func (f *fakePRGH) Login(context.Context) (string, error)                { return "me", nil }
func (f *fakePRGH) ListPRs(context.Context, string) ([]github.PR, error) { return nil, nil }
func (f *fakePRGH) RepoName(context.Context, string) (string, error)     { return "octo/repo", nil }

func (f *fakePRGH) PRDetail(context.Context, string, int) (github.PRReview, error) {
	return f.detail, f.detailErr
}

func (f *fakePRGH) SubmitReview(_ context.Context, _ string, in github.SubmitInput) (string, error) {
	f.submitIn = &in
	return f.submitURL, f.submitErr
}

func (f *fakePRGH) ReplyToThread(_ context.Context, _ string, in github.ReplyInput) error {
	f.replyIn = &in
	return f.replyErr
}

// prHarness wires the PR-review handlers over a real store and tree with an
// injected fake gh, keeping the Handlers pointer so tests can override aiDrafter.
type prHarness struct {
	t     *testing.T
	store *store.Store
	gh    *fakePRGH
	h     *Handlers
	ts    *httptest.Server
}

func newPRHarness(t *testing.T, gh *fakePRGH) *prHarness {
	t.Helper()
	st, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "grove.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	tr := tree.New(st)
	if _, err := tr.Bootstrap(t.Context(), "Workspace"); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	h := New(Config{Tree: tr, Store: st, Auth: NewAuth(testToken), GitHub: gh})
	ts := httptest.NewServer(h.Routes())
	t.Cleanup(ts.Close)

	return &prHarness{t: t, store: st, gh: gh, h: h, ts: ts}
}

func (h *prHarness) do(method, path string, body any) response {
	h.t.Helper()
	return (&harness{t: h.t, ts: h.ts}).do(method, path, body)
}

func (h *prHarness) decode(resp response, wantStatus int, v any) {
	h.t.Helper()
	if resp.status != wantStatus {
		h.t.Fatalf("status = %d, want %d (body: %s)", resp.status, wantStatus, resp.body)
	}
	if v != nil {
		if err := json.Unmarshal(resp.body, v); err != nil {
			h.t.Fatalf("decode body %q: %v", resp.body, err)
		}
	}
}

func TestPRReviewDetail(t *testing.T) {
	gh := &fakePRGH{detail: github.PRReview{
		Number: 12540, Title: "Add sync", Author: "alice", State: "OPEN",
		HeadSHA: "abc", BaseRef: "master", Checks: "passing",
		Files: []github.PRFile{{
			Path: "src/main.go", Status: "modified", Additions: 1,
			Hunks: []github.Hunk{{Header: "@@ -1 +1 @@", Lines: []github.DiffLine{
				{Op: "+", NewLine: 1, Text: "new"},
			}}},
		}},
		Threads: []github.Thread{{
			ID: "PRRT_1", Path: "src/main.go", Line: 42, Side: "RIGHT",
			Comments: []github.ThreadComment{{ID: "C1", Author: "me", Body: "nit", IsMine: true}},
		}},
	}}
	h := newPRHarness(t, gh)
	dir := newGitRepo(t)

	var got prReviewDTO
	h.decode(h.do(http.MethodGet, "/api/v1/reviews/pr?dir="+dir+"&pr=12540", nil), http.StatusOK, &got)
	if got.Number != 12540 || got.HeadSHA != "abc" || got.BaseRef != "master" {
		t.Errorf("detail = %+v, want number/head/base populated", got)
	}
	if len(got.Files) != 1 || got.Files[0].Path != "src/main.go" || len(got.Files[0].Hunks) != 1 {
		t.Errorf("files = %+v, want one file with one hunk", got.Files)
	}
	if got.Files[0].Hunks[0].Lines[0].Op != "+" {
		t.Errorf("line op = %q, want +", got.Files[0].Hunks[0].Lines[0].Op)
	}
	if len(got.Threads) != 1 || !got.Threads[0].Comments[0].IsMine {
		t.Errorf("threads = %+v, want one thread with is_mine comment", got.Threads)
	}
}

func TestPRReviewDetailBadQuery(t *testing.T) {
	h := newPRHarness(t, &fakePRGH{})
	// Non-absolute dir → 400.
	h.decode(h.do(http.MethodGet, "/api/v1/reviews/pr?dir=rel&pr=1", nil), http.StatusBadRequest, nil)
	// Missing/invalid pr → 400.
	h.decode(h.do(http.MethodGet, "/api/v1/reviews/pr?dir=/abs&pr=0", nil), http.StatusBadRequest, nil)
}

func TestPRReviewDetailGHError(t *testing.T) {
	gh := &fakePRGH{detailErr: &github.GHError{Args: []string{"pr", "view"}, ExitCode: 1, Stderr: "boom"}}
	h := newPRHarness(t, gh)
	dir := newGitRepo(t)
	// An upstream gh failure surfaces as 502.
	h.decode(h.do(http.MethodGet, "/api/v1/reviews/pr?dir="+dir+"&pr=1", nil), http.StatusBadGateway, nil)
}

func TestDraftCRUD(t *testing.T) {
	h := newPRHarness(t, &fakePRGH{})
	dir := newGitRepo(t)

	// Create.
	var created draftCommentDTO
	h.decode(h.do(http.MethodPost, "/api/v1/reviews/pr/drafts", map[string]any{
		"dir": dir, "pr": 7, "path": "src/main.go", "line": 42, "side": "RIGHT", "body": "consider this",
	}), http.StatusCreated, &created)
	if created.ID == "" || created.Side != "RIGHT" || created.Line != 42 {
		t.Fatalf("created draft = %+v, want id/side/line set", created)
	}

	// List returns it.
	var list struct {
		Drafts []draftCommentDTO `json:"drafts"`
	}
	h.decode(h.do(http.MethodGet, "/api/v1/reviews/pr/drafts?dir="+dir+"&pr=7", nil), http.StatusOK, &list)
	if len(list.Drafts) != 1 || list.Drafts[0].ID != created.ID {
		t.Fatalf("list = %+v, want the created draft", list.Drafts)
	}

	// Delete, then the list is empty.
	h.decode(h.do(http.MethodDelete, "/api/v1/reviews/pr/drafts/"+created.ID, nil), http.StatusNoContent, nil)
	h.decode(h.do(http.MethodGet, "/api/v1/reviews/pr/drafts?dir="+dir+"&pr=7", nil), http.StatusOK, &list)
	if len(list.Drafts) != 0 {
		t.Errorf("list after delete = %+v, want empty", list.Drafts)
	}
}

func TestDraftDefaultsSide(t *testing.T) {
	h := newPRHarness(t, &fakePRGH{})
	dir := newGitRepo(t)
	var created draftCommentDTO
	h.decode(h.do(http.MethodPost, "/api/v1/reviews/pr/drafts", map[string]any{
		"dir": dir, "pr": 7, "path": "x", "line": 1, "body": "b",
	}), http.StatusCreated, &created)
	if created.Side != "RIGHT" {
		t.Errorf("side = %q, want RIGHT default", created.Side)
	}
}

func TestDraftValidation(t *testing.T) {
	h := newPRHarness(t, &fakePRGH{})
	dir := newGitRepo(t)
	nonGit := t.TempDir()

	cases := []struct {
		name string
		body map[string]any
	}{
		{"non-git dir", map[string]any{"dir": nonGit, "pr": 1, "path": "x", "body": "b"}},
		{"relative dir", map[string]any{"dir": "rel", "pr": 1, "path": "x", "body": "b"}},
		{"empty body", map[string]any{"dir": dir, "pr": 1, "path": "x", "body": ""}},
		{"empty path", map[string]any{"dir": dir, "pr": 1, "path": "", "body": "b"}},
		{"bad side", map[string]any{"dir": dir, "pr": 1, "path": "x", "body": "b", "side": "MIDDLE"}},
		{"bad pr", map[string]any{"dir": dir, "pr": 0, "path": "x", "body": "b"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h.decode(h.do(http.MethodPost, "/api/v1/reviews/pr/drafts", tc.body), http.StatusBadRequest, nil)
		})
	}
}

func TestAIDraftPromptAndText(t *testing.T) {
	gh := &fakePRGH{detail: github.PRReview{
		Number: 7, Title: "T",
		Files: []github.PRFile{{Path: "a.go", Hunks: []github.Hunk{{
			Header: "@@ -1 +1 @@", Lines: []github.DiffLine{{Op: "+", NewLine: 1, Text: "func added() {}"}},
		}}}},
	}}
	h := newPRHarness(t, gh)
	dir := t.TempDir()

	var gotPrompt string
	h.h.aiDrafter = func(_ context.Context, _ string, prompt string) (string, error) {
		gotPrompt = prompt
		return "  suggested text  ", nil
	}

	var resp struct {
		Text string `json:"text"`
	}
	h.decode(h.do(http.MethodPost, "/api/v1/reviews/pr/ai-draft", map[string]any{
		"dir": dir, "pr": 7, "kind": "comment", "path": "a.go", "instruction": "be terse",
	}), http.StatusOK, &resp)

	if resp.Text != "suggested text" {
		t.Errorf("text = %q, want trimmed drafter output", resp.Text)
	}
	if !strings.Contains(gotPrompt, "func added() {}") {
		t.Errorf("prompt missing the diff:\n%s", gotPrompt)
	}
	if !strings.Contains(gotPrompt, "be terse") {
		t.Errorf("prompt missing the instruction:\n%s", gotPrompt)
	}
}

func TestAIDraftValidation(t *testing.T) {
	h := newPRHarness(t, &fakePRGH{})
	// Bad kind → 400.
	h.decode(h.do(http.MethodPost, "/api/v1/reviews/pr/ai-draft", map[string]any{
		"dir": "/abs", "pr": 1, "kind": "essay",
	}), http.StatusBadRequest, nil)
}

func TestAIDraftDrafterFailure(t *testing.T) {
	gh := &fakePRGH{detail: github.PRReview{Number: 1}}
	h := newPRHarness(t, gh)
	dir := t.TempDir()
	h.h.aiDrafter = func(context.Context, string, string) (string, error) {
		return "", &github.GHError{Args: []string{"claude"}, ExitCode: 1, Stderr: "claude failed"}
	}
	// A drafting failure surfaces as 502.
	h.decode(h.do(http.MethodPost, "/api/v1/reviews/pr/ai-draft", map[string]any{
		"dir": dir, "pr": 1, "kind": "comment", "path": "a.go",
	}), http.StatusBadGateway, nil)
}

func TestSubmitReviewClearsDrafts(t *testing.T) {
	gh := &fakePRGH{
		submitURL: "https://github.com/octo/repo/pull/7#pullrequestreview-1",
		detail: github.PRReview{Files: []github.PRFile{
			{Path: "a.go"}, {Path: "b.go"},
		}},
	}
	h := newPRHarness(t, gh)
	dir := newGitRepo(t)

	// Seed two anchored drafts.
	seedDraft(t, h.store, "d1", dir, "a.go")
	seedDraft(t, h.store, "d2", dir, "b.go")

	var resp struct {
		URL string `json:"url"`
	}
	h.decode(h.do(http.MethodPost, "/api/v1/reviews/pr/submit", map[string]any{
		"dir": dir, "pr": 7, "event": "COMMENT", "body": "overall", "draft_ids": []string{"d1", "d2"},
	}), http.StatusOK, &resp)

	if resp.URL != gh.submitURL {
		t.Errorf("url = %q, want the review url", resp.URL)
	}
	if gh.submitIn == nil || gh.submitIn.Event != "COMMENT" || len(gh.submitIn.Comments) != 2 {
		t.Fatalf("submit input = %+v, want COMMENT with 2 comments", gh.submitIn)
	}
	if gh.submitIn.Comments[0].Path != "a.go" || gh.submitIn.Comments[1].Path != "b.go" {
		t.Errorf("comment paths = %+v, want [a.go b.go]", gh.submitIn.Comments)
	}
	// Drafts are cleared after a successful submit.
	remaining, err := h.store.ListReviewDrafts(t.Context(), dir, 7)
	if err != nil {
		t.Fatalf("ListReviewDrafts: %v", err)
	}
	if len(remaining) != 0 {
		t.Errorf("remaining drafts = %d, want 0 after submit", len(remaining))
	}
}

func TestSubmitRejectsUnanchorableDrafts(t *testing.T) {
	gh := &fakePRGH{detail: github.PRReview{Files: []github.PRFile{{Path: "a.go"}}}}
	h := newPRHarness(t, gh)
	dir := newGitRepo(t)
	seedDraft(t, h.store, "ghost", dir, "not-in-pr.go")

	// The draft's path is not a changed file → 400, nothing submitted.
	resp := h.do(http.MethodPost, "/api/v1/reviews/pr/submit", map[string]any{
		"dir": dir, "pr": 7, "event": "COMMENT", "body": "x", "draft_ids": []string{"ghost"},
	})
	h.decode(resp, http.StatusBadRequest, nil)
	if !strings.Contains(string(resp.body), "ghost") {
		t.Errorf("error %q, want it to name the unanchorable draft", resp.body)
	}
	if gh.submitIn != nil {
		t.Error("submit was called despite an unanchorable draft")
	}
	// The draft is retained for the user to fix.
	remaining, _ := h.store.ListReviewDrafts(t.Context(), dir, 7)
	if len(remaining) != 1 {
		t.Errorf("remaining drafts = %d, want 1 (retained)", len(remaining))
	}
}

func TestSubmitValidatesEvent(t *testing.T) {
	h := newPRHarness(t, &fakePRGH{})
	dir := newGitRepo(t)
	h.decode(h.do(http.MethodPost, "/api/v1/reviews/pr/submit", map[string]any{
		"dir": dir, "pr": 7, "event": "LGTM", "body": "x",
	}), http.StatusBadRequest, nil)
}

func TestSubmitApproveWithoutDrafts(t *testing.T) {
	gh := &fakePRGH{submitURL: "https://example/1"}
	h := newPRHarness(t, gh)
	dir := newGitRepo(t)
	// An approve with no drafts needs no anchor check and no PRDetail call.
	var resp struct {
		URL string `json:"url"`
	}
	h.decode(h.do(http.MethodPost, "/api/v1/reviews/pr/submit", map[string]any{
		"dir": dir, "pr": 7, "event": "APPROVE", "body": "lgtm", "draft_ids": []string{},
	}), http.StatusOK, &resp)
	if gh.submitIn == nil || len(gh.submitIn.Comments) != 0 {
		t.Errorf("submit input = %+v, want an approve with no comments", gh.submitIn)
	}
}

func TestReplyToThread(t *testing.T) {
	gh := &fakePRGH{}
	h := newPRHarness(t, gh)
	dir := newGitRepo(t)
	h.decode(h.do(http.MethodPost, "/api/v1/reviews/pr/reply", map[string]any{
		"dir": dir, "pr": 7, "thread_id": "PRRT_9", "body": "thanks", "resolve": true,
	}), http.StatusNoContent, nil)

	if gh.replyIn == nil {
		t.Fatal("reply was not called")
	}
	if gh.replyIn.ThreadID != "PRRT_9" || gh.replyIn.Body != "thanks" || !gh.replyIn.Resolve {
		t.Errorf("reply input = %+v, want thread PRRT_9 body thanks resolve true", gh.replyIn)
	}
}

func TestReplyValidation(t *testing.T) {
	h := newPRHarness(t, &fakePRGH{})
	dir := newGitRepo(t)
	// Missing thread_id → 400.
	h.decode(h.do(http.MethodPost, "/api/v1/reviews/pr/reply", map[string]any{
		"dir": dir, "pr": 7, "body": "x",
	}), http.StatusBadRequest, nil)
	// Empty body → 400.
	h.decode(h.do(http.MethodPost, "/api/v1/reviews/pr/reply", map[string]any{
		"dir": dir, "pr": 7, "thread_id": "T", "body": "  ",
	}), http.StatusBadRequest, nil)
}

func TestBuildAIDraftReplyPrompt(t *testing.T) {
	pr := github.PRReview{
		Number: 7, Title: "T",
		Threads: []github.Thread{{
			ID: "PRRT_1", Path: "a.go", Line: 5, DiffHunk: "@@ -5 +5 @@",
			Comments: []github.ThreadComment{{Author: "bob", Body: "why this change?"}},
		}},
	}
	prompt := buildAIDraftPrompt(aiDraftRequest{
		Kind: "reply", Path: "a.go", Line: 5, ThreadID: "PRRT_1", Instruction: "stay polite",
	}, pr)
	if !strings.Contains(prompt, "why this change?") {
		t.Errorf("reply prompt missing thread context:\n%s", prompt)
	}
	if !strings.Contains(prompt, "@@ -5 +5 @@") {
		t.Errorf("reply prompt missing diff hunk:\n%s", prompt)
	}
	if !strings.Contains(prompt, "stay polite") {
		t.Errorf("reply prompt missing instruction:\n%s", prompt)
	}
}

func TestAIDraftPRDetailError(t *testing.T) {
	gh := &fakePRGH{detailErr: &github.GHError{Args: []string{"pr", "view"}, ExitCode: 1, Stderr: "boom"}}
	h := newPRHarness(t, gh)
	dir := t.TempDir()
	// PRDetail failure while assembling context surfaces as 502.
	h.decode(h.do(http.MethodPost, "/api/v1/reviews/pr/ai-draft", map[string]any{
		"dir": dir, "pr": 1, "kind": "comment", "path": "a.go",
	}), http.StatusBadGateway, nil)
}

func TestListDraftsBadQuery(t *testing.T) {
	h := newPRHarness(t, &fakePRGH{})
	h.decode(h.do(http.MethodGet, "/api/v1/reviews/pr/drafts?dir=rel&pr=1", nil), http.StatusBadRequest, nil)
}

func TestSubmitPRDetailError(t *testing.T) {
	gh := &fakePRGH{detailErr: &github.GHError{Args: []string{"pr", "view"}, ExitCode: 1, Stderr: "boom"}}
	h := newPRHarness(t, gh)
	dir := newGitRepo(t)
	seedDraft(t, h.store, "d1", dir, "a.go")
	// The anchor-validation PRDetail call fails → 502.
	h.decode(h.do(http.MethodPost, "/api/v1/reviews/pr/submit", map[string]any{
		"dir": dir, "pr": 7, "event": "COMMENT", "body": "x", "draft_ids": []string{"d1"},
	}), http.StatusBadGateway, nil)
}

func TestSubmitReviewGHError(t *testing.T) {
	gh := &fakePRGH{
		detail:    github.PRReview{Files: []github.PRFile{{Path: "a.go"}}},
		submitErr: &github.GHError{Args: []string{"api"}, ExitCode: 1, Stderr: "422"},
	}
	h := newPRHarness(t, gh)
	dir := newGitRepo(t)
	seedDraft(t, h.store, "d1", dir, "a.go")
	h.decode(h.do(http.MethodPost, "/api/v1/reviews/pr/submit", map[string]any{
		"dir": dir, "pr": 7, "event": "COMMENT", "body": "x", "draft_ids": []string{"d1"},
	}), http.StatusBadGateway, nil)
	// A failed submit must not clear the drafts.
	remaining, _ := h.store.ListReviewDrafts(t.Context(), dir, 7)
	if len(remaining) != 1 {
		t.Errorf("remaining drafts = %d, want 1 (submit failed)", len(remaining))
	}
}

func TestReplyGHError(t *testing.T) {
	gh := &fakePRGH{replyErr: &github.GHError{Args: []string{"api", "graphql"}, ExitCode: 1, Stderr: "boom"}}
	h := newPRHarness(t, gh)
	dir := newGitRepo(t)
	h.decode(h.do(http.MethodPost, "/api/v1/reviews/pr/reply", map[string]any{
		"dir": dir, "pr": 7, "thread_id": "T", "body": "hi",
	}), http.StatusBadGateway, nil)
}

func TestScrubClaudePATH(t *testing.T) {
	env := []string{
		"HOME=/home/me",
		"PATH=/usr/bin:/home/me/cmux-cli-shims/bin:/opt/cmux.app/shims:/usr/local/bin",
	}
	got := scrubClaudePATH(env)

	var path string
	var sawHome bool
	for _, e := range got {
		if e == "HOME=/home/me" {
			sawHome = true
		}
		if strings.HasPrefix(e, "PATH=") {
			path = strings.TrimPrefix(e, "PATH=")
		}
	}
	if !sawHome {
		t.Error("HOME was dropped, want non-PATH vars preserved")
	}
	if strings.Contains(path, "cmux") {
		t.Errorf("PATH still contains shim dirs: %q", path)
	}
	if !strings.Contains(path, "/usr/bin") || !strings.Contains(path, "/usr/local/bin") {
		t.Errorf("PATH dropped real dirs: %q", path)
	}
}

// fakeClaudeOnPath writes an executable `claude` script into a temp dir and
// prepends it to PATH so defaultAIDrafter resolves it instead of the real CLI.
func fakeClaudeOnPath(t *testing.T, script string) {
	t.Helper()
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake claude: %v", err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestDefaultAIDrafterSuccess(t *testing.T) {
	fakeClaudeOnPath(t, "#!/bin/sh\nprintf '  drafted comment  '\n")
	got, err := defaultAIDrafter(t.Context(), t.TempDir(), "a prompt")
	if err != nil {
		t.Fatalf("defaultAIDrafter() error = %v", err)
	}
	if got != "drafted comment" {
		t.Errorf("text = %q, want trimmed claude stdout", got)
	}
}

func TestDefaultAIDrafterFailure(t *testing.T) {
	fakeClaudeOnPath(t, "#!/bin/sh\necho 'model unavailable' >&2\nexit 1\n")
	_, err := defaultAIDrafter(t.Context(), t.TempDir(), "a prompt")
	if err == nil {
		t.Fatal("defaultAIDrafter() error = nil, want claude failure")
	}
	if !strings.Contains(err.Error(), "model unavailable") {
		t.Errorf("error = %v, want it to carry claude's stderr", err)
	}
}

// submitTestPR is the PR number the submit tests seed drafts against and send in
// their requests.
const submitTestPR = 7

// seedDraft inserts a draft directly into the store for submit tests, anchored
// to submitTestPR.
func seedDraft(t *testing.T, st *store.Store, id, dir, path string) {
	t.Helper()
	if err := st.SaveReviewDraft(t.Context(), store.ReviewDraft{
		ID: id, Dir: dir, PR: submitTestPR, Path: path, Line: 1, Side: "RIGHT", Body: "note " + id,
	}); err != nil {
		t.Fatalf("seed draft %s: %v", id, err)
	}
}
