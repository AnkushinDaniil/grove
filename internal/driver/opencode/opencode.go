// Package opencode implements driver.Driver for the OpenCode CLI,
// translating grove's normalized LaunchSpec into opencode's CLI flags and
// its `run --format json` JSON-lines output back into normalized core
// events. It is pure and stateless; see internal/driver for the shared
// contract every driver implements.
//
// OpenCode CLI is not installed in this development environment; every
// flag and JSON shape this package depends on is pinned from the official
// docs (https://opencode.ai/docs/cli/) or, where the docs were incomplete,
// read directly from the opencode source
// (https://github.com/anomalyco/opencode, dev branch, July 2026) — never
// guessed. Facts that could plausibly drift between CLI versions are called
// out at their use site with a `// ASSUMPTION(verify-on-install):` comment;
// the parser is built to skip unrecognized lines rather than crash or
// misreport when a guess is wrong.
package opencode

import (
	"context"

	"github.com/AnkushinDaniil/grove/internal/driver"
)

// Binary is the executable name grove looks up on PATH to launch OpenCode.
const Binary = "opencode"

// opencodeDriver implements driver.Driver for the OpenCode CLI. It is
// stateless and safe for concurrent use; per-run state lives in the Parser
// it creates.
type opencodeDriver struct{}

// New returns the OpenCode driver.
func New() driver.Driver { return opencodeDriver{} }

func (opencodeDriver) ID() string { return "opencode" }

// Capabilities reflects opencode.ai/docs/cli/ and
// packages/opencode/src/cli/cmd/{run,tui}.ts: opencode has no persistent
// stdin stream for headless follow-up turns (HeadlessStream false — every
// turn is its own `opencode run` invocation), a real --fork flag that
// branches an existing session (Fork true, requires --session or
// --continue), no native hook/notify channel (NativeHooks false), and every
// JSON event carries a sessionID field (EmitsSessionID true — see
// prompt.go/opencode.go's RecoverSessionID).
func (opencodeDriver) Capabilities() driver.Caps {
	return driver.Caps{
		Interactive:    true,
		Headless:       true,
		HeadlessStream: false,
		Resume:         true,
		Fork:           true,
		EmitsSessionID: true,
		NativeHooks:    false,
		MCP:            true,
	}
}

// RecoverSessionID is unsupported: every JSON event `run --format json`
// emits carries a top-level sessionID field
// (packages/opencode/src/cli/cmd/run.ts, emit()), so the session runtime
// never needs to recover one out-of-band.
func (opencodeDriver) RecoverSessionID(context.Context, driver.SessionInfo) (string, error) {
	return "", driver.ErrUnsupported
}
