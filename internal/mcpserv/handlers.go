package mcpserv

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/tree"
)

// pollWindow and pollThreshold drive the anti-poll guard: more than
// pollThreshold status calls within pollWindow appends a "stop polling" hint to
// the result (ORCHESTRATION.md §1).
const (
	pollWindow    = 60 * time.Second
	pollThreshold = 5
)

// toolResult is a handler's successful outcome. text is the human/JSON body;
// isError marks an outcome the model should react to (e.g. a denied spawn)
// without treating it as a transport failure.
type toolResult struct {
	text    string
	isError bool
}

// envelope renders the MCP tools/call result shape.
func (r toolResult) envelope() map[string]any {
	m := map[string]any{
		"content": []map[string]any{{"type": "text", "text": r.text}},
	}
	if r.isError {
		m["isError"] = true
	}
	return m
}

// okResult marshals v to a JSON text result.
func okResult(v any) (toolResult, *rpcError) {
	b, err := json.Marshal(v)
	if err != nil {
		return toolResult{}, newRPCError(codeInternalError, "marshal result: "+err.Error())
	}
	return toolResult{text: string(b)}, nil
}

// errResult is a visible, model-actionable tool failure (isError).
func errResult(format string, args ...any) (toolResult, *rpcError) {
	return toolResult{text: fmt.Sprintf(format, args...), isError: true}, nil
}

// callTool authenticates capability, dispatches to the named handler, and wraps
// the outcome in the MCP result envelope.
func (s *Server) callTool(ctx context.Context, sess connSession, params json.RawMessage) (any, *rpcError) {
	var call struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if len(params) > 0 {
		if err := json.Unmarshal(params, &call); err != nil {
			return nil, newRPCError(codeInvalidParams, "invalid tools/call params")
		}
	}
	// Honor mid-connection token revocation (a completed node's token is gone).
	role, ok := s.tokens.Resolve(sess.node, sess.token)
	if !ok {
		return nil, newRPCError(codeInvalidRequest, "token no longer valid for node "+string(sess.node))
	}
	sess.role = role

	args := call.Arguments
	res, rerr := s.route2(ctx, sess, call.Name, args)
	if rerr != nil {
		return nil, rerr
	}
	return res.envelope(), nil
}

// route2 dispatches a tool by name, enforcing orchestrator capability.
func (s *Server) route2(ctx context.Context, sess connSession, name string, args json.RawMessage) (toolResult, *rpcError) {
	switch name {
	case toolGetContext:
		return s.handleGetContext(sess)
	case toolReportProgress:
		return s.handleReportProgress(ctx, sess, args)
	case toolRaiseAttention:
		return s.handleRaiseAttention(ctx, sess, args)
	case toolComplete:
		return s.handleComplete(ctx, sess, args)
	case toolSendMessage:
		return s.handleSendMessage(ctx, sess, args)
	case toolSpawnChild:
		if !sess.role.CanOrchestrate() {
			return toolResult{}, newRPCError(codeInvalidRequest, name+" requires orchestrator role")
		}
		return s.handleSpawnChild(ctx, sess, args)
	case toolListChildren:
		if !sess.role.CanOrchestrate() {
			return toolResult{}, newRPCError(codeInvalidRequest, name+" requires orchestrator role")
		}
		return s.handleListChildren(sess, args)
	case toolNodeStatus:
		if !sess.role.CanOrchestrate() {
			return toolResult{}, newRPCError(codeInvalidRequest, name+" requires orchestrator role")
		}
		return s.handleNodeStatus(sess, args)
	default:
		return toolResult{}, newRPCError(codeMethodNotFound, "unknown tool: "+name)
	}
}

func (s *Server) handleGetContext(sess connSession) (toolResult, *rpcError) {
	node, ok := s.tree.Get(sess.node)
	if !ok {
		return toolResult{}, newRPCError(codeInternalError, "node not found: "+string(sess.node))
	}
	path, depth := treePath(s.tree.Get, sess.node)
	children := len(s.tree.Children(sess.node))
	roll := s.tree.Rollup(sess.node)
	workDir := node.WorkspaceDir
	if workDir == "" {
		workDir = node.WorkDir
	}
	return okResult(map[string]any{
		"node_id":   node.ID,
		"title":     node.Title,
		"kind":      node.Kind,
		"tree_path": path,
		"depth":     depth,
		"work_dir":  workDir,
		"role":      sess.role,
		"limits": map[string]any{
			"max_depth":             s.limits.MaxDepth,
			"max_children":          s.limits.MaxChildren,
			"max_descendants":       s.limits.MaxDescendants,
			"depth_remaining":       max(0, s.limits.MaxDepth-depth),
			"children_remaining":    max(0, s.limits.MaxChildren-children),
			"descendants_remaining": max(0, s.limits.MaxDescendants-roll.Total),
		},
		"subtree": map[string]any{"total": roll.Total, "attention": roll.Attention},
		"rules":   "Report progress at milestones; raise attention when blocked; call grove_complete to finish. Orchestrators: spawn async then end your turn — grove wakes you.",
	})
}

