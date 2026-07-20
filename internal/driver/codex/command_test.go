package codex

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/driver"
)

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
			want: driver.ExecSpec{Argv: []string{"codex"}, Dir: "/work"},
		},
		{
			name: "pty idle interactive with extra dir (no prompt)",
			spec: driver.LaunchSpec{Mode: core.ModePTY, CWD: "/work", ExtraDirs: []string{"/shared"}},
			want: driver.ExecSpec{Argv: []string{"codex", "--add-dir", "/shared"}, Dir: "/work"},
		},
		{
			name: "pty prompt resume",
			spec: driver.LaunchSpec{Mode: core.ModePTY, CWD: "/work", Prompt: "hello", ResumeID: "sess-1"},
			want: driver.ExecSpec{Argv: []string{"codex", "resume", "sess-1", "hello"}, Dir: "/work"},
		},
		{
			name: "pty resume without prompt",
			spec: driver.LaunchSpec{Mode: core.ModePTY, CWD: "/work", ResumeID: "sess-2"},
			want: driver.ExecSpec{Argv: []string{"codex", "resume", "sess-2"}, Dir: "/work"},
		},
		{
			name: "headless minimal",
			spec: driver.LaunchSpec{Mode: core.ModeHeadless, CWD: "/work", Prompt: "go"},
			want: driver.ExecSpec{Argv: []string{"codex", "exec", "--json", "go"}, Dir: "/work"},
		},
		{
			name: "headless resume without prompt is allowed",
			spec: driver.LaunchSpec{Mode: core.ModeHeadless, CWD: "/work", ResumeID: "sess-3"},
			want: driver.ExecSpec{Argv: []string{"codex", "exec", "resume", "--json", "sess-3"}, Dir: "/work"},
		},
		{
			name: "headless resume with prompt",
			spec: driver.LaunchSpec{Mode: core.ModeHeadless, CWD: "/work", ResumeID: "sess-3", Prompt: "again"},
			want: driver.ExecSpec{Argv: []string{"codex", "exec", "resume", "--json", "sess-3", "again"}, Dir: "/work"},
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
			if len(got.Files) != 0 {
				t.Errorf("Files = %v, want none (codex wiring needs no generated files)", got.Files)
			}
		})
	}
}

// TestNewCommandHeadlessFullHouse exercises every optional LaunchSpec field
// at once: mcp, hooks, add-dirs, permission and profile. Flag order must be
// deterministic, and the --config values for mcp/hooks are produced by
// encoding/json (a JSON string/array is valid TOML syntax for the same
// value — see mcp.go/hooks.go), so exact string comparison is safe here
// without re-parsing.
func TestNewCommandHeadlessFullHouse(t *testing.T) {
	d := New()
	spec := driver.LaunchSpec{
		Mode:       core.ModeHeadless,
		Prompt:     "go",
		CWD:        "/work",
		ExtraDirs:  []string{"/a", "/b"},
		Profile:    driver.ProfileRef{ConfigDir: "/cfg"},
		Permission: "workspace-write",
		Hooks: &driver.HookWiring{
			HookCommand: "/usr/local/bin/grove-hook",
			DaemonURL:   "http://127.0.0.1:4123",
			NodeID:      core.NodeID("node-1"),
			Token:       "hook-tok",
		},
		MCP: []driver.MCPRef{
			{Name: "ctx7", URL: "https://mcp.example/ctx7", Token: "mcp-tok"},
		},
	}

	got, err := d.NewCommand(spec)
	if err != nil {
		t.Fatalf("NewCommand() error = %v", err)
	}
	if got.Dir != "/work" {
		t.Errorf("Dir = %q, want /work", got.Dir)
	}
	wantEnv := []string{"CODEX_HOME=/cfg", "GROVE_MCP_CTX7_TOKEN=mcp-tok"}
	if !slices.Equal(got.Env, wantEnv) {
		t.Errorf("Env = %v, want %v", got.Env, wantEnv)
	}

	wantArgv := []string{
		"codex", "exec", "--json",
		"--add-dir", "/a", "--add-dir", "/b",
		"--sandbox", "workspace-write",
		"--config", `mcp_servers."ctx7".url="https://mcp.example/ctx7"`,
		"--config", `mcp_servers."ctx7".bearer_token_env_var="GROVE_MCP_CTX7_TOKEN"`,
		"--config", `notify=["/usr/local/bin/grove-hook","--driver","codex","--node","node-1","--token","hook-tok","--daemon","http://127.0.0.1:4123"]`,
		"go",
	}
	if !slices.Equal(got.Argv, wantArgv) {
		t.Errorf("Argv = %v, want %v", got.Argv, wantArgv)
	}

	// Spot-check that every --config value is valid, parseable JSON (and
	// thus valid TOML value syntax) rather than trusting the raw string.
	for i, a := range got.Argv {
		if a != "--config" || i+1 >= len(got.Argv) {
			continue
		}
		value := got.Argv[i+1]
		_, tomlValue, ok := strings.Cut(value, "=")
		if !ok {
			t.Fatalf("--config value %q has no key=value separator", value)
		}
		var probe any
		if err := json.Unmarshal([]byte(tomlValue), &probe); err != nil {
			t.Errorf("--config value %q is not valid JSON/TOML: %v", value, err)
		}
	}
}

