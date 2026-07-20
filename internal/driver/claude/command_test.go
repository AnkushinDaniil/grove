package claude

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/driver"
)

func mustFlagValue(t *testing.T, argv []string, flag string) string {
	t.Helper()
	for i, a := range argv {
		if a == flag {
			if i+1 >= len(argv) {
				t.Fatalf("flag %s has no value in argv %v", flag, argv)
			}
			return argv[i+1]
		}
	}
	t.Fatalf("flag %s not found in argv %v", flag, argv)
	return ""
}

func TestNewCommandSuccess(t *testing.T) {
	d := New()
	tests := []struct {
		name string
		spec driver.LaunchSpec
		want driver.ExecSpec
	}{
		{
			name: "pty minimal",
			spec: driver.LaunchSpec{Mode: core.ModePTY, CWD: "/work"},
			want: driver.ExecSpec{Argv: []string{"claude"}, Dir: "/work"},
		},
		{
			name: "pty idle interactive (no prompt)",
			spec: driver.LaunchSpec{Mode: core.ModePTY, CWD: "/work", ExtraDirs: []string{"/shared"}},
			want: driver.ExecSpec{Argv: []string{"claude", "--add-dir", "/shared"}, Dir: "/work"},
		},
		{
			name: "pty prompt resume fork",
			spec: driver.LaunchSpec{
				Mode:     core.ModePTY,
				CWD:      "/work",
				Prompt:   "hello",
				ResumeID: "sess-1",
				Fork:     true,
			},
			want: driver.ExecSpec{
				Argv: []string{"claude", "--resume", "sess-1", "--fork-session", "hello"},
				Dir:  "/work",
			},
		},
		{
			name: "pty resume without fork",
			spec: driver.LaunchSpec{
				Mode:     core.ModePTY,
				CWD:      "/work",
				ResumeID: "sess-2",
			},
			want: driver.ExecSpec{
				Argv: []string{"claude", "--resume", "sess-2"},
				Dir:  "/work",
			},
		},
		{
			name: "headless resume without prompt is allowed",
			spec: driver.LaunchSpec{
				Mode:     core.ModeHeadless,
				CWD:      "/work",
				ResumeID: "sess-3",
			},
			want: driver.ExecSpec{
				Argv: []string{"claude", "-p", "--output-format", "stream-json", "--verbose", "--resume", "sess-3"},
				Dir:  "/work",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := d.NewCommand(tt.spec)
			if err != nil {
				t.Fatalf("NewCommand() error = %v", err)
			}
			if !slices.Equal(got.Argv, tt.want.Argv) {
				t.Errorf("Argv = %v, want %v", got.Argv, tt.want.Argv)
			}
			if !slices.Equal(got.Env, tt.want.Env) {
				t.Errorf("Env = %v, want %v", got.Env, tt.want.Env)
			}
			if got.Dir != tt.want.Dir {
				t.Errorf("Dir = %q, want %q", got.Dir, tt.want.Dir)
			}
			if len(got.Files) != len(tt.want.Files) {
				t.Errorf("Files = %v, want %v", got.Files, tt.want.Files)
			}
		})
	}
}

