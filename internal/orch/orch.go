// Package orch is grove's event-wake scheduler: the runtime that turns a static
// tree of nodes into a living tree of agents. It implements the mcpserv.Spawner
// the MCP server delegates to (creating child nodes, enforcing limits, launching
// their sessions) and drives the wake loop that keeps orchestrators asleep until
// something happens.
//
// The contract (ORCHESTRATION.md §2, decision D2) is spawn-async, wake-by-event:
// an orchestrator delegates with grove_spawn_child and ends its turn; when a
// child completes, fails, raises attention, or is sent a message, the scheduler
// buffers a per-target digest, debounces it, and runs one headless turn that
// resumes the target's own conversation with the batched events. No blocking
// wait tool, no polling.
package orch

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/mcpserv"
	"github.com/AnkushinDaniil/grove/internal/session"
	"github.com/AnkushinDaniil/grove/internal/tree"
)

// Starter launches sessions. It is the subset of *session.Manager the scheduler
// needs: spawning children and running wake turns. Defined here so the scheduler
// can be tested against a fake.
type Starter interface {
	Start(ctx context.Context, nodeID core.NodeID, mode core.SessionMode, prompt, resumeID string, opts ...session.LaunchOption) (core.Session, error)
}

// Compile-time guarantees that the scheduler plugs into both seams: the real
// session manager is a Starter, and the scheduler is the MCP server's Spawner.
var (
	_ Starter         = (*session.Manager)(nil)
	_ mcpserv.Spawner = (*Scheduler)(nil)
)

// Default timing.
const (
	// defaultDebounce buffers non-urgent items (messages) before a wake.
	defaultDebounce = 30 * time.Second
	// defaultUrgentDelay coalesces a burst of simultaneous urgent items (a child
	// completing, failing, or raising attention) into one wake.
	defaultUrgentDelay = 250 * time.Millisecond
	// busyRetry re-arms a wake when the target is mid-turn, so two orchestrator
	// turns never run concurrently on one node.
	busyRetry = 1 * time.Second
)

// Deps are the scheduler's collaborators. Tree, Starter and Tokens are required.
type Deps struct {
	Tree        *tree.Tree
	Starter     Starter
	Tokens      *mcpserv.Registry
	Limits      mcpserv.Limits
	SocketPath  string // daemon MCP socket, mounted into every spawned agent
	GroveBin    string // absolute path to the grove binary hosting `grove mcp`
	Logger      *slog.Logger
	Now         func() time.Time
	Debounce    time.Duration // wake debounce for non-urgent items (messages)
	UrgentDelay time.Duration // coalescing delay for urgent items; 0 uses the default
}

// Scheduler creates child nodes, launches their sessions, and wakes orchestrators
// on subtree events. It is safe for concurrent use.
type Scheduler struct {
	tree     *tree.Tree
	starter  Starter
	tokens   *mcpserv.Registry
	limits   mcpserv.Limits
	socket   string
	groveBin string
	log      *slog.Logger
	now      func() time.Time
	debounce time.Duration
	urgent   time.Duration

	// runCtx bounds spawned sessions and wake turns to the scheduler's lifetime,
	// not to the request or timer that triggered them. Set once by Run.
	runCtx context.Context

	mu       sync.Mutex
	seen     map[core.NodeID]childState // change-detection memory per node
	pending  map[core.NodeID]*digest    // buffered wakes keyed by the node to wake
	leaseSet map[core.NodeID]struct{}   // children currently holding an active-leaf slot
	slots    chan struct{}              // active-leaf semaphore (nil disables)
}

// childState remembers what the scheduler last reported about a node so a delta
// stream never wakes a parent twice for the same completion.
type childState struct {
	reported bool           // completion (done/failed) already enqueued
	attn     core.Attention // last observed attention
}

