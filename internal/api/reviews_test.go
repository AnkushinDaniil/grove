package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/github"
	"github.com/AnkushinDaniil/grove/internal/store"
	"github.com/AnkushinDaniil/grove/internal/tree"
)

// fakeGH is a hand-driven GitHubClient: every method reads from per-dir maps so
// a test can script logins, repo names, PR lists, and failures without gh.
type fakeGH struct {
	login    string
	loginErr error
	names    map[string]string      // dir -> nameWithOwner
	nameErr  map[string]error       // dir -> RepoName error
	prs      map[string][]github.PR // dir -> open PRs
	prsErr   map[string]error       // dir -> ListPRs error
}

func (f *fakeGH) Login(context.Context) (string, error) { return f.login, f.loginErr }

func (f *fakeGH) RepoName(_ context.Context, dir string) (string, error) {
	if err := f.nameErr[dir]; err != nil {
		return "", err
	}
	return f.names[dir], nil
}

func (f *fakeGH) ListPRs(_ context.Context, dir string) ([]github.PR, error) {
	if err := f.prsErr[dir]; err != nil {
		return nil, err
	}
	return f.prs[dir], nil
}

// reviewHarness wires the review handlers over a real store and tree with an
// injected fake gh. Sessions and worktrees are unused by these endpoints and
// left nil.
type reviewHarness struct {
	t     *testing.T
	store *store.Store
	tree  *tree.Tree
	gh    *fakeGH
	ts    *httptest.Server
	root  core.Node
}

func newReviewHarness(t *testing.T, gh *fakeGH) *reviewHarness {
	t.Helper()
	st, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "grove.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	tr := tree.New(st)
	root, err := tr.Bootstrap(t.Context(), "Workspace")
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	h := New(Config{
		Tree:    tr,
		Store:   st,
		Auth:    NewAuth(testToken),
		Version: "v-test",
		Commit:  "commit-test",
		GitHub:  gh,
	})
	ts := httptest.NewServer(h.Routes())
	t.Cleanup(ts.Close)

	return &reviewHarness{t: t, store: st, tree: tr, gh: gh, ts: ts, root: root}
}

