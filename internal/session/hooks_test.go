package session

import (
	"errors"
	"testing"
	"time"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/driver"
	"github.com/AnkushinDaniil/grove/internal/testutil/fakeagent"
	"github.com/AnkushinDaniil/grove/internal/tree"
)

// hookFixture builds a manager and a tree carrying one running session applied
// directly (no process), so ApplyHook's tree effects are deterministic.
func hookFixture(t *testing.T) (*Manager, *tree.Tree, core.NodeID) {
	t.Helper()
	reg, err := driver.NewRegistry(fakeagent.NewDriver("/nonexistent", "/nonexistent"))
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	tr := tree.New(&recordStore{})
	ctx := t.Context()
	root, err := tr.Bootstrap(ctx, "ws")
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	proj, err := tr.CreateNode(ctx, tree.CreateSpec{
		ParentID: root.ID, Kind: core.KindProject, Title: "P", Driver: "fake",
	})
	if err != nil {
		t.Fatalf("CreateNode project: %v", err)
	}
	task, err := tr.CreateNode(ctx, tree.CreateSpec{ParentID: proj.ID, Kind: core.KindTask, Title: "t"})
	if err != nil {
		t.Fatalf("CreateNode task: %v", err)
	}
	sess := core.Session{
		ID: core.NewSessionID(), NodeID: task.ID, Driver: "fake",
		Mode: core.ModePTY, Status: core.SessionRunning, CWD: "/tmp/ws",
	}
	if _, err := tr.ApplySession(ctx, sess); err != nil {
		t.Fatalf("ApplySession: %v", err)
	}
	return NewManager(reg, tr, Config{}), tr, task.ID
}

func TestApplyHookNotification(t *testing.T) {
	tests := []struct {
		name       string
		payload    map[string]any
		wantAttn   core.Attention
		wantReason string
	}{
		{
			name:       "permission",
			payload:    map[string]any{"notification_type": "permission_prompt", "message": "Approve Bash(rm)?"},
			wantAttn:   core.AttentionPermission,
			wantReason: "Approve Bash(rm)?",
		},
		{
			name:       "question",
			payload:    map[string]any{"notification_type": "idle", "message": "Which file?"},
			wantAttn:   core.AttentionQuestion,
			wantReason: "Which file?",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, tr, node := hookFixture(t)
			if err := m.ApplyHook(t.Context(), node, "Notification", tt.payload); err != nil {
				t.Fatalf("ApplyHook: %v", err)
			}
			n, _ := tr.Get(node)
			if n.Attention != tt.wantAttn {
				t.Errorf("attention = %s, want %s", n.Attention, tt.wantAttn)
			}
			if n.AttentionReason != tt.wantReason {
				t.Errorf("attention reason = %q, want %q", n.AttentionReason, tt.wantReason)
			}
			if n.Status != core.StatusAwaitingInput {
				t.Errorf("node status = %s, want awaiting_input", n.Status)
			}
			if s, _ := tr.SessionFor(node); s.Status != core.SessionAwaitingInput {
				t.Errorf("session status = %s, want awaiting_input", s.Status)
			}
		})
	}
}

func TestApplyHookStop(t *testing.T) {
	m, tr, node := hookFixture(t)
	if err := m.ApplyHook(t.Context(), node, "Stop", map[string]any{}); err != nil {
		t.Fatalf("ApplyHook: %v", err)
	}
	n, _ := tr.Get(node)
	if n.Status != core.StatusAwaitingInput {
		t.Errorf("node status = %s, want awaiting_input", n.Status)
	}
	if n.Attention != core.AttentionDone || n.AttentionReason != "turn finished" {
		t.Errorf("attention = %s (%q), want done (turn finished)", n.Attention, n.AttentionReason)
	}
	if s, _ := tr.SessionFor(node); s.Status != core.SessionAwaitingInput {
		t.Errorf("session status = %s, want awaiting_input", s.Status)
	}
}

