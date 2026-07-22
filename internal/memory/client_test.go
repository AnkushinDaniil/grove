package memory

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AnkushinDaniil/grove/internal/core"
)

// projectTaskTree builds workspace → project → task and returns the project and
// task ids.
func projectTaskTree() (*fakeTree, core.NodeID, core.NodeID) {
	tr := newFakeTree()
	tr.add("ws", "", core.KindWorkspace, "Workspace")
	tr.add("proj", "ws", core.KindProject, "API")
	tr.add("task", "proj", core.KindTask, "Add auth")
	tr.add("sub", "task", core.KindTask, "Wire middleware")
	return tr, "proj", "task"
}

func newTestClient(t *testing.T, cf callFake, tr Tree) *Client {
	t.Helper()
	return NewClient(Options{
		Env:           cf.env(),
		Tree:          tr,
		SpoolPath:     filepath.Join(t.TempDir(), "spool", "memory.jsonl"),
		CallTimeout:   10 * time.Second,
		RecallTimeout: 10 * time.Second,
	})
}

func TestParseScope(t *testing.T) {
	cases := map[string]Scope{
		"self":      ScopeSelf,
		"subtree":   ScopeSubtree,
		"ancestors": ScopeAncestors,
		"":          ScopeSelf,
		"bogus":     ScopeSelf,
	}
	for in, want := range cases {
		if got := ParseScope(in); got != want {
			t.Errorf("ParseScope(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestScopeResolution(t *testing.T) {
	tr, proj, task := projectTaskTree()

	if got := wingFor(tr, task); got != "grove-proj" {
		t.Errorf("wingFor(task) = %q, want grove-proj", got)
	}
	if got := wingFor(tr, proj); got != "grove-proj" {
		t.Errorf("wingFor(proj) = %q, want grove-proj", got)
	}
	if got := roomFor(task); got != "task" {
		t.Errorf("roomFor(task) = %q, want task", got)
	}

	self := resolveScope(tr, task, ScopeSelf)
	if !self.valid() || !self.allows("task") || self.allows("proj") {
		t.Errorf("self scope rooms wrong: %+v", self.rooms)
	}

	// Subtree of the project spans the whole tree below it.
	sub := resolveScope(tr, proj, ScopeSubtree)
	for _, want := range []string{"proj", "task", "sub"} {
		if !sub.allows(want) {
			t.Errorf("subtree scope missing room %q: %+v", want, sub.rooms)
		}
	}

	// Ancestors of the leaf climb to the project anchor, not beyond.
	anc := resolveScope(tr, "sub", ScopeAncestors)
	for _, want := range []string{"sub", "task", "proj"} {
		if !anc.allows(want) {
			t.Errorf("ancestors scope missing room %q: %+v", want, anc.rooms)
		}
	}
	if anc.allows("ws") {
		t.Errorf("ancestors scope should stop at the project, not include the workspace")
	}

	// Recall unions subtree and ancestors.
	rooms := recallRooms(tr, "task")
	for _, want := range []string{"task", "sub", "proj"} {
		if !rooms[want] {
			t.Errorf("recallRooms(task) missing %q: %+v", want, rooms)
		}
	}
}

func TestHeaderRoundTrip(t *testing.T) {
	content := "We chose Postgres for durability."
	stored := withHeader(KindDecision, SourceAuto, content)
	kind, source, body := splitHeader(stored)
	if kind != KindDecision || source != SourceAuto || body != content {
		t.Errorf("round-trip = (%q,%q,%q), want (%q,%q,%q)", kind, source, body, KindDecision, SourceAuto, content)
	}
	// A drawer without grove's header reads back as an agent-authored fact.
	kind, source, body = splitHeader("just some content\nsecond line")
	if kind != KindFact || source != SourceAgent || body != "just some content\nsecond line" {
		t.Errorf("headerless = (%q,%q,%q), want (fact,agent,verbatim)", kind, source, body)
	}
}

func TestSearchParsesAndFiltersByRoom(t *testing.T) {
	cf := newCallFake(t)
	tr, _, task := projectTaskTree()
	c := newTestClient(t, cf, tr)

	cf.setSearch(t, `{"results":[
		{"text":"grove:auto:decision\nChose Postgres","wing":"grove-proj","room":"task","source_path":"grove://node/task","created_at":"2026-07-22T10:00:00"},
		{"text":"grove:agent:gotcha\nUnrelated","wing":"grove-proj","room":"other","source_path":"grove://node/other","created_at":"2026-07-22T11:00:00"}
	]}`)

	entries, healthy := c.Search(context.Background(), "postgres", resolveScope(tr, task, ScopeSelf), 10)
	if !healthy {
		t.Fatal("Search reported unhealthy against a working fake")
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1 (room-filtered): %+v", len(entries), entries)
	}
	e := entries[0]
	if e.Kind != KindDecision || e.Source != SourceAuto || e.Content != "Chose Postgres" || e.Room != "task" {
		t.Errorf("entry = %+v, want decision/auto/'Chose Postgres'/task", e)
	}
	if e.ID == "" {
		t.Error("entry ID should be synthesized, got empty")
	}
	if e.CreatedAt != "2026-07-22T10:00:00" {
		t.Errorf("CreatedAt = %q, want passthrough ISO", e.CreatedAt)
	}
}

func TestCaptureRecordsHeaderedDrawer(t *testing.T) {
	cf := newCallFake(t)
	tr, _, task := projectTaskTree()
	c := newTestClient(t, cf, tr)

	c.Capture(context.Background(), task, KindDecision, "Chose Postgres for durability", SourceAuto)

	lines := cf.drawers(t)
	if len(lines) != 1 {
		t.Fatalf("recorded %d drawers, want 1", len(lines))
	}
	var got drawerWrite
	if err := json.Unmarshal([]byte(lines[0]), &got); err != nil {
		t.Fatalf("decode recorded drawer: %v", err)
	}
	if got.Wing != "grove-proj" || got.Room != "task" {
		t.Errorf("scope = wing %q room %q, want grove-proj/task", got.Wing, got.Room)
	}
	if got.SourceFile != "grove://node/task" || got.AddedBy != addedBy {
		t.Errorf("attribution = source %q addedBy %q", got.SourceFile, got.AddedBy)
	}
	if !strings.HasPrefix(got.Content, "grove:auto:decision\n") || !strings.Contains(got.Content, "Chose Postgres") {
		t.Errorf("content missing grove header or body: %q", got.Content)
	}
}

func TestCaptureIgnoresEmptyAndUnknownNode(t *testing.T) {
	cf := newCallFake(t)
	tr, _, task := projectTaskTree()
	c := newTestClient(t, cf, tr)

	c.Capture(context.Background(), task, KindFact, "   ", SourceAuto) // empty content
	c.Capture(context.Background(), "ghost", KindFact, "orphan", SourceAuto)
	if lines := cf.drawers(t); len(lines) != 0 {
		t.Errorf("expected no drawers for empty/unknown captures, got %d", len(lines))
	}
}

func TestRecallRendersBoundedBlock(t *testing.T) {
	cf := newCallFake(t)
	tr, _, task := projectTaskTree()
	c := newTestClient(t, cf, tr)

	cf.setSearch(t, `{"results":[
		{"text":"grove:auto:decision\nChose Postgres","wing":"grove-proj","room":"task","source_path":"grove://node/task","created_at":"2026-07-22T10:00:00"},
		{"text":"grove:auto:gotcha\nMigrations lock the table","wing":"grove-proj","room":"sub","source_path":"grove://node/sub","created_at":"2026-07-22T11:00:00"}
	]}`)

	block := c.Recall(context.Background(), task, 4096)
	if !strings.HasPrefix(block, "## Memory") {
		t.Fatalf("recall block missing heading: %q", block)
	}
	if !strings.Contains(block, "Chose Postgres") || !strings.Contains(block, "Migrations lock the table") {
		t.Errorf("recall block missing entries: %q", block)
	}

	// A tiny budget still yields at least one entry but stays bounded.
	small := c.Recall(context.Background(), task, 60)
	if !strings.HasPrefix(small, "## Memory") {
		t.Errorf("bounded recall lost its heading: %q", small)
	}
	if len(small) > 200 {
		t.Errorf("bounded recall = %d bytes, want it near the budget", len(small))
	}
}

func TestNodeMemoryHealthyAndUnavailable(t *testing.T) {
	tr, _, task := projectTaskTree()

	cf := newCallFake(t)
	c := newTestClient(t, cf, tr)
	cf.setSearch(t, `{"results":[{"text":"grove:user:convention\nUse tabs","wing":"grove-proj","room":"task","source_path":"grove://node/task","created_at":"2026-07-22T10:00:00"}]}`)
	res := c.NodeMemory(context.Background(), task, ScopeSelf)
	if !res.Healthy || res.Backend != backendName || len(res.Entries) != 1 {
		t.Fatalf("healthy result = %+v, want healthy mempalace with 1 entry", res)
	}
	if res.Entries[0].Kind != KindConvention || res.Entries[0].Source != SourceUser {
		t.Errorf("entry = %+v, want convention/user", res.Entries[0])
	}

	// No binary on PATH: the endpoint degrades to unavailable, never errors.
	down := NewClient(Options{Env: Env{PATH: t.TempDir(), Home: t.TempDir()}, Tree: tr})
	got := down.NodeMemory(context.Background(), task, ScopeSelf)
	if got.Healthy || got.Backend != "" || len(got.Entries) != 0 {
		t.Errorf("unavailable result = %+v, want empty healthy=false", got)
	}
}

func TestWriteSpoolsWhenUnavailable(t *testing.T) {
	spool := filepath.Join(t.TempDir(), "spool", "memory.jsonl")
	c := NewClient(Options{Env: Env{PATH: t.TempDir(), Home: t.TempDir()}, SpoolPath: spool})

	w := drawerWrite{Wing: "grove-proj", Room: "task", Content: "grove:auto:fact\nnote", SourceFile: "grove://node/task", AddedBy: addedBy}
	if err := c.Write(context.Background(), w); err != nil {
		t.Fatalf("Write to spool: %v", err)
	}
	data, err := os.ReadFile(spool)
	if err != nil {
		t.Fatalf("read spool: %v", err)
	}
	var got drawerWrite
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(data))), &got); err != nil {
		t.Fatalf("decode spooled write: %v", err)
	}
	if got != w {
		t.Errorf("spooled = %+v, want %+v", got, w)
	}
}

