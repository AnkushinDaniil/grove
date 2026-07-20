package store

import "testing"

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
