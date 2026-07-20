package opencode

import (
	"errors"
	"slices"
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
			want: driver.ExecSpec{Argv: []string{"opencode"}, Dir: "/work"},
		},
		{
			name: "pty prompt",
			spec: driver.LaunchSpec{Mode: core.ModePTY, CWD: "/work", Prompt: "explain this project"},
			want: driver.ExecSpec{Argv: []string{"opencode", "--prompt", "explain this project"}, Dir: "/work"},
		},
		{
			name: "pty resume",
			spec: driver.LaunchSpec{Mode: core.ModePTY, CWD: "/work", ResumeID: "sess-1"},
			want: driver.ExecSpec{Argv: []string{"opencode", "--session", "sess-1"}, Dir: "/work"},
		},
		{
			name: "pty resume with fork",
			spec: driver.LaunchSpec{Mode: core.ModePTY, CWD: "/work", ResumeID: "sess-1", Fork: true},
			want: driver.ExecSpec{Argv: []string{"opencode", "--session", "sess-1", "--fork"}, Dir: "/work"},
		},
		{
			name: "pty auto permission",
			spec: driver.LaunchSpec{Mode: core.ModePTY, CWD: "/work", Permission: "auto"},
			want: driver.ExecSpec{Argv: []string{"opencode", "--auto"}, Dir: "/work"},
		},
		{
			name: "headless minimal",
			spec: driver.LaunchSpec{Mode: core.ModeHeadless, CWD: "/work", Prompt: "refactor src/auth"},
			want: driver.ExecSpec{Argv: []string{"opencode", "run", "--format", "json", "refactor src/auth"}, Dir: "/work"},
		},
		{
			name: "headless resume with prompt",
			spec: driver.LaunchSpec{Mode: core.ModeHeadless, CWD: "/work", Prompt: "keep going", ResumeID: "sess-2"},
			want: driver.ExecSpec{
				Argv: []string{"opencode", "run", "--format", "json", "--session", "sess-2", "keep going"},
				Dir:  "/work",
			},
		},
		{
			name: "headless resume without prompt is allowed",
			spec: driver.LaunchSpec{Mode: core.ModeHeadless, CWD: "/work", ResumeID: "sess-3"},
			want: driver.ExecSpec{Argv: []string{"opencode", "run", "--format", "json", "--session", "sess-3"}, Dir: "/work"},
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
			if len(got.Env) != 0 {
				t.Errorf("Env = %v, want none (profile is intentionally unwired)", got.Env)
			}
			if got.Dir != tt.want.Dir {
				t.Errorf("Dir = %q, want %q", got.Dir, tt.want.Dir)
			}
			if len(got.Files) != 0 {
				t.Errorf("Files = %v, want none (hooks/MCP are intentionally unwired)", got.Files)
			}
		})
	}
}

// TestNewCommandExtraDirsIgnored documents that opencode has no
// add-dir-equivalent flag: ExtraDirs must not appear in argv anywhere, and
// must not error either.
func TestNewCommandExtraDirsIgnored(t *testing.T) {
	d := New()
	got, err := d.NewCommand(driver.LaunchSpec{Mode: core.ModePTY, CWD: "/work", ExtraDirs: []string{"/shared", "/other"}})
	if err != nil {
		t.Fatalf("NewCommand() error = %v", err)
	}
	want := []string{"opencode"}
	if !slices.Equal(got.Argv, want) {
		t.Errorf("Argv = %v, want %v (ExtraDirs has no opencode flag)", got.Argv, want)
	}
}

func TestNewCommandHeadlessFullHouse(t *testing.T) {
	d := New()
	spec := driver.LaunchSpec{
		Mode:       core.ModeHeadless,
		Prompt:     "go",
		CWD:        "/work",
		Permission: "auto",
		ResumeID:   "sess-9",
		Fork:       true,
	}

	got, err := d.NewCommand(spec)
	if err != nil {
		t.Fatalf("NewCommand() error = %v", err)
	}
	want := []string{
		"opencode", "run", "--format", "json",
		"--session", "sess-9", "--fork",
		"--auto",
		"go",
	}
	if !slices.Equal(got.Argv, want) {
		t.Errorf("Argv = %v, want %v", got.Argv, want)
	}
	if got.Dir != "/work" {
		t.Errorf("Dir = %q, want /work", got.Dir)
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
		{"unknown permission", driver.LaunchSpec{Mode: core.ModePTY, CWD: "/work", Permission: "garbage"}},
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

func TestPermissionFlags(t *testing.T) {
	tests := []struct {
		name       string
		permission string
		want       []string
		wantErr    bool
	}{
		{"auto", "auto", []string{"--auto"}, false},
		{"unknown", "garbage", nil, true},
		{"claude-style value rejected", "acceptEdits", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := permissionFlags(tt.permission)
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
