package tree

import (
	"context"
	"fmt"

	"github.com/AnkushinDaniil/grove/internal/core"
)

// ApplySession upserts a session snapshot and derives the owning node's status
// from it. The session manager is the single producer of session snapshots.
func (t *Tree) ApplySession(ctx context.Context, s core.Session) (core.Session, error) {
	if err := s.Validate(); err != nil {
		return core.Session{}, err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	node, ok := t.nodes[s.NodeID]
	if !ok {
		return core.Session{}, fmt.Errorf("%w: node %s not found", core.ErrInvalid, s.NodeID)
	}
	if err := t.store.SaveSessions(ctx, []core.Session{s}); err != nil {
		return core.Session{}, fmt.Errorf("persist session: %w", err)
	}

	exitOK := s.ExitCode != nil && *s.ExitCode == 0
	node.Status = core.NodeStatusFor(s.Status, exitOK)
	node.CurrentSessionID = s.ID
	node.UpdatedAt = t.now()
	if err := t.store.SaveNodes(ctx, []core.Node{node}); err != nil {
		return core.Session{}, fmt.Errorf("persist node: %w", err)
	}

	t.sessions[s.ID] = s
	t.nodeSession[s.NodeID] = s.ID
	t.nodes[node.ID] = node
	t.broadcastLocked(Delta{Nodes: []core.Node{node}, Sessions: []core.Session{s}})
	return s, nil
}

// attentionRank orders attention kinds so a more urgent flag is never
// overwritten by a less urgent one until the user acks.
var attentionRank = map[core.Attention]int{
	core.AttentionNone:       0,
	core.AttentionDone:       1,
	core.AttentionReview:     2,
	core.AttentionQuestion:   3,
	core.AttentionPermission: 4,
	core.AttentionError:      5,
}

// IngestEvents appends normalized driver events for a node, raising node
// attention as a side effect. Events are persisted as one batch.
func (t *Tree) IngestEvents(ctx context.Context, nodeID core.NodeID, sessionID core.SessionID, inputs []core.EventInput) ([]core.Event, error) {
	if len(inputs) == 0 {
		return nil, nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	node, ok := t.nodes[nodeID]
	if !ok {
		return nil, fmt.Errorf("%w: node %s not found", core.ErrInvalid, nodeID)
	}

	now := t.now()
	events := make([]core.Event, 0, len(inputs))
	best := node.Attention
	bestReason := node.AttentionReason
	for _, in := range inputs {
		if !in.Type.Valid() {
			return nil, fmt.Errorf("%w: unknown event type %q", core.ErrInvalid, in.Type)
		}
		attn := core.AttentionFor(in.Type, in.Reason)
		payload := in.Payload
		if payload == "" {
			payload = "{}"
		}
		events = append(events, core.Event{
			ID:                core.NewEventID(),
			NodeID:            nodeID,
			SessionID:         sessionID,
			Type:              in.Type,
			Payload:           payload,
			RequiresAttention: attn != core.AttentionNone,
			CreatedAt:         now,
		})
		if attentionRank[attn] > attentionRank[best] {
			best = attn
			bestReason = in.Detail
		}
	}
	if err := t.store.AppendEvents(ctx, events); err != nil {
		return nil, fmt.Errorf("persist events: %w", err)
	}

	delta := Delta{Events: events}
	if best != node.Attention || bestReason != node.AttentionReason {
		node.Attention = best
		node.AttentionReason = bestReason
		node.AttentionSince = now
		node.UpdatedAt = now
		if err := t.store.SaveNodes(ctx, []core.Node{node}); err != nil {
			return nil, fmt.Errorf("persist node: %w", err)
		}
		t.nodes[node.ID] = node
		delta.Nodes = []core.Node{node}
	}
	t.broadcastLocked(delta)
	return events, nil
}