// New builds a Scheduler.
func New(d Deps) *Scheduler {
	if d.Logger == nil {
		d.Logger = slog.New(slog.NewTextHandler(discard{}, nil))
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Debounce <= 0 {
		d.Debounce = defaultDebounce
	}
	if d.UrgentDelay <= 0 {
		d.UrgentDelay = defaultUrgentDelay
	}
	if d.Limits == (mcpserv.Limits{}) {
		d.Limits = mcpserv.DefaultLimits()
	}
	s := &Scheduler{
		tree:     d.Tree,
		starter:  d.Starter,
		tokens:   d.Tokens,
		limits:   d.Limits,
		socket:   d.SocketPath,
		groveBin: d.GroveBin,
		log:      d.Logger,
		now:      d.Now,
		debounce: d.Debounce,
		urgent:   d.UrgentDelay,
		runCtx:   context.Background(), // replaced by Run; guards pre-Run spawns
		seen:     make(map[core.NodeID]childState),
		pending:  make(map[core.NodeID]*digest),
		leaseSet: make(map[core.NodeID]struct{}),
	}
	if d.Limits.MaxActiveLeaves > 0 {
		s.slots = make(chan struct{}, d.Limits.MaxActiveLeaves)
	}
	return s
}

// Run subscribes to the tree and drives the wake loop until ctx is canceled.
func (s *Scheduler) Run(ctx context.Context) error {
	s.mu.Lock()
	s.runCtx = ctx
	s.mu.Unlock()

	snap, deltas, cancel := s.tree.Subscribe()
	// A closure so the deferred call always releases the current subscription,
	// even after a reseed swaps cancel out.
	defer func() { cancel() }()
	s.seedState(snap)

	for {
		select {
		case <-ctx.Done():
			return nil
		case d, ok := <-deltas:
			if !ok {
				// Dropped for falling behind: release the old subscription and
				// reseed from a fresh snapshot.
				cancel()
				snap, deltas, cancel = s.tree.Subscribe()
				s.seedState(snap)
				continue
			}
			s.onDelta(d)
		}
	}
}

// seedState primes change-detection memory so already-terminal nodes present at
// startup don't trigger spurious wakes.
func (s *Scheduler) seedState(snap tree.Snapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seen = make(map[core.NodeID]childState, len(snap.Nodes))
	for _, n := range snap.Nodes {
		s.seen[n.ID] = childState{reported: s.isComplete(n), attn: n.Attention}
	}
}

// onDelta inspects upserted nodes for wake-worthy transitions and releases
// active-leaf slots held by children that just finished.
func (s *Scheduler) onDelta(d tree.Delta) {
	for _, n := range d.Nodes {
		if n.ParentID == "" || n.Archived() {
			s.forget(n.ID)
			continue
		}
		s.classify(n)
	}
}

// classify enqueues a parent wake for a child's completion or attention,
// deduplicating repeated observations of the same completion.
func (s *Scheduler) classify(n core.Node) {
	s.mu.Lock()
	st := s.seen[n.ID]
	complete := s.isComplete(n)
	var item digestItem
	var fire bool
	switch {
	case complete && !st.reported:
		st.reported = true
		fire = true
		item = digestItem{Kind: completionKind(n), Child: n.ID, Title: n.Title, Summary: completionSummary(n)}
	case !complete && isActionableAttention(n.Attention) && n.Attention != st.attn:
		fire = true
		item = digestItem{Kind: kindChildAttention, Child: n.ID, Title: n.Title, Attention: n.Attention, Summary: n.AttentionReason}
	}
	st.attn = n.Attention
	s.seen[n.ID] = st
	s.mu.Unlock()

	if complete {
		s.releaseLeaf(n.ID)
	}
	if fire {
		s.enqueue(n.ParentID, item, true)
	}
}

// forget drops change-detection memory and any held slot for a gone node.
func (s *Scheduler) forget(id core.NodeID) {
	s.mu.Lock()
	delete(s.seen, id)
	s.mu.Unlock()
	s.releaseLeaf(id)
}

// isComplete reports whether a node finished in a way that should wake its
// parent. The authoritative signal is an explicit grove_complete (recorded in
// meta); a crash (StatusFailed) counts too. A bare turn-end is ambiguous: for a
// worker, finishing its turn (attention "done") means done; for an orchestrator,
// it is only a turn boundary between wakes and never completes the node —
// otherwise every wake turn would spuriously wake the grandparent.
func (s *Scheduler) isComplete(n core.Node) bool {
	if hasCompletionMeta(n.Meta) {
		return true
	}
	if n.Status == core.StatusFailed || n.Attention == core.AttentionError {
		return true
	}
	if n.Attention == core.AttentionDone {
		role, ok := s.tokens.RoleOf(n.ID)
		return !ok || !role.CanOrchestrate()
	}
	return false
}

// isActionableAttention reports whether an attention flag warrants waking the
// parent on its own (a question, permission prompt, or review), as opposed to
// "done" (a turn boundary, handled by isComplete) or "none".
func isActionableAttention(a core.Attention) bool {
	switch a {
	case core.AttentionQuestion, core.AttentionPermission, core.AttentionReview:
		return true
	default:
		return false
	}
}

func completionKind(n core.Node) string {
	// The grove_complete result lands in meta first and is authoritative.
	if result, _, ok := metaCompletion(n.Meta); ok {
		if result == "failed" {
			return kindChildFailed
		}
		return kindChildCompleted
	}
	if n.Status == core.StatusFailed || n.Attention == core.AttentionError {
		return kindChildFailed
	}
	return kindChildCompleted
}

// completionSummary prefers the grove_complete summary recorded in meta, falling
// back to the node's attention reason.
func completionSummary(n core.Node) string {
	if _, summary, ok := metaCompletion(n.Meta); ok && summary != "" {
		return summary
	}
	return n.AttentionReason
}

// discard is an io.Writer sink for the default no-op logger.
type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }
