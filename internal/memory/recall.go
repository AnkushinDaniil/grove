package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/AnkushinDaniil/grove/internal/core"
)

// This file is the node-aware layer over the raw MCP client: it turns a grove
// node id into MemPalace wing/room scopes and drives the two zero-touch behaviors
// from ORCHESTRATION.md §8 — recall injection at spawn, and auto-capture on
// milestones. Both are best-effort: they never block or fail an agent's path.
//
// Deferred from §8 (phase-2 v1 scope): distill-up-on-archive (compacting a task
// room's key content one level toward the Wing when the subtree is archived) and
// the curated agent proxy tools (memory_write/memory_search/memory_digest mapped
// onto MemPalace's native tools, so sessions get a small curated subset instead
// of all 36). Recall injection currently runs at spawn only; wake turns reuse the
// resumed conversation with no briefing, so wake-time recall would inject into the
// digest prompt instead — also deferred.
//
// TODO(memory-phase2): distill-up-on-archive; curated agent proxy tools;
// wake-time recall injection.

// Memory kinds and sources. Kind classifies a drawer for the UI; source records
// who filed it. grove encodes both in a one-line header on the drawer content
// (MemPalace has no structured metadata field), and strips it back off on read.
const (
	KindFact       = "fact"
	KindDecision   = "decision"
	KindGotcha     = "gotcha"
	KindConvention = "convention"

	SourceAuto  = "auto"
	SourceAgent = "agent"
	SourceUser  = "user"
)

// headerPrefix marks grove's kind/source header line: "grove:<source>:<kind>".
// A drawer without it (e.g. one an agent files directly through a future curated
// tool) reads back as an agent-authored fact.
const headerPrefix = "grove:"

// addedBy attributes grove's automatic writes in MemPalace's own audit trail.
const addedBy = "grove"

