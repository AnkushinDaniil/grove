package memory

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/AnkushinDaniil/grove/internal/core"
)

// This file provides a hermetic fake MemPalace MCP server that answers
// tools/call (search, add_drawer, delete_by_source), so the Client's stdio
// exchange, result parsing, scope filtering and spooling can be tested end to
// end without a real palace. It is separate from fakes_test.go's CLI fake (which
// only serves the phase-1 initialize/tools/list probe) to keep those tests
// untouched. buildProg and copyBin are reused from fakes_test.go.

var (
	callFakeOnce sync.Once
	callFakeBin  string
	callFakeErr  error
)

// buildCallFake compiles the tools/call fake once per test binary run.
func buildCallFake(t testing.TB) string {
	t.Helper()
	callFakeOnce.Do(func() {
		dir, err := os.MkdirTemp("", "memcallfake-*")
		if err != nil {
			callFakeErr = err
			return
		}
		callFakeBin, callFakeErr = buildProg(dir, "mempalace-mcp", fakeCallSrc)
	})
	if callFakeErr != nil {
		t.Fatalf("buildCallFake: %v", callFakeErr)
	}
	return callFakeBin
}

// callFake is a per-test bin dir holding the tools/call fake as `mempalace-mcp`,
// plus a scratch home. Control files next to the binary steer its responses.
type callFake struct {
	binDir string
	home   string
}

// newCallFake installs the fake MCP server on a scratch PATH.
func newCallFake(t *testing.T) callFake {
	t.Helper()
	bin := buildCallFake(t)
	cf := callFake{binDir: t.TempDir(), home: t.TempDir()}
	copyBin(t, bin, filepath.Join(cf.binDir, MCPBinaryName))
	return cf
}

func (cf callFake) env() Env { return Env{PATH: cf.binDir, Home: cf.home} }

// setSearch pins the raw JSON payload the fake returns from mempalace_search.
func (cf callFake) setSearch(t *testing.T, payload string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(cf.binDir, "searchout"), []byte(payload), 0o600); err != nil {
		t.Fatalf("write searchout: %v", err)
	}
}

// drawers returns the JSON arguments of every add_drawer the fake recorded.
func (cf callFake) drawers(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(cf.binDir, "drawers.jsonl"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("read drawers: %v", err)
	}
	var out []string
	for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

// fakeTree is a trivial in-memory Tree for scope resolution tests.
type fakeTree struct {
	nodes map[core.NodeID]core.Node
	kids  map[core.NodeID][]core.NodeID
}

func newFakeTree() *fakeTree {
	return &fakeTree{nodes: map[core.NodeID]core.Node{}, kids: map[core.NodeID][]core.NodeID{}}
}

// add inserts a node and links it under its parent.
func (f *fakeTree) add(id, parent core.NodeID, kind core.Kind, title string) {
	f.nodes[id] = core.Node{ID: id, ParentID: parent, Kind: kind, Title: title}
	if parent != "" {
		f.kids[parent] = append(f.kids[parent], id)
	}
}

func (f *fakeTree) Get(id core.NodeID) (core.Node, bool) { n, ok := f.nodes[id]; return n, ok }

func (f *fakeTree) SubtreeIDs(id core.NodeID) []core.NodeID {
	if _, ok := f.nodes[id]; !ok {
		return nil
	}
	out := []core.NodeID{id}
	for _, c := range f.kids[id] {
		out = append(out, f.SubtreeIDs(c)...)
	}
	return out
}

// fakeCallSrc is a stand-in MemPalace MCP server: it answers initialize and
// tools/call over newline JSON-RPC. add_drawer appends its arguments to
// drawers.jsonl; search returns the searchout control file (or a default
// grove-headered result echoing the query); delete_by_source acks.
const fakeCallSrc = `package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func ctlDir() string {
	p, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(p)
}

func main() {
	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		var m map[string]any
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			continue
		}
		id, hasID := m["id"]
		if !hasID {
			continue
		}
		method, _ := m["method"].(string)
		switch method {
		case "initialize":
			writeResult(out, id, map[string]any{
				"protocolVersion": "2025-06-18",
				"serverInfo":      map[string]any{"name": "mempalace", "version": "3.6.0"},
				"capabilities":    map[string]any{"tools": map[string]any{}},
			})
		case "tools/call":
			params, _ := m["params"].(map[string]any)
			name, _ := params["name"].(string)
			args, _ := params["arguments"].(map[string]any)
			handleCall(out, id, name, args)
		default:
			writeErr(out, id, -32601, "method not found: "+method)
		}
		out.Flush()
	}
}

func handleCall(out *bufio.Writer, id any, name string, args map[string]any) {
	switch name {
	case "mempalace_add_drawer":
		b, _ := json.Marshal(args)
		if f, err := os.OpenFile(filepath.Join(ctlDir(), "drawers.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
			fmt.Fprintf(f, "%s\n", b)
			_ = f.Close()
		}
		textResult(out, id, "{\"success\":true,\"drawer_id\":\"fake-1\"}")
	case "mempalace_search":
		textResult(out, id, searchText(args))
	case "mempalace_delete_by_source":
		textResult(out, id, "{\"success\":true,\"deleted\":1}")
	default:
		writeErr(out, id, -32601, "unknown tool: "+name)
	}
}

func searchText(args map[string]any) string {
	if b, err := os.ReadFile(filepath.Join(ctlDir(), "searchout")); err == nil {
		return string(b)
	}
	q, _ := args["query"].(string)
	res := map[string]any{"results": []map[string]any{{
		"text":        "grove:auto:fact\n" + q,
		"wing":        args["wing"],
		"room":        "room-default",
		"source_path": "grove://node/room-default",
		"created_at":  "2026-07-22T00:00:00",
	}}}
	b, _ := json.Marshal(res)
	return string(b)
}

func textResult(out *bufio.Writer, id any, text string) {
	writeResult(out, id, map[string]any{"content": []map[string]any{{"type": "text", "text": text}}})
}

func writeResult(w *bufio.Writer, id, result any) {
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
	_, _ = w.Write(append(b, '\n'))
}

func writeErr(w *bufio.Writer, id any, code int, msg string) {
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": msg}})
	_, _ = w.Write(append(b, '\n'))
}
`
