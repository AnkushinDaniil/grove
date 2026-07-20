// Package codex implements driver.Driver for the Codex CLI, translating
// grove's normalized LaunchSpec into codex's CLI flags/config overrides and
// its `exec --json` event stream back into normalized core events. It is
// pure and stateless; see internal/driver for the shared contract every
// driver implements.
package codex

import (
	"context"

	"github.com/AnkushinDaniil/grove/internal/driver"
)

// Binary is the executable name grove looks up on PATH to launch Codex.
const Binary = "codex"

// codexDriver implements driver.Driver for the Codex CLI. It is stateless
// and safe for concurrent use; per-run state lives in the Parser it creates.
type codexDriver struct{}

// New returns the Codex driver.
func New() driver.Driver { return codexDriver{} }

func (codexDriver) ID() string { return "codex" }

func (codexDriver) Capabilities() driver.Caps {
	return driver.Caps{
		Interactive:    true,
		Headless:       true,
		HeadlessStream: false,
		Resume:         true,
		Fork:           false,
		EmitsSessionID: true,
		NativeHooks:    true,
		MCP:            true,
	}
}

// RecoverSessionID is unsupported: codex always announces its thread id in
// the `exec --json` stream (thread.started, verified by probe: codex-cli
// 0.143.0 emits {"type":"thread.started","thread_id":"..."} as the first
// line of every run, fresh or resumed), so the session runtime never needs
// to recover one out-of-band.
func (codexDriver) RecoverSessionID(context.Context, driver.SessionInfo) (string, error) {
	return "", driver.ErrUnsupported
}
