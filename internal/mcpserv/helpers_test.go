package mcpserv

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/tree"
)

// memStore is a no-op tree.Store: the tree keeps its state in memory, so tests
// need persistence only as a satisfied seam.
type memStore struct{}

func (memStore) SaveNodes(context.Context, []core.Node) error       { return nil }
func (memStore) SaveSessions(context.Context, []core.Session) error { return nil }
func (memStore) AppendEvents(context.Context, []core.Event) error   { return nil }
func (memStore) AckNodeEvents(context.Context, core.NodeID, time.Time) ([]core.Event, error) {
	return nil, nil
}

// newTree returns a tree with a bootstrapped workspace root.
func newTree(t *testing.T) (*tree.Tree, core.Node) {
	t.Helper()
	tr := tree.New(memStore{})
	root, err := tr.Bootstrap(context.Background(), "Workspace")
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	return tr, root
}

// mkNode creates a child node and fails the test on error.
func mkNode(t *testing.T, tr *tree.Tree, parent core.NodeID, kind core.Kind, title string) core.Node {
	t.Helper()
	n, err := tr.CreateNode(context.Background(), tree.CreateSpec{
		ParentID: parent,
		Kind:     kind,
		Title:    title,
		Driver:   "fake",
	})
	if err != nil {
		t.Fatalf("create node %q: %v", title, err)
	}
	return n
}

// fakeSpawner records Spawn/SendMessage calls and returns scripted results.
type fakeSpawner struct {
	mu       sync.Mutex
	spawns   []spawnCall
	messages []msgCall
	nextID   core.NodeID
	err      error
}

type spawnCall struct {
	parent core.NodeID
	req    SpawnRequest
}

type msgCall struct {
	from, to core.NodeID
	text     string
}

func (f *fakeSpawner) Spawn(_ context.Context, parent core.NodeID, req SpawnRequest) (core.NodeID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.spawns = append(f.spawns, spawnCall{parent: parent, req: req})
	if f.err != nil {
		return "", f.err
	}
	id := f.nextID
	if id == "" {
		id = core.NewNodeID()
	}
	return id, nil
}

func (f *fakeSpawner) SendMessage(_ context.Context, from, to core.NodeID, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.messages = append(f.messages, msgCall{from: from, to: to, text: text})
	return f.err
}

func (f *fakeSpawner) spawnCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.spawns)
}

func (f *fakeSpawner) messageCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.messages)
}

// testServer wires a Server over a fresh tree with a registered node and token.
type testServer struct {
	s    *Server
	tr   *tree.Tree
	reg  *Registry
	fake *fakeSpawner
}

func newTestServer(t *testing.T) (*testServer, core.Node) {
	t.Helper()
	tr, root := newTree(t)
	reg := NewRegistry()
	fake := &fakeSpawner{}
	s := New(Deps{Tree: tr, Tokens: reg, Spawner: fake, Version: "test"})
	return &testServer{s: s, tr: tr, reg: reg, fake: fake}, root
}

// session mints a token for node with role and returns the bound connSession.
func (ts *testServer) session(node core.NodeID, role Role) connSession {
	tok := ts.reg.Mint(node, role)
	return connSession{node: node, role: role, token: tok}
}

// call invokes a tool and returns the decoded JSON text result plus isError.
func (ts *testServer) call(t *testing.T, sess connSession, name string, args map[string]any) (map[string]any, bool, *rpcError) {
	t.Helper()
	argsRaw, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	params, err := json.Marshal(map[string]any{"name": name, "arguments": json.RawMessage(argsRaw)})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	raw, rerr := ts.s.callTool(context.Background(), sess, params)
	if rerr != nil {
		return nil, false, rerr
	}
	env, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("result is not an envelope: %T", raw)
	}
	isErr, _ := env["isError"].(bool)
	content, ok := env["content"].([]map[string]any)
	if !ok || len(content) == 0 {
		t.Fatalf("envelope has no content: %v", env)
	}
	text, _ := content[0]["text"].(string)
	var out map[string]any
	if text != "" {
		_ = json.Unmarshal([]byte(text), &out) // isError results may be plain text
		if out == nil {
			out = map[string]any{"_text": text}
		}
	}
	return out, isErr, nil
}
