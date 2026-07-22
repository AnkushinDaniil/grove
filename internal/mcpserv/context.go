package mcpserv

import (
	"fmt"
	"strings"

	"github.com/AnkushinDaniil/grove/internal/core"
)

// protocolInstructions is the durable, role-agnostic protocol summary returned
// in the initialize result. The MCP server re-sends it on every connection, so
// it survives conversation compaction even when the first-message briefing is
// forgotten (ORCHESTRATION.md §2, briefing layer 1).
const protocolInstructions = `You are a node in a grove agent tree. Identity is implicit: every grove tool acts as your own node — there is no "self" parameter, and you cannot act as another node.

Core protocol:
- Report progress at milestones with grove_report_progress (free-text summary + optional checklist). This is how the tree shows your status; it does not interrupt anyone.
- Raise a blocker, question or decision for the human with grove_raise_attention.
- Finish with grove_complete{result: done|failed, summary}. A node is done ONLY when it calls grove_complete — idleness never completes it.
- Re-orient after compaction with grove_get_context.

Orchestrators can additionally spawn and coordinate children:
- grove_spawn_child is asynchronous: it returns immediately with status "spawning". Delegate, then END YOUR TURN — grove wakes you with a batched digest when children report, complete or need attention. Do NOT poll grove_node_status in a loop.
- Prefer a few well-briefed children over many; do trivial work yourself.
- Talk to a child (or, as a child, to your parent) with grove_send_message.`

// BriefingParams carries the node facts the first-message briefing interpolates.
type BriefingParams struct {
	NodeID   core.NodeID
	Title    string
	Path     string // human tree path, e.g. "Workspace / API / Add auth"
	Role     Role
	WorkDir  string
	Depth    int // node depth (root = 0)
	Children int // current direct children
	Limits   Limits

	// Memory is an optional pre-rendered "## Memory" markdown block recalled from
	// MemPalace for this node (ORCHESTRATION.md §8, recall injection). The orch
	// scheduler fills it when composing a briefing; empty means no memory was
	// recalled (backend down, nothing relevant) and the section is omitted.
	Memory string
}

// ComposeBriefing builds the node-context header prepended to a node's first
// task prompt (ORCHESTRATION.md §2, briefing layer 2): identity, tree path,
// limits, and the ORCHESTRATOR-vs-WORKER operating rules. Wake turns reuse the
// resumed conversation and pass no briefing.
func ComposeBriefing(p BriefingParams) string {
	var b strings.Builder
	b.WriteString("# grove node context\n")
	fmt.Fprintf(&b, "You are grove node %s (%q).\n", p.NodeID, p.Title)
	if p.Path != "" {
		fmt.Fprintf(&b, "Tree path: %s\n", p.Path)
	}
	if p.WorkDir != "" {
		fmt.Fprintf(&b, "Working directory: %s\n", p.WorkDir)
	}
	if p.Role.CanOrchestrate() {
		b.WriteString("Role: ORCHESTRATOR\n\n")
		b.WriteString("You coordinate children instead of doing all the work yourself. Spawn a child with grove_spawn_child — it is asynchronous and returns immediately, so after delegating you should END YOUR TURN; grove will wake you when a child reports, completes, or needs attention. Never poll for status in a loop. Prefer a few well-briefed children over many, and do trivial work directly.\n\n")
		fmt.Fprintf(&b, "Limits (inherited and clamped): depth %d/%d, direct children %d/%d, subtree ≤%d nodes.\n",
			p.Depth, p.Limits.MaxDepth, p.Children, p.Limits.MaxChildren, p.Limits.MaxDescendants)
	} else {
		b.WriteString("Role: WORKER\n\n")
		b.WriteString("Do the work yourself. Report progress at milestones with grove_report_progress. If you are blocked or need a decision, call grove_raise_attention. When finished, call grove_complete with a result summary — that is the only signal that marks you done.\n")
	}
	// Recall injection (ORCHESTRATION.md §8): append node-scoped memory recalled
	// from MemPalace, so the agent starts with relevant prior decisions and
	// gotchas even if it never queries memory itself. Empty when nothing was
	// recalled; the block already carries its own "## Memory" heading.
	if mem := strings.TrimSpace(p.Memory); mem != "" {
		b.WriteString("\n")
		b.WriteString(mem)
		if !strings.HasSuffix(mem, "\n") {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// treePath renders the human-readable path from the root down to nodeID using
// node titles. Used by grove_get_context and briefings.
func treePath(get func(core.NodeID) (core.Node, bool), nodeID core.NodeID) (string, int) {
	var titles []string
	depth := 0
	cur := nodeID
	for {
		n, ok := get(cur)
		if !ok {
			break
		}
		titles = append(titles, n.Title)
		if n.ParentID == "" {
			break
		}
		cur = n.ParentID
		depth++
	}
	// Reverse to root-first order.
	for i, j := 0, len(titles)-1; i < j; i, j = i+1, j-1 {
		titles[i], titles[j] = titles[j], titles[i]
	}
	return strings.Join(titles, " / "), depth
}
