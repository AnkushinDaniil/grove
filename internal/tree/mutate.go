package tree

import (
	"context"
	"fmt"
	"time"

	"github.com/AnkushinDaniil/grove/internal/core"
)

// CreateSpec describes a node to create.
type CreateSpec struct {
	ParentID  core.NodeID // empty only for the root workspace
	Kind      core.Kind
	Title     string
	Brief     string
	Driver    string         // empty = inherit
	ProfileID core.ProfileID // empty = inherit
	WorkDir   string         // empty = inherit; absolute when set
}

// Bootstrap returns the root workspace, creating it if the tree is empty.
func (t *Tree) Bootstrap(ctx context.Context, title string) (core.Node, error) {
	t.mu.Lock()
	if root, ok := t.rootLocked(); ok {
		t.mu.Unlock()
		return root, nil
	}
	t.mu.Unlock()
	return t.CreateNode(ctx, CreateSpec{Kind: core.KindWorkspace, Title: title})
}

func (t *Tree) rootLocked() (core.Node, bool) {
	for _, n := range t.nodes {
		if n.Kind == core.KindWorkspace && !n.Archived() {
			return n, true
		}
	}
	return core.Node{}, false
}

// CreateNode validates the spec against the tree, persists and applies it.
func (t *Tree) CreateNode(ctx context.Context, spec CreateSpec) (core.Node, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if spec.Kind == core.KindWorkspace {
		if _, exists := t.rootLocked(); exists {
			return core.Node{}, fmt.Errorf("%w: workspace root already exists", core.ErrInvalid)
		}
	} else {
		parent, ok := t.nodes[spec.ParentID]
		if !ok {
			return core.Node{}, fmt.Errorf("%w: parent %s not found", core.ErrInvalid, spec.ParentID)
		}
		if parent.Archived() {
			return core.Node{}, fmt.Errorf("%w: parent %s is archived", core.ErrInvalid, spec.ParentID)
		}
		if !core.CanParent(spec.Kind, parent.Kind) {
			return core.Node{}, fmt.Errorf("%w: %s cannot be created under %s", core.ErrInvalid, spec.Kind, parent.Kind)
		}
	}

	now := t.now()
	node := core.Node{
		ID:        core.NewNodeID(),
		ParentID:  spec.ParentID,
		Kind:      spec.Kind,
		Title:     spec.Title,
		Brief:     spec.Brief,
		Status:    core.StatusIdle,
		Attention: core.AttentionNone,
		Driver:    spec.Driver,
		ProfileID: spec.ProfileID,
		WorkDir:   spec.WorkDir,
		Meta:      "{}",
		Position:  len(t.children[spec.ParentID]),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := node.Validate(); err != nil {
		return core.Node{}, err
	}
	if err := t.store.SaveNodes(ctx, []core.Node{node}); err != nil {
		return core.Node{}, fmt.Errorf("persist node: %w", err)
	}
	t.nodes[node.ID] = node
	if node.ParentID != "" {
		t.children[node.ParentID] = append(t.children[node.ParentID], node.ID)
	}
	t.broadcastLocked(Delta{Nodes: []core.Node{node}})
	return node, nil
}

// Patch updates optional node fields; nil means "leave unchanged".
type Patch struct {
	Title     *string
	Brief     *string
	Driver    *string
	ProfileID *core.ProfileID
	WorkDir   *string
	Meta      *string
}

// UpdateNode applies a patch to a live node.
func (t *Tree) UpdateNode(ctx context.Context, id core.NodeID, p Patch) (core.Node, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	node, ok := t.nodes[id]
	if !ok {
		return core.Node{}, fmt.Errorf("%w: node %s not found", core.ErrInvalid, id)
	}
	if node.Archived() {
		return core.Node{}, fmt.Errorf("%w: node %s is archived", core.ErrInvalid, id)
	}
	if p.Title != nil {
		node.Title = *p.Title
	}
	if p.Brief != nil {
		node.Brief = *p.Brief
	}
	if p.Driver != nil {
		node.Driver = *p.Driver
	}
	if p.ProfileID != nil {
		node.ProfileID = *p.ProfileID
	}
	if p.WorkDir != nil {
		node.WorkDir = *p.WorkDir
	}
	if p.Meta != nil {
		node.Meta = *p.Meta
	}
	node.UpdatedAt = t.now()
	if err := node.Validate(); err != nil {
		return core.Node{}, err
	}
	if err := t.store.SaveNodes(ctx, []core.Node{node}); err != nil {
		return core.Node{}, fmt.Errorf("persist node: %w", err)
	}
	t.nodes[id] = node
	t.broadcastLocked(Delta{Nodes: []core.Node{node}})
	return node, nil
}

// SetWorkspaceDir records the task workspace directory created by the worktree
// engine.
func (t *Tree) SetWorkspaceDir(ctx context.Context, id core.NodeID, dir string) (core.Node, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	node, ok := t.nodes[id]
	if !ok {
		return core.Node{}, fmt.Errorf("%w: node %s not found", core.ErrInvalid, id)
	}
	node.WorkspaceDir = dir
	node.UpdatedAt = t.now()
	if err := t.store.SaveNodes(ctx, []core.Node{node}); err != nil {
		return core.Node{}, fmt.Errorf("persist node: %w", err)
	}
	t.nodes[id] = node
	t.broadcastLocked(Delta{Nodes: []core.Node{node}})
	return node, nil
}

// Ack clears the node's attention flag.
func (t *Tree) Ack(ctx context.Context, id core.NodeID) (core.Node, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	node, ok := t.nodes[id]
	if !ok {
		return core.Node{}, fmt.Errorf("%w: node %s not found", core.ErrInvalid, id)
	}
	if node.Attention == core.AttentionNone {
		return node, nil
	}
	node.Attention = core.AttentionNone
	node.AttentionReason = ""
	node.AttentionSince = time.Time{}
	node.UpdatedAt = t.now()
	if err := t.store.SaveNodes(ctx, []core.Node{node}); err != nil {
		return core.Node{}, fmt.Errorf("persist node: %w", err)
	}
	t.nodes[id] = node
	t.broadcastLocked(Delta{Nodes: []core.Node{node}})
	return node, nil
}

// Archive marks the node and all live descendants archived and returns the
// affected IDs (deepest last). The caller is responsible for stopping sessions
// and cleaning worktrees for the returned nodes. The root workspace cannot be
// archived.
func (t *Tree) Archive(ctx context.Context, id core.NodeID) ([]core.NodeID, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	node, ok := t.nodes[id]
	if !ok {
		return nil, fmt.Errorf("%w: node %s not found", core.ErrInvalid, id)
	}
	if node.Kind == core.KindWorkspace {
		return nil, fmt.Errorf("%w: the workspace root cannot be archived", core.ErrInvalid)
	}
	if node.Archived() {
		return nil, nil
	}
	ids := t.subtreeLocked(id)
	now := t.now()
	updated := make([]core.Node, 0, len(ids))
	for _, nid := range ids {
		n := t.nodes[nid]
		if n.Archived() {
			continue
		}
		n.ArchivedAt = now
		n.UpdatedAt = now
		updated = append(updated, n)
	}
	if err := t.store.SaveNodes(ctx, updated); err != nil {
		return nil, fmt.Errorf("persist archive: %w", err)
	}
	archived := make([]core.NodeID, 0, len(updated))
	for _, n := range updated {
		t.nodes[n.ID] = n
		archived = append(archived, n.ID)
	}
	t.broadcastLocked(Delta{Nodes: updated})
	return archived, nil
}
