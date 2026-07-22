// Package profile manages grove's provider accounts: named, isolated CLI config
// directories (CLAUDE_CONFIG_DIR, CODEX_HOME, ...) that let different nodes run
// under different subscription or API accounts. It owns profile creation
// (config-dir defaulting + provisioning), first-run default seeding, and the
// doctor health probes. Persistence lives in internal/store and the launch-time
// env wiring in internal/session; this package holds neither processes nor a DB
// handle so every unit is table-testable behind small seams.
package profile

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/AnkushinDaniil/grove/internal/core"
)

// SupportedDrivers are the driver families a profile may target. Only claude is
// fully wired for session isolation in v1; the others are accepted so accounts
// can be pre-created ahead of that support landing.
var SupportedDrivers = []string{"claude", "codex", "gemini", "opencode"}

// DefaultProfileName is the name of the auto-seeded profile that adopts the
// CLI's own config dir untouched (zero-migration first run).
const DefaultProfileName = "default"

// Store is the persistence seam the profile operations need; *store.Store
// satisfies it. Kept minimal so this package does not depend on the store.
type Store interface {
	ListProfiles(ctx context.Context) ([]core.Profile, error)
	SaveProfile(ctx context.Context, p core.Profile) error
}

// IsSupportedDriver reports whether driver is a family a profile may target.
func IsSupportedDriver(driver string) bool {
	for _, d := range SupportedDrivers {
		if d == driver {
			return true
		}
	}
	return false
}

// DefaultConfigDir is the config dir a profile adopts when the caller supplies
// none: <profilesRoot>/<driver>/<name>.
func DefaultConfigDir(profilesRoot, driver, name string) string {
	return filepath.Join(profilesRoot, driver, name)
}

// Create builds a profile and provisions its config directory (0700). A blank
// configDir defaults to DefaultConfigDir(profilesRoot, driver, name). The
// returned profile is not persisted: the caller saves it so a duplicate
// (driver, name) surfaces as the store's unique-constraint error.
func Create(driver, name, configDir, profilesRoot string, now time.Time) (core.Profile, error) {
	if configDir == "" {
		configDir = DefaultConfigDir(profilesRoot, driver, name)
	}
	if !filepath.IsAbs(configDir) {
		return core.Profile{}, fmt.Errorf("%w: profile config dir %q must be absolute", core.ErrInvalid, configDir)
	}
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return core.Profile{}, fmt.Errorf("create profile config dir %s: %w", configDir, err)
	}
	p := core.Profile{
		ID:        core.NewProfileID(),
		Driver:    driver,
		Name:      name,
		ConfigDir: configDir,
		CreatedAt: now,
	}
	if err := p.Validate(); err != nil {
		return core.Profile{}, err
	}
	return p, nil
}

// EnsureDefault seeds the "default" claude profile on first run: it adopts the
// CLI's own config dir (<homeDir>/.claude) untouched so an existing install
// keeps working with zero migration. It is idempotent — an already-present
// default profile is returned unchanged and nothing new is written.
func EnsureDefault(ctx context.Context, store Store, homeDir string) (core.Profile, error) {
	profiles, err := store.ListProfiles(ctx)
	if err != nil {
		return core.Profile{}, fmt.Errorf("list profiles: %w", err)
	}
	for _, p := range profiles {
		if p.IsDefault {
			return p, nil
		}
	}
	def := core.Profile{
		ID:        core.NewProfileID(),
		Driver:    "claude",
		Name:      DefaultProfileName,
		ConfigDir: filepath.Join(homeDir, ".claude"),
		IsDefault: true,
		CreatedAt: time.Now(),
	}
	if err := store.SaveProfile(ctx, def); err != nil {
		return core.Profile{}, fmt.Errorf("save default profile: %w", err)
	}
	return def, nil
}