// do issues a JSON request against the harness server and drains the response.
func (h *reviewHarness) do(method, path string, body any) response {
	h.t.Helper()
	var r io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			h.t.Fatalf("marshal request: %v", err)
		}
		r = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(h.t.Context(), method, h.ts.URL+path, r)
	if err != nil {
		h.t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := h.ts.Client().Do(req)
	if err != nil {
		h.t.Fatalf("do request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		h.t.Fatalf("read body: %v", err)
	}
	return response{status: resp.StatusCode, body: respBody, cookies: resp.Cookies()}
}

// decode asserts the response status and unmarshals its body into v (if non-nil).
func (h *reviewHarness) decode(resp response, wantStatus int, v any) {
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

func TestReviewsClassifiesBuckets(t *testing.T) {
	dir := newGitRepo(t)
	gh := &fakeGH{
		login: "me",
		names: map[string]string{dir: "octo/repo"},
		prs: map[string][]github.PR{dir: {
			{Number: 1, Title: "theirs", Author: "alice", HeadRefOid: "h1", Checks: "passing"},
			{Number: 2, Title: "mine", Author: "me", HeadRefOid: "h2"},
			{
				Number: 3, Title: "reviewed", Author: "bob", HeadRefOid: "h3",
				LatestReviews: []github.Review{{Author: "me", State: "APPROVED"}},
			},
		}},
	}
	h := newReviewHarness(t, gh)

	// Register the source explicitly so the read is deterministic.
	h.decode(h.do(http.MethodPost, "/api/v1/reviews/sources", map[string]any{
		"dirs": []string{dir},
	}), http.StatusOK, nil)

	var body reviewsResponse
	h.decode(h.do(http.MethodGet, "/api/v1/reviews", nil), http.StatusOK, &body)

	if body.Login != "me" {
		t.Errorf("login = %q, want me", body.Login)
	}
	if len(body.Errors) != 0 {
		t.Errorf("errors = %v, want none", body.Errors)
	}
	if len(body.Repos) != 1 {
		t.Fatalf("repos = %d, want 1", len(body.Repos))
	}
	repo := body.Repos[0]
	if repo.Dir != dir || repo.NameWithOwner != "octo/repo" {
		t.Errorf("repo = %+v, want dir %q name octo/repo", repo, dir)
	}
	if n := len(repo.Buckets.NeedsReview); n != 1 || repo.Buckets.NeedsReview[0].Number != 1 {
		t.Errorf("needs_review = %+v, want [1]", repo.Buckets.NeedsReview)
	}
	if n := len(repo.Buckets.Reviewed); n != 1 || repo.Buckets.Reviewed[0].Number != 3 {
		t.Errorf("reviewed = %+v, want [3]", repo.Buckets.Reviewed)
	}
	if n := len(repo.Buckets.Mine); n != 1 || repo.Buckets.Mine[0].Number != 2 {
		t.Errorf("mine = %+v, want [2]", repo.Buckets.Mine)
	}
	if repo.Buckets.NeedsReview[0].Checks != "passing" {
		t.Errorf("pr checks = %q, want passing", repo.Buckets.NeedsReview[0].Checks)
	}
}

func TestReviewsReReviewAcrossReads(t *testing.T) {
	dir := newGitRepo(t)
	gh := &fakeGH{
		login: "me",
		names: map[string]string{dir: "octo/repo"},
		prs: map[string][]github.PR{dir: {
			{
				Number: 7, Author: "bob", HeadRefOid: "old",
				LatestReviews: []github.Review{{Author: "me", State: "APPROVED"}},
			},
		}},
	}
	h := newReviewHarness(t, gh)
	h.decode(h.do(http.MethodPost, "/api/v1/reviews/sources", map[string]any{"dirs": []string{dir}}), http.StatusOK, nil)

	// First read: never seen before, so it stays in reviewed and records head "old".
	var first reviewsResponse
	h.decode(h.do(http.MethodGet, "/api/v1/reviews", nil), http.StatusOK, &first)
	if len(first.Repos[0].Buckets.ReReview) != 0 {
		t.Fatalf("first read re_review = %+v, want empty", first.Repos[0].Buckets.ReReview)
	}
	if len(first.Repos[0].Buckets.Reviewed) != 1 {
		t.Fatalf("first read reviewed = %+v, want [7]", first.Repos[0].Buckets.Reviewed)
	}

	// New commits land: the head moves. The second read must detect re-review
	// from the persisted head state.
	gh.prs[dir] = []github.PR{
		{
			Number: 7, Author: "bob", HeadRefOid: "new",
			LatestReviews: []github.Review{{Author: "me", State: "APPROVED"}},
		},
	}
	var second reviewsResponse
	h.decode(h.do(http.MethodGet, "/api/v1/reviews", nil), http.StatusOK, &second)
	if n := len(second.Repos[0].Buckets.ReReview); n != 1 || second.Repos[0].Buckets.ReReview[0].Number != 7 {
		t.Errorf("second read re_review = %+v, want [7]", second.Repos[0].Buckets.ReReview)
	}
	if len(second.Repos[0].Buckets.Reviewed) != 0 {
		t.Errorf("second read reviewed = %+v, want empty", second.Repos[0].Buckets.Reviewed)
	}
}

func TestReviewsSourcesSeedsFromTree(t *testing.T) {
	dir := newGitRepo(t)
	nonGit := t.TempDir() // set as a work_dir but not a git repo → not seeded
	gh := &fakeGH{login: "me"}
	h := newReviewHarness(t, gh)

	// A project node with a git work_dir seeds the sources; a non-git one does not.
	if _, err := h.tree.CreateNode(t.Context(), tree.CreateSpec{
		ParentID: h.root.ID, Kind: core.KindProject, Title: "P", WorkDir: dir,
	}); err != nil {
		t.Fatalf("CreateNode git: %v", err)
	}
	if _, err := h.tree.CreateNode(t.Context(), tree.CreateSpec{
		ParentID: h.root.ID, Kind: core.KindProject, Title: "Q", WorkDir: nonGit,
	}); err != nil {
		t.Fatalf("CreateNode non-git: %v", err)
	}

	var body sourcesResponse
	h.decode(h.do(http.MethodGet, "/api/v1/reviews/sources", nil), http.StatusOK, &body)
	if len(body.Dirs) != 1 || body.Dirs[0] != dir {
		t.Errorf("seeded dirs = %v, want [%s]", body.Dirs, dir)
	}
}

func TestSetReviewSourcesValidation(t *testing.T) {
	gitDir := newGitRepo(t)
	nonGit := t.TempDir()
	h := newReviewHarness(t, &fakeGH{login: "me"})

	// Non-absolute → 400.
	h.decode(h.do(http.MethodPost, "/api/v1/reviews/sources", map[string]any{
		"dirs": []string{"relative/path"},
	}), http.StatusBadRequest, nil)

	// Absolute existing dir but not a git repo → 400.
	h.decode(h.do(http.MethodPost, "/api/v1/reviews/sources", map[string]any{
		"dirs": []string{nonGit},
	}), http.StatusBadRequest, nil)

	// Valid git dirs, with a duplicate that must be deduped.
	var body sourcesResponse
	h.decode(h.do(http.MethodPost, "/api/v1/reviews/sources", map[string]any{
		"dirs": []string{gitDir, gitDir},
	}), http.StatusOK, &body)
	if len(body.Dirs) != 1 || body.Dirs[0] != gitDir {
		t.Errorf("normalized dirs = %v, want [%s]", body.Dirs, gitDir)
	}

	// The set persists and reads back.
	var readBack sourcesResponse
	h.decode(h.do(http.MethodGet, "/api/v1/reviews/sources", nil), http.StatusOK, &readBack)
	if len(readBack.Dirs) != 1 || readBack.Dirs[0] != gitDir {
		t.Errorf("read-back dirs = %v, want [%s]", readBack.Dirs, gitDir)
	}
}

func TestReviewStartCreatesProjectAndTask(t *testing.T) {
	dir := newGitRepo(t)
	gh := &fakeGH{login: "me", names: map[string]string{dir: "octo/repo"}}
	h := newReviewHarness(t, gh)

	var task NodeDTO
	h.decode(h.do(http.MethodPost, "/api/v1/reviews/start", map[string]any{
		"dir": dir, "pr": 42,
	}), http.StatusCreated, &task)

	if task.Kind != string(core.KindTask) {
		t.Errorf("kind = %q, want task", task.Kind)
	}
	if task.Title != "Review octo/repo#42" {
		t.Errorf("title = %q, want Review octo/repo#42", task.Title)
	}
	if task.WorkDir != dir {
		t.Errorf("work_dir = %q, want %q", task.WorkDir, dir)
	}
	if task.Brief == "" {
		t.Error("brief is empty, want a review instruction")
	}

	// The task hangs off a project (named for the repo) under the root.
	parent, ok := h.tree.Get(core.NodeID(task.ParentID))
	if !ok || parent.Kind != core.KindProject || parent.Title != "octo/repo" {
		t.Fatalf("parent = %+v, want project octo/repo", parent)
	}
	if parent.ParentID != h.root.ID {
		t.Errorf("project parent = %q, want root %q", parent.ParentID, h.root.ID)
	}

	// A second start for the same repo reuses the project rather than making a new one.
	var task2 NodeDTO
	h.decode(h.do(http.MethodPost, "/api/v1/reviews/start", map[string]any{
		"dir": dir, "pr": 43,
	}), http.StatusCreated, &task2)
	if task2.ParentID != task.ParentID {
		t.Errorf("second task parent = %q, want reused project %q", task2.ParentID, task.ParentID)
	}
	projects := 0
	for _, n := range h.tree.Children(h.root.ID) {
		if n.Kind == core.KindProject {
			projects++
		}
	}
	if projects != 1 {
		t.Errorf("project count = %d, want 1 (reused)", projects)
	}
}

func TestReviewStartRejectsBadInput(t *testing.T) {
	dir := newGitRepo(t)
	h := newReviewHarness(t, &fakeGH{login: "me", names: map[string]string{dir: "octo/repo"}})

	// Non-absolute dir → 400.
	h.decode(h.do(http.MethodPost, "/api/v1/reviews/start", map[string]any{
		"dir": "rel", "pr": 1,
	}), http.StatusBadRequest, nil)

	// Non-positive pr → 400.
	h.decode(h.do(http.MethodPost, "/api/v1/reviews/start", map[string]any{
		"dir": dir, "pr": 0,
	}), http.StatusBadRequest, nil)
}

func TestReviewStartRepoNameError(t *testing.T) {
	dir := newGitRepo(t)
	gh := &fakeGH{
		login: "me",
		nameErr: map[string]error{
			dir: &github.GHError{Args: []string{"repo", "view"}, Dir: dir, ExitCode: 1, Stderr: "no remote"},
		},
	}
	h := newReviewHarness(t, gh)
	// gh cannot resolve the repo name → 400, and no node is created.
	h.decode(h.do(http.MethodPost, "/api/v1/reviews/start", map[string]any{
		"dir": dir, "pr": 5,
	}), http.StatusBadRequest, nil)
	if kids := h.tree.Children(h.root.ID); len(kids) != 0 {
		t.Errorf("root gained %d children on a failed start, want 0", len(kids))
	}
}

func TestReviewSourcesRejectsMalformedBody(t *testing.T) {
	h := newReviewHarness(t, &fakeGH{login: "me"})
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost,
		h.ts.URL+"/api/v1/reviews/sources", bytes.NewReader([]byte("{not json")))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.ts.Client().Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 on malformed body", resp.StatusCode)
	}
}

