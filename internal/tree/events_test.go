package tree

import (
	"errors"
	"testing"

	"github.com/AnkushinDaniil/grove/internal/core"
)

func startedSession(node core.NodeID) core.Session {
	return core.Session{
		ID: core.NewSessionID(), NodeID: node, Driver: "claude",
		Mode: core.ModePTY, Status: core.SessionRunning, CWD: "/tmp/ws",
	}
}

func TestApplySessionDrivesNodeStatus(t *testing.T) {
	tr, root, _ := newTestTree(t)
	p := mustCreate(t, tr, CreateSpec{ParentID: root.ID, Kind: core.KindProject, Title: "P"})
	task := mustCreate(t, tr, CreateSpec{ParentID: p.ID, Kind: core.KindTask, Title: "t"})

	s := startedSession(task.ID)
	if _, err := tr.ApplySession(t.Context(), s); err != nil {
		t.Fatalf("ApplySession: %v", err)
	}
	if n, _ := tr.Get(task.ID); n.Status != core.StatusRunning || n.CurrentSessionID != s.ID {
		t.Fatalf("node after running session: %+v", n)
	}

	zero := 0
	s.Status = core.SessionExited
	s.ExitCode = &zero
	if _, err := tr.ApplySession(t.Context(), s); err != nil {
		t.Fatalf("ApplySession exited: %v", err)
	}
	if n, _ := tr.Get(task.ID); n.Status != core.StatusDone {
		t.Fatalf("node status = %s, want done", n.Status)
	}

	one := 1
	s2 := startedSession(task.ID)
	s2.Status = core.SessionExited
	s2.ExitCode = &one
	if _, err := tr.ApplySession(t.Context(), s2); err != nil {
		t.Fatalf("ApplySession failed-exit: %v", err)
	}
	if n, _ := tr.Get(task.ID); n.Status != core.StatusFailed {
		t.Fatalf("node status = %s, want failed", n.Status)
	}

	got, ok := tr.SessionFor(task.ID)
	if !ok || got.ID != s2.ID {
		t.Fatalf("SessionFor = (%+v, %v), want latest session", got, ok)
	}

	if _, err := tr.ApplySession(t.Context(), startedSession("missing")); !errors.Is(err, core.ErrInvalid) {
		t.Fatalf("ApplySession unknown node = %v, want ErrInvalid", err)
	}
}

