package store

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/AnkushinDaniil/grove/internal/core"
)

func TestParseMigrationFilename(t *testing.T) {
	tests := []struct {
		filename    string
		wantVersion int64
		wantName    string
		wantErr     bool
	}{
		{"0001_init.sql", 1, "init", false},
		{"0042_add_worktrees.sql", 42, "add_worktrees", false},
		{"init.sql", 0, "", true},      // missing _ separator
		{"abcd_init.sql", 0, "", true}, // non-numeric prefix
	}
	for _, tt := range tests {
		version, name, err := parseMigrationFilename(tt.filename)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseMigrationFilename(%q): want error, got nil", tt.filename)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseMigrationFilename(%q): unexpected error: %v", tt.filename, err)
			continue
		}
		if version != tt.wantVersion || name != tt.wantName {
			t.Errorf("parseMigrationFilename(%q) = (%d, %q), want (%d, %q)",
				tt.filename, version, name, tt.wantVersion, tt.wantName)
		}
	}
}

func TestLoadMigrationsSortedByVersion(t *testing.T) {
	migrations, err := loadMigrations()
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	if len(migrations) == 0 {
		t.Fatal("loadMigrations returned no migrations")
	}
	for i := 1; i < len(migrations); i++ {
		if migrations[i-1].version >= migrations[i].version {
			t.Errorf("migrations not strictly sorted: version %d at index %d, version %d at index %d",
				migrations[i-1].version, i-1, migrations[i].version, i)
		}
	}
	if migrations[0].version != 1 {
		t.Errorf("first migration version = %d, want 1", migrations[0].version)
	}
}

// TestMigrateWorkDirFromZero opens a fresh database (no migrations applied) and
// verifies 0002 lands, giving nodes an inheritable work_dir column that
// round-trips.
func TestMigrateWorkDirFromZero(t *testing.T) {
	s := newTestStore(t)

	assertMigrationApplied(t, s, 2)

	n := testNode(core.NewNodeID(), "")
	n.WorkDir = "/abs/work/from/zero"
	mustSaveNode(t, s, n)
	if got := loadNodeDirect(t, s, n.ID); got.WorkDir != n.WorkDir {
		t.Errorf("WorkDir = %q, want %q", got.WorkDir, n.WorkDir)
	}
}

// TestMigrateWorkDirFromExisting0001DB simulates a database created before 0002
// existed (only 0001 applied) and verifies opening it through the store applies
// the pending 0002 migration without disturbing the 0001 data.
func TestMigrateWorkDirFromExisting0001DB(t *testing.T) {
	path := filepath.Join(t.TempDir(), "grove.db")
	seedDBAtMigration1(t, path)

	s, err := Open(t.Context(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})

	assertMigrationApplied(t, s, 2)

	n := testNode(core.NewNodeID(), "")
	n.WorkDir = "/abs/work/upgraded"
	mustSaveNode(t, s, n)
	if got := loadNodeDirect(t, s, n.ID); got.WorkDir != n.WorkDir {
		t.Errorf("WorkDir = %q, want %q", got.WorkDir, n.WorkDir)
	}
}

// seedDBAtMigration1 builds a database at path with only migration 0001 applied
// and recorded, reproducing the on-disk state of a store that predates 0002.
func seedDBAtMigration1(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path) // driver registered by store.go's blank import
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	defer func() { _ = db.Close() }()

	migrations, err := loadMigrations()
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	first := migrations[0]
	if first.version != 1 {
		t.Fatalf("first migration version = %d, want 1", first.version)
	}
	if _, err := db.ExecContext(t.Context(), createMigrationsTableSQL); err != nil {
		t.Fatalf("create schema_migrations table: %v", err)
	}
	if _, err := db.ExecContext(t.Context(), first.sql); err != nil {
		t.Fatalf("apply migration 0001: %v", err)
	}
	if _, err := db.ExecContext(t.Context(),
		"INSERT INTO schema_migrations (version, applied_at) VALUES (1, ?)", time.Now().UnixMilli(),
	); err != nil {
		t.Fatalf("record migration version 1: %v", err)
	}
}

// assertMigrationApplied fails unless exactly one schema_migrations row records
// the given version.
func assertMigrationApplied(t *testing.T, s *Store, version int64) {
	t.Helper()
	var count int
	if err := s.db.QueryRowContext(t.Context(),
		"SELECT COUNT(*) FROM schema_migrations WHERE version = ?", version).Scan(&count); err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	if count != 1 {
		t.Errorf("schema_migrations version %d count = %d, want 1", version, count)
	}
}
