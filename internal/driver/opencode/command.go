package opencode

import (
	"fmt"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/driver"
)

// NewCommand builds argv for an OpenCode launch. It is pure: no process or
// filesystem access, so every branch is table-testable.
//
// Profile is intentionally unwired: OPENCODE_CONFIG_DIR
// (packages/opencode/src/config/paths.ts, Flag.OPENCODE_CONFIG_DIR) only
// redirects config discovery (agents/commands/modes/plugins) — credentials
// and MCP OAuth tokens live at a separate, fixed XDG data path with no
// override found (packages/opencode/src/auth/index.ts,
// packages/opencode/src/mcp/auth.ts: Global.Path.data/{auth,mcp-auth}.json).
// A true CLAUDE_CONFIG_DIR/CODEX_HOME equivalent would need both redirected
// together, which is out of scope for v1; profile isolation for opencode is
// left as a documented gap for a future milestone.
//
// Hooks and MCP are intentionally unwired for the same "for v1" reason
// spelt out in docs/ORCHESTRATION.md §1/§2: opencode has no native hook
// channel (Capabilities().NativeHooks is false) and MCP mounting goes
// through a worktree-local opencode.json file the worktree engine writes
// directly, not through ExecSpec. (For when that lands: opencode's remote
// MCP config shape is {"mcp":{"<name>":{"type":"remote","url":...,
// "headers":{...}}}} — packages/opencode/src/mcp/index.ts — so an MCPRef's
// bearer token would go in headers, same idea as Claude's --mcp-config.)
//
// ExtraDirs has no opencode equivalent: neither `opencode run` nor the
// TUI's `$0 [project]` command exposes an add-dir/include-directories style
// flag (their builders were read in full), so spec.ExtraDirs is silently
// ignored.
func (opencodeDriver) NewCommand(spec driver.LaunchSpec) (driver.ExecSpec, error) {
	if spec.CWD == "" {
		return driver.ExecSpec{}, fmt.Errorf("%w: cwd is empty", core.ErrInvalid)
	}
	if spec.Fork && spec.ResumeID == "" {
		return driver.ExecSpec{}, fmt.Errorf("%w: fork requires a resume id", core.ErrInvalid)
	}
	if spec.Mode == core.ModeHeadless && spec.Prompt == "" && spec.ResumeID == "" {
		return driver.ExecSpec{}, fmt.Errorf("%w: headless run requires a prompt or a resume id", core.ErrInvalid)
	}

	flags, err := commandFlags(spec)
	if err != nil {
		return driver.ExecSpec{}, err
	}

	argv := make([]string, 0, len(flags)+6)
	argv = append(argv, Binary)
	switch spec.Mode {
	case core.ModeHeadless:
		argv = append(argv, "run", "--format", "json")
		argv = append(argv, flags...)
		// `run [message..]` is a positional array that opencode re-joins
		// with quote-wrapping around any element containing a space
		// (packages/opencode/src/cli/cmd/run.ts: the "message" .map/.join
		// ahead of client.session.prompt). Passed as a single argv element
		// this only adds cosmetic wrapping quotes around the whole prompt
		// (and escapes embedded quotes) — every character, including
		// internal newlines, still reaches the model unchanged. There is no
		// flag-based alternative for `run`, unlike the PTY's --prompt.
		if spec.Prompt != "" {
			argv = append(argv, spec.Prompt)
		}
	default: // core.ModePTY
		argv = append(argv, flags...)
		if spec.Prompt != "" {
			argv = append(argv, "--prompt", spec.Prompt)
		}
	}

	return driver.ExecSpec{Argv: argv, Dir: spec.CWD}, nil
}

// commandFlags builds the mode-independent flag sequence in a fixed order —
// session/fork, auto — shared by `run` and the bare TUI command (both
// expose the same --session/-s, --fork and --auto options).
func commandFlags(spec driver.LaunchSpec) ([]string, error) {
	var flags []string
	if spec.ResumeID != "" {
		// --session addresses a specific id; grove's ResumeID is always a
		// specific prior session, never "whichever was last used"
		// (--continue), so --continue is intentionally never emitted.
		flags = append(flags, "--session", spec.ResumeID)
		if spec.Fork {
			flags = append(flags, "--fork")
		}
	}
	if spec.Permission != "" {
		auto, err := permissionFlags(spec.Permission)
		if err != nil {
			return nil, err
		}
		flags = append(flags, auto...)
	}
	return flags, nil
}

// permissionFlags validates spec.Permission against the one permission
// switch both `run` and the TUI expose: --auto ("auto-approve permissions
// that are not explicitly denied"). There is no enum of named modes like
// Claude's --permission-mode or Codex's --sandbox/--ask-for-approval, just
// this one boolean, so "auto" is the only accepted value.
func permissionFlags(permission string) ([]string, error) {
	if permission == "auto" {
		return []string{"--auto"}, nil
	}
	return nil, fmt.Errorf("%w: unknown opencode permission %q (only \"auto\" is supported)", core.ErrInvalid, permission)
}
