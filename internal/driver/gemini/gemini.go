// Package gemini implements driver.Driver for the Gemini CLI, translating
// grove's normalized LaunchSpec into gemini's CLI flags and its headless
// `--output-format json` output back into normalized core events. It is pure
// and stateless; see internal/driver for the shared contract every driver
// implements.
//
// Gemini CLI is not installed in this development environment; every flag,
// JSON shape and file-layout fact this package depends on is pinned from the
// official docs (https://github.com/google-gemini/gemini-cli/tree/main/docs/cli)
// or read directly from the gemini-cli source
// (https://github.com/google-gemini/gemini-cli, main branch, July 2026) —
// never guessed. Facts that could plausibly drift between CLI versions are
// called out at their use site with a `// ASSUMPTION(verify-on-install):`
// comment; the parser and RecoverSessionID are built to degrade to "found
// nothing" rather than crash or return a wrong answer when a guess is wrong.
package gemini

import "github.com/AnkushinDaniil/grove/internal/driver"

// Binary is the executable name grove looks up on PATH to launch Gemini CLI.
const Binary = "gemini"

// geminiDriver implements driver.Driver for Gemini CLI. It is stateless and
// safe for concurrent use; per-run state lives in the Parser it creates.
type geminiDriver struct{}

// New returns the Gemini CLI driver.
func New() driver.Driver { return geminiDriver{} }

func (geminiDriver) ID() string { return "gemini" }

// Capabilities reflects docs/cli/cli-reference.md and
// docs/cli/session-management.md: gemini has no persistent stdin stream for
// headless follow-up turns (HeadlessStream false — every turn, including
// grove's wake batches, is its own process per docs/ORCHESTRATION.md §2), no
// id-addressed forking (Fork false — --resume only continues), no native
// hook/notify channel (NativeHooks false), and its headless JSON schema
// omits the session id (EmitsSessionID false — see recover.go, gemini-cli
// issue #14435).
func (geminiDriver) Capabilities() driver.Caps {
	return driver.Caps{
		Interactive:    true,
		Headless:       true,
		HeadlessStream: false,
		Resume:         true,
		Fork:           false,
		EmitsSessionID: false,
		NativeHooks:    false,
		MCP:            true,
	}
}
