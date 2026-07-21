package api

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/store"
	"github.com/AnkushinDaniil/grove/internal/tree"
)

// resumeFixture builds a Handlers with a real store+tree, a claude-driver
// project/task, and a fake home holding transcripts for the given ids.
func resumeFixture(t *testing.T, transcripts ...string) (*Handlers, core.NodeID, *store.Store) {
	t.Helper()
	home := t.TempDir()
	projects := filepath.Join(home, ".claude", "projects", "-tmp-ws")
	if err := os.MkdirAll(projects, 0o750); err != nil {
		t.Fatal(err)
	}
	for _, id := range transcripts {
		if err := os.WriteFile(filepath.Join(projects, id+".jsonl"), []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	st, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "grove.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	tr := tree.New(st)
	root, err := tr.Bootstrap(t.Context(), "ws")
	if err != nil {
		t.Fatal(err)
	}
	proj, err := tr.CreateNode(t.Context(), tree.CreateSpec{ParentID: root.ID, Kind: core.KindProject, Title: "P", Driver: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	task, err := tr.CreateNode(t.Context(), tree.CreateSpec{ParentID: proj.ID, Kind: core.KindTask, Title: "T"})
	if err != nil {
		t.Fatal(err)
	}

	h := New(Config{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Home:   func() (string, error) { return home, nil },
		Tree:   tr,
		Store:  st,
	})
	return h, task.ID, st
}

func saveSession(t *testing.T, st *store.Store, node core.NodeID, driverSessionID string) {
	t.Helper()
	s := core.Session{
		ID: core.NewSessionID(), NodeID: node, Driver: "claude", Mode: core.ModePTY,
		DriverSessionID: driverSessionID, Status: core.SessionInterrupted, CWD: "/tmp",
	}
	if err := st.SaveSessions(t.Context(), []core.Session{s}); err != nil {
		t.Fatal(err)
	}
}

func TestResolveResumeID(t *testing.T) {
	const good = "e777f079-fa66-474d-9200-e3e1235b76b1"
	const poisoned = "9e28192c-52fe-4d47-9147-c9f7110ec6db"

	t.Run("existing transcript passes through", func(t *testing.T) {
		h, node, _ := resumeFixture(t, good)
		got, err := h.resolveResumeID(t.Context(), node, good)
		if err != nil || got != good {
			t.Fatalf("resolveResumeID = (%q, %v), want passthrough", got, err)
		}
	})

	t.Run("poisoned id falls back to history", func(t *testing.T) {
		h, node, st := resumeFixture(t, good)
		saveSession(t, st, node, good)     // older, resumable
		saveSession(t, st, node, poisoned) // latest, transcript missing
		got, err := h.resolveResumeID(t.Context(), node, poisoned)
		if err != nil || got != good {
			t.Fatalf("resolveResumeID = (%q, %v), want fallback to %q", got, err, good)
		}
	})

	t.Run("no resumable history errors", func(t *testing.T) {
		h, node, st := resumeFixture(t) // no transcripts at all
		saveSession(t, st, node, poisoned)
		if _, err := h.resolveResumeID(t.Context(), node, poisoned); err == nil {
			t.Fatal("expected an error when nothing is resumable")
		}
	})
}
