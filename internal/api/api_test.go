package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/driver"
	"github.com/AnkushinDaniil/grove/internal/gitcli"
	"github.com/AnkushinDaniil/grove/internal/session"
	"github.com/AnkushinDaniil/grove/internal/store"
	"github.com/AnkushinDaniil/grove/internal/testutil/fakeagent"
	"github.com/AnkushinDaniil/grove/internal/tree"
	"github.com/AnkushinDaniil/grove/internal/worktree"
)

const testToken = "test-daemon-token"

// harness is a fully wired API stack over real components: a temp SQLite store,
// a real tree, a session manager driving the fake agent, and a worktree engine
// over temp git repos. Handlers.Routes() is served through httptest with no auth
// middleware (that lives in internal/server), so these tests exercise handler
// behavior directly.
type harness struct {
	t          *testing.T
	store      *store.Store
	tree       *tree.Tree
	mgr        *session.Manager
	engine     *worktree.Engine
	hookTokens *HookTokens
	ts         *httptest.Server
	scrollback string
	root       core.Node
}

func newHarness(t *testing.T, script []fakeagent.Step) *harness {
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

	drv := fakeagent.NewDriver(fakeagent.Build(t), fakeagent.WriteScript(t, script))
	reg, err := driver.NewRegistry(drv)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	scrollback := filepath.Join(t.TempDir(), "scrollback")
	mgr := session.NewManager(reg, tr, session.Config{ScrollbackDir: scrollback})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = mgr.Shutdown(ctx)
	})

	engine := worktree.NewEngine(gitcli.NewRunner(), filepath.Join(t.TempDir(), "worktrees"), time.Now)
	hookTokens := NewHookTokens()

	h := New(Config{
		Tree:       tr,
		Sessions:   mgr,
		Store:      st,
		Worktrees:  engine,
		Auth:       NewAuth(testToken),
		HookTokens: hookTokens,
		Version:    "v-test",
		Commit:     "commit-test",
	})
	ts := httptest.NewServer(h.Routes())
	t.Cleanup(ts.Close)

	return &harness{
		t: t, store: st, tree: tr, mgr: mgr, engine: engine,
		hookTokens: hookTokens, ts: ts, scrollback: scrollback, root: root,
	}
}

// response is a fully read HTTP response: the body is drained and closed inside
// the harness so call sites never juggle an open body.
type response struct {
	status  int
	body    []byte
	cookies []*http.Cookie
}

// doReq executes req and returns the drained response.
func (h *harness) doReq(req *http.Request) response {
	h.t.Helper()
	resp, err := h.ts.Client().Do(req)
	if err != nil {
		h.t.Fatalf("do request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		h.t.Fatalf("read body: %v", err)
	}
	return response{status: resp.StatusCode, body: body, cookies: resp.Cookies()}
}

// do issues a JSON request against the harness server.
func (h *harness) do(method, path string, body any) response {
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
	return h.doReq(req)
}

// decode asserts the response status and unmarshals its body into v (if non-nil).
func (h *harness) decode(resp response, wantStatus int, v any) {
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

// createNode is a helper posting POST /nodes and returning the created node DTO.
func (h *harness) createNode(parent core.NodeID, kind core.Kind, title, driverID string) NodeDTO {
	h.t.Helper()
	var node NodeDTO
	resp := h.do(http.MethodPost, "/api/v1/nodes", map[string]string{
		"parent_id": string(parent),
		"kind":      string(kind),
		"title":     title,
		"driver":    driverID,
	})
	h.decode(resp, http.StatusCreated, &node)
	return node
}

// newGitRepo initializes a temp git repository with one commit on main.
func newGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), "git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	run("config", "user.name", "grove-test")
	run("config", "user.email", "grove-test@example.com")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("init\n"), 0o644); err != nil {
		t.Fatalf("write initial file: %v", err)
	}
	run("add", "-A")
	run("commit", "-q", "-m", "initial commit")
	return dir
}

// projectWithRepo creates a project (driver=fake) under root and registers a
// repo on it, returning the project node.
func (h *harness) projectWithRepo(name string) NodeDTO {
	h.t.Helper()
	project := h.createNode(h.root.ID, core.KindProject, "Project", "fake")
	repo := core.Repo{
		ID:         core.NewRepoID(),
		ProjectID:  core.NodeID(project.ID),
		Name:       name,
		SourcePath: newGitRepo(h.t),
		CreatedAt:  time.Now(),
	}
	if err := h.store.SaveRepo(h.t.Context(), repo); err != nil {
		h.t.Fatalf("SaveRepo: %v", err)
	}
	return project
}