// TestNewCommandHeadlessFullHouse exercises every optional LaunchSpec field
// at once: mcp, hooks, add-dirs, permission and profile. Flag order must be
// deterministic; the mcp-config and settings file contents are asserted by
// parsing, not by comparing raw JSON text.
func TestNewCommandHeadlessFullHouse(t *testing.T) {
	d := New()
	spec := driver.LaunchSpec{
		Mode:       core.ModeHeadless,
		Prompt:     "go",
		CWD:        "/work",
		ExtraDirs:  []string{"/a", "/b"},
		Profile:    driver.ProfileRef{ConfigDir: "/cfg"},
		Permission: "acceptEdits",
		Hooks: &driver.HookWiring{
			HookCommand: "/usr/local/bin/grove-hook",
			DaemonURL:   "http://127.0.0.1:4123",
			NodeID:      core.NodeID("node-1"),
			Token:       "hook-tok",
		},
		MCP: []driver.MCPRef{
			{Name: "ctx7", URL: "https://mcp.example/ctx7", Token: "mcp-tok"},
			{Name: "grove", URL: "https://mcp.example/grove"},
		},
	}

	got, err := d.NewCommand(spec)
	if err != nil {
		t.Fatalf("NewCommand() error = %v", err)
	}

	if got.Dir != "/work" {
		t.Errorf("Dir = %q, want /work", got.Dir)
	}
	if !slices.Equal(got.Env, []string{"CLAUDE_CONFIG_DIR=/cfg"}) {
		t.Errorf("Env = %v, want [CLAUDE_CONFIG_DIR=/cfg]", got.Env)
	}

	// --mcp-config: parsed, not string-compared.
	mcpRaw := mustFlagValue(t, got.Argv, "--mcp-config")
	var mcp mcpConfig
	if err := json.Unmarshal([]byte(mcpRaw), &mcp); err != nil {
		t.Fatalf("unmarshal mcp config: %v", err)
	}
	if len(mcp.MCPServers) != 2 {
		t.Fatalf("mcpServers len = %d, want 2: %+v", len(mcp.MCPServers), mcp.MCPServers)
	}
	ctx7 := mcp.MCPServers["ctx7"]
	if ctx7.Type != "http" || ctx7.URL != "https://mcp.example/ctx7" {
		t.Errorf("ctx7 server = %+v", ctx7)
	}
	if ctx7.Headers["Authorization"] != "Bearer mcp-tok" {
		t.Errorf("ctx7 Authorization = %q, want %q", ctx7.Headers["Authorization"], "Bearer mcp-tok")
	}
	grove := mcp.MCPServers["grove"]
	if len(grove.Headers) != 0 {
		t.Errorf("grove headers = %v, want none (empty token omits headers)", grove.Headers)
	}

	// --settings + Files: parsed, not string-compared.
	settingsPath := mustFlagValue(t, got.Argv, "--settings")
	if settingsPath != hookSettingsPath {
		t.Errorf("--settings = %q, want %q", settingsPath, hookSettingsPath)
	}
	settingsRaw, ok := got.Files[hookSettingsPath]
	if !ok {
		t.Fatalf("Files missing %q; got %v", hookSettingsPath, got.Files)
	}
	var settings claudeSettings
	if err := json.Unmarshal([]byte(settingsRaw), &settings); err != nil {
		t.Fatalf("unmarshal hook settings: %v", err)
	}
	wantCmd := "/usr/local/bin/grove-hook --node node-1 --token hook-tok --daemon http://127.0.0.1:4123"
	groups := [][]hookMatcher{
		settings.Hooks.SessionStart, settings.Hooks.Notification,
		settings.Hooks.Stop, settings.Hooks.SessionEnd,
	}
	for i, group := range groups {
		if len(group) != 1 || len(group[0].Hooks) != 1 || group[0].Hooks[0].Command != wantCmd {
			t.Errorf("hook group[%d] = %+v, want single command %q", i, group, wantCmd)
		}
		if len(group) == 1 && len(group[0].Hooks) == 1 && group[0].Hooks[0].Type != "command" {
			t.Errorf("hook group[%d] type = %q, want %q", i, group[0].Hooks[0].Type, "command")
		}
	}

	// Argv order is fixed: mode prefix, add-dir*, permission-mode,
	// mcp-config (placeholder), settings, then the positional prompt.
	wantPrefix := []string{
		"claude", "-p", "--output-format", "stream-json", "--verbose",
		"--add-dir", "/a", "--add-dir", "/b",
		"--permission-mode", "acceptEdits",
	}
	wantFull := append(append([]string{}, wantPrefix...),
		"--mcp-config", "<mcp>",
		"--settings", hookSettingsPath,
		"go",
	)
	normalized := slices.Clone(got.Argv)
	for i, a := range normalized {
		if a == "--mcp-config" && i+1 < len(normalized) {
			normalized[i+1] = "<mcp>"
		}
	}
	if !slices.Equal(normalized, wantFull) {
		t.Errorf("normalized Argv = %v, want %v", normalized, wantFull)
	}
}

func TestNewCommandErrors(t *testing.T) {
	d := New()
	tests := []struct {
		name string
		spec driver.LaunchSpec
	}{
		{"empty cwd", driver.LaunchSpec{Mode: core.ModePTY}},
		{"fork without resume", driver.LaunchSpec{Mode: core.ModePTY, CWD: "/work", Fork: true}},
		{"headless without prompt or resume", driver.LaunchSpec{Mode: core.ModeHeadless, CWD: "/work"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := d.NewCommand(tt.spec)
			if !errors.Is(err, core.ErrInvalid) {
				t.Fatalf("NewCommand() error = %v, want ErrInvalid", err)
			}
		})
	}
}
