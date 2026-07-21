package notify

import (
	"context"
	"log/slog"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/tree"
)

// Runner watches the tree and dispatches a notification each time a node's
// attention transitions to a non-none value. It tracks the last-seen attention
// per node so a delta that merely re-reports an unchanged node does not re-fire.
type Runner struct {
	tree      *tree.Tree
	sink      Sink
	daemonURL string
	logger    *slog.Logger
}

// NewRunner builds a Runner delivering to sink (typically a Coalescer) and
// deep-linking banners at daemonURL.
func NewRunner(tr *tree.Tree, sink Sink, daemonURL string, logger *slog.Logger) *Runner {
	if logger == nil {
		logger = slog.Default()
	}
	return &Runner{tree: tr, sink: sink, daemonURL: daemonURL, logger: logger}
}

// Run subscribes to the tree and dispatches notifications until ctx is canceled.
// A dropped subscription (slow consumer) is transparently re-established from a
// fresh snapshot; the reseed does not notify on already-standing attention.
func (r *Runner) Run(ctx context.Context) {
	for {
		if r.watch(ctx) {
			return
		}
		r.logger.Debug("notify subscription dropped, resubscribing")
	}
}

// watch runs one subscription. It seeds attention state from the snapshot
// without notifying (standing attention on connect is not news), then notifies
// on transitions until the feed closes or ctx is done. It returns true when ctx
// was canceled and the caller should stop.
func (r *Runner) watch(ctx context.Context) bool {
	snap, deltas, cancel := r.tree.Subscribe()
	defer cancel()

	prev := make(map[core.NodeID]core.Attention, len(snap.Nodes))
	for _, n := range snap.Nodes {
		prev[n.ID] = n.Attention
	}

	for {
		select {
		case <-ctx.Done():
			return true
		case d, ok := <-deltas:
			if !ok {
				return false
			}
			r.dispatch(d, prev)
		}
	}
}

// dispatch notifies for every node in the delta whose attention changed to a
// new non-none value, updating the tracked previous attention.
func (r *Runner) dispatch(d tree.Delta, prev map[core.NodeID]core.Attention) {
	for _, n := range d.Nodes {
		was := prev[n.ID]
		prev[n.ID] = n.Attention
		if n.Attention == core.AttentionNone || n.Attention == was {
			continue
		}
		notif, ok := notificationFor(n, r.daemonURL)
		if !ok {
			continue
		}
		r.sink.Notify(notif)
	}
}
