package store

import (
	"path/filepath"
	"testing"

	"github.com/AnkushinDaniil/grove/internal/core"
)

func TestOpenCreatesParentDirAndMigrates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "sub", "grove.db")

	s, err := Open(t.Context(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() {
		if err := s.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	var count int
	if err := s.db.QueryRowContext(t.Context(),
		"SELECT COUNT(*) FROM schema_migrations WHERE version = 1").Scan(&count); err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	if count != 1 {
		t.Errorf("schema_migrations version 1 count = %d, want 1", count)
	}
}

func TestOpenTwiceIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "grove.db")

	s1, err := Open(t.Context(), path)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	if err := s1.Close(); err != nil {
		t.Fatalf("close first store: %v", err)
	}

	s2, err := Open(t.Context(), path)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer func() {
		if err := s2.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	migrations, err := loadMigrations()
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	var count int
	if err := s2.db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	if count != len(migrations) {
		t.Errorf("schema_migrations row count after reopen = %d, want %d (each migration applied exactly once)",
			count, len(migrations))
	}

	// The reopened store must still be fully usable.
	mustSaveNode(t, s2, testNode(core.NewNodeID(), ""))
}

func TestOpenAppliesPragmas(t *testing.T) {
	s := newTestStore(t)

	var journalMode string
	if err := s.db.QueryRowContext(t.Context(), "PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatalf("PRAGMA journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("PRAGMA journal_mode = %q, want %q", journalMode, "wal")
	}

	var foreignKeys int
	if err := s.db.QueryRowContext(t.Context(), "PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		t.Fatalf("PRAGMA foreign_keys: %v", err)
	}
	if foreignKeys != 1 {
		t.Errorf("PRAGMA foreign_keys = %d, want 1", foreignKeys)
	}

	var synchronous int
	if err := s.db.QueryRowContext(t.Context(), "PRAGMA synchronous").Scan(&synchronous); err != nil {
		t.Fatalf("PRAGMA synchronous: %v", err)
	}
	if synchronous != 1 {
		t.Errorf("PRAGMA synchronous = %d, want 1 (NORMAL)", synchronous)
	}

	var busyTimeout int
	if err := s.db.QueryRowContext(t.Context(), "PRAGMA busy_timeout").Scan(&busyTimeout); err != nil {
		t.Fatalf("PRAGMA busy_timeout: %v", err)
	}
	if busyTimeout != 5000 {
		t.Errorf("PRAGMA busy_timeout = %d, want 5000", busyTimeout)
	}
}

func TestForeignKeysEnforced(t *testing.T) {
	s := newTestStore(t)
	sess := testSession(core.NewSessionID(), core.NewNodeID()) // node does not exist

	if err := s.SaveSessions(t.Context(), []core.Session{sess}); err == nil {
		t.Fatal("SaveSessions with a session referencing a missing node: want error, got nil")
	}
}
