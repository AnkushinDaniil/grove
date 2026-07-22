package orch

import (
	"context"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/mcpserv"
	"github.com/AnkushinDaniil/grove/internal/session"
)

// LaunchOrchestrator starts (or resumes) a session on nodeID as a root
// orchestrator: it mints the node's MCP token, mounts grove's tools with an
// orchestrator briefing, and starts the session synchronously so the caller
// (the REST session-start path) can return the running session. Once this root
// is live, every child it spawns is mounted automatically by the scheduler.
//
// resumeID continues an existing conversation (a fresh briefing is still
// prepended; the driver resumes the same transcript). It is the seam that turns
// a user's "Run headless" into a tree-driving agent.
func (s *Scheduler) LaunchOrchestrator(
	ctx context.Context,
	nodeID core.NodeID,
	prompt, resumeID string,
) (core.Session, error) {
	token := s.tokens.Mint(nodeID, mcpserv.RoleOrchestrator)
	opt := session.WithOrchestration(session.OrchParams{
		NodeID:     nodeID,
		Token:      token,
		SocketPath: s.socket,
		Role:       string(mcpserv.RoleOrchestrator),
		GroveBin:   s.groveBin,
		Briefing:   s.composeBriefing(nodeID, mcpserv.RoleOrchestrator),
	})
	return s.starter.Start(ctx, nodeID, core.ModeHeadless, prompt, resumeID, opt)
}