func TestReviewsPerRepoErrorSurfaces(t *testing.T) {
	good := newGitRepo(t)
	bad := newGitRepo(t) // passes source validation but gh fails on it
	gh := &fakeGH{
		login: "me",
		names: map[string]string{good: "octo/good"},
		prs:   map[string][]github.PR{good: {{Number: 1, Author: "alice", HeadRefOid: "h1"}}},
		nameErr: map[string]error{
			bad: &github.GHError{Args: []string{"repo", "view"}, Dir: bad, ExitCode: 1, Stderr: "no remote"},
		},
	}
	h := newReviewHarness(t, gh)
	h.decode(h.do(http.MethodPost, "/api/v1/reviews/sources", map[string]any{
		"dirs": []string{good, bad},
	}), http.StatusOK, nil)

	// A single repo's gh failure never 500s: partial results plus an errors[] entry.
	var body reviewsResponse
	h.decode(h.do(http.MethodGet, "/api/v1/reviews", nil), http.StatusOK, &body)
	if len(body.Repos) != 1 || body.Repos[0].NameWithOwner != "octo/good" {
		t.Errorf("repos = %+v, want just octo/good", body.Repos)
	}
	if len(body.Errors) != 1 {
		t.Fatalf("errors = %v, want 1 entry", body.Errors)
	}
	if !strings.Contains(body.Errors[0], bad) {
		t.Errorf("error %q, want it to name the failing dir %q", body.Errors[0], bad)
	}
}

