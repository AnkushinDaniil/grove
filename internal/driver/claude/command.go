package claude

import (
	"fmt"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/driver"
)

// NewCommand builds argv/env/files for a Claude Code launch. It is pure: no
// process or filesystem access, so every branch is table-testable.
func (claudeDriver) NewCommand(spec driver.LaunchSpec) (driver.ExecSpec, error) {
	if spec.CWD == "" {
		return driver.ExecSpec{}, fmt.Errorf("%w: cwd is empty", core.ErrInvalid)
	}
	if spec.Fork && spec.ResumeID == "" {
		return driver.ExecSpec{}, fmt.Errorf("%w: fork requires a resume id", core.ErrInvalid)
	}
	if spec.Mode == core.ModeHeadless && spec.Prompt == "" && spec.ResumeID == "" {
		return driver.ExecSpec{}, fmt.Errorf("%w: headless run requires a prompt or a resume id", core.ErrInvalid)
	}

	var env []string
	if spec.Profile.ConfigDir != "" {
		env = append(env, "CLAUDE_CONFIG_DIR="+spec.Profile.ConfigDir)
	}

	flags, err := commandFlags(spec)
	if err != nil {
		return driver.ExecSpec{}, err
	}

	var files map[string]string
	if spec.Hooks != nil {
		settingsJSON, hookErr := marshalHookSettings(*spec.Hooks)
		if hookErr != nil {
			return driver.ExecSpec{}, hookErr
		}
		files = map[string]string{hookSettingsPath: settingsJSON}
		flags = append(flags, "--settings", hookSettingsPath)
	}

	argv := make([]string, 0, len(flags)+6)
	if spec.Mode == core.ModeHeadless {
		argv = append(argv, Binary, "-p", "--output-format", "stream-json", "--verbose")
	} else {
		argv = append(argv, Binary)
	}
	argv = append(argv, flags...)
	if spec.Prompt != "" {
		argv = append(argv, spec.Prompt)
	}

	return driver.ExecSpec{Argv: argv, Env: env, Dir: spec.CWD, Files: files}, nil
}

// commandFlags builds the mode-independent flag sequence in a fixed order,
// so NewCommand's argv output is deterministic: add-dir*, permission-mode,
// resume/fork-session, mcp-config. (--settings is appended by the caller,
// since it also owns the generated Files entry.)
func commandFlags(spec driver.LaunchSpec) ([]string, error) {
	var flags []string
	for _, d := range spec.ExtraDirs {
		flags = append(flags, "--add-dir", d)
	}
	if spec.Permission != "" {
		flags = append(flags, "--permission-mode", spec.Permission)
	}
	if spec.ResumeID != "" {
		flags = append(flags, "--resume", spec.ResumeID)
		if spec.Fork {
			flags = append(flags, "--fork-session")
		}
	}
	if len(spec.MCP) > 0 {
		mcpJSON, err := marshalMCPConfig(spec.MCP)
		if err != nil {
			return nil, err
		}
		flags = append(flags, "--mcp-config", mcpJSON)
	}
	return flags, nil
}
