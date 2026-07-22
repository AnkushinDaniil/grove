package profile

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/AnkushinDaniil/grove/internal/core"
)

// memStore is an in-memory profile.Store for exercising EnsureDefault without a
// real database.
type memStore struct {
	profiles []core.Profile
	saves    int
}

func (s *memStore) ListProfiles(context.Context) ([]core.Profile, error) {
	return s.profiles, nil
}

func (s *memStore) SaveProfile(_ context.Context, p core.Profile) error {
	s.saves++
	s.profiles = append(s.profiles, p)
	return nil
}

func TestCreateDefaultsConfigDir(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(1_700_000_000, 0)

	p, err := Create("claude", "work", "", root, now)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	want := filepath.Join(root, "claude", "work")
	if p.ConfigDir != want {
		t.Errorf("ConfigDir = %q, want %q", p.ConfigDir, want)
	}
	if p.Driver != "claude" || p.Name != "work" || p.IsDefault {
		t.Errorf("profile fields = %+v, want driver claude / name work / not default", p)
	}
	if p.ID == "" {
		t.Error("ID is empty, want a generated id")
	}
	if info, err := os.Stat(want); err != nil || !info.IsDir() {
		t.Errorf("config dir not provisioned: stat %q -> %v", want, err)
	}
}

func TestCreateUsesExplicitConfigDir(t *testing.T) {
	explicit := filepath.Join(t.TempDir(), "custom", "dir")
	p, err := Create("codex", "personal", explicit, t.TempDir(), time.Now())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.ConfigDir != explicit {
		t.Errorf("ConfigDir = %q, want explicit %q", p.ConfigDir, explicit)
	}
	if info, err := os.Stat(explicit); err != nil || !info.IsDir() {
		t.Errorf("explicit config dir not provisioned: stat %q -> %v", explicit, err)
	}
}

func TestCreateRejectsRelativeConfigDir(t *testing.T) {
	if _, err := Create("claude", "work", "relative/dir", t.TempDir(), time.Now()); err == nil {
		t.Fatal("Create with a relative config_dir succeeded, want an error")
	}
}

func TestEnsureDefaultSeedsThenIsIdempotent(t *testing.T) {
	home := "/home/tester"
	store := &memStore{}

	first, err := EnsureDefault(t.Context(), store, home)
	if err != nil {
		t.Fatalf("EnsureDefault (seed): %v", err)
	}
	if !first.IsDefault || first.Driver != "claude" || first.Name != DefaultProfileName {
		t.Errorf("default = %+v, want is_default claude/%s", first, DefaultProfileName)
	}
	if want := filepath.Join(home, ".claude"); first.ConfigDir != want {
		t.Errorf("default ConfigDir = %q, want %q (CLI dir adopted untouched)", first.ConfigDir, want)
	}

	second, err := EnsureDefault(t.Context(), store, home)
	if err != nil {
		t.Fatalf("EnsureDefault (idempotent): %v", err)
	}
	if second.ID != first.ID {
		t.Errorf("second EnsureDefault id = %q, want the same profile %q", second.ID, first.ID)
	}
	if store.saves != 1 {
		t.Errorf("SaveProfile called %d times, want exactly 1 (idempotent)", store.saves)
	}
}

func TestIsSupportedDriver(t *testing.T) {
	for _, d := range []string{"claude", "codex", "gemini", "opencode"} {
		if !IsSupportedDriver(d) {
			t.Errorf("IsSupportedDriver(%q) = false, want true", d)
		}
	}
	for _, d := range []string{"", "cursor", "aider"} {
		if IsSupportedDriver(d) {
			t.Errorf("IsSupportedDriver(%q) = true, want false", d)
		}
	}
}
