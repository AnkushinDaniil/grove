package core

import "github.com/google/uuid"

// Typed IDs keep the different entity keys from being mixed up at compile time.
// All IDs are UUIDv7 strings: time-sortable, so append-only tables order by primary key.
type (
	NodeID     string
	SessionID  string
	EventID    string
	RepoID     string
	WorktreeID string
	ProfileID  string
)

func newV7() string { return uuid.Must(uuid.NewV7()).String() }

func NewNodeID() NodeID         { return NodeID(newV7()) }
func NewSessionID() SessionID   { return SessionID(newV7()) }
func NewEventID() EventID       { return EventID(newV7()) }
func NewRepoID() RepoID         { return RepoID(newV7()) }
func NewWorktreeID() WorktreeID { return WorktreeID(newV7()) }
func NewProfileID() ProfileID   { return ProfileID(newV7()) }
