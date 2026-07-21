package notify

import (
	"sync"
	"testing"
	"time"

	"github.com/AnkushinDaniil/grove/internal/core"
)

// fakeSink records every notification it receives for assertions.
type fakeSink struct {
	mu  sync.Mutex
	got []Notification
}

func (f *fakeSink) Notify(n Notification) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.got = append(f.got, n)
}

func (f *fakeSink) all() []Notification {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Notification(nil), f.got...)
}

// clock is a manual time source for deterministic coalescer tests.
type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }
func newClock() *clock                   { return &clock{t: time.Unix(1_700_000_000, 0)} }
func banner(id string) Notification      { return Notification{NodeID: core.NodeID(id), Body: "x"} }
func soundBanner(id string) Notification {
	return Notification{NodeID: core.NodeID(id), Body: "x", Sound: true}
}

// countIndividual counts non-digest (per-node) banners.
func countIndividual(ns []Notification) int {
	n := 0
	for _, x := range ns {
		if x.NodeID != digestGroup {
			n++
		}
	}
	return n
}

func lastDigest(ns []Notification) (Notification, bool) {
	for i := len(ns) - 1; i >= 0; i-- {
		if ns[i].NodeID == digestGroup {
			return ns[i], true
		}
	}
	return Notification{}, false
}

// TestCoalescerPerNodeDedup asserts a single node's repeat banners are dropped
// within perNodeInterval and allowed again once the window elapses.
func TestCoalescerPerNodeDedup(t *testing.T) {
	sink := &fakeSink{}
	clk := newClock()
	c := NewCoalescer(sink, clk.now)

	c.Notify(banner("a")) // dispatched
	c.Notify(banner("a")) // same instant → dropped
	clk.advance(perNodeInterval - time.Nanosecond)
	c.Notify(banner("a")) // still inside window → dropped
	clk.advance(time.Nanosecond)
	c.Notify(banner("a")) // window elapsed → dispatched

	if got := countIndividual(sink.all()); got != 2 {
		t.Fatalf("dispatched individual banners = %d, want 2", got)
	}
}

// TestCoalescerDistinctNodesNotDeduped asserts the per-node window is per node:
// different nodes at the same instant each notify.
func TestCoalescerDistinctNodesNotDeduped(t *testing.T) {
	sink := &fakeSink{}
	clk := newClock()
	c := NewCoalescer(sink, clk.now)

	c.Notify(banner("a"))
	c.Notify(banner("b"))
	c.Notify(banner("c"))

	if got := countIndividual(sink.all()); got != 3 {
		t.Fatalf("dispatched individual banners = %d, want 3", got)
	}
}

// TestCoalescerGlobalCapDigest asserts that past globalCap individual banners in
// the window, further nodes collapse into a single running-count digest banner
// under the digest group, and individual dispatch resumes once the window rolls.
func TestCoalescerGlobalCapDigest(t *testing.T) {
	sink := &fakeSink{}
	clk := newClock()
	c := NewCoalescer(sink, clk.now)

	// Fill the cap with distinct nodes at the same instant.
	for _, id := range []string{"n1", "n2", "n3", "n4", "n5", "n6"} {
		c.Notify(banner(id))
	}
	// Two more distinct nodes overflow into the digest.
	c.Notify(banner("n7"))
	c.Notify(banner("n8"))

	all := sink.all()
	if got := countIndividual(all); got != globalCap {
		t.Fatalf("individual banners = %d, want %d", got, globalCap)
	}
	dg, ok := lastDigest(all)
	if !ok {
		t.Fatal("no digest banner emitted past the global cap")
	}
	if dg.Body != "2 nodes need attention" {
		t.Errorf("digest body = %q, want %q", dg.Body, "2 nodes need attention")
	}

	// Roll past the window; individual dispatch resumes for a fresh node.
	clk.advance(globalWindow + time.Second)
	c.Notify(banner("n9"))
	if got := countIndividual(sink.all()); got != globalCap+1 {
		t.Fatalf("individual banners after window roll = %d, want %d", got, globalCap+1)
	}
}

// TestCoalescerSoundPassthrough asserts Sound is preserved on both the direct
// pass-through and the collapsed digest.
func TestCoalescerSoundPassthrough(t *testing.T) {
	sink := &fakeSink{}
	clk := newClock()
	c := NewCoalescer(sink, clk.now)

	c.Notify(soundBanner("a"))
	got := sink.all()
	if len(got) != 1 || !got[0].Sound {
		t.Fatalf("first banner = %+v, want Sound=true", got)
	}

	// Overflow with a sounded trigger; the digest must carry the sound.
	for _, id := range []string{"b", "c", "d", "e", "f", "g"} {
		c.Notify(banner(id))
	}
	c.Notify(soundBanner("h")) // overflow trigger with sound
	dg, ok := lastDigest(sink.all())
	if !ok {
		t.Fatal("no digest emitted")
	}
	if !dg.Sound {
		t.Errorf("digest Sound = false, want true (passed through from trigger)")
	}
}
