package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/AnkushinDaniil/grove/internal/core"
)

// rowScanner is satisfied by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

// collect drains rows with scan into a slice, closing rows before returning.
func collect[T any](rows *sql.Rows, scan func(rowScanner) (T, error)) ([]T, error) {
	defer func() { _ = rows.Close() }()
	var out []T
	for rows.Next() {
		v, err := scan(rows)
		if err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}
	return out, nil
}

// msFromTime converts a NOT NULL timestamp column's Go value to unix
// milliseconds.
func msFromTime(t time.Time) int64 { return t.UnixMilli() }

// timeFromMS converts a NOT NULL timestamp column back to time.Time.
func timeFromMS(ms int64) time.Time { return time.UnixMilli(ms).UTC() }

// nullMS converts t to a nullable unix-millisecond column value: a zero
// time.Time maps to SQL NULL.
func nullMS(t time.Time) sql.NullInt64 {
	if t.IsZero() {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: t.UnixMilli(), Valid: true}
}

// timeFromNullMS converts a nullable unix-millisecond column back to
// time.Time: SQL NULL maps to the zero time.Time.
func timeFromNullMS(v sql.NullInt64) time.Time {
	if !v.Valid {
		return time.Time{}
	}
	return time.UnixMilli(v.Int64).UTC()
}

// nullStr converts s to a nullable TEXT column value: an empty string maps
// to SQL NULL. Used for nodes.parent_id, which is a foreign key and so
// cannot hold the empty string the root node's ParentID uses to mean "none".
func nullStr(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// strFromNull converts a nullable TEXT column back to a string: SQL NULL
// maps to "".
func strFromNull(v sql.NullString) string {
	if !v.Valid {
		return ""
	}
	return v.String
}

// nullIntFromPtr converts a *int to a nullable INTEGER column value: nil
// maps to SQL NULL. Used for core.Session.ExitCode.
func nullIntFromPtr(p *int) sql.NullInt64 {
	if p == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*p), Valid: true}
}

// ptrFromNullInt converts a nullable INTEGER column back to a *int: SQL NULL
// maps to nil.
func ptrFromNullInt(v sql.NullInt64) *int {
	if !v.Valid {
		return nil
	}
	i := int(v.Int64)
	return &i
}

// scanNode scans one row shaped like the nodes table's full column list (see
// selectLiveNodesSQL / upsertNodeSQL) into a core.Node.
func scanNode(row rowScanner) (core.Node, error) {
	var (
		n                                      core.Node
		id, kind, status, attention, profileID string
		currentSessionID                       string
		parentID                               sql.NullString
		attentionSince, archivedAt             sql.NullInt64
		createdAt, updatedAt                   int64
	)
	if err := row.Scan(
		&id, &parentID, &kind, &n.Title, &n.Brief, &status, &attention, &n.AttentionReason,
		&attentionSince, &n.Driver, &profileID, &currentSessionID, &n.WorkspaceDir,
		&n.Meta, &n.Position, &createdAt, &updatedAt, &archivedAt,
	); err != nil {
		return core.Node{}, fmt.Errorf("scan node row: %w", err)
	}
	n.ID = core.NodeID(id)
	n.ParentID = core.NodeID(strFromNull(parentID))
	n.Kind = core.Kind(kind)
	n.Status = core.NodeStatus(status)
	n.Attention = core.Attention(attention)
	n.AttentionSince = timeFromNullMS(attentionSince)
	n.ProfileID = core.ProfileID(profileID)
	n.CurrentSessionID = core.SessionID(currentSessionID)
	n.CreatedAt = timeFromMS(createdAt)
	n.UpdatedAt = timeFromMS(updatedAt)
	n.ArchivedAt = timeFromNullMS(archivedAt)
	return n, nil
}

