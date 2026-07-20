// Package fakeagent provides a scripted fake CLI agent for session integration
// tests: a Build helper that compiles the companion main program once per test
// binary, a WriteScript helper, and a driver.Driver ("fake") that runs it.
package fakeagent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/driver"
)

// Step mirrors the wire format consumed by the fakeagent main program. Exactly
// one field should be set per step.
type Step struct {
	Emit          string `json:"emit,omitempty"`
	SleepMS       int    `json:"sleep_ms,omitempty"`
	WaitStdinLine bool   `json:"wait_stdin_line,omitempty"`
	ExitCode      *int   `json:"exit_code,omitempty"`
}

var (
	buildOnce sync.Once
	binPath   string
	buildErr  error
)

// Build compiles the fakeagent binary once per test binary run and returns its
// path. The binary lives in a temp dir that is intentionally leaked for the
// lifetime of the test process (shared via sync.Once across all tests); the OS
// reclaims TMPDIR afterwards.
func Build(t testing.TB) string {
	t.Helper()
	buildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "fakeagent-*")
		if err != nil {
			buildErr = fmt.Errorf("temp dir: %w", err)
			return
		}
		bin := filepath.Join(dir, "fakeagent")
		//nolint:gosec // G204: fixed build of an in-repo package, no external input.
		cmd := exec.CommandContext(context.Background(), "go", "build", "-o", bin,
			"github.com/AnkushinDaniil/grove/internal/testutil/fakeagent/main")
		if out, err := cmd.CombinedOutput(); err != nil {
			buildErr = fmt.Errorf("build fakeagent: %w\n%s", err, out)
			return
		}
		binPath = bin
	})
	if buildErr != nil {
		t.Fatalf("fakeagent.Build: %v", buildErr)
	}
	return binPath
}

// WriteScript marshals steps into a {"steps":[...]} script file in the test's
// temp dir and returns its path.
func WriteScript(t testing.TB, steps []Step) string {
	t.Helper()
	data, err := json.Marshal(map[string]any{"steps": steps})
	if err != nil {
		t.Fatalf("marshal script: %v", err)
	}
	path := filepath.Join(t.TempDir(), "script.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write script: %v", err)
	}
	return path
}

// NewDriver returns a driver.Driver that runs the compiled fakeagent binary
// with the given script, ignoring most of LaunchSpec. Its parser treats each
// line either as a typed grove event (when the JSON carries a valid "event"
// field) or as plain assistant text.
func NewDriver(binPath, scriptPath string) driver.Driver {
	return fakeDriver{binPath: binPath, scriptPath: scriptPath}
}

type fakeDriver struct {
	binPath    string
	scriptPath string
}

func (fakeDriver) ID() string { return "fake" }

func (fakeDriver) Capabilities() driver.Caps {
	return driver.Caps{
		Interactive:    true,
		Headless:       true,
		HeadlessStream: true,
		EmitsSessionID: true,
	}
}

func (d fakeDriver) NewCommand(spec driver.LaunchSpec) (driver.ExecSpec, error) {
	return driver.ExecSpec{
		Argv: []string{d.binPath},
		Env:  []string{"FAKEAGENT_SCRIPT=" + d.scriptPath},
		Dir:  spec.CWD,
	}, nil
}

func (fakeDriver) NewParser() driver.Parser { return fakeParser{} }

func (fakeDriver) FormatPrompt(text string) ([]byte, error) {
	return []byte(text + "\n"), nil
}

func (fakeDriver) RecoverSessionID(context.Context, driver.SessionInfo) (string, error) {
	return "", driver.ErrUnsupported
}

// fakeParser maps a scripted line onto a normalized event.
type fakeParser struct{}

// wireEvent is the optional typed shape a script line may take.
type wireEvent struct {
	Event   core.EventType      `json:"event"`
	Payload json.RawMessage     `json:"payload"`
	Reason  core.AwaitingReason `json:"reason"`
	Detail  string              `json:"detail"`
}

func (fakeParser) Feed(line []byte) ([]core.EventInput, error) {
	trimmed := bytes.TrimRight(line, "\r\n")
	if len(bytes.TrimSpace(trimmed)) == 0 {
		return nil, nil
	}
	var ev wireEvent
	if err := json.Unmarshal(trimmed, &ev); err == nil && ev.Event.Valid() {
		payload := ""
		if len(ev.Payload) > 0 {
			payload = string(ev.Payload)
		}
		return []core.EventInput{{
			Type:    ev.Event,
			Payload: payload,
			Reason:  ev.Reason,
			Detail:  ev.Detail,
		}}, nil
	}
	payload, err := core.MarshalPayload(core.TextPayload{Text: string(trimmed)})
	if err != nil {
		return nil, fmt.Errorf("encode text payload: %w", err)
	}
	return []core.EventInput{{Type: core.EventText, Payload: payload}}, nil
}

func (fakeParser) Close() ([]core.EventInput, error) { return nil, nil }
