package ws

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/testutil/fakeagent"
	"github.com/AnkushinDaniil/grove/internal/tree"
)

// writeJSONFrame writes v as a JSON text frame.
func writeJSONFrame(t *testing.T, ctx context.Context, c *websocket.Conn, v any) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal frame: %v", err)
	}
	wctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := c.Write(wctx, websocket.MessageText, data); err != nil {
		t.Fatalf("write frame: %v", err)
	}
}

func TestTermLiveEchoResizeExit(t *testing.T) {
	h := newWSHarness(t, []fakeagent.Step{
		{Emit: "READY"},
		{WaitStdinLine: true}, // stay alive until the client sends a line
		{ExitCode: intPtr(0)},
	})
	sid := h.startPTYSession("")

	c := h.dial("/ws/term/" + string(sid))
	defer func() { _ = c.CloseNow() }()
	ctx := t.Context()

	// The first client frame is the initial resize (per the contract).
	writeJSONFrame(t, ctx, c, resizeMsg{T: "resize", Cols: 80, Rows: 24})

	var out []byte
	sentInput := false
	readCtx, cancel := context.WithTimeout(ctx, settleTimeout)
	defer cancel()
	for {
		typ, data, err := c.Read(readCtx)
		if err != nil {
			t.Fatalf("read: %v (accumulated %q)", err, out)
		}
		if typ == websocket.MessageBinary {
			out = append(out, data...)
			continue
		}
		var ctrl controlFrame
		if err := json.Unmarshal(data, &ctrl); err != nil {
			t.Fatalf("unmarshal control %q: %v", data, err)
		}
		switch ctrl.T {
		case "live":
			// Attached: send a line. The PTY echoes it and the agent then exits.
			if !sentInput {
				if err := c.Write(readCtx, websocket.MessageBinary, []byte("hello\n")); err != nil {
					t.Fatalf("write input: %v", err)
				}
				sentInput = true
			}
		case "exit":
			if ctrl.Code != 0 {
				t.Errorf("exit code = %d, want 0", ctrl.Code)
			}
			if !bytes.Contains(out, []byte("READY")) {
				t.Errorf("terminal output missing initial READY: %q", out)
			}
			if !bytes.Contains(out, []byte("hello")) {
				t.Errorf("terminal output missing echoed input: %q", out)
			}
			return
		}
	}
}

func TestTermReplayFinishedSession(t *testing.T) {
	h := newWSHarness(t, nil)
	ctx := t.Context()

	proj, err := h.tree.CreateNode(ctx, tree.CreateSpec{
		ParentID: h.root.ID, Kind: core.KindProject, Title: "P", Driver: "fake",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	task, err := h.tree.CreateNode(ctx, tree.CreateSpec{ParentID: proj.ID, Kind: core.KindTask, Title: "T"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	// A finished (exited) session with a persisted scrollback file, but no live
	// handle in the manager, so the term endpoint serves replay-only.
	sid := core.NewSessionID()
	if err := os.WriteFile(filepath.Join(h.scrollback, string(sid)+".bin"), []byte("previous output\r\n"), 0o600); err != nil {
		t.Fatalf("write scrollback: %v", err)
	}
	code := 0
	if _, err := h.tree.ApplySession(ctx, core.Session{
		ID: sid, NodeID: task.ID, Driver: "fake", Mode: core.ModePTY,
		Status: core.SessionExited, ExitCode: &code, CWD: t.TempDir(),
		StartedAt: time.Now(), EndedAt: time.Now(),
	}); err != nil {
		t.Fatalf("ApplySession: %v", err)
	}

	c := h.dial("/ws/term/" + string(sid))
	defer func() { _ = c.CloseNow() }()

	var out []byte
	readCtx, cancel := context.WithTimeout(ctx, settleTimeout)
	defer cancel()
	for {
		typ, data, err := c.Read(readCtx)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if typ == websocket.MessageBinary {
			out = append(out, data...)
			continue
		}
		var ctrl controlFrame
		if err := json.Unmarshal(data, &ctrl); err != nil {
			t.Fatalf("unmarshal control %q: %v", data, err)
		}
		if ctrl.T == "exit" {
			if ctrl.Code != 0 {
				t.Errorf("replay exit code = %d, want 0", ctrl.Code)
			}
			if !bytes.Contains(out, []byte("previous output")) {
				t.Errorf("replay missing scrollback bytes: %q", out)
			}
			return
		}
	}
}
