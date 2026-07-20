package core

import (
	"fmt"
	"path/filepath"
	"time"
)

// Profile is one provider account: a named, isolated CLI config directory
// (CLAUDE_CONFIG_DIR, CODEX_HOME, ...). Credentials live inside the config dir
// (or the OS keychain keyed by it) and are never read or stored by grove.
type Profile struct {
	ID        ProfileID
	Driver    string // "claude", "codex", ...
	Name      string // "personal", "work"
	ConfigDir string
	IsDefault bool
	CreatedAt time.Time
}

func (p Profile) Validate() error {
	if p.ID == "" {
		return fmt.Errorf("%w: profile id is empty", ErrInvalid)
	}
	if p.Driver == "" {
		return fmt.Errorf("%w: profile driver is empty", ErrInvalid)
	}
	if p.Name == "" {
		return fmt.Errorf("%w: profile name is empty", ErrInvalid)
	}
	if !filepath.IsAbs(p.ConfigDir) {
		return fmt.Errorf("%w: profile config dir %q must be absolute", ErrInvalid, p.ConfigDir)
	}
	return nil
}