func TestApplyHookSessionStart(t *testing.T) {
	m, tr, node := hookFixture(t)
	if err := m.ApplyHook(t.Context(), node, "SessionStart", map[string]any{
		"session_id":      "claude-uuid-9",
		"transcript_path": "/tmp/claude.jsonl",
	}); err != nil {
		t.Fatalf("ApplyHook: %v", err)
	}
	s, _ := tr.SessionFor(node)
	if s.DriverSessionID != "claude-uuid-9" {
		t.Errorf("DriverSessionID = %q, want claude-uuid-9", s.DriverSessionID)
	}
	if s.TranscriptPath != "/tmp/claude.jsonl" {
		t.Errorf("TranscriptPath = %q, want /tmp/claude.jsonl", s.TranscriptPath)
	}
	if s.Status != core.SessionRunning {
		t.Errorf("SessionStart changed status to %s, want unchanged running", s.Status)
	}
}

func TestApplyHookSessionEnd(t *testing.T) {
	m, tr, node := hookFixture(t)
	_, ch, cancel := tr.Subscribe()
	defer cancel()

	// With a reason: an EventSessionEnded is appended; status is left to the exit path.
	if err := m.ApplyHook(t.Context(), node, "SessionEnd", map[string]any{"reason": "logout"}); err != nil {
		t.Fatalf("ApplyHook with reason: %v", err)
	}
	if !sawEvent(ch, core.EventSessionEnded, time.Second) {
		t.Fatal("no EventSessionEnded appended for SessionEnd with reason")
	}
	if n, _ := tr.Get(node); n.Status != core.StatusRunning {
		t.Errorf("node status = %s, want unchanged running", n.Status)
	}

	// Without a reason: a no-op.
	if err := m.ApplyHook(t.Context(), node, "SessionEnd", map[string]any{}); err != nil {
		t.Fatalf("ApplyHook without reason: %v", err)
	}
	if sawEvent(ch, core.EventSessionEnded, 200*time.Millisecond) {
		t.Fatal("SessionEnd without reason should be a no-op")
	}
}

func TestApplyHookErrors(t *testing.T) {
	m, _, node := hookFixture(t)
	if err := m.ApplyHook(t.Context(), node, "Frobnicate", nil); !errors.Is(err, core.ErrInvalid) {
		t.Errorf("unknown event = %v, want ErrInvalid", err)
	}
	if err := m.ApplyHook(t.Context(), "missing", "Stop", nil); !errors.Is(err, core.ErrInvalid) {
		t.Errorf("unknown node = %v, want ErrInvalid", err)
	}
}

// TestApplyHookLiveSession exercises the status transition against a real live
// session (the live branch of setSessionStatus).
func TestApplyHookLiveSession(t *testing.T) {
	m, tr, node := newFixture(t, Config{}, []fakeagent.Step{
		{Emit: `{"event":"session_started","payload":{"driver_session_id":"live-1"}}`},
		{WaitStdinLine: true},
		{ExitCode: new(0)},
	})
	if _, err := m.Start(t.Context(), node, core.ModeHeadless, "", ""); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitSession(t, tr, node, func(s core.Session) bool {
		return s.Status == core.SessionRunning && s.DriverSessionID == "live-1"
	})

	if err := m.ApplyHook(t.Context(), node, "Stop", map[string]any{}); err != nil {
		t.Fatalf("ApplyHook Stop: %v", err)
	}
	waitSession(t, tr, node, func(s core.Session) bool {
		return s.Status == core.SessionAwaitingInput
	})
	if n, _ := tr.Get(node); n.Status != core.StatusAwaitingInput {
		t.Errorf("node status = %s, want awaiting_input", n.Status)
	}
}

func sawEvent(ch <-chan tree.Delta, want core.EventType, within time.Duration) bool {
	deadline := time.After(within)
	for {
		select {
		case d, ok := <-ch:
			if !ok {
				return false
			}
			for _, e := range d.Events {
				if e.Type == want {
					return true
				}
			}
		case <-deadline:
			return false
		}
	}
}
