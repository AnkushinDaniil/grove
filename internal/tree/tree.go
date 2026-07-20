// Package tree owns the materialized node tree: the single serialized writer
// for all tree state. Every mutation — REST calls, driver events, hook posts,
// the janitor — goes through Tree, which persists via Store, bumps a monotonic
// revision, and broadcasts deltas to subscribers.
//
// Nodes, sessions and events are immutable snapshots: mutations copy structs,
// never modify them in place. Parent rollups are derived on demand, never stored.
package tree

import (
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/AnkushinDaniil/grove/internal/core"
)

// Snapshot is a consistent view of the tree at one revision.
type Snapshot struct {
	Rev      uint64
	Nodes    []core.Node
	Sessions []core.Session
}

// Delta is one atomic batch of changes at a revision. Consumers that observe a
// gap in Rev must refetch a Snapshot.
type Delta struct {
	Rev      uint64
	Nodes    []core.Node    // upserted node snapshots
	Sessions []core.Session // upserted session snapshots
	Events   []core.Event   // appended events
}

// subBuffer bounds a subscriber's delta queue. A subscriber that falls this
// far behind is dropped (channel closed) and must re-subscribe for a snapshot.
const subBuffer = 256

type subscriber struct {
	ch chan Delta
}

// Tree is safe for concurrent use. Internally a single mutex serializes all
// writes, which gives mutations a total order matching broadcast order.
type Tree struct {
	store Store
	now   func() time.Time

	mu          sync.Mutex
	nodes       map[core.NodeID]core.Node
	children    map[core.NodeID][]core.NodeID // insertion-ordered sibling lists
	sessions    map[core.SessionID]core.Session
	nodeSession map[core.NodeID]core.SessionID // latest session per node
	rev         uint64
	subs        map[uint64]*subscriber
	nextSubID   uint64
}

// Option configures a Tree.
type Option func(*Tree)

// WithClock overrides the time source (tests).
func WithClock(now func() time.Time) Option {
	return func(t *Tree) { t.now = now }
}

// New creates an empty tree persisting through store.
func New(store Store, opts ...Option) *Tree {
	t := &Tree{
		store:       store,
		now:         time.Now,
		nodes:       make(map[core.NodeID]core.Node),
		children:    make(map[core.NodeID][]core.NodeID),
		sessions:    make(map[core.SessionID]core.Session),
		nodeSession: make(map[core.NodeID]core.SessionID),
		subs:        make(map[uint64]*subscriber),
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// Load populates the tree from persisted state. It must be called before the
// tree is shared; it does not broadcast.
func (t *Tree) Load(nodes []core.Node, sessions []core.Session) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	byID := make(map[core.NodeID]core.Node, len(nodes))
	for _, n := range nodes {
		if err := n.Validate(); err != nil {
			return fmt.Errorf("load node %s: %w", n.ID, err)
		}
		byID[n.ID] = n
	}
	for _, n := range nodes {
		if n.ParentID != "" {
			if _, ok := byID[n.ParentID]; !ok {
				return fmt.Errorf("%w: node %s references missing parent %s", core.ErrInvalid, n.ID, n.ParentID)
			}
		}
	}
	t.nodes = byID
	t.children = make(map[core.NodeID][]core.NodeID, len(nodes))
	ordered := slices.Clone(nodes)
	slices.SortStableFunc(ordered, func(a, b core.Node) int { return a.Position - b.Position })
	for _, n := range ordered {
		if n.ParentID != "" {
			t.children[n.ParentID] = append(t.children[n.ParentID], n.ID)
		}
	}
	for _, s := range sessions {
		if err := s.Validate(); err != nil {
			return fmt.Errorf("load session %s: %w", s.ID, err)
		}
		t.sessions[s.ID] = s
		t.nodeSession[s.NodeID] = s.ID
	}
	return nil
}

// Snapshot returns a consistent copy of all live state.
func (t *Tree) Snapshot() Snapshot {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.snapshotLocked()
}

func (t *Tree) snapshotLocked() Snapshot {
	nodes := make([]core.Node, 0, len(t.nodes))
	for _, n := range t.nodes {
		nodes = append(nodes, n)
	}
	slices.SortFunc(nodes, func(a, b core.Node) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	sessions := make([]core.Session, 0, len(t.sessions))
	for _, s := range t.sessions {
		sessions = append(sessions, s)
	}
	slices.SortFunc(sessions, func(a, b core.Session) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	return Snapshot{Rev: t.rev, Nodes: nodes, Sessions: sessions}
}

// Subscribe returns the current snapshot and a delta feed starting after it.
// The returned cancel func must be called to release the subscription. The
// channel closes when the subscriber is dropped for falling behind.
func (t *Tree) Subscribe() (Snapshot, <-chan Delta, func()) {
	t.mu.Lock()
	defer t.mu.Unlock()
	id := t.nextSubID
	t.nextSubID++
	sub := &subscriber{ch: make(chan Delta, subBuffer)}
	t.subs[id] = sub
	snap := t.snapshotLocked()
	cancel := func() {
		t.mu.Lock()
		defer t.mu.Unlock()
		if s, ok := t.subs[id]; ok {
			delete(t.subs, id)
			close(s.ch)
		}
	}
	return snap, sub.ch, cancel
}

// broadcastLocked bumps rev, stamps it on the delta, and fans out. Callers
// hold t.mu and must have already persisted the change.
func (t *Tree) broadcastLocked(d Delta) {
	t.rev++
	d.Rev = t.rev
	for id, sub := range t.subs {
		select {
		case sub.ch <- d:
		default:
			// Subscriber fell subBuffer behind: drop it. Closing signals the
			// consumer to re-subscribe and start from a fresh snapshot.
			delete(t.subs, id)
			close(sub.ch)
		}
	}
}
