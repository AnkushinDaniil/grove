package core

import (
	"fmt"
	"time"
)

// Kind is the structural role of a node in the tree.
// "Orchestrator vs worker" is deliberately not a kind: any node with children may run
// an orchestrator session; leaves default to interactive worker sessions.
type Kind string

const (
	KindWorkspace Kind = "workspace" // single root
	KindProject   Kind = "project"   // owns repos
	KindTask      Kind = "task"      // nested arbitrarily under project or task
)

func (k Kind) Valid() bool {
	switch k {
	case KindWorkspace, KindProject, KindTask:
		return true
	}
	return false
}

// CanParent reports whether a node of kind child may be created under a node of
// kind parent. A workspace has no parent (checked separately in Node.Validate).
func CanParent(child, parent Kind) bool {
	switch child {
	case KindProject:
		return parent == KindWorkspace
	case KindTask:
		return parent == KindProject || parent == KindTask
	}
	return false
}

// NodeStatus is derived from the node's current session (or absence of one).
type NodeStatus string

const (
	StatusIdle          NodeStatus = "idle" // no active session
	StatusStarting      NodeStatus = "starting"
	StatusRunning       NodeStatus = "running"
	StatusAwaitingInput NodeStatus = "awaiting_input"
	StatusDone          NodeStatus = "done"
	StatusFailed        NodeStatus = "failed"
	StatusInterrupted   NodeStatus = "interrupted" // session died with the daemon
)

func (s NodeStatus) Valid() bool {
	switch s {
	case StatusIdle, StatusStarting, StatusRunning, StatusAwaitingInput,
		StatusDone, StatusFailed, StatusInterrupted:
		return true
	}
	return false
}

// Active reports whether the node currently has a live session doing work.
func (s NodeStatus) Active() bool {
	return s == StatusStarting || s == StatusRunning || s == StatusAwaitingInput
}

// Terminal reports whether the node reached an end state for its current task.
func (s NodeStatus) Terminal() bool { return s == StatusDone || s == StatusFailed }

// Attention flags why a node needs the user. It is sticky until acknowledged
// or cleared by user input reaching the session.
type Attention string

const (
	AttentionNone       Attention = "none"
	AttentionPermission Attention = "permission" // tool/permission prompt
	AttentionQuestion   Attention = "question"   // agent asked or idles on input
	AttentionDone       Attention = "done"       // finished, review the result
	AttentionError      Attention = "error"
	AttentionReview     Attention = "review" // human review needed (dirty worktree, PR comments)
)

func (a Attention) Valid() bool {
	switch a {
	case AttentionNone, AttentionPermission, AttentionQuestion,
		AttentionDone, AttentionError, AttentionReview:
		return true
	}
	return false
}

// Node is an immutable snapshot of one tree node. Mutations happen in the tree
// actor by copying the struct; never modify a Node reached from a snapshot.
type Node struct {
	ID              NodeID
	ParentID        NodeID // empty for the root workspace
	Kind            Kind
	Title           string
	Brief           string // task description / initial prompt
	Status          NodeStatus
	Attention       Attention
	AttentionReason string
	AttentionSince  time.Time // zero when Attention == AttentionNone

	// Driver and ProfileID are empty to inherit from the parent chain;
	// resolution happens in the tree actor, never denormalized.
	Driver    string
	ProfileID ProfileID

	CurrentSessionID SessionID
	WorkspaceDir     string // task workspace dir; empty until a worktree exists
	Meta             string // JSON bag (PR URLs, pinned, ...)
	Position         int    // sibling sort order

	CreatedAt  time.Time
	UpdatedAt  time.Time
	ArchivedAt time.Time // zero = live
}

func (n Node) Archived() bool { return !n.ArchivedAt.IsZero() }

// Validate checks the node's own invariants. Parent-kind compatibility needs
// the parent node and is enforced by the tree actor via CanParent.
func (n Node) Validate() error {
	if n.ID == "" {
		return fmt.Errorf("%w: node id is empty", ErrInvalid)
	}
	if !n.Kind.Valid() {
		return fmt.Errorf("%w: unknown node kind %q", ErrInvalid, n.Kind)
	}
	if n.Title == "" {
		return fmt.Errorf("%w: node title is empty", ErrInvalid)
	}
	if n.Kind == KindWorkspace && n.ParentID != "" {
		return fmt.Errorf("%w: workspace node must not have a parent", ErrInvalid)
	}
	if n.Kind != KindWorkspace && n.ParentID == "" {
		return fmt.Errorf("%w: %s node requires a parent", ErrInvalid, n.Kind)
	}
	if !n.Status.Valid() {
		return fmt.Errorf("%w: unknown node status %q", ErrInvalid, n.Status)
	}
	if !n.Attention.Valid() {
		return fmt.Errorf("%w: unknown attention %q", ErrInvalid, n.Attention)
	}
	if n.Attention != AttentionNone && n.AttentionSince.IsZero() {
		return fmt.Errorf("%w: attention %q requires attention_since", ErrInvalid, n.Attention)
	}
	return nil
}
