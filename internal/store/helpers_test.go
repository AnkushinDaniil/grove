package store

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"modernc.org/sqlite"

	"github.com/AnkushinDaniil/grove/internal/core"
)

// assertUniqueViolation fails the test unless err is a SQLite UNIQUE
// constraint failure.
func assertUniqueViolation(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("want a UNIQUE constraint error, got nil")
	}
	var sqliteErr *sqlite.Error
	if !errors.As(err, &sqliteErr) {
		t.Fatalf("error = %v (%T), want a *sqlite.Error", err, err)
	}
	if !strings.Contains(sqliteErr.Error(), "UNIQUE") {
		t.Errorf("error = %q, want it to mention UNIQUE", sqliteErr.Error())
	}
}

// newTestStore opens a fresh SQLite-backed Store in a temp directory,
// closing it automatically at test cleanup. WAL mode needs a real file, so
// this deliberately does not use ":memory:".
func newTestStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "grove.db")
	s, err := Open(t.Context(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return s
}

// msTime builds a time.Time the same way the store's scan helpers hand them
// back (UnixMilli + UTC), so fixtures compare equal to round-tripped values
// with reflect.DeepEqual.
func msTime(ms int64) time.Time { return time.UnixMilli(ms).UTC() }

// testNode returns a fully populated node fixture. parentID may be "" for a
// root node.
func testNode(id, parentID core.NodeID) core.Node {
	return core.Node{
		ID:               id,
		ParentID:         parentID,
		Kind:             core.KindTask,
		Title:            "Test node " + string(id),
		Brief:            "brief text",
		Status:           core.StatusIdle,
		Attention:        core.AttentionNone,
		Driver:           "claude",
		ProfileID:        core.ProfileID("profile-1"),
		CurrentSessionID: "",
		WorkspaceDir:     "/tmp/ws",
		WorkDir:          "/tmp/work",
		Meta:             "{}",
		Position:         0,
		CreatedAt:        msTime(1_700_000_000_000),
		UpdatedAt:        msTime(1_700_000_000_000),
	}
}

func mustSaveNode(t *testing.T, s *Store, n core.Node) {
	t.Helper()
	if err := s.SaveNodes(t.Context(), []core.Node{n}); err != nil {
		t.Fatalf("SaveNodes(%s): %v", n.ID, err)
	}
}

// loadNodeDirect reads one node straight from the nodes table via scanNode,
// independent of LoadLive's archived-filtering/join logic.
func loadNodeDirect(t *testing.T, s *Store, id core.NodeID) core.Node {
	t.Helper()
	row := s.db.QueryRowContext(t.Context(), `
		SELECT id, parent_id, kind, title, brief, status, attention, attention_reason,
			attention_since, driver, profile_id, current_session_id, workspace_dir,
			work_dir, meta, position, created_at, updated_at, archived_at
		FROM nodes WHERE id = ?`, string(id))
	n, err := scanNode(row)
	if err != nil {
		t.Fatalf("scanNode(%s): %v", id, err)
	}
	return n
}

// testSession returns a fully populated, live (no exit code, no ended_at)
// session fixture bound to nodeID.
func testSession(id core.SessionID, nodeID core.NodeID) core.Session {
	return core.Session{
		ID:                    id,
		NodeID:                nodeID,
		Driver:                "claude",
		ProfileID:             core.ProfileID("profile-1"),
		Mode:                  core.ModeHeadless,
		DriverSessionID:       "driver-sess-1",
		ParentDriverSessionID: "",
		Status:                core.SessionRunning,
		ExitCode:              nil,
		TranscriptPath:        "/tmp/transcript.jsonl",
		CWD:                   "/tmp/ws",
		StartedAt:             msTime(1_700_000_000_000),
	}
}

func mustSaveSession(t *testing.T, s *Store, sess core.Session) {
	t.Helper()
	if err := s.SaveSessions(t.Context(), []core.Session{sess}); err != nil {
		t.Fatalf("SaveSessions(%s): %v", sess.ID, err)
	}
}

// loadSessionDirect reads one session straight from the sessions table via
// scanSession.
func loadSessionDirect(t *testing.T, s *Store, id core.SessionID) core.Session {
	t.Helper()
	row := s.db.QueryRowContext(t.Context(), `
		SELECT id, node_id, driver, profile_id, mode, driver_session_id,
			parent_driver_session_id, status, exit_code, transcript_path, cwd,
			started_at, ended_at
		FROM sessions WHERE id = ?`, string(id))
	sess, err := scanSession(row)
	if err != nil {
		t.Fatalf("scanSession(%s): %v", id, err)
	}
	return sess
}
