package gemini

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
			want: driver.ExecSpec{Argv: []string{"gemini"}, Dir: "/work"},
		},
		{
			name: "pty prompt continues interactively",
			spec: driver.LaunchSpec{Mode: core.ModePTY, CWD: "/work", Prompt: "explain this project"},
			want: driver.ExecSpec{Argv: []string{"gemini", "explain this project"}, Dir: "/work"},
		},
		{
			name: "pty extra dirs (no prompt)",
			spec: driver.LaunchSpec{Mode: core.ModePTY, CWD: "/work", ExtraDirs: []string{"/shared"}},
			want: driver.ExecSpec{Argv: []string{"gemini", "--include-directories", "/shared"}, Dir: "/work"},
		},
		{
			name: "pty resume",
			spec: driver.LaunchSpec{Mode: core.ModePTY, CWD: "/work", ResumeID: "sess-1"},
			want: driver.ExecSpec{Argv: []string{"gemini", "--resume", "sess-1"}, Dir: "/work"},
		},
		{
			name: "pty resume with new prompt",
			spec: driver.LaunchSpec{Mode: core.ModePTY, CWD: "/work", ResumeID: "sess-1", Prompt: "check for type errors"},
			want: driver.ExecSpec{Argv: []string{"gemini", "--resume", "sess-1", "check for type errors"}, Dir: "/work"},
		},
		{
			name: "headless minimal",
			spec: driver.LaunchSpec{Mode: core.ModeHeadless, CWD: "/work", Prompt: "summarize README.md"},
			want: driver.ExecSpec{Argv: []string{"gemini", "-p", "summarize README.md", "--output-format", "json"}, Dir: "/work"},
		},
		{
			name: "headless resume with prompt",
			spec: driver.LaunchSpec{Mode: core.ModeHeadless, CWD: "/work", Prompt: "finish this PR", ResumeID: "sess-2"},
			want: driver.ExecSpec{
				Argv: []string{"gemini", "-p", "finish this PR", "--output-format", "json", "--resume", "sess-2"},
				Dir:  "/work",
			},
		},
		{
			name: "headless resume without prompt is allowed",
			spec: driver.LaunchSpec{Mode: core.ModeHeadless, CWD: "/work", ResumeID: "sess-3"},
			want: driver.ExecSpec{Argv: []string{"gemini", "--output-format", "json", "--resume", "sess-3"}, Dir: "/work"},
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

// TestNewCommandHeadlessFullHouse exercises every optional LaunchSpec field
// this driver honors at once: extra dirs, approval mode and resume. Flag
// order must be deterministic.
func TestNewCommandHeadlessFullHouse(t *testing.T) {
	d := New()
	spec := driver.LaunchSpec{
		Mode:       core.ModeHeadless,
		Prompt:     "go",
		CWD:        "/work",
		ExtraDirs:  []string{"/a", "/b"},
		Permission: "auto_edit",
		ResumeID:   "sess-9",
	}

	got, err := d.NewCommand(spec)
	if err != nil {
		t.Fatalf("NewCommand() error = %v", err)
	}
	want := []string{
		"gemini", "-p", "go", "--output-format", "json",
		"--include-directories", "/a", "--include-directories", "/b",
		"--approval-mode", "auto_edit",
		"--resume", "sess-9",
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
		name    string
		spec    driver.LaunchSpec
		wantErr error
	}{
		{"empty cwd", driver.LaunchSpec{Mode: core.ModePTY}, core.ErrInvalid},
		{"fork unsupported", driver.LaunchSpec{Mode: core.ModePTY, CWD: "/work", Fork: true, ResumeID: "sess-1"}, driver.ErrUnsupported},
		{"fork unsupported even without resume id", driver.LaunchSpec{Mode: core.ModePTY, CWD: "/work", Fork: true}, driver.ErrUnsupported},
		{"headless without prompt or resume", driver.LaunchSpec{Mode: core.ModeHeadless, CWD: "/work"}, core.ErrInvalid},
		{
			"unknown approval mode",
			driver.LaunchSpec{Mode: core.ModePTY, CWD: "/work", Permission: "garbage"},
			core.ErrInvalid,
		},
		{
			"deprecated yolo shorthand is rejected, not silently mapped",
			driver.LaunchSpec{Mode: core.ModePTY, CWD: "/work", Permission: "-y"},
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

func TestApprovalModeFlags(t *testing.T) {
	tests := []struct {
		name       string
		permission string
		want       []string
		wantErr    bool
	}{
		{"default", "default", []string{"--approval-mode", "default"}, false},
		{"auto_edit", "auto_edit", []string{"--approval-mode", "auto_edit"}, false},
		{"yolo", "yolo", []string{"--approval-mode", "yolo"}, false},
		{"plan", "plan", []string{"--approval-mode", "plan"}, false},
		{"unknown", "garbage", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := approvalModeFlags(tt.permission)
			if tt.wantErr {
				if !errors.Is(err, core.ErrInvalid) {
					t.Fatalf("approvalModeFlags() error = %v, want ErrInvalid", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("approvalModeFlags() error = %v", err)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("approvalModeFlags() = %v, want %v", got, tt.want)
			}
		})
	}
}
