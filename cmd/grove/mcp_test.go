package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestBridgeMCP is a loopback test of the stdio<->UDS shim: a fake daemon reads
// the auth preamble and one JSON-RPC request, replies, and the reply must reach
// the bridge's stdout.
func TestBridgeMCP(t *testing.T) {
	dir, err := os.MkdirTemp("", "gb")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "s")

	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	var (
		gotAuth   map[string]string
		gotMethod string
		srvErr    error
		wg        sync.WaitGroup
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, aerr := ln.Accept()
		if aerr != nil {
			srvErr = aerr
			return
		}
		defer func() { _ = conn.Close() }()
		r := bufio.NewReader(conn)

		authLine, _ := r.ReadBytes('\n')
		if e := json.Unmarshal(bytes.TrimSpace(authLine), &gotAuth); e != nil {
			srvErr = e
			return
		}
		reqLine, _ := r.ReadBytes('\n')
		var req struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		if e := json.Unmarshal(bytes.TrimSpace(reqLine), &req); e != nil {
			srvErr = e
			return
		}
		gotMethod = req.Method
		resp, _ := json.Marshal(map[string]any{
			"jsonrpc": "2.0", "id": req.ID,
			"result": map[string]any{"serverInfo": map[string]any{"name": "grove"}},
		})
		_, _ = conn.Write(append(resp, '\n'))
	}()

	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize"}` + "\n")
	var out bytes.Buffer
	if err := bridgeMCP(sock, "node-1", "tok-1", in, &out); err != nil {
		t.Fatalf("bridge: %v", err)
	}

	waitTimeout(t, &wg, 2*time.Second)
	if srvErr != nil {
		t.Fatalf("server: %v", srvErr)
	}
	if gotAuth["grove_node"] != "node-1" || gotAuth["grove_token"] != "tok-1" {
		t.Errorf("auth preamble = %v, want node-1/tok-1", gotAuth)
	}
	if gotMethod != "initialize" {
		t.Errorf("forwarded method = %q, want initialize", gotMethod)
	}
	if !strings.Contains(out.String(), `"grove"`) {
		t.Errorf("bridge stdout missing daemon response: %q", out.String())
	}
}

func TestBridgeMCPDialFailure(t *testing.T) {
	err := bridgeMCP(filepath.Join(t.TempDir(), "nonexistent.sock"), "n", "t", strings.NewReader(""), &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected a dial error for a missing socket")
	}
}

func waitTimeout(t *testing.T, wg *sync.WaitGroup, d time.Duration) {
	t.Helper()
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatal("server goroutine did not finish")
	}
}
