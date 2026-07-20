// Package claude implements driver.Driver for Claude Code, translating
// grove's normalized LaunchSpec into Claude Code's CLI flags and its
// stream-json output back into normalized core events. It is pure and
// stateless; see internal/driver for the shared contract every driver
// implements.
package claude

import (
	"context"

	"github.com/AnkushinDaniil/grove/internal/driver"
)

// Binary is the executable name grove looks up on PATH to launch Claude Code.
const Binary = "claude"

// claudeDriver implements driver.Driver for Claude Code. It is stateless and
// safe for concurrent use; per-run state lives in the Parser it creates.
type claudeDriver struct{}

// New returns the Claude Code driver.
func New() driver.Driver { return claudeDriver{} }

func (claudeDriver) ID() string { return "claude" }

func (claudeDriver) Capabilities() driver.Caps {
	return driver.Caps{
		Interactive:    true,
		Headless:       true,
		HeadlessStream: true,
		Resume:         true,
		Fork:           true,
		EmitsSessionID: true,
		NativeHooks:    true,
		MCP:            true,
	}
}

// RecoverSessionID is unsupported: Claude Code always announces its session
// id in the stream (system/init), so the session runtime never needs to
// recover one out-of-band.
func (claudeDriver) RecoverSessionID(context.Context, driver.SessionInfo) (string, error) {
	return "", driver.ErrUnsupported
}
