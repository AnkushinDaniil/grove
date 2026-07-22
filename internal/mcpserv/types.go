package mcpserv

import (
	"context"
	"errors"

	"github.com/AnkushinDaniil/grove/internal/core"
)

// ErrLimit wraps a spawn denied by orchestration limits. The MCP server reports
// it back to the calling agent as a visible (isError) tool result so the model
// can adapt, rather than as a transport failure.
var ErrLimit = errors.New("orchestration limit exceeded")

// Limits bounds a subtree of orchestrated nodes, cgroups-style: inherited by
// children and clamped, never widened. The MCP server reports the remaining
// headroom in grove_get_context; the scheduler enforces them at spawn.
type Limits struct {
	MaxDepth        int // deepest orchestrated node below the root (root = depth 0)
	MaxChildren     int // direct children of any one orchestrator
	MaxDescendants  int // live nodes anywhere below one orchestrator
	MaxActiveLeaves int // concurrent running leaf sessions per workspace (backpressure)
}

// DefaultLimits are the ORCHESTRATION.md §2 defaults.
func DefaultLimits() Limits {
	return Limits{MaxDepth: 5, MaxChildren: 12, MaxDescendants: 40, MaxActiveLeaves: 6}
}

// SpawnRequest is a validated grove_spawn_child call. The parent (and thus the
// subtree, driver and profile the child inherits) is implicit in the caller's
// identity; the request never names a parent.
type SpawnRequest struct {
	Title  string
	Prompt string
	Role   Role             // worker (default) or orchestrator
	Mode   core.SessionMode // headless (default) or pty
	Driver string           // empty inherits from the parent chain
	Repos  []string         // repo ids/names to attach (v1: recorded, best-effort)
	Limits map[string]int   // caller-requested clamps (never widen inherited)
}

// Spawner is the orchestration runtime the MCP server delegates side effects to.
// The server owns the protocol (auth, catalog, tree reads/writes); the Spawner
// owns node creation, session launch, limits and event-wake — implemented by
// package orch. Both methods are safe for concurrent use.
type Spawner interface {
	// Spawn creates a child under parent and queues its session, returning the
	// new node id immediately (status "spawning"); the launch is asynchronous.
	Spawn(ctx context.Context, parent core.NodeID, req SpawnRequest) (core.NodeID, error)
	// SendMessage queues a text message from one node to an adjacent one (parent
	// to child or child to parent), waking the target.
	SendMessage(ctx context.Context, from, to core.NodeID, text string) error
}
