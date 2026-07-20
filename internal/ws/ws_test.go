package ws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/AnkushinDaniil/grove/internal/api"
	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/driver"
	"github.com/AnkushinDaniil/grove/internal/session"
	"github.com/AnkushinDaniil/grove/internal/store"
	"github.com/AnkushinDaniil/grove/internal/testutil/fakeagent"
	"github.com/AnkushinDaniil/grove/internal/tree"
)

// wsFrame captures either a /ws/state hello or delta frame for assertions.
type wsFrame struct {
	T        string           `json:"t"`
	Rev      uint64           `json:"rev"`
	Nodes    []api.NodeDTO    `json:"nodes"`
	Sessions []api.SessionDTO `json:"sessions"`
	Events   []api.EventDTO   `json:"events"`
	Inbox    []api.EventDTO   `json:"inbox"`
}

// controlFrame captures a /ws/term text control frame.
type controlFrame struct {
	T    string `json:"t"`
	Code int    `json:"code"`
}

// settleTimeout bounds waits for out-of-band terminal/process progress.
const settleTimeout = 10 * time.Second

// wsHarness is a wired ws stack over real components served through httptest.
type wsHarness struct {
	t          *testing.T
	store      *store.Store
	tree       *tree.Tree
	mgr        *session.Manager
	ts         *httptest.Server
	scrollback string
	root       core.Node
}

func newWSHarness(t *testing.T, script []fakeagent.Step) *wsHarness {
	t.Helper()

	st, err := store.Open(t.Context(), t.TempDir()+"/grove.db")
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
	scrollback := t.TempDir()
	mgr := session.NewManager(reg, tr, session.Config{ScrollbackDir: scrollback})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = mgr.Shutdown(ctx)
	})

	h := New(Config{Tree: tr, Sessions: mgr, Store: st, ScrollbackDir: scrollback})
	ts := httptest.NewServer(h.Routes())
	t.Cleanup(ts.Close)

	return &wsHarness{t: t, store: st, tree: tr, mgr: mgr, ts: ts, scrollback: scrollback, root: root}
}

// dial opens a WebSocket to the given path on the harness server.
func (h *wsHarness) dial(path string) *websocket.Conn {
	h.t.Helper()
	url := "ws" + strings.TrimPrefix(h.ts.URL, "http") + path
	ctx, cancel := context.WithTimeout(h.t.Context(), 5*time.Second)
	defer cancel()
	c, resp, err := websocket.Dial(ctx, url, &websocket.DialOptions{HTTPClient: h.ts.Client()})
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		h.t.Fatalf("dial %s: %v", path, err)
	}
	return c
}

// readFrame reads one text frame and unmarshals it into v.
func readFrame(t *testing.T, ctx context.Context, c *websocket.Conn, v any) {
	t.Helper()
	rctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	typ, data, err := c.Read(rctx)
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	if typ != websocket.MessageText {
		t.Fatalf("frame type = %v, want text", typ)
	}
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("unmarshal frame %q: %v", data, err)
	}
}

func containsNode(nodes []api.NodeDTO, id string) bool {
	for _, n := range nodes {
		if n.ID == id {
			return true
		}
	}
	return false
}

func intPtr(i int) *int { return &i }

func TestStateHelloAndDelta(t *testing.T) {
	h := newWSHarness(t, nil)
	c := h.dial("/ws/state")
	defer func() { _ = c.CloseNow() }()
	ctx := t.Context()

	var hello wsFrame
	readFrame(t, ctx, c, &hello)
	if hello.T != "hello" {
		t.Fatalf("first frame t = %q, want hello", hello.T)
	}
	if !containsNode(hello.Nodes, string(h.root.ID)) {
		t.Error("hello nodes missing the root workspace")
	}

	proj, err := h.tree.CreateNode(ctx, tree.CreateSpec{
		ParentID: h.root.ID, Kind: core.KindProject, Title: "P", Driver: "fake",
	})
	if err != nil {
		t.Fatalf("CreateNode: %v", err)
	}

	var delta wsFrame
	readFrame(t, ctx, c, &delta)
	if delta.T != "delta" {
		t.Fatalf("second frame t = %q, want delta", delta.T)
	}
	if delta.Rev != hello.Rev+1 {
		t.Errorf("delta rev = %d, want %d (consecutive)", delta.Rev, hello.Rev+1)
	}
	if !containsNode(delta.Nodes, string(proj.ID)) {
		t.Error("delta nodes missing the created project")
	}
}

func TestStateLagDropClosesSocket(t *testing.T) {
	h := newWSHarness(t, nil)
	c := h.dial("/ws/state")
	defer func() { _ = c.CloseNow() }()
	ctx := t.Context()

	var hello wsFrame
	readFrame(t, ctx, c, &hello)

	// Flood the tree without reading: the subscriber buffer (or the per-frame
	// write timeout) trips, the server drops the subscription and closes.
	for i := range 2000 {
		if _, err := h.tree.CreateNode(ctx, tree.CreateSpec{
			ParentID: h.root.ID, Kind: core.KindProject, Title: fmt.Sprintf("P%d", i), Driver: "fake",
		}); err != nil {
			t.Fatalf("flood mutate %d: %v", i, err)
		}
	}

	readCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	for {
		_, _, err := c.Read(readCtx)
		if err == nil {
			continue // drain buffered frames until the close arrives
		}
		if errors.Is(err, context.DeadlineExceeded) {
			t.Fatal("socket stayed open under flood; expected a lag-drop close")
		}
		return // server closed the lagging socket, as expected
	}
}

// startPTYSession creates a fake-driver task with a workspace and starts a PTY
// session, returning its id.
func (h *wsHarness) startPTYSession(prompt string) core.SessionID {
	h.t.Helper()
	ctx := h.t.Context()
	proj, err := h.tree.CreateNode(ctx, tree.CreateSpec{
		ParentID: h.root.ID, Kind: core.KindProject, Title: "P", Driver: "fake",
	})
	if err != nil {
		h.t.Fatalf("create project: %v", err)
	}
	task, err := h.tree.CreateNode(ctx, tree.CreateSpec{ParentID: proj.ID, Kind: core.KindTask, Title: "T"})
	if err != nil {
		h.t.Fatalf("create task: %v", err)
	}
	if _, err := h.tree.SetWorkspaceDir(ctx, task.ID, h.t.TempDir()); err != nil {
		h.t.Fatalf("SetWorkspaceDir: %v", err)
	}
	sess, err := h.mgr.Start(ctx, task.ID, core.ModePTY, prompt, "")
	if err != nil {
		h.t.Fatalf("Start PTY: %v", err)
	}
	return sess.ID
}