func (s *Server) handleReportProgress(ctx context.Context, sess connSession, args json.RawMessage) (toolResult, *rpcError) {
	var in struct {
		Summary   string   `json:"summary"`
		Checklist []string `json:"checklist"`
		Percent   *int     `json:"percent"`
	}
	if rerr := decodeArgs(args, &in); rerr != nil {
		return toolResult{}, rerr
	}
	if strings.TrimSpace(in.Summary) == "" {
		return errResult("summary is required")
	}
	node, ok := s.tree.Get(sess.node)
	if !ok {
		return toolResult{}, newRPCError(codeInternalError, "node not found")
	}
	progress := map[string]any{"summary": in.Summary, "at": s.now().UTC()}
	if in.Checklist != nil {
		progress["checklist"] = in.Checklist
	}
	if in.Percent != nil {
		progress["percent"] = *in.Percent
	}
	if rerr := s.setMeta(ctx, node, "progress", progress); rerr != nil {
		return toolResult{}, rerr
	}
	payload, _ := core.MarshalPayload(core.TextPayload{Text: in.Summary, Role: "progress"})
	if _, err := s.tree.IngestEvents(ctx, sess.node, node.CurrentSessionID, []core.EventInput{
		{Type: core.EventText, Payload: payload},
	}); err != nil {
		return toolResult{}, newRPCError(codeInternalError, "record progress: "+err.Error())
	}
	return okResult(map[string]any{"ok": true})
}

func (s *Server) handleRaiseAttention(ctx context.Context, sess connSession, args json.RawMessage) (toolResult, *rpcError) {
	var in struct {
		Kind    string   `json:"kind"`
		Message string   `json:"message"`
		Options []string `json:"options"`
	}
	if rerr := decodeArgs(args, &in); rerr != nil {
		return toolResult{}, rerr
	}
	if strings.TrimSpace(in.Message) == "" {
		return errResult("message is required")
	}
	node, ok := s.tree.Get(sess.node)
	if !ok {
		return toolResult{}, newRPCError(codeInternalError, "node not found")
	}
	detail := in.Message
	if len(in.Options) > 0 {
		detail += " [options: " + strings.Join(in.Options, ", ") + "]"
	}
	var input core.EventInput
	switch in.Kind {
	case "error":
		payload, _ := core.MarshalPayload(core.ErrorPayload{Message: detail})
		input = core.EventInput{Type: core.EventError, Payload: payload, Detail: detail}
	default: // question, decision, blocked, review, or anything → a question-flavored prompt
		payload, _ := core.MarshalPayload(core.AwaitingPayload{Reason: core.AwaitQuestion, Detail: detail})
		input = core.EventInput{Type: core.EventAwaitingInput, Payload: payload, Reason: core.AwaitQuestion, Detail: detail}
	}
	if _, err := s.tree.IngestEvents(ctx, sess.node, node.CurrentSessionID, []core.EventInput{input}); err != nil {
		return toolResult{}, newRPCError(codeInternalError, "raise attention: "+err.Error())
	}
	return okResult(map[string]any{"ok": true, "raised": in.Kind})
}

func (s *Server) handleComplete(ctx context.Context, sess connSession, args json.RawMessage) (toolResult, *rpcError) {
	var in struct {
		Result    string   `json:"result"`
		Summary   string   `json:"summary"`
		Artifacts []string `json:"artifacts"`
	}
	if rerr := decodeArgs(args, &in); rerr != nil {
		return toolResult{}, rerr
	}
	if in.Result != "done" && in.Result != "failed" {
		return errResult("result must be \"done\" or \"failed\"")
	}
	if strings.TrimSpace(in.Summary) == "" {
		return errResult("summary is required")
	}
	node, ok := s.tree.Get(sess.node)
	if !ok {
		return toolResult{}, newRPCError(codeInternalError, "node not found")
	}
	completion := map[string]any{"result": in.Result, "summary": in.Summary, "at": s.now().UTC()}
	if in.Artifacts != nil {
		completion["artifacts"] = in.Artifacts
	}
	if rerr := s.setMeta(ctx, node, "completion", completion); rerr != nil {
		return toolResult{}, rerr
	}
	var input core.EventInput
	if in.Result == "done" {
		payload, _ := core.MarshalPayload(core.TurnDonePayload{ResultText: in.Summary})
		input = core.EventInput{Type: core.EventTurnDone, Payload: payload, Detail: in.Summary}
	} else {
		payload, _ := core.MarshalPayload(core.ErrorPayload{Message: in.Summary, Fatal: true})
		input = core.EventInput{Type: core.EventError, Payload: payload, Detail: in.Summary}
	}
	if _, err := s.tree.IngestEvents(ctx, sess.node, node.CurrentSessionID, []core.EventInput{input}); err != nil {
		return toolResult{}, newRPCError(codeInternalError, "record completion: "+err.Error())
	}
	// Terminal: the node's token is spent; later calls on this connection fail.
	s.tokens.Revoke(sess.node)
	return okResult(map[string]any{"ok": true, "result": in.Result})
}

