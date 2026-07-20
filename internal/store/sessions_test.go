package store

import (
	"reflect"
	"testing"

	"github.com/AnkushinDaniil/grove/internal/core"
)

func TestSaveSessionsRoundTrip(t *testing.T) {
	s := newTestStore(t)
	n := testNode(core.NewNodeID(), "")
	mustSaveNode(t, s, n)

	sess := testSession(core.NewSessionID(), n.ID)
	exitCode := 0
	sess.ExitCode = &exitCode
	sess.EndedAt = msTime(1_700_000_005_000)
	sess.Status = core.SessionExited
	mustSaveSession(t, s, sess)

	got := loadSessionDirect(t, s, sess.ID)
	if !reflect.DeepEqual(got, sess) {
		t.Errorf("session round trip mismatch:\ngot  %+v\nwant %+v", got, sess)
	}
}

func TestSaveSessionsNullExitCodeAndEndedAt(t *testing.T) {
	s := newTestStore(t)
	n := testNode(core.NewNodeID(), "")
	mustSaveNode(t, s, n)

	sess := testSession(core.NewSessionID(), n.ID) // ExitCode nil, EndedAt zero: still live
	mustSaveSession(t, s, sess)

	got := loadSessionDirect(t, s, sess.ID)
	if got.ExitCode != nil {
		t.Errorf("ExitCode = %d, want nil", *got.ExitCode)
	}
	if !got.EndedAt.IsZero() {
		t.Errorf("EndedAt = %v, want zero", got.EndedAt)
	}
}

func TestSaveSessionsUpsertUpdatesInPlace(t *testing.T) {
	s := newTestStore(t)
	n := testNode(core.NewNodeID(), "")
	mustSaveNode(t, s, n)

	sess := testSession(core.NewSessionID(), n.ID)
	mustSaveSession(t, s, sess)

	sess.Status = core.SessionAwaitingInput
	mustSaveSession(t, s, sess)

	got := loadSessionDirect(t, s, sess.ID)
	if got.Status != core.SessionAwaitingInput {
		t.Errorf("Status = %s, want %s", got.Status, core.SessionAwaitingInput)
	}

	var count int
	if err := s.db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM sessions WHERE id = ?", string(sess.ID)).
		Scan(&count); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if count != 1 {
		t.Errorf("row count for session %s = %d, want 1 (upsert must not duplicate)", sess.ID, count)
	}
}

func TestMarkInterrupted(t *testing.T) {
	s := newTestStore(t)
	n := testNode(core.NewNodeID(), "")
	mustSaveNode(t, s, n)

	starting := testSession(core.NewSessionID(), n.ID)
	starting.Status = core.SessionStarting

	awaiting := testSession(core.NewSessionID(), n.ID)
	awaiting.Status = core.SessionAwaitingInput

	exited := testSession(core.NewSessionID(), n.ID)
	exited.Status = core.SessionExited
	exited.EndedAt = msTime(1_700_000_009_000)

	mustSaveSession(t, s, starting)
	mustSaveSession(t, s, awaiting)
	mustSaveSession(t, s, exited)

	at := msTime(1_700_000_010_000)
	affected, err := s.MarkInterrupted(t.Context(), at)
	if err != nil {
		t.Fatalf("MarkInterrupted: %v", err)
	}
	if affected != 2 {
		t.Errorf("MarkInterrupted affected = %d, want 2", affected)
	}

	for _, id := range []core.SessionID{starting.ID, awaiting.ID} {
		got := loadSessionDirect(t, s, id)
		if got.Status != core.SessionInterrupted {
			t.Errorf("session %s Status = %s, want %s", id, got.Status, core.SessionInterrupted)
		}
		if !got.EndedAt.Equal(at) {
			t.Errorf("session %s EndedAt = %v, want %v", id, got.EndedAt, at)
		}
	}

	gotExited := loadSessionDirect(t, s, exited.ID)
	if gotExited.Status != core.SessionExited {
		t.Errorf("already-terminal session Status = %s, want unchanged %s", gotExited.Status, core.SessionExited)
	}
	if !gotExited.EndedAt.Equal(exited.EndedAt) {
		t.Errorf("already-terminal session EndedAt = %v, want unchanged %v", gotExited.EndedAt, exited.EndedAt)
	}
}

func TestMarkInterruptedNoLiveSessions(t *testing.T) {
	s := newTestStore(t)
	affected, err := s.MarkInterrupted(t.Context(), msTime(1_700_000_000_000))
	if err != nil {
		t.Fatalf("MarkInterrupted: %v", err)
	}
	if affected != 0 {
		t.Errorf("MarkInterrupted affected = %d, want 0", affected)
	}
}

func TestSaveSessionsEmptyIsNoop(t *testing.T) {
	s := newTestStore(t)
	if err := s.SaveSessions(t.Context(), nil); err != nil {
		t.Errorf("SaveSessions(nil): %v", err)
	}
}