func TestReviewsLoginErrorSurfaces(t *testing.T) {
	dir := newGitRepo(t)
	gh := &fakeGH{
		loginErr: &github.GHError{Args: []string{"api", "user"}, ExitCode: 1, Stderr: "not logged in"},
		names:    map[string]string{dir: "octo/repo"},
		prs:      map[string][]github.PR{dir: {{Number: 1, Author: "alice", HeadRefOid: "h1"}}},
	}
	h := newReviewHarness(t, gh)
	h.decode(h.do(http.MethodPost, "/api/v1/reviews/sources", map[string]any{"dirs": []string{dir}}), http.StatusOK, nil)

	var body reviewsResponse
	h.decode(h.do(http.MethodGet, "/api/v1/reviews", nil), http.StatusOK, &body)
	if body.Login != "" {
		t.Errorf("login = %q, want empty on gh login failure", body.Login)
	}
	if len(body.Errors) != 1 {
		t.Errorf("errors = %v, want 1 login error", body.Errors)
	}
	// Repos still come back (with empty user-classification under an unknown login).
	if len(body.Repos) != 1 {
		t.Errorf("repos = %d, want 1 despite login failure", len(body.Repos))
	}
}

func TestReviewsEmptySources(t *testing.T) {
	h := newReviewHarness(t, &fakeGH{login: "me"})
	var body reviewsResponse
	h.decode(h.do(http.MethodGet, "/api/v1/reviews", nil), http.StatusOK, &body)
	if body.Repos == nil || len(body.Repos) != 0 {
		t.Errorf("repos = %v, want empty array", body.Repos)
	}
	if body.Errors == nil {
		t.Error("errors = null, want empty array")
	}
}
