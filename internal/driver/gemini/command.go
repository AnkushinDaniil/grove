package gemini

import (
	"fmt"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/driver"
)

// NewCommand builds argv for a Gemini CLI launch. It is pure: no process or
// filesystem access, so every branch is table-testable.
//
// Profile is intentionally unwired: gemini-cli has no publicly documented
// CODEX_HOME/CLAUDE_CONFIG_DIR-style environment variable for redirecting
// its whole config/credentials tree (docs/cli/*.md and
// docs/get-started/configuration.md, which does not exist upstream, were
// checked). The source does read an internal GEMINI_CLI_HOME override
// (packages/core/src/utils/paths.ts, homedir()), but it is undocumented,
// unverifiable without an install, and would still need pairing with a
// worktree-local settings file for the account's MCP/hook config, so grove
// does not depend on it here; profile isolation for gemini is left as a
// documented gap for a future milestone.
//
// Hooks and MCP are intentionally unwired for the same "for v1" reason
// spelt out in docs/ORCHESTRATION.md §1/§2: gemini has no --settings-style
// native hook channel (Capabilities().NativeHooks is false) and MCP
// mounting for gemini goes through a worktree-local .gemini/settings.json
// file the worktree engine writes directly, not through ExecSpec — see
// ORCHESTRATION §1 "Mounting per driver".
func (geminiDriver) NewCommand(spec driver.LaunchSpec) (driver.ExecSpec, error) {
	if spec.CWD == "" {
		return driver.ExecSpec{}, fmt.Errorf("%w: cwd is empty", core.ErrInvalid)
	}
	if spec.Fork {
		return driver.ExecSpec{}, fmt.Errorf("%w: gemini has no session-forking flag; resuming a session always continues it", driver.ErrUnsupported)
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
	if spec.Mode == core.ModeHeadless {
		// -p takes the prompt text directly as its value (string type per
		// docs/cli/cli-reference.md), unlike Claude's boolean -p/--print;
		// omitted entirely when resuming with no new prompt.
		if spec.Prompt != "" {
			argv = append(argv, "-p", spec.Prompt)
		}
		argv = append(argv, "--output-format", "json")
	}
	argv = append(argv, flags...)
	if spec.Mode == core.ModePTY && spec.Prompt != "" {
		// PTY prompts are a bare positional (docs/cli/cli-reference.md:
		// `gemini "query"` "Query and continue interactively") — never -p,
		// which "Forces non-interactive mode".
		argv = append(argv, spec.Prompt)
	}

	return driver.ExecSpec{Argv: argv, Dir: spec.CWD}, nil
}

// commandFlags builds the mode-independent flag sequence in a fixed order —
// include-directories*, approval-mode, resume — so NewCommand's argv output
// is deterministic.
func commandFlags(spec driver.LaunchSpec) ([]string, error) {
	var flags []string
	for _, d := range spec.ExtraDirs {
		flags = append(flags, "--include-directories", d)
	}
	if spec.Permission != "" {
		mode, err := approvalModeFlags(spec.Permission)
		if err != nil {
			return nil, err
		}
		flags = append(flags, mode...)
	}
	if spec.ResumeID != "" {
		flags = append(flags, "--resume", spec.ResumeID)
	}
	return flags, nil
}

// approvalModeFlags validates spec.Permission against gemini's
// --approval-mode enum (docs/cli/cli-reference.md: default, auto_edit,
// yolo, plan). The deprecated --yolo/-y boolean shorthand is intentionally
// never emitted; --approval-mode=yolo is its documented replacement.
func approvalModeFlags(permission string) ([]string, error) {
	switch permission {
	case "default", "auto_edit", "yolo", "plan":
		return []string{"--approval-mode", permission}, nil
	default:
		return nil, fmt.Errorf("%w: unknown gemini approval mode %q", core.ErrInvalid, permission)
	}
}