// Capture files a milestone entry for a node, best-effort. It resolves the
// node's wing/room, prepends the kind/source header, and writes (spooling if the
// backend is down). A nil client or unknown node is a silent no-op — capture must
// never disrupt the orchestration path (ORCHESTRATION.md §8: auto-capture is
// zero-touch and failure-tolerant).
func (c *Client) Capture(ctx context.Context, nodeID core.NodeID, kind, content, source string) {
	if c == nil {
		return
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	wing := wingFor(c.tree, nodeID)
	if wing == "" {
		return // node not in the tree; nothing to scope to
	}
	if err := c.Write(ctx, drawerWrite{
		Wing:       wing,
		Room:       roomFor(nodeID),
		Content:    withHeader(normalizeKind(kind), normalizeSource(source), content),
		SourceFile: sourceRef(nodeID),
		AddedBy:    addedBy,
	}); err != nil {
		c.log.Warn("memory capture failed", "node", nodeID, "err", err)
	}
}

// Recall returns a bounded "## Memory" markdown block for a node's briefing:
// the top relevant entries from its subtree and ancestor chain. An unavailable
// or slow backend, or an empty result, yields "" so the briefing simply omits
// the section (ORCHESTRATION.md §8: the agent benefits even if it never calls a
// tool). budgetBytes caps the rendered block.
func (c *Client) Recall(ctx context.Context, nodeID core.NodeID, budgetBytes int) string {
	if c == nil {
		return ""
	}
	node, ok := c.tree.Get(nodeID)
	if !ok {
		return ""
	}
	wing := wingFor(c.tree, nodeID)
	if wing == "" {
		return ""
	}
	rctx, cancel := context.WithTimeout(ctx, c.recallTO)
	defer cancel()
	filter := scopeFilter{wing: wing, rooms: recallRooms(c.tree, nodeID)}
	entries, ok := c.Search(rctx, recallQuery(node), filter, recallLimit)
	if !ok || len(entries) == 0 {
		return ""
	}
	return renderMemory(entries, budgetBytes)
}

// NodeMemory lists a node's in-scope memory for the REST endpoint
// (GET /nodes/{id}/memory). Backend is the memory backend name and Healthy is
// whether it answered; an unavailable backend returns an empty, healthy=false
// result rather than an error (docs/API.md: the tab shows a hint, never a 500).
func (c *Client) NodeMemory(ctx context.Context, nodeID core.NodeID, scope Scope) Result {
	if c == nil {
		return Result{}
	}
	node, ok := c.tree.Get(nodeID)
	if !ok {
		return Result{}
	}
	filter := resolveScope(c.tree, nodeID, scope)
	if !filter.valid() {
		return Result{}
	}
	// Seed the search with the node's own words; scope is enforced by wing + room
	// membership and distance filtering is off, so this lists the node's memory
	// ranked by relevance to what it is working on.
	entries, healthy := c.Search(ctx, recallQuery(node), filter, endpointLimit)
	if !healthy {
		return Result{}
	}
	return Result{Entries: entries, Backend: backendName, Healthy: true}
}

// Result is the outcome of a node-memory read: the in-scope entries plus the
// backend's identity and health, mirroring the REST response shape.
type Result struct {
	Entries []Entry
	Backend string
	Healthy bool
}

// withHeader prepends grove's kind/source header to drawer content.
func withHeader(kind, source, content string) string {
	return headerPrefix + source + ":" + kind + "\n" + content
}

// splitHeader parses grove's header off a drawer's stored text, returning the
// kind, source, and the content with the header removed. Text without a valid
// grove header is treated as an agent-authored fact — the safe default for
// drawers grove did not write.
func splitHeader(text string) (kind, source, content string) {
	line, rest, found := strings.Cut(text, "\n")
	line = strings.TrimSpace(line)
	if found && strings.HasPrefix(line, headerPrefix) {
		fields := strings.Split(strings.TrimPrefix(line, headerPrefix), ":")
		if len(fields) == 2 {
			return normalizeKind(fields[1]), normalizeSource(fields[0]), rest
		}
	}
	return KindFact, SourceAgent, text
}

// normalizeKind clamps an arbitrary string to a known kind, defaulting to fact.
func normalizeKind(k string) string {
	switch k {
	case KindDecision, KindGotcha, KindConvention, KindFact:
		return k
	default:
		return KindFact
	}
}

// normalizeSource clamps an arbitrary string to a known source, defaulting to
// agent (a drawer grove did not attribute must have come from an agent tool).
func normalizeSource(s string) string {
	switch s {
	case SourceAuto, SourceUser, SourceAgent:
		return s
	default:
		return SourceAgent
	}
}

// sourceRef is the source_file grove stamps on every drawer it writes: a stable
// grove:// URI naming the node. It doubles as the delete_by_source key for
// cleaning up a node's memory.
func sourceRef(nodeID core.NodeID) string { return "grove://node/" + string(nodeID) }

// entryID synthesizes a stable id for an entry. MemPalace search results carry
// no drawer id, so grove derives a deterministic one from the drawer's identity
// (source + room + timestamp + content) — stable across reads for use as a UI
// key without inventing state.
func entryID(sourcePath, room, createdAt, content string) string {
	sum := sha256.Sum256([]byte(sourcePath + "\x00" + room + "\x00" + createdAt + "\x00" + content))
	return hex.EncodeToString(sum[:8])
}

// renderMemory formats entries as a "## Memory" markdown block bounded by
// budgetBytes. Entries are emitted whole, newest-relevance first, until the next
// would exceed the budget. A zero/negative budget applies a sane default.
func renderMemory(entries []Entry, budgetBytes int) string {
	if budgetBytes <= 0 {
		budgetBytes = 2048
	}
	const header = "## Memory\n\nRelevant memory recalled for this node (from grove's shared MemPalace):\n"
	var b strings.Builder
	b.WriteString(header)
	wrote := false
	for _, e := range entries {
		line := fmt.Sprintf("- **%s**: %s\n", e.Kind, collapse(e.Content))
		if b.Len()+len(line) > budgetBytes && wrote {
			break
		}
		b.WriteString(line)
		wrote = true
	}
	if !wrote {
		return ""
	}
	return b.String()
}

// collapse flattens a drawer's content to a single line for the recall block,
// trimming runaway length so one entry cannot dominate the budget.
func collapse(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	const maxEntry = 400
	if len(s) > maxEntry {
		s = s[:maxEntry] + "…"
	}
	return s
}
