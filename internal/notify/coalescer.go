package notify

import (
	"fmt"
	"sync"
	"time"

	"github.com/AnkushinDaniil/grove/internal/core"
)

const (
	// perNodeInterval is the minimum spacing between a single node's banners.
	perNodeInterval = 30 * time.Second
	// globalWindow and globalCap bound how many individual banners fire across
	// all nodes in a rolling window before overflow collapses into a digest.
	globalWindow = time.Minute
	globalCap    = 6
	// digestGroup is the fixed group the overflow digest is delivered under, so a
	// notifier replaces the prior digest banner rather than stacking them.
	digestGroup = "digest"
)

// Coalescer wraps a Sink and damps notification storms two ways: it drops a
// node's repeat banners within perNodeInterval, and it caps individual banners
// at globalCap per globalWindow. Overflow past the cap is collapsed into a
// single running-count digest banner delivered under a fixed group, so a
// grouping notifier shows one "N nodes need attention" banner. It is safe for
// concurrent use and deterministic under an injected clock.
type Coalescer struct {
	sink Sink
	now  func() time.Time

	mu      sync.Mutex
	perNode map[core.NodeID]time.Time // last banner time per node
	recent  []time.Time               // individual-banner times within globalWindow
	digest  map[core.NodeID]struct{}  // nodes folded into the current overflow burst
}

// NewCoalescer wraps sink. A nil clock defaults to time.Now.
func NewCoalescer(sink Sink, now func() time.Time) *Coalescer {
	if now == nil {
		now = time.Now
	}
	return &Coalescer{
		sink:    sink,
		now:     now,
		perNode: make(map[core.NodeID]time.Time),
		digest:  make(map[core.NodeID]struct{}),
	}
}

// Notify applies the dedup and cap policy, then forwards to the wrapped sink.
func (c *Coalescer) Notify(n Notification) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()

	if last, ok := c.perNode[n.NodeID]; ok && now.Sub(last) < perNodeInterval {
		return // per-node dedup window
	}
	c.pruneLocked(now)

	if len(c.recent) < globalCap {
		c.perNode[n.NodeID] = now
		c.recent = append(c.recent, now)
		clear(c.digest) // a banner got through: the overflow burst (if any) is over
		c.sink.Notify(n)
		return
	}

	// Global cap reached: fold this node into the overflow digest. The digest
	// re-fires with a running count under digestGroup, replacing its own prior
	// banner, so the user sees one "N nodes need attention" instead of a storm.
	c.perNode[n.NodeID] = now
	c.digest[n.NodeID] = struct{}{}
	c.sink.Notify(digestNotification(len(c.digest), n.Sound))
}

// pruneLocked drops individual-banner timestamps older than globalWindow.
func (c *Coalescer) pruneLocked(now time.Time) {
	cutoff := now.Add(-globalWindow)
	kept := c.recent[:0]
	for _, t := range c.recent {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	c.recent = kept
}

// digestNotification builds the collapsed overflow banner for count nodes,
// carrying through the triggering event's sound.
func digestNotification(count int, sound bool) Notification {
	noun := "nodes need attention"
	if count == 1 {
		noun = "node needs attention"
	}
	return Notification{
		NodeID: digestGroup,
		Title:  "grove",
		Body:   fmt.Sprintf("%d %s", count, noun),
		Sound:  sound,
	}
}
