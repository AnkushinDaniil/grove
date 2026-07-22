package orch

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/driver"
	"github.com/AnkushinDaniil/grove/internal/mcpserv"
	"github.com/AnkushinDaniil/grove/internal/session"
	"github.com/AnkushinDaniil/grove/internal/tree"
)

// memStore is a no-op tree.Store.
type memStore struct{}

func (memStore) SaveNodes(context.Context, []core.Node) error       { return nil }
func (memStore) SaveSessions(context.Context, []core.Session) error { return nil }
func (memStore) AppendEvents(context.Context, []core.Event) error   { return nil }
func (memStore) AckNodeEvents(context.Context, core.NodeID, time.Time) ([]core.Event, error) {
	return nil, nil
}

func newTree(t *testing.T) (*tree.Tree, core.Node) {
	t.Helper()
	tr := tree.New(memStore{})
	root, err := tr.Bootstrap(context.Background(), "Workspace")
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	return tr, root
}

// startCall captures one Starter.Start invocation, including the effect of the
// applied launch options (MCP mount and briefing/prompt).
type startCall struct {
	node       core.NodeID
	mode       core.SessionMode
	prompt     string
	resumeID   string
	mcpMounted bool
}

// fakeStarter records Start calls and publishes them on a channel so tests can
// await asynchronous spawns and wakes.
type fakeStarter struct {
	mu    sync.Mutex
	calls []startCall
	ch    chan startCall
	err   error
}

func newFakeStarter() *fakeStarter {
	return &fakeStarter{ch: make(chan startCall, 16)}
}

func (f *fakeStarter) Start(_ context.Context, node core.NodeID, mode core.SessionMode, prompt, resumeID string, opts ...session.LaunchOption) (core.Session, error) {
	spec := driver.LaunchSpec{Mode: mode, Prompt: prompt, ResumeID: resumeID}
	for _, o := range opts {
		o(&spec)
	}
	call := startCall{node: node, mode: mode, prompt: spec.Prompt, resumeID: spec.ResumeID, mcpMounted: len(spec.MCP) > 0}
	f.mu.Lock()
	f.calls = append(f.calls, call)
	err := f.err
	f.mu.Unlock()
	if err != nil {
		return core.Session{}, err
	}
	f.ch <- call
	return core.Session{
		ID: core.NewSessionID(), NodeID: node, Driver: "fake",
		Mode: mode, Status: core.SessionRunning, CWD: "/tmp",
	}, nil
}

// await returns the next Start call for node within the timeout.
func (f *fakeStarter) await(t *testing.T, node core.NodeID, timeout time.Duration) startCall {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case c := <-f.ch:
			if c.node == node {
				return c
			}
		case <-deadline:
			t.Fatalf("no Start for node %s within %s", node, timeout)
		}
	}
}

// runScheduler builds a scheduler over tr and runs it until the test ends.
func runScheduler(t *testing.T, tr *tree.Tree, starter Starter, reg *mcpserv.Registry, opts func(*Deps)) *Scheduler {
	t.Helper()
	d := Deps{
		Tree:        tr,
		Starter:     starter,
		Tokens:      reg,
		SocketPath:  "/tmp/grove.sock",
		GroveBin:    "/usr/bin/grove",
		UrgentDelay: 10 * time.Millisecond,
	}
	if opts != nil {
		opts(&d)
	}
	s := New(d)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = s.Run(ctx)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("scheduler did not stop")
		}
	})
	// Give Run a moment to Subscribe before the test mutates the tree, so events
	// arrive as deltas rather than being missed between snapshot and subscribe.
	time.Sleep(20 * time.Millisecond)
	return s
}

// completeChild simulates a worker calling grove_complete: it records the
// completion in meta and raises the done/error attention, exactly as the MCP
// handler does.
func completeChild(t *testing.T, tr *tree.Tree, id core.NodeID, result, summary string) {
	t.Helper()
	meta := `{"completion":{"result":"` + result + `","summary":"` + summary + `"}}`
	if _, err := tr.UpdateNode(context.Background(), id, tree.Patch{Meta: &meta}); err != nil {
		t.Fatalf("update meta: %v", err)
	}
	evType := core.EventTurnDone
	var payload string
	if result == "failed" {
		evType = core.EventError
		payload, _ = core.MarshalPayload(core.ErrorPayload{Message: summary, Fatal: true})
	} else {
		payload, _ = core.MarshalPayload(core.TurnDonePayload{ResultText: summary})
	}
	if _, err := tr.IngestEvents(context.Background(), id, "", []core.EventInput{
		{Type: evType, Payload: payload, Detail: summary},
	}); err != nil {
		t.Fatalf("ingest completion: %v", err)
	}
}
