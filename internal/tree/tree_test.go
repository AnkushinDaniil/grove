package tree

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/AnkushinDaniil/grove/internal/core"
)

// fakeStore records saves and can fail on demand.
type fakeStore struct {
	mu       sync.Mutex
	nodes    int
	sessions int
	events   int
	fail     error
}

func (f *fakeStore) SaveNodes(_ context.Context, ns []core.Node) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail != nil {
		return f.fail
	}
	f.nodes += len(ns)
	return nil
}

func (f *fakeStore) SaveSessions(_ context.Context, ss []core.Session) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail != nil {
		return f.fail
	}
	f.sessions += len(ss)
	return nil
}

func (f *fakeStore) AppendEvents(_ context.Context, es []core.Event) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail != nil {
		return f.fail
	}
	f.events += len(es)
	return nil
}

func (f *fakeStore) setFail(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fail = err
}

// newTestTree returns a tree with a bootstrapped workspace root.
func newTestTree(t *testing.T) (*Tree, core.Node, *fakeStore) {
	t.Helper()
	fs := &fakeStore{}
	tr := New(fs)
	root, err := tr.Bootstrap(t.Context(), "Workspace")
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	return tr, root, fs
}

func mustCreate(t *testing.T, tr *Tree, spec CreateSpec) core.Node {
	t.Helper()
	n, err := tr.CreateNode(t.Context(), spec)
	if err != nil {
		t.Fatalf("CreateNode(%+v): %v", spec, err)
	}
	return n
}

func TestBootstrapIdempotent(t *testing.T) {
	tr, root, _ := newTestTree(t)
	again, err := tr.Bootstrap(t.Context(), "Other")
	if err != nil {
		t.Fatalf("second Bootstrap: %v", err)
	}
	if again.ID != root.ID {
		t.Fatalf("Bootstrap created a second root: %s != %s", again.ID, root.ID)
	}
}

