package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveLayoutEnvOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvHome, dir)
	l, err := ResolveLayout()
	if err != nil {
		t.Fatalf("ResolveLayout: %v", err)
	}
	if l.Home != dir {
		t.Fatalf("Home = %q, want %q", l.Home, dir)
	}
	for name, p := range map[string]string{
		"DBPath": l.DBPath, "TokenPath": l.TokenPath, "Scrollback": l.Scrollback,
		"Worktrees": l.Worktrees, "Profiles": l.Profiles, "Shared": l.Shared,
	} {
		if !strings.HasPrefix(p, dir) || !filepath.IsAbs(p) {
			t.Errorf("%s = %q, want absolute path under %q", name, p, dir)
		}
	}
}

func TestResolveLayoutDefault(t *testing.T) {
	t.Setenv(EnvHome, "")
	l, err := ResolveLayout()
	if err != nil {
		t.Fatalf("ResolveLayout: %v", err)
	}
	if filepath.Base(l.Home) != ".grove" {
		t.Fatalf("Home = %q, want ~/.grove", l.Home)
	}
}

func TestEnsureCreatesDirs(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "home")
	t.Setenv(EnvHome, dir)
	l, err := ResolveLayout()
	if err != nil {
		t.Fatalf("ResolveLayout: %v", err)
	}
	if err := l.Ensure(); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if err := l.Ensure(); err != nil {
		t.Fatalf("Ensure twice: %v", err)
	}
}

func TestTokenStableAcrossLoads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	first, err := LoadOrCreateToken(path)
	if err != nil {
		t.Fatalf("LoadOrCreateToken: %v", err)
	}
	if len(first) != 64 {
		t.Fatalf("token length = %d, want 64 hex chars", len(first))
	}
	second, err := LoadOrCreateToken(path)
	if err != nil {
		t.Fatalf("second LoadOrCreateToken: %v", err)
	}
	if first != second {
		t.Fatal("token changed between loads")
	}
}
