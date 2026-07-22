package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/memory"
	"github.com/AnkushinDaniil/grove/internal/store"
	"github.com/AnkushinDaniil/grove/internal/tree"
)

// fakeMemory is a stand-in Memory backend recording the scope it was queried
// with and returning a canned result.
type fakeMemory struct {
	res      memory.Result
	gotNode  core.NodeID
	gotScope memory.Scope
	calls    int
}

func (f *fakeMemory) NodeMemory(_ context.Context, nodeID core.NodeID, scope memory.Scope) memory.Result {
	f.calls++
	f.gotNode = nodeID
	f.gotScope = scope
	return f.res
}

// newMemoryHarness builds a handler over a real tree (so node lookups work) with
// the given Memory backend, and returns the server plus the bootstrapped root id.
func newMemoryHarness(t *testing.T, mem Memory) (*httptest.Server, core.NodeID) {
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
	h := New(Config{Tree: tr, Memory: mem, Auth: NewAuth(testToken)})
	ts := httptest.NewServer(h.Routes())
	t.Cleanup(ts.Close)
	return ts, root.ID
}

func getMemory(t *testing.T, ts *httptest.Server, path string) (int, memoryResponse) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, ts.URL+path, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var body memoryResponse
	if resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
	}
	return resp.StatusCode, body
}

func TestNodeMemoryEndpointHealthy(t *testing.T) {
	mem := &fakeMemory{res: memory.Result{
		Backend: "mempalace",
		Healthy: true,
		Entries: []memory.Entry{
			{ID: "a1", Kind: memory.KindDecision, Content: "Chose Postgres", Source: memory.SourceAuto, CreatedAt: "2026-07-22T10:00:00"},
		},
	}}
	ts, root := newMemoryHarness(t, mem)

	status, body := getMemory(t, ts, "/api/v1/nodes/"+string(root)+"/memory?scope=subtree")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if !body.Healthy || body.Backend != "mempalace" || len(body.Entries) != 1 {
		t.Fatalf("body = %+v, want healthy mempalace with 1 entry", body)
	}
	e := body.Entries[0]
	if e.ID != "a1" || e.Kind != memory.KindDecision || e.Content != "Chose Postgres" || e.Source != memory.SourceAuto {
		t.Errorf("entry = %+v, want the canned decision", e)
	}
	if mem.gotNode != root || mem.gotScope != memory.ScopeSubtree {
		t.Errorf("queried node %q scope %q, want %q/subtree", mem.gotNode, mem.gotScope, root)
	}
}

func TestNodeMemoryEndpointDefaultsToSelfScope(t *testing.T) {
	mem := &fakeMemory{res: memory.Result{Backend: "mempalace", Healthy: true}}
	ts, root := newMemoryHarness(t, mem)

	if status, _ := getMemory(t, ts, "/api/v1/nodes/"+string(root)+"/memory"); status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if mem.gotScope != memory.ScopeSelf {
		t.Errorf("default scope = %q, want self", mem.gotScope)
	}
}

func TestNodeMemoryEndpointUnavailable(t *testing.T) {
	// A nil Memory backend (MemPalace not wired) must still be a healthy 200 with
	// an empty, non-nil entries slice — the UI shows an install hint, not an error.
	ts, root := newMemoryHarness(t, nil)

	status, body := getMemory(t, ts, "/api/v1/nodes/"+string(root)+"/memory?scope=self")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if body.Healthy || body.Backend != "" {
		t.Errorf("body = %+v, want healthy=false backend=''", body)
	}
	if body.Entries == nil {
		t.Error("entries must be a non-nil [] when unavailable")
	}
}

func TestNodeMemoryEndpointUnknownNode(t *testing.T) {
	mem := &fakeMemory{res: memory.Result{Healthy: true, Backend: "mempalace"}}
	ts, _ := newMemoryHarness(t, mem)

	status, _ := getMemory(t, ts, "/api/v1/nodes/does-not-exist/memory")
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for unknown node", status)
	}
	if mem.calls != 0 {
		t.Errorf("backend queried %d times for an unknown node, want 0", mem.calls)
	}
}
