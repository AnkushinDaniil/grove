// Package driver defines the seam between grove and the CLI agents it runs.
//
// Drivers are pure: they build argv/env for a launch, parse native output
// lines into normalized core events, and declare capabilities. They never
// touch processes or the filesystem — the session runtime owns that. This
// keeps every driver table-driven-testable against recorded fixtures.
package driver

import (
	"context"
	"errors"
	"time"

	"github.com/AnkushinDaniil/grove/internal/core"
)

// ErrUnsupported is returned for operations a driver's capabilities exclude.
var ErrUnsupported = errors.New("unsupported by driver")

// Caps declares what a CLI can do; the session runtime branches on these.
type Caps struct {
	Interactive    bool // full PTY TUI mode
	Headless       bool // one-shot -p / exec style run
	HeadlessStream bool // persistent stdin JSON stream (multi-turn in one process)
	Resume         bool // resume a previous driver session by id
	Fork           bool // branch a new session from an existing one
	EmitsSessionID bool // session id appears in the output stream
	NativeHooks    bool // hook/notify integration for attention detection
	MCP            bool // can mount MCP servers
}

// ProfileRef points a launch at an isolated account config dir.
type ProfileRef struct {
	// ConfigDir is the account isolation dir (CLAUDE_CONFIG_DIR, CODEX_HOME).
	// Empty means the CLI's default.
	ConfigDir string
}

// HookWiring tells the driver how to phone events home. Drivers that support
// native hooks embed `<HookCommand> --node <NodeID> --token <Token>` into the
// generated settings they return via ExecSpec.Files.
type HookWiring struct {
	HookCommand string // absolute path to the grove binary's hook subcommand
	DaemonURL   string // http://127.0.0.1:<port>
	NodeID      core.NodeID
	Token       string
}

// MCPRef mounts one MCP server into the agent.
type MCPRef struct {
	Name  string
	URL   string // streamable HTTP endpoint
	Token string // bearer token, empty if none
}

// LaunchSpec is everything a driver needs to construct a command.
type LaunchSpec struct {
	Mode     core.SessionMode
	Prompt   string // initial prompt; empty starts interactive idle
	ResumeID string // driver session id to resume; empty starts fresh
	Fork     bool   // with ResumeID: branch instead of continuing

	CWD       string
	ExtraDirs []string // extra dirs the agent may access (--add-dir style)

	Profile    ProfileRef
	Hooks      *HookWiring // nil = no hook wiring
	MCP        []MCPRef
	Permission string // driver-specific permission mode, empty = default
}

// ExecSpec is a fully constructed command, plus files the session runtime
// must materialize before spawning (generated settings, notify configs).
// Env entries are KEY=VAL pairs appended to the scrubbed base environment.
type ExecSpec struct {
	Argv  []string
	Env   []string
	Dir   string
	Files map[string]string // path (absolute or Dir-relative) → content
}

// Parser converts one native stdout line into zero or more normalized events.
// Feed is called per line; Close flushes any trailing state at EOF.
type Parser interface {
	Feed(line []byte) ([]core.EventInput, error)
	Close() ([]core.EventInput, error)
}

// SessionInfo helps RecoverSessionID locate a session the CLI did not
// announce in its output (e.g. Gemini, issue #14435).
type SessionInfo struct {
	ConfigDir string
	CWD       string
	StartedAt time.Time
}

// Driver is implemented once per supported CLI. Implementations must be
// stateless and safe for concurrent use; per-run state lives in Parsers.
type Driver interface {
	ID() string
	Capabilities() Caps
	NewCommand(spec LaunchSpec) (ExecSpec, error)
	NewParser() Parser
	// FormatPrompt encodes a follow-up prompt for a HeadlessStream stdin.
	// Drivers without HeadlessStream return ErrUnsupported.
	FormatPrompt(text string) ([]byte, error)
	// RecoverSessionID finds the driver session id out-of-band. Drivers with
	// EmitsSessionID return ErrUnsupported.
	RecoverSessionID(ctx context.Context, info SessionInfo) (string, error)
}
