package session

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/AnkushinDaniil/grove/internal/core"
)

func TestExtractResumeID(t *testing.T) {
	const idA = "e777f079-fa66-474d-9200-e3e1235b76b1"
	const idB = "e73d8638-65bd-417f-99e1-229187512ad4"
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"none", "plain terminal output, no farewell", ""},
		{"simple", "Resume this session with:\nclaude --resume " + idA + "\n", idA},
		{"last wins", "claude --resume " + idA + "\r\n…later…\nclaude --resume " + idB, idB},
		{"embedded ansi", "\x1b[2mclaude --resume " + idA + "\x1b[0m", idA},
		{"uppercase not matched", "claude --resume E777F079-FA66-474D-9200-E3E1235B76B1", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExtractResumeID([]byte(tt.raw)); got != tt.want {
				t.Fatalf("ExtractResumeID = %q, want %q", got, tt.want)
			}
		})
	}
}

type fakeBackfillStore struct {
	mu       sync.Mutex
	sessions []core.Session
	saved    []core.Session
}

func (f *fakeBackfillStore) SessionsWithoutResumeID(context.Context) ([]core.Session, error) {
	return f.sessions, nil
}

func (f *fakeBackfillStore) SaveSessions(_ context.Context, ss []core.Session) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.saved = append(f.saved, ss...)
	return nil
}

func TestBackfillResumeIDs(t *testing.T) {
	dir := t.TempDir()
	const conv = "e777f079-fa66-474d-9200-e3e1235b76b1"

	withFarewell := core.Session{ID: core.NewSessionID(), NodeID: "n1", Driver: "claude", Mode: core.ModePTY, Status: core.SessionInterrupted, CWD: "/tmp"}
	noFarewell := core.Session{ID: core.NewSessionID(), NodeID: "n2", Driver: "claude", Mode: core.ModePTY, Status: core.SessionExited, CWD: "/tmp"}
	noScrollback := core.Session{ID: core.NewSessionID(), NodeID: "n3", Driver: "claude", Mode: core.ModePTY, Status: core.SessionExited, CWD: "/tmp"}

	writeScrollback := func(id core.SessionID, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, string(id)+".bin"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeScrollback(withFarewell.ID, "goodbye\nclaude --resume "+conv+"\n")
	writeScrollback(noFarewell.ID, "just output, no hint")

	st := &fakeBackfillStore{sessions: []core.Session{withFarewell, noFarewell, noScrollback}}
	n, err := BackfillResumeIDs(t.Context(), st, dir)
	if err != nil {
		t.Fatalf("BackfillResumeIDs: %v", err)
	}
	if n != 1 || len(st.saved) != 1 {
		t.Fatalf("healed %d (saved %d), want exactly 1", n, len(st.saved))
	}
	if st.saved[0].ID != withFarewell.ID || st.saved[0].DriverSessionID != conv {
		t.Fatalf("healed wrong session: %+v", st.saved[0])
	}

	// Disabled persistence dir is a no-op.
	if n, err := BackfillResumeIDs(t.Context(), st, ""); err != nil || n != 0 {
		t.Fatalf("empty dir backfill = (%d, %v), want (0, nil)", n, err)
	}
}
