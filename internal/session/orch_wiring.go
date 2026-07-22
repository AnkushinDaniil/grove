package session

import (
	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/driver"
)

// Environment variables the daemon injects into a spawned CLI's grove MCP shim
// (via the mounted server's env block, not the process base env — so they ride
// the 0600 mcp config, never the scrubbed daemon environment). The `grove mcp`
// subcommand reads them to authenticate its UDS connection to the node.
const (
	EnvNodeID = "GROVE_NODE_ID"
	// EnvNodeToken names the env var carrying the node token; the name is not a
	// secret (the value is).
	EnvNodeToken = "GROVE_NODE_TOKEN" //nolint:gosec // G101: env var name, not a credential
	EnvSocket    = "GROVE_SOCKET"
	EnvNodeRole  = "GROVE_NODE_ROLE"
)

// mcpServerName is the name grove's own MCP server is mounted under.
const mcpServerName = "grove"

// OrchParams configures the grove MCP mount and node-context briefing for one
// orchestrated launch. The daemon fills it per node when starting an
// orchestrator or worker session (and on every wake turn, with Briefing empty
// since the resumed conversation already carries it).
type OrchParams struct {
	NodeID     core.NodeID
	Token      string // the node's MCP token (minted by mcpserv)
	SocketPath string // the daemon's MCP Unix socket
	Role       string // "orchestrator" | "worker"
	GroveBin   string // absolute path to the grove binary hosting `grove mcp`
	Briefing   string // node-context header prepended to the task prompt; empty on wake
}

// WithOrchestration mounts grove's MCP server into the launch as the `grove mcp`
// stdio shim, carrying the node's identity in the mounted server's env, and
// prepends the node-context briefing to the prompt. It is the seam the daemon
// uses so a spawned agent can drive the tree (report, spawn, complete) and be
// woken by events.
func WithOrchestration(p OrchParams) LaunchOption {
	return func(spec *driver.LaunchSpec) {
		spec.MCP = append(spec.MCP, driver.MCPRef{
			Name:    mcpServerName,
			Command: p.GroveBin,
			Args:    []string{"mcp"},
			Env: map[string]string{
				EnvNodeID:    string(p.NodeID),
				EnvNodeToken: p.Token,
				EnvSocket:    p.SocketPath,
				EnvNodeRole:  p.Role,
			},
		})
		if p.Briefing == "" {
			return
		}
		if spec.Prompt == "" {
			spec.Prompt = p.Briefing
			return
		}
		spec.Prompt = p.Briefing + "\n\n" + spec.Prompt
	}
}
