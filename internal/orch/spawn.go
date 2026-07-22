package orch

import (
	"context"
	"fmt"
	"strings"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/mcpserv"
	"github.com/AnkushinDaniil/grove/internal/session"
	"github.com/AnkushinDaniil/grove/internal/tree"
)

// Spawn implements mcpserv.Spawner: it validates limits, creates the child node
// synchronously (so the id can be returned immediately), and launches its
// session asynchronously. The child inherits driver/profile down the tree.
func (s *Scheduler) Spawn(ctx context.Context, parent core.NodeID, req mcpserv.SpawnRequest) (core.NodeID, error) {
	parentNode, ok := s.tree.Get(parent)
	if !ok {
		return "", fmt.Errorf("parent %s not found", parent)
	}
	if parentNode.Archived() {
		return "", fmt.Errorf("parent %s is archived", parent)
	}
	if err := s.checkLimits(parent); err != nil {
		return "", err
	}

	child, err := s.tree.CreateNode(ctx, tree.CreateSpec{
		ParentID: parent,
		Kind:     childKind(parentNode.Kind),
		Title:    req.Title,
		Brief:    req.Prompt,
		Driver:   req.Driver,
	})
	if err != nil {
		return "", fmt.Errorf("create child: %w", err)
	}

	role := req.Role
	if !role.Valid() {
		role = mcpserv.RoleWorker
	}
	mode := req.Mode
	if !mode.Valid() {
		mode = core.ModeHeadless
	}
	token := s.tokens.Mint(child.ID, role)

	go s.startChild(child.ID, req.Prompt, mode, role, token)
	return child.ID, nil
}

// SendMessage implements mcpserv.Spawner: it buffers a message for the target
// node and schedules a wake. Adjacency is enforced by the MCP server before the
// call reaches here.
func (s *Scheduler) SendMessage(_ context.Context, from, to core.NodeID, text string) error {
	if _, ok := s.tree.Get(to); !ok {
		return fmt.Errorf("target %s not found", to)
	}
	s.enqueue(to, digestItem{Kind: kindMessage, From: from, Text: text}, false)
	return nil
}

// checkLimits enforces the depth, direct-children and subtree-size clamps before
// a spawn (ORCHESTRATION.md §2). A breach returns ErrLimit, which the MCP server
// surfaces to the agent as a visible, actionable failure.
func (s *Scheduler) checkLimits(parent core.NodeID) error {
	_, depth := s.pathOf(parent)
	if depth+1 > s.limits.MaxDepth {
		return fmt.Errorf("%w: max depth %d reached", mcpserv.ErrLimit, s.limits.MaxDepth)
	}
	if len(s.tree.Children(parent)) >= s.limits.MaxChildren {
		return fmt.Errorf("%w: max %d direct children reached", mcpserv.ErrLimit, s.limits.MaxChildren)
	}
	if s.tree.Rollup(parent).Total >= s.limits.MaxDescendants {
		return fmt.Errorf("%w: max %d descendants reached", mcpserv.ErrLimit, s.limits.MaxDescendants)
	}
	return nil
}

// startChild launches a spawned child's session after acquiring an active-leaf
// slot (backpressure). It runs on the scheduler's lifetime context so the
// session outlives the spawning request. A launch failure marks the child failed
// so the parent is woken about it.
func (s *Scheduler) startChild(childID core.NodeID, prompt string, mode core.SessionMode, role mcpserv.Role, token string) {
	s.mu.Lock()
	ctx := s.runCtx
	s.mu.Unlock()

	if !s.acquireLeaf(ctx, childID) {
		return // scheduler shutting down
	}

	opt := session.WithOrchestration(session.OrchParams{
		NodeID:     childID,
		Token:      token,
		SocketPath: s.socket,
		Role:       string(role),
		GroveBin:   s.groveBin,
		Briefing:   s.composeBriefing(childID, role),
	})
	if _, err := s.starter.Start(ctx, childID, mode, prompt, "", opt); err != nil {
		s.log.Warn("orch spawn start failed", "node", childID, "err", err)
		s.releaseLeaf(childID)
		s.markSpawnFailed(ctx, childID, err)
	}
}

// markSpawnFailed records a launch failure as an error event on the child, which
// flips it to failed and wakes the parent.
func (s *Scheduler) markSpawnFailed(ctx context.Context, childID core.NodeID, cause error) {
	payload, _ := core.MarshalPayload(core.ErrorPayload{Message: "spawn failed: " + cause.Error(), Fatal: true})
	if _, err := s.tree.IngestEvents(ctx, childID, "", []core.EventInput{
		{Type: core.EventError, Payload: payload, Detail: "spawn failed"},
	}); err != nil {
		s.log.Warn("orch mark spawn failed", "node", childID, "err", err)
	}
}

// composeBriefing builds the node-context header for a spawned child's first
// prompt from its place in the tree, injecting recalled memory when available.
func (s *Scheduler) composeBriefing(id core.NodeID, role mcpserv.Role) string {
	node, ok := s.tree.Get(id)
	if !ok {
		return ""
	}
	path, depth := s.pathOf(id)
	workDir := node.WorkspaceDir
	if workDir == "" {
		workDir = node.WorkDir
	}
	return mcpserv.ComposeBriefing(mcpserv.BriefingParams{
		NodeID:   id,
		Title:    node.Title,
		Path:     path,
		Role:     role,
		WorkDir:  workDir,
		Depth:    depth,
		Children: len(s.tree.Children(id)),
		Limits:   s.limits,
		Memory:   s.recall(id),
	})
}

// recall fetches the node-scoped memory block for a spawn briefing (recall
// injection, ORCHESTRATION.md §8). Empty when memory is disabled or nothing is
// recalled; bounded internally so it cannot stall the spawn.
func (s *Scheduler) recall(id core.NodeID) string {
	if s.mem == nil {
		return ""
	}
	s.mu.Lock()
	ctx := s.runCtx
	s.mu.Unlock()
	return s.mem.Recall(ctx, id, recallBudgetBytes)
}

// pathOf renders a node's root-first title path and its depth (root = 0).
func (s *Scheduler) pathOf(id core.NodeID) (string, int) {
	var titles []string
	depth := 0
	n, ok := s.tree.Get(id)
	for ok {
		titles = append(titles, n.Title)
		if n.ParentID == "" {
			break
		}
		depth++
		n, ok = s.tree.Get(n.ParentID)
	}
	for i, j := 0, len(titles)-1; i < j; i, j = i+1, j-1 {
		titles[i], titles[j] = titles[j], titles[i]
	}
	return strings.Join(titles, " / "), depth
}

// childKind picks the structural kind for a node spawned under parentKind.
func childKind(parentKind core.Kind) core.Kind {
	if parentKind == core.KindWorkspace {
		return core.KindProject
	}
	return core.KindTask
}