func TestCreateNodeHierarchy(t *testing.T) {
	tr, root, _ := newTestTree(t)
	project := mustCreate(t, tr, CreateSpec{ParentID: root.ID, Kind: core.KindProject, Title: "Nethermind"})
	task := mustCreate(t, tr, CreateSpec{ParentID: project.ID, Kind: core.KindTask, Title: "RPC opt"})
	mustCreate(t, tr, CreateSpec{ParentID: task.ID, Kind: core.KindTask, Title: "Bench subtask"})

	tests := []struct {
		name string
		spec CreateSpec
	}{
		{"second workspace", CreateSpec{Kind: core.KindWorkspace, Title: "Root2"}},
		{"task under workspace", CreateSpec{ParentID: root.ID, Kind: core.KindTask, Title: "x"}},
		{"project under project", CreateSpec{ParentID: project.ID, Kind: core.KindProject, Title: "x"}},
		{"unknown parent", CreateSpec{ParentID: "nope", Kind: core.KindTask, Title: "x"}},
		{"empty title", CreateSpec{ParentID: project.ID, Kind: core.KindTask, Title: ""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := tr.CreateNode(t.Context(), tt.spec); !errors.Is(err, core.ErrInvalid) {
				t.Fatalf("CreateNode = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestCreateUnderArchivedParent(t *testing.T) {
	tr, root, _ := newTestTree(t)
	project := mustCreate(t, tr, CreateSpec{ParentID: root.ID, Kind: core.KindProject, Title: "P"})
	if _, err := tr.Archive(t.Context(), project.ID); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if _, err := tr.CreateNode(t.Context(), CreateSpec{ParentID: project.ID, Kind: core.KindTask, Title: "x"}); !errors.Is(err, core.ErrInvalid) {
		t.Fatalf("CreateNode under archived = %v, want ErrInvalid", err)
	}
}

func TestSiblingPositions(t *testing.T) {
	tr, root, _ := newTestTree(t)
	p := mustCreate(t, tr, CreateSpec{ParentID: root.ID, Kind: core.KindProject, Title: "P"})
	a := mustCreate(t, tr, CreateSpec{ParentID: p.ID, Kind: core.KindTask, Title: "a"})
	b := mustCreate(t, tr, CreateSpec{ParentID: p.ID, Kind: core.KindTask, Title: "b"})
	if a.Position != 0 || b.Position != 1 {
		t.Fatalf("positions = %d, %d; want 0, 1", a.Position, b.Position)
	}
	kids := tr.Children(p.ID)
	if len(kids) != 2 || kids[0].ID != a.ID || kids[1].ID != b.ID {
		t.Fatalf("Children order wrong: %+v", kids)
	}
}

func TestUpdateNodePatch(t *testing.T) {
	tr, root, _ := newTestTree(t)
	p := mustCreate(t, tr, CreateSpec{ParentID: root.ID, Kind: core.KindProject, Title: "P"})
	title := "Renamed"
	driver := "claude"
	got, err := tr.UpdateNode(t.Context(), p.ID, Patch{Title: &title, Driver: &driver})
	if err != nil {
		t.Fatalf("UpdateNode: %v", err)
	}
	if got.Title != "Renamed" || got.Driver != "claude" || got.Brief != "" {
		t.Fatalf("patch applied wrong: %+v", got)
	}
	if _, err := tr.UpdateNode(t.Context(), "nope", Patch{}); !errors.Is(err, core.ErrInvalid) {
		t.Fatalf("UpdateNode unknown = %v, want ErrInvalid", err)
	}
	empty := ""
	if _, err := tr.UpdateNode(t.Context(), p.ID, Patch{Title: &empty}); !errors.Is(err, core.ErrInvalid) {
		t.Fatalf("UpdateNode empty title = %v, want ErrInvalid", err)
	}
}

func TestArchiveCascade(t *testing.T) {
	tr, root, _ := newTestTree(t)
	p := mustCreate(t, tr, CreateSpec{ParentID: root.ID, Kind: core.KindProject, Title: "P"})
	t1 := mustCreate(t, tr, CreateSpec{ParentID: p.ID, Kind: core.KindTask, Title: "t1"})
	t2 := mustCreate(t, tr, CreateSpec{ParentID: t1.ID, Kind: core.KindTask, Title: "t2"})
	other := mustCreate(t, tr, CreateSpec{ParentID: root.ID, Kind: core.KindProject, Title: "Other"})

	ids, err := tr.Archive(t.Context(), p.ID)
	if err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if len(ids) != 3 {
		t.Fatalf("archived %d nodes, want 3 (%v)", len(ids), ids)
	}
	for _, id := range []core.NodeID{p.ID, t1.ID, t2.ID} {
		n, _ := tr.Get(id)
		if !n.Archived() {
			t.Errorf("node %s not archived", id)
		}
	}
	if n, _ := tr.Get(other.ID); n.Archived() {
		t.Error("sibling project archived by cascade")
	}
	if kids := tr.Children(root.ID); len(kids) != 1 || kids[0].ID != other.ID {
		t.Fatalf("root children after archive: %+v", kids)
	}
	// Idempotent: second archive is a no-op.
	ids, err = tr.Archive(t.Context(), p.ID)
	if err != nil || ids != nil {
		t.Fatalf("re-archive = (%v, %v), want (nil, nil)", ids, err)
	}
	// Root cannot be archived.
	if _, err := tr.Archive(t.Context(), root.ID); !errors.Is(err, core.ErrInvalid) {
		t.Fatalf("Archive(root) = %v, want ErrInvalid", err)
	}
}

func TestResolveInheritance(t *testing.T) {
	tr, root, _ := newTestTree(t)
	prof := core.NewProfileID()
	driver := "claude"
	if _, err := tr.UpdateNode(t.Context(), root.ID, Patch{Driver: &driver, ProfileID: &prof}); err != nil {
		t.Fatalf("UpdateNode root: %v", err)
	}
	p := mustCreate(t, tr, CreateSpec{ParentID: root.ID, Kind: core.KindProject, Title: "P"})
	task := mustCreate(t, tr, CreateSpec{ParentID: p.ID, Kind: core.KindTask, Title: "t", Driver: "codex"})

	res, ok := tr.Resolve(task.ID)
	if !ok {
		t.Fatal("Resolve: node not found")
	}
	if res.Driver != "codex" {
		t.Errorf("Driver = %q, want codex (own value wins)", res.Driver)
	}
	if res.ProfileID != prof {
		t.Errorf("ProfileID = %q, want inherited %q", res.ProfileID, prof)
	}
}

func TestResolveWorkDir(t *testing.T) {
	tr, root, _ := newTestTree(t)
	projDir := "/home/user/project"
	if _, err := tr.UpdateNode(t.Context(), root.ID, Patch{WorkDir: &projDir}); err != nil {
		t.Fatalf("UpdateNode root: %v", err)
	}
	p := mustCreate(t, tr, CreateSpec{ParentID: root.ID, Kind: core.KindProject, Title: "P"})
	nested := mustCreate(t, tr, CreateSpec{ParentID: p.ID, Kind: core.KindTask, Title: "t"})
	own := mustCreate(t, tr, CreateSpec{
		ParentID: p.ID, Kind: core.KindTask, Title: "own", WorkDir: "/home/user/task",
	})

	// Nested task with no own value inherits the nearest ancestor's (root's).
	if got, ok := tr.ResolveWorkDir(nested.ID); !ok || got != projDir {
		t.Errorf("ResolveWorkDir(nested) = (%q, %v), want (%q, true)", got, ok, projDir)
	}
	// A node's own value wins over the ancestor's.
	if got, ok := tr.ResolveWorkDir(own.ID); !ok || got != "/home/user/task" {
		t.Errorf("ResolveWorkDir(own) = (%q, %v), want (/home/user/task, true)", got, ok)
	}

	// Unset everywhere resolves to ("", true).
	tr2, root2, _ := newTestTree(t)
	task := mustCreate(t, tr2, CreateSpec{
		ParentID: mustCreate(t, tr2, CreateSpec{ParentID: root2.ID, Kind: core.KindProject, Title: "P"}).ID,
		Kind:     core.KindTask, Title: "t",
	})
	if got, ok := tr2.ResolveWorkDir(task.ID); !ok || got != "" {
		t.Errorf("ResolveWorkDir(unset) = (%q, %v), want (\"\", true)", got, ok)
	}

	// A missing node reports ok=false.
	if got, ok := tr2.ResolveWorkDir("does-not-exist"); ok || got != "" {
		t.Errorf("ResolveWorkDir(missing) = (%q, %v), want (\"\", false)", got, ok)
	}
}

func TestStoreFailureIsAtomic(t *testing.T) {
	tr, root, fs := newTestTree(t)
	before := tr.Snapshot()
	fs.setFail(errors.New("disk full"))
	if _, err := tr.CreateNode(t.Context(), CreateSpec{ParentID: root.ID, Kind: core.KindProject, Title: "P"}); err == nil {
		t.Fatal("CreateNode succeeded despite store failure")
	}
	after := tr.Snapshot()
	if after.Rev != before.Rev || len(after.Nodes) != len(before.Nodes) {
		t.Fatalf("state changed after failed mutation: rev %d→%d, nodes %d→%d",
			before.Rev, after.Rev, len(before.Nodes), len(after.Nodes))
	}
}

func TestSubscribeDeltasAndRev(t *testing.T) {
	tr, root, _ := newTestTree(t)
	snap, ch, cancel := tr.Subscribe()
	defer cancel()

	p := mustCreate(t, tr, CreateSpec{ParentID: root.ID, Kind: core.KindProject, Title: "P"})
	d := <-ch
	if d.Rev != snap.Rev+1 {
		t.Fatalf("delta rev = %d, want %d", d.Rev, snap.Rev+1)
	}
	if len(d.Nodes) != 1 || d.Nodes[0].ID != p.ID {
		t.Fatalf("delta nodes = %+v", d.Nodes)
	}
	mustCreate(t, tr, CreateSpec{ParentID: p.ID, Kind: core.KindTask, Title: "t"})
	d2 := <-ch
	if d2.Rev != d.Rev+1 {
		t.Fatalf("rev not monotonic: %d then %d", d.Rev, d2.Rev)
	}
}

func TestSlowSubscriberDropped(t *testing.T) {
	tr, root, _ := newTestTree(t)
	_, ch, cancel := tr.Subscribe()
	defer cancel()
	// Never read: overflow the buffer to force the drop.
	for range subBuffer + 1 {
		mustCreate(t, tr, CreateSpec{ParentID: root.ID, Kind: core.KindProject, Title: "p"})
	}
	deadline := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return // dropped as designed
			}
		case <-deadline:
			t.Fatal("slow subscriber was not dropped")
		}
	}
}

func TestLoadRoundTrip(t *testing.T) {
	tr, root, _ := newTestTree(t)
	p := mustCreate(t, tr, CreateSpec{ParentID: root.ID, Kind: core.KindProject, Title: "P"})
	task := mustCreate(t, tr, CreateSpec{ParentID: p.ID, Kind: core.KindTask, Title: "t"})
	if _, err := tr.ApplySession(t.Context(), startedSession(task.ID)); err != nil {
		t.Fatalf("ApplySession: %v", err)
	}
	snap := tr.Snapshot()

	tr2 := New(&fakeStore{})
	if err := tr2.Load(snap.Nodes, snap.Sessions); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got, ok := tr2.Root(); !ok || got.ID != root.ID {
		t.Fatalf("Root after load = (%+v, %v)", got, ok)
	}
	if kids := tr2.Children(p.ID); len(kids) != 1 || kids[0].ID != task.ID {
		t.Fatalf("Children after load = %+v", kids)
	}
	if s, ok := tr2.SessionFor(task.ID); !ok || s.NodeID != task.ID {
		t.Fatalf("SessionFor after load = (%+v, %v)", s, ok)
	}
	snap2 := tr2.Snapshot()
	if len(snap2.Nodes) != len(snap.Nodes) || len(snap2.Sessions) != len(snap.Sessions) {
		t.Fatalf("snapshot mismatch after load: %d/%d nodes, %d/%d sessions",
			len(snap2.Nodes), len(snap.Nodes), len(snap2.Sessions), len(snap.Sessions))
	}
}

func TestLoadRejectsOrphans(t *testing.T) {
	fs := &fakeStore{}
	tr := New(fs)
	now := time.Now()
	orphan := core.Node{
		ID: core.NewNodeID(), ParentID: core.NewNodeID(), Kind: core.KindTask,
		Title: "orphan", Status: core.StatusIdle, Attention: core.AttentionNone,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := tr.Load([]core.Node{orphan}, nil); !errors.Is(err, core.ErrInvalid) {
		t.Fatalf("Load orphan = %v, want ErrInvalid", err)
	}
}

func TestConcurrentCreates(t *testing.T) {
	tr, root, _ := newTestTree(t)
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			_, err := tr.CreateNode(context.Background(), CreateSpec{
				ParentID: root.ID, Kind: core.KindProject, Title: "p",
			})
			if err != nil {
				t.Errorf("concurrent CreateNode: %v", err)
			}
		})
	}
	wg.Wait()
	if kids := tr.Children(root.ID); len(kids) != 32 {
		t.Fatalf("children = %d, want 32", len(kids))
	}
	snap := tr.Snapshot()
	if snap.Rev != 33 { // 1 bootstrap + 32 creates
		t.Fatalf("rev = %d, want 33", snap.Rev)
	}
}
