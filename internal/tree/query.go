package tree

import (
	"github.com/AnkushinDaniil/grove/internal/core"
)

// Get returns a node snapshot by ID.
func (t *Tree) Get(id core.NodeID) (core.Node, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	n, ok := t.nodes[id]
	return n, ok
}

// Root returns the workspace root if it exists.
func (t *Tree) Root() (core.Node, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.rootLocked()
}

// Children returns the live children of a node in sibling order.
func (t *Tree) Children(id core.NodeID) []core.Node {
	t.mu.Lock()
	defer t.mu.Unlock()
	ids := t.children[id]
	out := make([]core.Node, 0, len(ids))
	for _, cid := range ids {
		n := t.nodes[cid]
		if !n.Archived() {
			out = append(out, n)
		}
	}
	return out
}

// SubtreeIDs returns the live subtree of id (DFS, id first).
func (t *Tree) SubtreeIDs(id core.NodeID) []core.NodeID {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, ok := t.nodes[id]; !ok {
		return nil
	}
	return t.subtreeLocked(id)
}

func (t *Tree) subtreeLocked(id core.NodeID) []core.NodeID {
	out := []core.NodeID{id}
	for _, cid := range t.children[id] {
		if t.nodes[cid].Archived() {
			continue
		}
		out = append(out, t.subtreeLocked(cid)...)
	}
	return out
}

// SessionFor returns the latest session bound to a node.
func (t *Tree) SessionFor(id core.NodeID) (core.Session, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	sid, ok := t.nodeSession[id]
	if !ok {
		return core.Session{}, false
	}
	s, ok := t.sessions[sid]
	return s, ok
}

// Resolved is the effective driver/profile for a node after walking the
// parent chain (nearest non-empty wins).
type Resolved struct {
	Driver    string
	ProfileID core.ProfileID
}

// Resolve computes the effective driver and profile for a node.
func (t *Tree) Resolve(id core.NodeID) (Resolved, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	n, ok := t.nodes[id]
	if !ok {
		return Resolved{}, false
	}
	var res Resolved
	for {
		if res.Driver == "" {
			res.Driver = n.Driver
		}
		if res.ProfileID == "" {
			res.ProfileID = n.ProfileID
		}
		if (res.Driver != "" && res.ProfileID != "") || n.ParentID == "" {
			return res, true
		}
		n, ok = t.nodes[n.ParentID]
		if !ok {
			return res, true
		}
	}
}

// Rollup aggregates the live descendants of a node (excluding the node
// itself). Derived on demand; never stored.
type Rollup struct {
	Total     int
	ByStatus  map[core.NodeStatus]int
	Attention int // descendants with unacked attention
}

// Rollup computes the aggregate for a node's subtree.
func (t *Tree) Rollup(id core.NodeID) Rollup {
	t.mu.Lock()
	defer t.mu.Unlock()
	r := Rollup{ByStatus: make(map[core.NodeStatus]int)}
	if _, ok := t.nodes[id]; !ok {
		return r
	}
	for _, nid := range t.subtreeLocked(id) {
		if nid == id {
			continue
		}
		n := t.nodes[nid]
		r.Total++
		r.ByStatus[n.Status]++
		if n.Attention != core.AttentionNone {
			r.Attention++
		}
	}
	return r
}