func (s *Server) handleSpawnChild(ctx context.Context, sess connSession, args json.RawMessage) (toolResult, *rpcError) {
	var in struct {
		Title  string   `json:"title"`
		Prompt string   `json:"prompt"`
		Role   string   `json:"role"`
		Mode   string   `json:"mode"`
		Driver string   `json:"driver"`
		Repos  []string `json:"repos"`
	}
	if rerr := decodeArgs(args, &in); rerr != nil {
		return toolResult{}, rerr
	}
	if strings.TrimSpace(in.Title) == "" || strings.TrimSpace(in.Prompt) == "" {
		return errResult("title and prompt are required")
	}
	role := RoleWorker
	if in.Role == string(RoleOrchestrator) {
		role = RoleOrchestrator
	} else if in.Role != "" && in.Role != string(RoleWorker) {
		return errResult("role must be \"worker\" or \"orchestrator\"")
	}
	mode := core.ModeHeadless
	if in.Mode != "" {
		mode = core.SessionMode(in.Mode)
		if !mode.Valid() {
			return errResult("mode must be \"headless\" or \"pty\"")
		}
	}
	childID, err := s.spawner.Spawn(ctx, sess.node, SpawnRequest{
		Title:  in.Title,
		Prompt: in.Prompt,
		Role:   role,
		Mode:   mode,
		Driver: in.Driver,
		Repos:  in.Repos,
	})
	if err != nil {
		if errors.Is(err, ErrLimit) {
			return errResult("spawn denied: %s", err.Error())
		}
		return toolResult{}, newRPCError(codeInternalError, "spawn child: "+err.Error())
	}
	return okResult(map[string]any{"node_id": childID, "status": "spawning"})
}

func (s *Server) handleListChildren(sess connSession, args json.RawMessage) (toolResult, *rpcError) {
	var in struct {
		Depth int `json:"depth"`
	}
	if rerr := decodeArgs(args, &in); rerr != nil {
		return toolResult{}, rerr
	}
	depth := in.Depth
	if depth < 1 {
		depth = 1
	}
	result := map[string]any{"children": s.childrenView(sess.node, depth)}
	if hint := s.recordPoll(sess.node); hint != "" {
		result["hint"] = hint
	}
	return okResult(result)
}

func (s *Server) handleNodeStatus(sess connSession, args json.RawMessage) (toolResult, *rpcError) {
	var in struct {
		NodeID  string `json:"node_id"`
		Subtree *bool  `json:"subtree"`
	}
	if rerr := decodeArgs(args, &in); rerr != nil {
		return toolResult{}, rerr
	}
	target := sess.node
	if in.NodeID != "" {
		target = core.NodeID(in.NodeID)
		if !s.inSubtree(sess.node, target) {
			return errResult("node %s is not in your subtree", target)
		}
	}
	node, ok := s.tree.Get(target)
	if !ok {
		return errResult("node %s not found", target)
	}
	roll := s.tree.Rollup(target)
	byStatus := make(map[string]int, len(roll.ByStatus))
	for st, n := range roll.ByStatus {
		byStatus[string(st)] = n
	}
	percent := 0
	if roll.Total > 0 {
		percent = roll.ByStatus[core.StatusDone] * 100 / roll.Total
	}
	result := map[string]any{
		"node_id":   node.ID,
		"title":     node.Title,
		"status":    node.Status,
		"attention": node.Attention,
		"subtree": map[string]any{
			"total":     roll.Total,
			"by_status": byStatus,
			"attention": roll.Attention,
			"percent":   percent,
		},
	}
	if hint := s.recordPoll(sess.node); hint != "" {
		result["hint"] = hint
	}
	return okResult(result)
}