// scanSession scans one row shaped like the sessions table's full column
// list into a core.Session.
func scanSession(row rowScanner) (core.Session, error) {
	var (
		sess                                core.Session
		id, nodeID, profileID, mode, status string
		exitCode                            sql.NullInt64
		startedAt                           int64
		endedAt                             sql.NullInt64
	)
	if err := row.Scan(
		&id, &nodeID, &sess.Driver, &profileID, &mode, &sess.DriverSessionID,
		&sess.ParentDriverSessionID, &status, &exitCode, &sess.TranscriptPath, &sess.CWD,
		&startedAt, &endedAt,
	); err != nil {
		return core.Session{}, fmt.Errorf("scan session row: %w", err)
	}
	sess.ID = core.SessionID(id)
	sess.NodeID = core.NodeID(nodeID)
	sess.ProfileID = core.ProfileID(profileID)
	sess.Mode = core.SessionMode(mode)
	sess.Status = core.SessionStatus(status)
	sess.ExitCode = ptrFromNullInt(exitCode)
	sess.StartedAt = timeFromMS(startedAt)
	sess.EndedAt = timeFromNullMS(endedAt)
	return sess, nil
}

// scanEvent scans one row shaped like the events table's full column list
// into a core.Event.
func scanEvent(row rowScanner) (core.Event, error) {
	var (
		ev                               core.Event
		id, nodeID, sessionID, eventType string
		requiresAttention                bool
		ackedAt                          sql.NullInt64
		createdAt                        int64
	)
	if err := row.Scan(
		&id, &nodeID, &sessionID, &eventType, &ev.Payload, &requiresAttention, &ackedAt, &createdAt,
	); err != nil {
		return core.Event{}, fmt.Errorf("scan event row: %w", err)
	}
	ev.ID = core.EventID(id)
	ev.NodeID = core.NodeID(nodeID)
	ev.SessionID = core.SessionID(sessionID)
	ev.Type = core.EventType(eventType)
	ev.RequiresAttention = requiresAttention
	ev.AckedAt = timeFromNullMS(ackedAt)
	ev.CreatedAt = timeFromMS(createdAt)
	return ev, nil
}

// scanProfile scans one row shaped like the profiles table's full column
// list into a core.Profile.
func scanProfile(row rowScanner) (core.Profile, error) {
	var (
		p                core.Profile
		id, driver, name string
		configDir        string
		isDefault        bool
		createdAt        int64
	)
	if err := row.Scan(&id, &driver, &name, &configDir, &isDefault, &createdAt); err != nil {
		return core.Profile{}, fmt.Errorf("scan profile row: %w", err)
	}
	p.ID = core.ProfileID(id)
	p.Driver = driver
	p.Name = name
	p.ConfigDir = configDir
	p.IsDefault = isDefault
	p.CreatedAt = timeFromMS(createdAt)
	return p, nil
}

// scanRepo scans one row shaped like the repos table's full column list into
// a core.Repo.
func scanRepo(row rowScanner) (core.Repo, error) {
	var (
		r                       core.Repo
		id, projectID, name     string
		sourcePath, defaultBase string
		createdAt               int64
	)
	if err := row.Scan(&id, &projectID, &name, &sourcePath, &defaultBase, &createdAt); err != nil {
		return core.Repo{}, fmt.Errorf("scan repo row: %w", err)
	}
	r.ID = core.RepoID(id)
	r.ProjectID = core.NodeID(projectID)
	r.Name = name
	r.SourcePath = sourcePath
	r.DefaultBase = defaultBase
	r.CreatedAt = timeFromMS(createdAt)
	return r, nil
}

// scanWorktree scans one row shaped like the worktrees table's full column
// list into a core.Worktree.
func scanWorktree(row rowScanner) (core.Worktree, error) {
	var (
		w                             core.Worktree
		id, nodeID, repoID            string
		path, branch, baseRef, status string
		createdAt                     int64
		removedAt                     sql.NullInt64
	)
	if err := row.Scan(&id, &nodeID, &repoID, &path, &branch, &baseRef, &status, &createdAt, &removedAt); err != nil {
		return core.Worktree{}, fmt.Errorf("scan worktree row: %w", err)
	}
	w.ID = core.WorktreeID(id)
	w.NodeID = core.NodeID(nodeID)
	w.RepoID = core.RepoID(repoID)
	w.Path = path
	w.Branch = branch
	w.BaseRef = baseRef
	w.Status = core.WorktreeStatus(status)
	w.CreatedAt = timeFromMS(createdAt)
	w.RemovedAt = timeFromNullMS(removedAt)
	return w, nil
}
