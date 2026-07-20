package core

import (
	"errors"
	"testing"
	"time"
)

func TestKindValid(t *testing.T) {
	tests := []struct {
		kind Kind
		want bool
	}{
		{KindWorkspace, true},
		{KindProject, true},
		{KindTask, true},
		{Kind(""), false},
		{Kind("subtask"), false},
	}
	for _, tt := range tests {
		if got := tt.kind.Valid(); got != tt.want {
			t.Errorf("Kind(%q).Valid() = %v, want %v", tt.kind, got, tt.want)
		}
	}
}

func TestCanParent(t *testing.T) {
	tests := []struct {
		child, parent Kind
		want          bool
	}{
		{KindProject, KindWorkspace, true},
		{KindTask, KindProject, true},
		{KindTask, KindTask, true},
		{KindProject, KindProject, false},
		{KindProject, KindTask, false},
		{KindTask, KindWorkspace, false},
		{KindWorkspace, KindWorkspace, false},
		{KindWorkspace, KindProject, false},
	}
	for _, tt := range tests {
		if got := CanParent(tt.child, tt.parent); got != tt.want {
			t.Errorf("CanParent(%s, %s) = %v, want %v", tt.child, tt.parent, got, tt.want)
		}
	}
}

func TestNodeStatusPredicates(t *testing.T) {
	tests := []struct {
		status   NodeStatus
		valid    bool
		active   bool
		terminal bool
	}{
		{StatusIdle, true, false, false},
		{StatusStarting, true, true, false},
		{StatusRunning, true, true, false},
		{StatusAwaitingInput, true, true, false},
		{StatusDone, true, false, true},
		{StatusFailed, true, false, true},
		{StatusInterrupted, true, false, false},
		{NodeStatus("paused"), false, false, false},
	}
	for _, tt := range tests {
		if got := tt.status.Valid(); got != tt.valid {
			t.Errorf("%q.Valid() = %v, want %v", tt.status, got, tt.valid)
		}
		if got := tt.status.Active(); got != tt.active {
			t.Errorf("%q.Active() = %v, want %v", tt.status, got, tt.active)
		}
		if got := tt.status.Terminal(); got != tt.terminal {
			t.Errorf("%q.Terminal() = %v, want %v", tt.status, got, tt.terminal)
		}
	}
}

func validNode() Node {
	now := time.Now()
	return Node{
		ID:        NewNodeID(),
		ParentID:  NewNodeID(),
		Kind:      KindTask,
		Title:     "Optimize RPC",
		Status:    StatusIdle,
		Attention: AttentionNone,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func TestNodeValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Node)
		wantErr bool
	}{
		{"valid task", func(n *Node) {}, false},
		{"valid workspace", func(n *Node) { n.Kind = KindWorkspace; n.ParentID = "" }, false},
		{"empty id", func(n *Node) { n.ID = "" }, true},
		{"bad kind", func(n *Node) { n.Kind = "folder" }, true},
		{"empty title", func(n *Node) { n.Title = "" }, true},
		{"workspace with parent", func(n *Node) { n.Kind = KindWorkspace }, true},
		{"task without parent", func(n *Node) { n.ParentID = "" }, true},
		{"bad status", func(n *Node) { n.Status = "sleeping" }, true},
		{"bad attention", func(n *Node) { n.Attention = "urgent" }, true},
		{"attention without since", func(n *Node) { n.Attention = AttentionError }, true},
		{"attention with since", func(n *Node) {
			n.Attention = AttentionError
			n.AttentionSince = time.Now()
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := validNode()
			tt.mutate(&n)
			err := n.Validate()
			if tt.wantErr {
				if !errors.Is(err, ErrInvalid) {
					t.Fatalf("Validate() = %v, want ErrInvalid", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
		})
	}
}

func TestNodeArchived(t *testing.T) {
	n := validNode()
	if n.Archived() {
		t.Fatal("fresh node reported archived")
	}
	n.ArchivedAt = time.Now()
	if !n.Archived() {
		t.Fatal("archived node not reported archived")
	}
}