func intPtr(i int) *int { return &i }

func TestTreeSnapshot(t *testing.T) {
	h := newHarness(t, nil)
	project := h.createNode(h.root.ID, core.KindProject, "Proj", "fake")

	var body treeResponse
	h.decode(h.do(http.MethodGet, "/api/v1/tree", nil), http.StatusOK, &body)

	if body.Rev == 0 {
		t.Error("rev = 0, want a non-zero revision")
	}
	ids := map[string]bool{}
	for _, n := range body.Nodes {
		ids[n.ID] = true
	}
	if !ids[string(h.root.ID)] || !ids[project.ID] {
		t.Errorf("tree missing root or project: %v", ids)
	}
	// The root workspace serializes parent_id as an empty string, not omitted.
	for _, n := range body.Nodes {
		if n.ID == string(h.root.ID) && n.ParentID != "" {
			t.Errorf("root parent_id = %q, want empty", n.ParentID)
		}
	}
}

func TestCreateNodeValidation(t *testing.T) {
	h := newHarness(t, nil)

	// Unknown parent → ErrInvalid, not found flavored → 404.
	resp := h.do(http.MethodPost, "/api/v1/nodes", map[string]string{
		"parent_id": "does-not-exist", "kind": "task", "title": "T",
	})
	h.decode(resp, http.StatusNotFound, nil)

	// Empty title → validation error → 400.
	resp = h.do(http.MethodPost, "/api/v1/nodes", map[string]string{
		"parent_id": string(h.root.ID), "kind": "project", "title": "",
	})
	h.decode(resp, http.StatusBadRequest, nil)
}

func TestPatchNode(t *testing.T) {
	h := newHarness(t, nil)
	project := h.createNode(h.root.ID, core.KindProject, "Proj", "fake")

	var patched NodeDTO
	h.decode(h.do(http.MethodPatch, "/api/v1/nodes/"+project.ID, map[string]any{
		"title": "Renamed",
		"meta":  map[string]any{"pinned": true},
	}), http.StatusOK, &patched)

	if patched.Title != "Renamed" {
		t.Errorf("title = %q, want Renamed", patched.Title)
	}
	var meta map[string]any
	if err := json.Unmarshal(patched.Meta, &meta); err != nil {
		t.Fatalf("meta not a JSON object: %v", err)
	}
	if meta["pinned"] != true {
		t.Errorf("meta.pinned = %v, want true", meta["pinned"])
	}
}

func TestPatchNodeInvalidMeta(t *testing.T) {
	h := newHarness(t, nil)
	project := h.createNode(h.root.ID, core.KindProject, "Proj", "fake")

	// meta must be a JSON object, not an array.
	resp := h.do(http.MethodPatch, "/api/v1/nodes/"+project.ID, map[string]any{
		"meta": []int{1, 2, 3},
	})
	h.decode(resp, http.StatusBadRequest, nil)
}

func TestCreateNodeWithWorkDir(t *testing.T) {
	h := newHarness(t, nil)
	dir := t.TempDir()

	var node NodeDTO
	h.decode(h.do(http.MethodPost, "/api/v1/nodes", map[string]any{
		"parent_id": string(h.root.ID),
		"kind":      "project",
		"title":     "P",
		"driver":    "fake",
		"work_dir":  dir,
	}), http.StatusCreated, &node)

	if node.WorkDir != dir {
		t.Errorf("work_dir = %q, want %q", node.WorkDir, dir)
	}
}

func TestCreateNodeRejectsBadWorkDir(t *testing.T) {
	h := newHarness(t, nil)

	// Relative path → 400.
	h.decode(h.do(http.MethodPost, "/api/v1/nodes", map[string]any{
		"parent_id": string(h.root.ID), "kind": "project", "title": "P",
		"work_dir": "relative/dir",
	}), http.StatusBadRequest, nil)

	// Absolute but nonexistent → 400.
	h.decode(h.do(http.MethodPost, "/api/v1/nodes", map[string]any{
		"parent_id": string(h.root.ID), "kind": "project", "title": "P",
		"work_dir": filepath.Join(t.TempDir(), "does-not-exist"),
	}), http.StatusBadRequest, nil)
}

