package notify

import (
	"context"
	"testing"
	"time"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/tree"
)

func TestNopSinkDoesNotPanic(t *testing.T) {
	NopSink{}.Notify(Notification{Body: "ignored"})
}

// TestNotificationForPolicy pins the v1 notify/sound policy per attention kind.
func TestNotificationForPolicy(t *testing.T) {
	tests := []struct {
		attention core.Attention
		wantNotif bool
		wantSound bool
	}{
		{core.AttentionPermission, true, true},
		{core.AttentionQuestion, true, true},
		{core.AttentionError, true, true},
		{core.AttentionDone, true, false},
		{core.AttentionReview, false, false},
		{core.AttentionNone, false, false},
	}
	for _, tt := range tests {
		t.Run(string(tt.attention), func(t *testing.T) {
			node := core.Node{
				ID:              "node-1",
				Title:           "Fix bug",
				Attention:       tt.attention,
				AttentionReason: "why",
			}
			got, ok := notificationFor(node, "http://127.0.0.1:7433")
			if ok != tt.wantNotif {
				t.Fatalf("notificationFor ok = %v, want %v", ok, tt.wantNotif)
			}
			if !ok {
				return
			}
			if got.Sound != tt.wantSound {
				t.Errorf("Sound = %v, want %v", got.Sound, tt.wantSound)
			}
			if got.Title != "grove: Fix bug" {
				t.Errorf("Title = %q, want %q", got.Title, "grove: Fix bug")
			}
			if got.Subtitle != string(tt.attention) {
				t.Errorf("Subtitle = %q, want %q", got.Subtitle, tt.attention)
			}
			if got.Body != "why" {
				t.Errorf("Body = %q, want %q", got.Body, "why")
			}
			if got.URL != "http://127.0.0.1:7433/n/node-1" {
				t.Errorf("URL = %q, want %q", got.URL, "http://127.0.0.1:7433/n/node-1")
			}
		})
	}
}

func TestNodeURLTrimsTrailingSlash(t *testing.T) {
	if got := nodeURL("http://127.0.0.1:7433/", "abc"); got != "http://127.0.0.1:7433/n/abc" {
		t.Errorf("nodeURL = %q, want %q", got, "http://127.0.0.1:7433/n/abc")
	}
	if got := nodeURL("", "abc"); got != "" {
		t.Errorf("nodeURL(empty) = %q, want empty", got)
	}
}

// nopStore is a tree.Store that persists nothing, mirroring the recordStore
// pattern used elsewhere; the runner test only needs in-memory tree state.
type nopStore struct{}

func (nopStore) SaveNodes(context.Context, []core.Node) error       { return nil }
func (nopStore) SaveSessions(context.Context, []core.Session) error { return nil }
func (nopStore) AppendEvents(context.Context, []core.Event) error   { return nil }

// TestRunnerDispatchTransitions exercises the runner's transition tracking
// directly (no goroutine), which is the load-bearing logic: it fires on a change
// to a non-none attention and stays quiet on a redundant re-report.
func TestRunnerDispatchTransitions(t *testing.T) {
	sink := &fakeSink{}
	r := NewRunner(nil, sink, "http://127.0.0.1:7433", nil)
	prev := map[core.NodeID]core.Attention{}

	node := func(a core.Attention) core.Node {
		return core.Node{ID: "n1", Title: "T", Attention: a, AttentionReason: "r"}
	}
	feed := func(a core.Attention) { r.dispatch(tree.Delta{Nodes: []core.Node{node(a)}}, prev) }

	feed(core.AttentionPermission) // none → permission: fire
	feed(core.AttentionPermission) // permission → permission: quiet
	feed(core.AttentionError)      // permission → error: fire
	feed(core.AttentionNone)       // error → none: quiet (none never fires)
	feed(core.AttentionDone)       // none → done: fire (silent)

	got := sink.all()
	if len(got) != 3 {
		t.Fatalf("notifications = %d, want 3: %+v", len(got), got)
	}
	if !got[0].Sound || !got[1].Sound {
		t.Errorf("permission/error banners should sound: %+v", got[:2])
	}
	if got[2].Sound {
		t.Errorf("done banner should be silent: %+v", got[2])
	}
}

// TestRunnerRunNotifiesLive exercises the full Run loop over a real tree. It
// toggles attention until the subscription (established asynchronously inside
// Run) observes a transition, so it is robust to subscribe/raise ordering.
func TestRunnerRunNotifiesLive(t *testing.T) {
	tr := tree.New(nopStore{})
	ctx := t.Context()
	root, err := tr.Bootstrap(ctx, "ws")
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	proj, err := tr.CreateNode(ctx, tree.CreateSpec{ParentID: root.ID, Kind: core.KindProject, Title: "P", Driver: "fake"})
	if err != nil {
		t.Fatalf("CreateNode: %v", err)
	}

	sink := &fakeSink{}
	r := NewRunner(tr, sink, "http://127.0.0.1:7433", nil)
	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { r.Run(runCtx); close(done) }()

	raise := func(reason core.AwaitingReason) {
		if _, err := tr.IngestEvents(ctx, proj.ID, "", []core.EventInput{{
			Type: core.EventAwaitingInput, Reason: reason, Detail: "d",
		}}); err != nil {
			t.Errorf("IngestEvents: %v", err)
		}
	}
	// Toggle between two non-none attentions so every step is a real transition;
	// once Run has subscribed it observes one and notifies.
	deadline := time.Now().Add(2 * time.Second)
	for i := 0; len(sink.all()) == 0 && time.Now().Before(deadline); i++ {
		if i%2 == 0 {
			raise(core.AwaitQuestion)
		} else {
			raise(core.AwaitPermission)
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-done

	got := sink.all()
	if len(got) == 0 {
		t.Fatal("runner never notified on a live attention transition")
	}
	for _, n := range got {
		if n.NodeID != proj.ID {
			t.Errorf("notified node = %q, want %q", n.NodeID, proj.ID)
		}
		if !n.Sound {
			t.Errorf("question/permission banner should sound: %+v", n)
		}
	}
}

// TestRunnerSeedsSnapshotWithoutNotifying asserts standing attention present at
// subscribe time is not announced as news.
func TestRunnerSeedsSnapshotWithoutNotifying(t *testing.T) {
	tr := tree.New(nopStore{})
	ctx := t.Context()
	root, err := tr.Bootstrap(ctx, "ws")
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	proj, err := tr.CreateNode(ctx, tree.CreateSpec{ParentID: root.ID, Kind: core.KindProject, Title: "P", Driver: "fake"})
	if err != nil {
		t.Fatalf("CreateNode: %v", err)
	}
	// Raise attention BEFORE the runner subscribes.
	if _, err := tr.IngestEvents(ctx, proj.ID, "", []core.EventInput{{
		Type: core.EventAwaitingInput, Reason: core.AwaitPermission, Detail: "standing",
	}}); err != nil {
		t.Fatalf("IngestEvents: %v", err)
	}

	sink := &fakeSink{}
	r := NewRunner(tr, sink, "http://127.0.0.1:7433", nil)
	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { r.Run(runCtx); close(done) }()

	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	if got := sink.all(); len(got) != 0 {
		t.Fatalf("notifications = %d, want 0 (standing attention must not notify on connect): %+v", len(got), got)
	}
}
