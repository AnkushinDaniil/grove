// Package config resolves grove's state directory layout and daemon settings.
package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// EnvHome overrides the state directory location.
const EnvHome = "GROVE_HOME"

// Layout is the resolved on-disk state layout. All paths are absolute.
type Layout struct {
	Home       string // ~/.grove or $GROVE_HOME
	DBPath     string // sqlite database
	TokenPath  string // API auth token
	Scrollback string // persisted terminal ring buffers
	Worktrees  string // task workspaces
	Profiles   string // per-account CLI config dirs
	Shared     string // context shared across profiles (skills, agents, ...)
	Logs       string // daemon log files (background/service runs)
}

// ResolveLayout computes the layout from $GROVE_HOME or the user home dir.
func ResolveLayout() (Layout, error) {
	home := os.Getenv(EnvHome)
	if home == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return Layout{}, fmt.Errorf("resolve user home: %w", err)
		}
		home = filepath.Join(userHome, ".grove")
	}
	if !filepath.IsAbs(home) {
		abs, err := filepath.Abs(home)
		if err != nil {
			return Layout{}, fmt.Errorf("resolve %s: %w", EnvHome, err)
		}
		home = abs
	}
	return Layout{
		Home:       home,
		DBPath:     filepath.Join(home, "grove.db"),
		TokenPath:  filepath.Join(home, "token"),
		Scrollback: filepath.Join(home, "scrollback"),
		Worktrees:  filepath.Join(home, "worktrees"),
		Profiles:   filepath.Join(home, "profiles"),
		Shared:     filepath.Join(home, "shared"),
		Logs:       filepath.Join(home, "logs"),
	}, nil
}

// Ensure creates the layout directories with owner-only permissions.
func (l Layout) Ensure() error {
	for _, dir := range []string{l.Home, l.Scrollback, l.Worktrees, l.Profiles, l.Shared, l.Logs} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create state dir %s: %w", dir, err)
		}
	}
	return nil
}
