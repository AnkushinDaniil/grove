package mcpserv

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/AnkushinDaniil/grove/internal/core"
)

// startServer runs a Server on a short-pathed temp Unix socket and returns the
// socket path; it is torn down when the test ends.
func startServer(t *testing.T, ts *testServer) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "gm")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "s")

	ln, err := Listen(context.Background(), sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	// Verify the socket is owner-only.
	if info, err := os.Stat(sock); err != nil {
		t.Fatalf("stat socket: %v", err)
	} else if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("socket perms = %o, want 600", perm)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = ts.s.Serve(ctx, ln)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("server did not shut down")
		}
	})
	return sock
}

// rpcConn is a JSON-RPC client over the socket, used for integration tests.
type rpcConn struct {
	t    *testing.T
	conn net.Conn
	r    *bufio.Reader
	id   int
}

func dial(t *testing.T, sock string, node core.NodeID, token string) *rpcConn {
	t.Helper()
	conn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	preamble, _ := json.Marshal(map[string]string{"grove_node": string(node), "grove_token": token})
	if _, err := conn.Write(append(preamble, '\n')); err != nil {
		t.Fatalf("write preamble: %v", err)
	}
	return &rpcConn{t: t, conn: conn, r: bufio.NewReader(conn)}
}

// call sends a request and decodes the matching response.
func (c *rpcConn) call(method string, params any) (json.RawMessage, *rpcError) {
	c.t.Helper()
	c.id++
	req := map[string]any{"jsonrpc": "2.0", "id": c.id, "method": method}
	if params != nil {
		req["params"] = params
	}
	line, _ := json.Marshal(req)
	if _, err := c.conn.Write(append(line, '\n')); err != nil {
		c.t.Fatalf("write %s: %v", method, err)
	}
	_ = c.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	resp, err := c.r.ReadBytes('\n')
	if err != nil {
		c.t.Fatalf("read %s response: %v", method, err)
	}
	var out struct {
		Result json.RawMessage `json:"result"`
		Error  *rpcError       `json:"error"`
	}
	if err := json.Unmarshal(resp, &out); err != nil {
		c.t.Fatalf("decode %s response: %v (raw %s)", method, err, resp)
	}
	return out.Result, out.Error
}

func TestServeInitializeAndToolsList(t *testing.T) {
	ts, root := newTestServer(t)
	p, _, _ := orchestratorSubtree(t, ts, root)
	tok := ts.reg.Mint(p.ID, RoleOrchestrator)
	sock := startServer(t, ts)

	c := dial(t, sock, p.ID, tok)

	// initialize
	res, rerr := c.call("initialize", map[string]any{"protocolVersion": mcpProtocolVersion})
	if rerr != nil {
		t.Fatalf("initialize error: %v", rerr)
	}
	var initRes struct {
		ProtocolVersion string `json:"protocolVersion"`
		ServerInfo      struct {
			Name string `json:"name"`
		} `json:"serverInfo"`
		Instructions string `json:"instructions"`
	}
	if err := json.Unmarshal(res, &initRes); err != nil {
		t.Fatalf("decode initialize: %v", err)
	}
	if initRes.ServerInfo.Name != "grove" {
		t.Errorf("server name = %q, want grove", initRes.ServerInfo.Name)
	}
	if initRes.Instructions == "" {
		t.Error("initialize should carry protocol instructions")
	}

	// tools/list for an orchestrator: worker tools + orchestrator tools.
	res, rerr = c.call("tools/list", map[string]any{})
	if rerr != nil {
		t.Fatalf("tools/list error: %v", rerr)
	}
	var listRes struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(res, &listRes); err != nil {
		t.Fatalf("decode tools/list: %v", err)
	}
	names := map[string]bool{}
	for _, tool := range listRes.Tools {
		names[tool.Name] = true
	}
	t.Logf("orchestrator tools/list: %d tools %v", len(listRes.Tools), keys(names))
	for _, want := range []string{toolGetContext, toolReportProgress, toolComplete, toolSpawnChild, toolListChildren, toolNodeStatus} {
		if !names[want] {
			t.Errorf("tools/list missing %q", want)
		}
	}
	if len(listRes.Tools) != len(workerTools())+len(orchestratorTools()) {
		t.Errorf("orchestrator tool count = %d, want %d", len(listRes.Tools), len(workerTools())+len(orchestratorTools()))
	}
}

func TestServeToolsCallGetContext(t *testing.T) {
	ts, root := newTestServer(t)
	p, _, _ := orchestratorSubtree(t, ts, root)
	tok := ts.reg.Mint(p.ID, RoleOrchestrator)
	sock := startServer(t, ts)
	c := dial(t, sock, p.ID, tok)

	res, rerr := c.call("tools/call", map[string]any{"name": toolGetContext, "arguments": map[string]any{}})
	if rerr != nil {
		t.Fatalf("tools/call error: %v", rerr)
	}
	var callRes struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(res, &callRes); err != nil {
		t.Fatalf("decode tools/call: %v", err)
	}
	if len(callRes.Content) == 0 {
		t.Fatal("tools/call returned no content")
	}
	var ctxInfo struct {
		NodeID string `json:"node_id"`
		Role   string `json:"role"`
	}
	if err := json.Unmarshal([]byte(callRes.Content[0].Text), &ctxInfo); err != nil {
		t.Fatalf("decode context text: %v", err)
	}
	if ctxInfo.NodeID != string(p.ID) {
		t.Errorf("context node_id = %q, want %s", ctxInfo.NodeID, p.ID)
	}
}

func TestServeWorkerCannotSpawn(t *testing.T) {
	ts, root := newTestServer(t)
	_, a, _ := orchestratorSubtree(t, ts, root)
	tok := ts.reg.Mint(a.ID, RoleWorker)
	sock := startServer(t, ts)
	c := dial(t, sock, a.ID, tok)

	// Worker's tools/list must not advertise spawn.
	res, _ := c.call("tools/list", map[string]any{})
	var listRes struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	_ = json.Unmarshal(res, &listRes)
	if len(listRes.Tools) != len(workerTools()) {
		t.Errorf("worker tool count = %d, want %d", len(listRes.Tools), len(workerTools()))
	}

	// And calling it is rejected.
	_, rerr := c.call("tools/call", map[string]any{"name": toolSpawnChild, "arguments": map[string]any{"title": "x", "prompt": "y"}})
	if rerr == nil {
		t.Fatal("worker spawn_child should be rejected")
	}
}

func TestServeRejectsBadToken(t *testing.T) {
	ts, root := newTestServer(t)
	p, _, _ := orchestratorSubtree(t, ts, root)
	ts.reg.Mint(p.ID, RoleOrchestrator)
	sock := startServer(t, ts)

	// Wrong token: the server closes the connection without serving.
	conn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()
	preamble, _ := json.Marshal(map[string]string{"grove_node": string(p.ID), "grove_token": "not-the-token"})
	_, _ = conn.Write(append(preamble, '\n'))

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1)
	if _, err := conn.Read(buf); err == nil {
		t.Fatal("expected the connection to be closed on bad auth")
	}
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
