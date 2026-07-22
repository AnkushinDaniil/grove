package memory

import (
	"strings"

	"github.com/AnkushinDaniil/grove/internal/core"
)

// Scope selects which slice of a node's tree lineage a memory query covers. It
// maps directly onto the REST endpoint's ?scope= values (docs/API.md, Node
// memory) and onto MemPalace room filters via resolveScope.
type Scope string

const (
	// ScopeSelf is the node's own room only.
	ScopeSelf Scope = "self"
	// ScopeSubtree is the node and every live descendant.
	ScopeSubtree Scope = "subtree"
	// ScopeAncestors is the node and every ancestor up to (and including) the
	// project that anchors its wing.
	ScopeAncestors Scope = "ancestors"
)

// ParseScope maps a query-string value to a Scope, defaulting to ScopeSelf for
// empty or unknown input so a malformed request degrades to the narrowest view.
func ParseScope(s string) Scope {
	switch Scope(s) {
	case ScopeSubtree:
		return ScopeSubtree
	case ScopeAncestors:
		return ScopeAncestors
	default:
		return ScopeSelf
	}
}

// wingPrefix namespaces every grove-owned wing so grove's tree-scoped memory
// never collides with wings a human curates directly in their palace. The
// mapping is project-node ↔ wing (ORCHESTRATION.md §8). MemPalace validates the
// wing charset and rejects colons, so the separator is a hyphen; node ids are
// UUIDv7 strings (hex + hyphens), which are already valid.
const wingPrefix = "grove-"

// Tree is the read-only slice of the node tree memory scoping needs: resolve a
// node, walk its ancestors, and enumerate its subtree. *tree.Tree satisfies it;
// the interface keeps this package decoupled from the tree actor and testable
// with a trivial fake.
type Tree interface {
	Get(core.NodeID) (core.Node, bool)
	SubtreeIDs(core.NodeID) []core.NodeID
}

// roomFor is a node's room: its own id. One room per node gives ScopeSelf an
// exact wing+room filter and lets subtree/ancestor scopes post-filter by room
// membership.
func roomFor(id core.NodeID) string { return string(id) }

// wingAnchor walks from id up to the nearest project ancestor (inclusive) — the
// node whose id names the wing. A node above any project (the workspace root, or
// an orphaned chain) anchors on the root of its own chain. The bool is false
// only when id is unknown to the tree.
func wingAnchor(t Tree, id core.NodeID) (core.NodeID, bool) {
	n, ok := t.Get(id)
	if !ok {
		return "", false
	}
	for {
		if n.Kind == core.KindProject || n.ParentID == "" {
			return n.ID, true
		}
		parent, ok := t.Get(n.ParentID)
		if !ok {
			return n.ID, true
		}
		n = parent
	}
}

// wingFor is the palace wing a node's memory lives in: the grove-namespaced id
// of its wing anchor. Returns "" only when the node is unknown.
func wingFor(t Tree, id core.NodeID) string {
	anchor, ok := wingAnchor(t, id)
	if !ok {
		return ""
	}
	return wingPrefix + string(anchor)
}

// ancestorRooms is the room set from id up to (and including) its wing anchor.
func ancestorRooms(t Tree, id core.NodeID) []string {
	var rooms []string
	n, ok := t.Get(id)
	for ok {
		rooms = append(rooms, roomFor(n.ID))
		if n.Kind == core.KindProject || n.ParentID == "" {
			break
		}
		n, ok = t.Get(n.ParentID)
	}
	return rooms
}

// scopeFilter is a resolved MemPalace query scope: one wing plus, for narrowed
// scopes, the set of rooms whose entries are in range. A nil rooms map means
// "every room in the wing" (used only when a node's own scope spans the wing).
type scopeFilter struct {
	wing  string
	rooms map[string]bool // nil => no room restriction beyond the wing
}

// valid reports whether the filter resolved to a usable wing.
func (f scopeFilter) valid() bool { return f.wing != "" }

// allows reports whether an entry filed under room is in scope.
func (f scopeFilter) allows(room string) bool {
	if f.rooms == nil {
		return true
	}
	return f.rooms[room]
}

// resolveScope translates a node id and scope into a MemPalace wing plus the
// room set that scope admits. MemPalace search takes a single exact room filter,
// so subtree/ancestor scopes query the whole wing and post-filter by room
// membership (a node's subtree and ancestors both stay within its wing).
func resolveScope(t Tree, id core.NodeID, scope Scope) scopeFilter {
	wing := wingFor(t, id)
	if wing == "" {
		return scopeFilter{}
	}
	switch scope {
	case ScopeSubtree:
		rooms := make(map[string]bool)
		for _, nid := range t.SubtreeIDs(id) {
			rooms[roomFor(nid)] = true
		}
		return scopeFilter{wing: wing, rooms: rooms}
	case ScopeAncestors:
		rooms := make(map[string]bool)
		for _, r := range ancestorRooms(t, id) {
			rooms[r] = true
		}
		return scopeFilter{wing: wing, rooms: rooms}
	default: // ScopeSelf
		return scopeFilter{wing: wing, rooms: map[string]bool{roomFor(id): true}}
	}
}

// recallRooms is the room set Recall injects from: the node's subtree (its own
// notes plus what its children learned) unioned with its ancestor chain (project
// decisions above it). ORCHESTRATION.md §8: "own room + subtree + ancestor
// wings".
func recallRooms(t Tree, id core.NodeID) map[string]bool {
	rooms := make(map[string]bool)
	for _, nid := range t.SubtreeIDs(id) {
		rooms[roomFor(nid)] = true
	}
	for _, r := range ancestorRooms(t, id) {
		rooms[r] = true
	}
	return rooms
}

// recallQuery builds the keyword query Recall searches with from a node's title
// and brief, bounded to MemPalace's 250-char query limit. Search wants keywords,
// not prose, so this is a best-effort relevance seed, not a precise question.
func recallQuery(n core.Node) string {
	q := strings.TrimSpace(n.Title + " " + n.Brief)
	const maxQueryLen = 250
	if len(q) > maxQueryLen {
		q = q[:maxQueryLen]
	}
	return q
}
