package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/mcpserv"
	"github.com/AnkushinDaniil/grove/internal/tree"
)

// memStore is a no-op tree.Store for the integration harness.
type memStore struct{}

func (memStore) SaveNodes(context.Context, []core.Node) error       { return nil }
func (memStore) SaveSessions(context.Context, []core.Session) error { return nil }
func (memStore) AppendEvents(context.Context, []core.Event) error   { return nil }
func (memStore) AckNodeEvents(context.Context, core.NodeID, time.Time) ([]core.Event, error) {
	return nil, nil
}

// TestGroveMCPEndToEnd drives the real `grove mcp` binary against a live
// in-process MCP server over a Unix socket, exactly as a spawned CLI would:
// initialize, then tools/list. It proves the whole transport (auth preamble →
// bridge → daemon dispatch) end to end and is the source of the tools/list
// evidence in the report.
func TestGroveMCPEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the grove binary; skipped under -short")
	}

	// Build the grove binary.
	dir := t.TempDir()
	bin := filepath.Join(dir, "grove")
	build := exec.Command("go", "build", "-o", bin, "github.com/AnkushinDaniil/grove/cmd/grove")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build grove: %v\n%s", err, out)
	}

	// Stand up a tree with an orchestrator project node and mint its token.
	tr := tree.New(memStore{})
	root, err := tr.Bootstrap(context.Background(), "Workspace")
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	proj, err := tr.CreateNode(context.Background(), tree.CreateSpec{
		ParentID: root.ID, Kind: core.KindProject, Title: "API", Driver: "claude",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	reg := mcpserv.NewRegistry()
	token := reg.Mint(proj.ID, mcpserv.RoleOrchestrator)

	// Serve the MCP protocol on a short-pathed socket.
	sockDir, err := os.MkdirTemp("", "gm")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(sockDir) })
	socket := filepath.Join(sockDir, "s")
	ln, err := mcpserv.Listen(context.Background(), socket)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := mcpserv.New(mcpserv.Deps{Tree: tr, Tokens: reg, Spawner: noopSpawner{}, Version: "itest"})
	serveCtx, cancelServe := context.WithCancel(context.Background())
	defer cancelServe()
	go func() { _ = srv.Serve(serveCtx, ln) }()

	// Run the real `grove mcp` shim, piping two JSON-RPC requests.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "mcp")
	cmd.Env = append(os.Environ(),
		"GROVE_SOCKET="+socket,
		"GROVE_NODE_ID="+string(proj.ID),
		"GROVE_NODE_TOKEN="+token,
	)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start grove mcp: %v", err)
	}
	defer func() { _ = cmd.Process.Kill() }()

	writeLine(t, stdin, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{}})
	writeLine(t, stdin, map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/list", "params": map[string]any{}})

	tools := readToolsList(t, stdout)
	_ = stdin.Close()

	names := map[string]bool{}
	for _, name := range tools {
		names[name] = true
	}
	t.Logf("grove mcp tools/list returned %d tools: %v", len(tools), tools)
	for _, want := range []string{"grove_get_context", "grove_report_progress", "grove_complete", "grove_spawn_child", "grove_list_children", "grove_node_status"} {
		if !names[want] {
			t.Errorf("tools/list missing %q", want)
		}
	}
}

func writeLine(t *testing.T, w io.Writer, v any) {
	t.Helper()
	line, _ := json.Marshal(v)
	if _, err := w.Write(append(line, '\n')); err != nil {
		t.Fatalf("write request: %v", err)
	}
}

// readToolsList scans the shim's stdout for the tools/list (id 2) response and
// returns the tool names.
func readToolsList(t *testing.T, r io.Reader) []string {
	t.Helper()
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var resp struct {
			ID     int `json:"id"`
			Result struct {
				Tools []struct {
					Name string `json:"name"`
				} `json:"tools"`
			} `json:"result"`
		}
		if json.Unmarshal(scanner.Bytes(), &resp) != nil || resp.ID != 2 {
			continue
		}
		names := make([]string, 0, len(resp.Result.Tools))
		for _, tool := range resp.Result.Tools {
			names = append(names, tool.Name)
		}
		return names
	}
	t.Fatal("did not receive a tools/list response")
	return nil
}

// noopSpawner satisfies mcpserv.Spawner for a protocol-only harness.
type noopSpawner struct{}

func (noopSpawner) Spawn(context.Context, core.NodeID, mcpserv.SpawnRequest) (core.NodeID, error) {
	return core.NewNodeID(), nil
}
func (noopSpawner) SendMessage(context.Context, core.NodeID, core.NodeID, string) error { return nil }