func TestPatchNodeWorkDirSetAndClear(t *testing.T) {
	h := newHarness(t, nil)
	project := h.createNode(h.root.ID, core.KindProject, "Proj", "fake")
	dir := t.TempDir()

	var patched NodeDTO
	h.decode(h.do(http.MethodPatch, "/api/v1/nodes/"+project.ID, map[string]any{
		"work_dir": dir,
	}), http.StatusOK, &patched)
	if patched.WorkDir != dir {
		t.Errorf("after set, work_dir = %q, want %q", patched.WorkDir, dir)
	}

	// An explicit empty string clears the override without an existence check.
	h.decode(h.do(http.MethodPatch, "/api/v1/nodes/"+project.ID, map[string]any{
		"work_dir": "",
	}), http.StatusOK, &patched)
	if patched.WorkDir != "" {
		t.Errorf("after clear, work_dir = %q, want empty", patched.WorkDir)
	}
}

func TestPatchNodeRejectsBadWorkDir(t *testing.T) {
	h := newHarness(t, nil)
	project := h.createNode(h.root.ID, core.KindProject, "Proj", "fake")

	// Relative path → 400.
	h.decode(h.do(http.MethodPatch, "/api/v1/nodes/"+project.ID, map[string]any{
		"work_dir": "relative/dir",
	}), http.StatusBadRequest, nil)

	// Absolute but nonexistent → 400.
	h.decode(h.do(http.MethodPatch, "/api/v1/nodes/"+project.ID, map[string]any{
		"work_dir": filepath.Join(t.TempDir(), "nope"),
	}), http.StatusBadRequest, nil)
}

func TestVersionUsageStats(t *testing.T) {
	h := newHarness(t, nil)

	var version versionResponse
	h.decode(h.do(http.MethodGet, "/api/v1/version", nil), http.StatusOK, &version)
	if version.Version != "v-test" || version.Commit != "commit-test" {
		t.Errorf("version = %+v, want v-test/commit-test", version)
	}

	var usage map[string][]any
	h.decode(h.do(http.MethodGet, "/api/v1/usage", nil), http.StatusOK, &usage)
	if usage["profiles"] == nil || len(usage["profiles"]) != 0 {
		t.Errorf("usage.profiles = %v, want empty array", usage["profiles"])
	}

	h.decode(h.do(http.MethodGet, "/api/v1/stats", nil), http.StatusNotImplemented, nil)
}

func TestAuthSessionAndMe(t *testing.T) {
	h := newHarness(t, nil)

	// Wrong token → 401, no cookie.
	resp := h.do(http.MethodPost, PathAuthSession, map[string]string{"token": "wrong"})
	h.decode(resp, http.StatusUnauthorized, nil)

	// Correct token → 204 + cookie.
	resp = h.do(http.MethodPost, PathAuthSession, map[string]string{"token": testToken})
	var cookie *http.Cookie
	for _, c := range resp.cookies {
		if c.Name == authCookie {
			cookie = c
		}
	}
	h.decode(resp, http.StatusNoContent, nil)
	if cookie == nil || cookie.Value != testToken {
		t.Fatalf("auth cookie = %+v, want value %q", cookie, testToken)
	}
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Errorf("cookie flags = HttpOnly:%v SameSite:%v, want HttpOnly Strict", cookie.HttpOnly, cookie.SameSite)
	}

	// /auth/me with the cookie → 204; without → 401.
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, h.ts.URL+PathAuthMe, nil)
	if err != nil {
		t.Fatalf("new auth/me request: %v", err)
	}
	req.AddCookie(cookie)
	h.decode(h.doReq(req), http.StatusNoContent, nil)

	h.decode(h.do(http.MethodGet, PathAuthMe, nil), http.StatusUnauthorized, nil)
}

func TestStatusForError(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{session.ErrSessionNotFound, http.StatusNotFound},
		{session.ErrBudgetExhausted, http.StatusTooManyRequests},
		{session.ErrNoDriver, http.StatusBadRequest},
		{core.ErrInvalid, http.StatusBadRequest},
	}
	for _, tc := range cases {
		if got := statusForError(tc.err); got != tc.want {
			t.Errorf("statusForError(%v) = %d, want %d", tc.err, got, tc.want)
		}
	}
}
