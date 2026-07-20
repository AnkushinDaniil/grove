package store

import (
	"testing"

	"github.com/AnkushinDaniil/grove/internal/core"
)

func TestLoadLiveExcludesArchivedNodes(t *testing.T) {
	s := newTestStore(t)
	live := testNode(core.NewNodeID(), "")
	archived := testNode(core.NewNodeID(), "")
	archived.ArchivedAt = msTime(1_700_000_003_000)
	if err := s.SaveNodes(t.Context(), []core.Node{live, archived}); err != nil {
		t.Fatalf("SaveNodes: %v", err)
	}

	nodes, _, err := s.LoadLive(t.Context())
	if err != nil {
		t.Fatalf("LoadLive: %v", err)
	}
	if len(nodes) != 1 || nodes[0].ID != live.ID {
		t.Errorf("LoadLive nodes = %v, want only [%s]", nodeIDs(nodes), live.ID)
	}
}

func nodeIDs(nodes []core.Node) []core.NodeID {
	out := make([]core.NodeID, len(nodes))
	for i, n := range nodes {
		out[i] = n.ID
	}
	return out
}

func TestLoadLivePicksLatestSessionPerNode(t *testing.T) {
	s := newTestStore(t)
	n := testNode(core.NewNodeID(), "")
	mustSaveNode(t, s, n)

	older := testSession(core.NewSessionID(), n.ID)
	older.StartedAt = msTime(1_700_000_000_000)
	newer := testSession(core.NewSessionID(), n.ID)
	newer.StartedAt = msTime(1_700_000_100_000)
	if err := s.SaveSessions(t.Context(), []core.Session{older, newer}); err != nil {
		t.Fatalf("SaveSessions: %v", err)
	}

	_, sessions, err := s.LoadLive(t.Context())
	if err != nil {
		t.Fatalf("LoadLive: %v", err)
	}
	if len(sessions) != 1 || sessions[0].ID != newer.ID {
		t.Errorf("LoadLive sessions = %v, want only the newer session %s", sessionIDs(sessions), newer.ID)
	}
}

func sessionIDs(sessions []core.Session) []core.SessionID {
	out := make([]core.SessionID, len(sessions))
	for i, sess := range sessions {
		out[i] = sess.ID
	}
	return out
}

func TestLoadLiveOmitsSessionForNodeWithNone(t *testing.T) {
	s := newTestStore(t)
	n := testNode(core.NewNodeID(), "")
	mustSaveNode(t, s, n)

	nodes, sessions, err := s.LoadLive(t.Context())
	if err != nil {
		t.Fatalf("LoadLive: %v", err)
	}
	if len(nodes) != 1 {
		t.Errorf("len(nodes) = %d, want 1", len(nodes))
	}
	if len(sessions) != 0 {
		t.Errorf("LoadLive sessions = %v, want none", sessionIDs(sessions))
	}
}

func TestLoadLiveOmitsSessionsOfArchivedNodes(t *testing.T) {
	s := newTestStore(t)
	archived := testNode(core.NewNodeID(), "")
	archived.ArchivedAt = msTime(1_700_000_003_000)
	mustSaveNode(t, s, archived)

	sess := testSession(core.NewSessionID(), archived.ID)
	if err := s.SaveSessions(t.Context(), []core.Session{sess}); err != nil {
		t.Fatalf("SaveSessions: %v", err)
	}

	nodes, sessions, err := s.LoadLive(t.Context())
	if err != nil {
		t.Fatalf("LoadLive: %v", err)
	}
	if len(nodes) != 0 {
		t.Errorf("LoadLive nodes = %v, want none (archived)", nodeIDs(nodes))
	}
	if len(sessions) != 0 {
		t.Errorf("LoadLive sessions = %v, want none (owning node archived)", sessionIDs(sessions))
	}
}

func TestLoadLiveEmptyDatabase(t *testing.T) {
	s := newTestStore(t)
	nodes, sessions, err := s.LoadLive(t.Context())
	if err != nil {
		t.Fatalf("LoadLive: %v", err)
	}
	if len(nodes) != 0 || len(sessions) != 0 {
		t.Errorf("LoadLive on empty db = (%v, %v), want (nil, nil)", nodes, sessions)
	}
}