func TestIngestEventsRaisesAttention(t *testing.T) {
	tr, root, fs := newTestTree(t)
	p := mustCreate(t, tr, CreateSpec{ParentID: root.ID, Kind: core.KindProject, Title: "P"})
	task := mustCreate(t, tr, CreateSpec{ParentID: p.ID, Kind: core.KindTask, Title: "t"})
	sid := core.NewSessionID()

	events, err := tr.IngestEvents(t.Context(), task.ID, sid, []core.EventInput{
		{Type: core.EventText, Payload: `{"text":"working"}`},
		{Type: core.EventAwaitingInput, Reason: core.AwaitPermission, Detail: "Bash needs approval"},
	})
	if err != nil {
		t.Fatalf("IngestEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
	if events[0].RequiresAttention || !events[1].RequiresAttention {
		t.Fatalf("RequiresAttention flags wrong: %+v", events)
	}
	n, _ := tr.Get(task.ID)
	if n.Attention != core.AttentionPermission || n.AttentionReason != "Bash needs approval" {
		t.Fatalf("attention = %s (%q), want permission", n.Attention, n.AttentionReason)
	}
	if n.AttentionSince.IsZero() {
		t.Fatal("AttentionSince not set")
	}
	if fs.events != 2 {
		t.Fatalf("persisted events = %d, want 2", fs.events)
	}
}

func TestAttentionPriorityAndAck(t *testing.T) {
	tr, root, _ := newTestTree(t)
	p := mustCreate(t, tr, CreateSpec{ParentID: root.ID, Kind: core.KindProject, Title: "P"})
	task := mustCreate(t, tr, CreateSpec{ParentID: p.ID, Kind: core.KindTask, Title: "t"})
	sid := core.NewSessionID()

	ingest := func(in core.EventInput) {
		t.Helper()
		if _, err := tr.IngestEvents(t.Context(), task.ID, sid, []core.EventInput{in}); err != nil {
			t.Fatalf("IngestEvents: %v", err)
		}
	}

	ingest(core.EventInput{Type: core.EventError, Detail: "boom"})
	// A lower-priority event must not downgrade the sticky error flag.
	ingest(core.EventInput{Type: core.EventTurnDone, Detail: "finished"})
	if n, _ := tr.Get(task.ID); n.Attention != core.AttentionError {
		t.Fatalf("attention downgraded to %s", n.Attention)
	}

	if _, err := tr.Ack(t.Context(), task.ID); err != nil {
		t.Fatalf("Ack: %v", err)
	}
	n, _ := tr.Get(task.ID)
	if n.Attention != core.AttentionNone || n.AttentionReason != "" || !n.AttentionSince.IsZero() {
		t.Fatalf("ack did not clear attention: %+v", n)
	}

	// After ack, a new lower-priority event raises attention again.
	ingest(core.EventInput{Type: core.EventTurnDone, Detail: "finished"})
	if n, _ := tr.Get(task.ID); n.Attention != core.AttentionDone {
		t.Fatalf("attention = %s, want done", n.Attention)
	}
}

func TestIngestRejectsUnknownTypeAndNode(t *testing.T) {
	tr, root, _ := newTestTree(t)
	p := mustCreate(t, tr, CreateSpec{ParentID: root.ID, Kind: core.KindProject, Title: "P"})

	if _, err := tr.IngestEvents(t.Context(), p.ID, "", []core.EventInput{{Type: "weird"}}); !errors.Is(err, core.ErrInvalid) {
		t.Fatalf("unknown type = %v, want ErrInvalid", err)
	}
	if _, err := tr.IngestEvents(t.Context(), "missing", "", []core.EventInput{{Type: core.EventText}}); !errors.Is(err, core.ErrInvalid) {
		t.Fatalf("unknown node = %v, want ErrInvalid", err)
	}
	if evs, err := tr.IngestEvents(t.Context(), p.ID, "", nil); err != nil || evs != nil {
		t.Fatalf("empty ingest = (%v, %v), want (nil, nil)", evs, err)
	}
}

func TestRollup(t *testing.T) {
	tr, root, _ := newTestTree(t)
	p := mustCreate(t, tr, CreateSpec{ParentID: root.ID, Kind: core.KindProject, Title: "P"})
	t1 := mustCreate(t, tr, CreateSpec{ParentID: p.ID, Kind: core.KindTask, Title: "t1"})
	t2 := mustCreate(t, tr, CreateSpec{ParentID: p.ID, Kind: core.KindTask, Title: "t2"})
	t3 := mustCreate(t, tr, CreateSpec{ParentID: t1.ID, Kind: core.KindTask, Title: "t3"})

	if _, err := tr.ApplySession(t.Context(), startedSession(t1.ID)); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.IngestEvents(t.Context(), t2.ID, "", []core.EventInput{
		{Type: core.EventError, Detail: "x"},
	}); err != nil {
		t.Fatal(err)
	}
	_ = t3

	r := tr.Rollup(p.ID)
	if r.Total != 3 {
		t.Fatalf("Total = %d, want 3", r.Total)
	}
	if r.ByStatus[core.StatusRunning] != 1 || r.ByStatus[core.StatusIdle] != 2 {
		t.Fatalf("ByStatus = %+v", r.ByStatus)
	}
	if r.Attention != 1 {
		t.Fatalf("Attention = %d, want 1", r.Attention)
	}
	if empty := tr.Rollup("missing"); empty.Total != 0 {
		t.Fatalf("Rollup(missing) = %+v", empty)
	}
}
