package orch

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/mcpserv"
	"github.com/AnkushinDaniil/grove/internal/session"
)

// Digest event kinds delivered in a wake batch.
const (
	kindChildCompleted = "child_completed"
	kindChildFailed    = "child_failed"
	kindChildAttention = "child_attention"
	kindMessage        = "message"
)

// digestItem is one buffered event in a node's next wake batch.
type digestItem struct {
	Kind      string         `json:"kind"`
	Child     core.NodeID    `json:"node_id,omitempty"`
	Title     string         `json:"title,omitempty"`
	Attention core.Attention `json:"attention,omitempty"`
	Summary   string         `json:"summary,omitempty"`
	From      core.NodeID    `json:"from,omitempty"`
	Text      string         `json:"text,omitempty"`
}

// digest is the buffer of pending items for one node to wake, plus the timer
// that will flush them.
type digest struct {
	items []digestItem
	timer *time.Timer
}

// enqueue buffers an item for the node to wake and (re)arms its flush timer.
// Urgent items (completion, attention) flush after a short coalescing delay;
// non-urgent items (messages) wait out the debounce window.
func (s *Scheduler) enqueue(wake core.NodeID, item digestItem, urgent bool) {
	delay := s.debounce
	if urgent {
		delay = s.urgent
	}
	s.mu.Lock()
	d := s.pending[wake]
	if d == nil {
		d = &digest{}
		s.pending[wake] = d
	}
	d.items = append(d.items, item)
	if d.timer == nil {
		d.timer = time.AfterFunc(delay, func() { s.flush(wake) })
	} else if urgent {
		d.timer.Reset(delay) // pull an urgent wake in ahead of a long debounce
	}
	s.mu.Unlock()
}

// flush wakes one node with its buffered digest, unless the node is gone or
// mid-turn (in which case it re-arms to avoid two concurrent orchestrator turns).
func (s *Scheduler) flush(wake core.NodeID) {
	s.mu.Lock()
	d := s.pending[wake]
	if d == nil {
		s.mu.Unlock()
		return
	}
	node, ok := s.tree.Get(wake)
	if !ok || node.Archived() {
		delete(s.pending, wake)
		s.mu.Unlock()
		return
	}
	if node.Status.Active() {
		// Target is running: keep the buffer and try again shortly.
		d.timer.Reset(busyRetry)
		s.mu.Unlock()
		return
	}
	items := d.items
	delete(s.pending, wake)
	ctx := s.runCtx
	s.mu.Unlock()

	if len(items) == 0 {
		return
	}
	s.wake(ctx, node, items)
}

// wake runs one headless turn on node, resuming its own conversation with the
// batched events as the injected prompt.
func (s *Scheduler) wake(ctx context.Context, node core.Node, items []digestItem) {
	role := mcpserv.RoleOrchestrator
	if r, ok := s.tokens.RoleOf(node.ID); ok {
		role = r
	}
	token := s.tokens.Mint(node.ID, role)

	resumeID := ""
	if sess, ok := s.tree.SessionFor(node.ID); ok {
		resumeID = sess.DriverSessionID
	}

	prompt := composeDigest(items)
	opt := session.WithOrchestration(session.OrchParams{
		NodeID:     node.ID,
		Token:      token,
		SocketPath: s.socket,
		Role:       string(role),
		GroveBin:   s.groveBin,
	})
	if _, err := s.starter.Start(ctx, node.ID, core.ModeHeadless, prompt, resumeID, opt); err != nil {
		s.log.Warn("orch wake failed", "node", node.ID, "err", err)
	}
}

// composeDigest renders a wake batch: a machine-readable events block plus one
// instruction line (ORCHESTRATION.md §2 wake format).
func composeDigest(items []digestItem) string {
	body, err := json.Marshal(items)
	if err != nil {
		body = []byte("[]")
	}
	return fmt.Sprintf(
		"<grove-events count=%d>\n%s\n</grove-events>\nThese are updates from your subtree while you were asleep. Inspect with grove_list_children or grove_node_status if you need more, act on them, then end your turn.",
		len(items), body,
	)
}

// acquireLeaf blocks until an active-leaf slot is free (backpressure) or ctx is
// canceled, recording that child holds the slot. It is a no-op when the
// semaphore is disabled.
func (s *Scheduler) acquireLeaf(ctx context.Context, child core.NodeID) bool {
	if s.slots == nil {
		return true
	}
	select {
	case s.slots <- struct{}{}:
		s.mu.Lock()
		s.leaseSet[child] = struct{}{}
		s.mu.Unlock()
		return true
	case <-ctx.Done():
		return false
	}
}

// releaseLeaf frees the active-leaf slot a child held, if any.
func (s *Scheduler) releaseLeaf(child core.NodeID) {
	if s.slots == nil {
		return
	}
	s.mu.Lock()
	_, held := s.leaseSet[child]
	if held {
		delete(s.leaseSet, child)
	}
	s.mu.Unlock()
	if held {
		<-s.slots
	}
}

// metaCompletion reads the completion object grove_complete records in a node's
// JSON meta. present is false when the node has not completed. result is the
// authoritative done/failed outcome — it lands in meta before the matching
// attention event, so it is the signal completionKind trusts.
func metaCompletion(meta string) (result, summary string, present bool) {
	if meta == "" {
		return "", "", false
	}
	var m struct {
		Completion *struct {
			Result  string `json:"result"`
			Summary string `json:"summary"`
		} `json:"completion"`
	}
	if json.Unmarshal([]byte(meta), &m) != nil || m.Completion == nil {
		return "", "", false
	}
	return m.Completion.Result, m.Completion.Summary, true
}

// hasCompletionMeta reports whether grove_complete has recorded a completion.
func hasCompletionMeta(meta string) bool {
	_, _, ok := metaCompletion(meta)
	return ok
}
