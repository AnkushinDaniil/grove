package codex

import (
	"fmt"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/driver"
)

// NewCommand builds argv/env for a Codex launch. It is pure: no process or
// filesystem access, so every branch is table-testable.
func (codexDriver) NewCommand(spec driver.LaunchSpec) (driver.ExecSpec, error) {
	if spec.CWD == "" {
		return driver.ExecSpec{}, fmt.Errorf("%w: cwd is empty", core.ErrInvalid)
	}
	if spec.Fork {
		// codex has no id-addressed fork: `codex fork` only branches the
		// most recently used interactive session (picker or --last), not a
		// specific ResumeID, so grove's Fork+ResumeID semantics have no
		// equivalent (Caps.Fork is false).
		return driver.ExecSpec{}, fmt.Errorf("%w: codex does not support id-addressed fork", driver.ErrUnsupported)
	}
	if spec.Mode == core.ModeHeadless && spec.Prompt == "" && spec.ResumeID == "" {
		return driver.ExecSpec{}, fmt.Errorf("%w: headless run requires a prompt or a resume id", core.ErrInvalid)
	}

	var env []string
	if spec.Profile.ConfigDir != "" {
		env = append(env, "CODEX_HOME="+spec.Profile.ConfigDir)
	}

	flags, mcpEnv, err := commandFlags(spec)
	if err != nil {
		return driver.ExecSpec{}, err
	}
	env = append(env, mcpEnv...)

	return driver.ExecSpec{Argv: buildArgv(spec, flags), Env: env, Dir: spec.CWD}, nil
}

// buildArgv assembles the mode-specific argv prefix/positionals around the
// shared flags. Codex addresses a resumed session with a positional argument
// on its own `resume` subcommand (`codex resume <id>` for PTY, `codex exec
// resume <id> --json` for headless) rather than a --resume flag like Claude,
// so the resume id and the caller's prompt are both positionals ordered id
// then prompt.
func buildArgv(spec driver.LaunchSpec, flags []string) []string {
	var argv []string
	switch {
	case spec.Mode == core.ModeHeadless && spec.ResumeID != "":
		argv = append(argv, Binary, "exec", "resume", "--json")
		argv = append(argv, flags...)
		argv = append(argv, spec.ResumeID)
	case spec.Mode == core.ModeHeadless:
		argv = append(argv, Binary, "exec", "--json")
		argv = append(argv, flags...)
	case spec.ResumeID != "":
		argv = append(argv, Binary, "resume")
		argv = append(argv, flags...)
		argv = append(argv, spec.ResumeID)
	default:
		argv = append(argv, Binary)
		argv = append(argv, flags...)
	}
	if spec.Prompt != "" {
		argv = append(argv, spec.Prompt)
	}
	return argv
}

// commandFlags builds the mode-independent flag sequence in a fixed order —
// add-dir*, permission, mcp*, hooks — so NewCommand's argv output is
// deterministic. Returns the flags plus any env entries the MCP wiring
// needs (bearer token values).
func commandFlags(spec driver.LaunchSpec) ([]string, []string, error) {
	var flags []string
	for _, d := range spec.ExtraDirs {
		flags = append(flags, "--add-dir", d)
	}

	permFlags, err := permissionFlags(spec.Mode, spec.Permission)
	if err != nil {
		return nil, nil, err
	}
	flags = append(flags, permFlags...)

	mcpServerFlags, mcpEnv, err := mcpFlags(spec.MCP)
	if err != nil {
		return nil, nil, err
	}
	flags = append(flags, mcpServerFlags...)

	if spec.Hooks != nil {
		notify, err := notifyFlag(*spec.Hooks)
		if err != nil {
			return nil, nil, err
		}
		flags = append(flags, notify...)
	}

	return flags, mcpEnv, nil
}

// permissionFlags translates spec.Permission into codex's sandbox/approval
// flags. Codex splits what Claude calls one "permission mode" into two
// independent axes — filesystem sandbox (--sandbox) and command approval
// policy (--ask-for-approval) — plus one flag that collapses both
// (--dangerously-bypass-approvals-and-sandbox). `codex exec` (headless) has
// no --ask-for-approval at all: exec never blocks on a human, so
// approval-policy values only apply in PTY mode. Verified against `codex
// --help`, `codex exec --help`, `codex resume --help` and `codex exec
// resume --help` on codex-cli 0.143.0: --ask-for-approval is present on the
// two interactive commands and absent from both exec commands.
//
//	spec.Permission        PTY flag                                     Headless flag
//	-----------------------------------------------------------------------------------------------------
//	""                     (default)                                    (default)
//	"read-only"            --sandbox read-only                          --sandbox read-only
//	"workspace-write"      --sandbox workspace-write                    --sandbox workspace-write
//	"danger-full-access"   --sandbox danger-full-access                 --sandbox danger-full-access
//	"untrusted"            --ask-for-approval untrusted                 unsupported (ErrInvalid)
//	"on-request"           --ask-for-approval on-request                unsupported (ErrInvalid)
//	"never"                --ask-for-approval never                     unsupported (ErrInvalid)
//	"bypass"               --dangerously-bypass-approvals-and-sandbox   --dangerously-bypass-approvals-and-sandbox
func permissionFlags(mode core.SessionMode, permission string) ([]string, error) {
	switch permission {
	case "":
		return nil, nil
	case "read-only", "workspace-write", "danger-full-access":
		return []string{"--sandbox", permission}, nil
	case "untrusted", "on-request", "never":
		if mode != core.ModePTY {
			return nil, fmt.Errorf("%w: permission %q requires PTY mode; codex exec has no approval-policy flag", core.ErrInvalid, permission)
		}
		return []string{"--ask-for-approval", permission}, nil
	case "bypass":
		return []string{"--dangerously-bypass-approvals-and-sandbox"}, nil
	default:
		return nil, fmt.Errorf("%w: unknown codex permission %q", core.ErrInvalid, permission)
	}
}