func (s *Server) handleSendMessage(ctx context.Context, sess connSession, args json.RawMessage) (toolResult, *rpcError) {
	var in struct {
		NodeID string `json:"node_id"`
		Text   string `json:"text"`
	}
	if rerr := decodeArgs(args, &in); rerr != nil {
		return toolResult{}, rerr
	}
	if in.NodeID == "" || strings.TrimSpace(in.Text) == "" {
		return errResult("node_id and text are required")
	}
	target := core.NodeID(in.NodeID)
	if !s.adjacent(sess.node, target) {
		return errResult("node %s is not your parent or child; messages only flow between adjacent nodes", target)
	}
	if err := s.spawner.SendMessage(ctx, sess.node, target, in.Text); err != nil {
		return toolResult{}, newRPCError(codeInternalError, "send message: "+err.Error())
	}
	return okResult(map[string]any{"ok": true})
}

// childrenView gathers a node's children (down to depth levels) with their
// status, latest progress and completion, read from node meta.
func (s *Server) childrenView(parent core.NodeID, depth int) []map[string]any {
	kids := s.tree.Children(parent)
	out := make([]map[string]any, 0, len(kids))
	for _, c := range kids {
		view := map[string]any{
			"node_id":   c.ID,
			"title":     c.Title,
			"status":    c.Status,
			"attention": c.Attention,
		}
		if prog := metaField(c.Meta, "progress"); prog != nil {
			view["progress"] = prog
		}
		if comp := metaField(c.Meta, "completion"); comp != nil {
			view["completion"] = comp
		}
		if depth > 1 {
			if grand := s.childrenView(c.ID, depth-1); len(grand) > 0 {
				view["children"] = grand
			}
		}
		out = append(out, view)
	}
	return out
}

// setMeta merges one key into a node's JSON meta bag and persists it.
func (s *Server) setMeta(ctx context.Context, node core.Node, key string, val any) *rpcError {
	merged, err := mergeMeta(node.Meta, key, val)
	if err != nil {
		return newRPCError(codeInternalError, "encode meta: "+err.Error())
	}
	if _, err := s.tree.UpdateNode(ctx, node.ID, tree.Patch{Meta: &merged}); err != nil {
		return newRPCError(codeInternalError, "persist meta: "+err.Error())
	}
	return nil
}

// inSubtree reports whether target is root or one of its live descendants.
func (s *Server) inSubtree(root, target core.NodeID) bool {
	for _, id := range s.tree.SubtreeIDs(root) {
		if id == target {
			return true
		}
	}
	return false
}

// adjacent reports whether a and b are parent and child in either direction.
func (s *Server) adjacent(a, b core.NodeID) bool {
	if na, ok := s.tree.Get(a); ok && na.ParentID == b {
		return true
	}
	if nb, ok := s.tree.Get(b); ok && nb.ParentID == a {
		return true
	}
	return false
}

// recordPoll tracks status-call frequency per node and returns an anti-poll hint
// when the caller exceeds the threshold within the window.
func (s *Server) recordPoll(node core.NodeID) string {
	now := s.now()
	s.pollMu.Lock()
	defer s.pollMu.Unlock()
	cutoff := now.Add(-pollWindow)
	kept := s.polls[node][:0]
	for _, t := range s.polls[node] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	kept = append(kept, now)
	s.polls[node] = kept
	if len(kept) > pollThreshold {
		return "Stop polling. End your turn; grove will wake you when children report."
	}
	return ""
}

// decodeArgs unmarshals tool arguments, tolerating an absent object.
func decodeArgs(args json.RawMessage, v any) *rpcError {
	if len(args) == 0 {
		return nil
	}
	if err := json.Unmarshal(args, v); err != nil {
		return newRPCError(codeInvalidParams, "invalid arguments: "+err.Error())
	}
	return nil
}

// mergeMeta sets key to val in a node's JSON meta object, tolerating malformed
// prior meta by starting fresh.
func mergeMeta(meta, key string, val any) (string, error) {
	m := map[string]json.RawMessage{}
	if meta != "" {
		_ = json.Unmarshal([]byte(meta), &m)
	}
	raw, err := json.Marshal(val)
	if err != nil {
		return "", fmt.Errorf("marshal meta value: %w", err)
	}
	m[key] = raw
	out, err := json.Marshal(m)
	if err != nil {
		return "", fmt.Errorf("marshal meta: %w", err)
	}
	return string(out), nil
}

// metaField extracts one key from a node's JSON meta bag as generic JSON.
func metaField(meta, key string) any {
	if meta == "" {
		return nil
	}
	var m map[string]json.RawMessage
	if json.Unmarshal([]byte(meta), &m) != nil {
		return nil
	}
	raw, ok := m[key]
	if !ok {
		return nil
	}
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return nil
	}
	return v
}