func TestHealthyWriteDrainsSpool(t *testing.T) {
	cf := newCallFake(t)
	c := newTestClient(t, cf, newFakeTree())

	// Pre-seed the spool with a backlog write, as if buffered while down.
	backlog := drawerWrite{Wing: "grove-proj", Room: "task", Content: "grove:auto:fact\nbacklog", SourceFile: "grove://node/task", AddedBy: addedBy}
	if err := c.spoolWrite(backlog); err != nil {
		t.Fatalf("seed spool: %v", err)
	}

	// A live write drains the backlog first, then files itself.
	fresh := drawerWrite{Wing: "grove-proj", Room: "task", Content: "grove:auto:fact\nfresh", SourceFile: "grove://node/task", AddedBy: addedBy}
	if err := c.Write(context.Background(), fresh); err != nil {
		t.Fatalf("live write: %v", err)
	}

	lines := cf.drawers(t)
	if len(lines) != 2 {
		t.Fatalf("recorded %d drawers, want 2 (backlog + fresh)", len(lines))
	}
	if !strings.Contains(lines[0], "backlog") || !strings.Contains(lines[1], "fresh") {
		t.Errorf("drain order wrong: %v", lines)
	}
	if _, err := os.Stat(c.spool); !os.IsNotExist(err) {
		t.Errorf("spool file should be removed after a clean drain, stat err = %v", err)
	}
}

func TestNilClientIsNoOp(t *testing.T) {
	var c *Client
	// None of these should panic on a nil client.
	c.Capture(context.Background(), "task", KindFact, "x", SourceAuto)
	if got := c.Recall(context.Background(), "task", 1024); got != "" {
		t.Errorf("nil Recall = %q, want empty", got)
	}
	if got := c.NodeMemory(context.Background(), "task", ScopeSelf); got.Healthy {
		t.Errorf("nil NodeMemory = %+v, want unhealthy", got)
	}
}