func TestNewCommandErrors(t *testing.T) {
	d := New()
	tests := []struct {
		name    string
		spec    driver.LaunchSpec
		wantErr error
	}{
		{"empty cwd", driver.LaunchSpec{Mode: core.ModePTY}, core.ErrInvalid},
		{"fork unsupported", driver.LaunchSpec{Mode: core.ModePTY, CWD: "/work", Fork: true, ResumeID: "sess-1"}, driver.ErrUnsupported},
		{"fork unsupported even without resume id", driver.LaunchSpec{Mode: core.ModePTY, CWD: "/work", Fork: true}, driver.ErrUnsupported},
		{"headless without prompt or resume", driver.LaunchSpec{Mode: core.ModeHeadless, CWD: "/work"}, core.ErrInvalid},
		{
			"headless approval-policy permission unsupported",
			driver.LaunchSpec{Mode: core.ModeHeadless, CWD: "/work", Prompt: "go", Permission: "never"},
			core.ErrInvalid,
		},
		{
			"unknown permission value",
			driver.LaunchSpec{Mode: core.ModePTY, CWD: "/work", Permission: "garbage"},
			core.ErrInvalid,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := d.NewCommand(tt.spec)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("NewCommand() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestPermissionFlags(t *testing.T) {
	tests := []struct {
		name       string
		mode       core.SessionMode
		permission string
		want       []string
		wantErr    bool
	}{
		{"empty", core.ModePTY, "", nil, false},
		{"pty read-only", core.ModePTY, "read-only", []string{"--sandbox", "read-only"}, false},
		{"pty workspace-write", core.ModePTY, "workspace-write", []string{"--sandbox", "workspace-write"}, false},
		{"pty danger-full-access", core.ModePTY, "danger-full-access", []string{"--sandbox", "danger-full-access"}, false},
		{"pty untrusted", core.ModePTY, "untrusted", []string{"--ask-for-approval", "untrusted"}, false},
		{"pty on-request", core.ModePTY, "on-request", []string{"--ask-for-approval", "on-request"}, false},
		{"pty never", core.ModePTY, "never", []string{"--ask-for-approval", "never"}, false},
		{"pty bypass", core.ModePTY, "bypass", []string{"--dangerously-bypass-approvals-and-sandbox"}, false},
		{"pty unknown", core.ModePTY, "garbage", nil, true},
		{"headless read-only", core.ModeHeadless, "read-only", []string{"--sandbox", "read-only"}, false},
		{"headless danger-full-access", core.ModeHeadless, "danger-full-access", []string{"--sandbox", "danger-full-access"}, false},
		{"headless bypass", core.ModeHeadless, "bypass", []string{"--dangerously-bypass-approvals-and-sandbox"}, false},
		{"headless untrusted unsupported", core.ModeHeadless, "untrusted", nil, true},
		{"headless on-request unsupported", core.ModeHeadless, "on-request", nil, true},
		{"headless never unsupported", core.ModeHeadless, "never", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := permissionFlags(tt.mode, tt.permission)
			if tt.wantErr {
				if !errors.Is(err, core.ErrInvalid) {
					t.Fatalf("permissionFlags() error = %v, want ErrInvalid", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("permissionFlags() error = %v", err)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("permissionFlags() = %v, want %v", got, tt.want)
			}
		})
	}
}
