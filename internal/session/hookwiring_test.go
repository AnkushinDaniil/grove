package session

import (
	"context"
	"sync"
	"testing"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/driver"
	"github.com/AnkushinDaniil/grove/internal/testutil/fakeagent"
)

// captureDriver wraps the fakeagent driver so a launch still spawns and exits
// cleanly, while recording the LaunchSpec the manager built and letting a test
// override the advertised capabilities (notably NativeHooks). Its ID stays
// "fake" so the test fixture's node resolves to it.
type captureDriver struct {
	inner driver.Driver
	caps  driver.Caps

	mu   sync.Mutex
	last *driver.LaunchSpec
}

func newCaptureDriver(t *testing.T, caps driver.Caps) *captureDriver {
	t.Helper()
	bin := fakeagent.Build(t)
	script := fakeagent.WriteScript(t, []fakeagent.Step{{ExitCode: new(0)}})
	return &captureDriver{inner: fakeagent.NewDriver(bin, script), caps: caps}
}

func (d *captureDriver) ID() string                { return d.inner.ID() }
func (d *captureDriver) Capabilities() driver.Caps { return d.caps }
func (d *captureDriver) NewParser() driver.Parser  { return d.inner.NewParser() }

func (d *captureDriver) FormatPrompt(text string) ([]byte, error) {
	return d.inner.FormatPrompt(text)
}

func (d *captureDriver) RecoverSessionID(ctx context.Context, info driver.SessionInfo) (string, error) {
	return d.inner.RecoverSessionID(ctx, info)
}

func (d *captureDriver) NewCommand(spec driver.LaunchSpec) (driver.ExecSpec, error) {
	d.mu.Lock()
	captured := spec
	d.last = &captured
	d.mu.Unlock()
	return d.inner.NewCommand(spec)
}

func (d *captureDriver) spec() *driver.LaunchSpec {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.last
}

// TestStartWiresHooksForNativeHookDriver asserts the manager builds HookWiring
// from its config for a driver that advertises NativeHooks, minting the token
// per node.
func TestStartWiresHooksForNativeHookDriver(t *testing.T) {
	drv := newCaptureDriver(t, driver.Caps{Headless: true, HeadlessStream: true, EmitsSessionID: true, NativeHooks: true})
	cfg := Config{
		HookCommand:   "/usr/local/bin/grove hook",
		DaemonURL:     "http://127.0.0.1:7433",
		MintHookToken: func(id core.NodeID) string { return "tok-" + string(id) },
	}
	m, _, node := newFixtureWithDriver(t, cfg, drv)

	if _, err := m.Start(t.Context(), node, core.ModeHeadless, "do it", ""); err != nil {
		t.Fatalf("Start: %v", err)
	}

	spec := drv.spec()
	if spec == nil {
		t.Fatal("driver NewCommand was never called")
	}
	if spec.Hooks == nil {
		t.Fatal("spec.Hooks is nil; expected wiring for a NativeHooks driver")
	}
	if spec.Hooks.NodeID != node {
		t.Errorf("Hooks.NodeID = %q, want %q", spec.Hooks.NodeID, node)
	}
	if want := "tok-" + string(node); spec.Hooks.Token != want {
		t.Errorf("Hooks.Token = %q, want %q", spec.Hooks.Token, want)
	}
	if spec.Hooks.HookCommand != cfg.HookCommand {
		t.Errorf("Hooks.HookCommand = %q, want %q", spec.Hooks.HookCommand, cfg.HookCommand)
	}
	if spec.Hooks.DaemonURL != cfg.DaemonURL {
		t.Errorf("Hooks.DaemonURL = %q, want %q", spec.Hooks.DaemonURL, cfg.DaemonURL)
	}
}

// TestStartSkipsHooksWhenConfigIncomplete asserts a NativeHooks driver still
// launches unwired when any hook input is missing, preserving the current
// behavior for managers that were never given the wiring inputs.
func TestStartSkipsHooksWhenConfigIncomplete(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
	}{
		{"no mint func", Config{HookCommand: "/x/grove hook", DaemonURL: "http://127.0.0.1:7433"}},
		{"no hook command", Config{DaemonURL: "http://127.0.0.1:7433", MintHookToken: func(core.NodeID) string { return "tok" }}},
		{"no daemon url", Config{HookCommand: "/x/grove hook", MintHookToken: func(core.NodeID) string { return "tok" }}},
		{"empty minted token", Config{
			HookCommand: "/x/grove hook", DaemonURL: "http://127.0.0.1:7433",
			MintHookToken: func(core.NodeID) string { return "" },
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			drv := newCaptureDriver(t, driver.Caps{Headless: true, NativeHooks: true})
			m, _, node := newFixtureWithDriver(t, tt.cfg, drv)
			if _, err := m.Start(t.Context(), node, core.ModeHeadless, "go", ""); err != nil {
				t.Fatalf("Start: %v", err)
			}
			if spec := drv.spec(); spec == nil || spec.Hooks != nil {
				t.Fatalf("expected no hook wiring, got spec %+v", spec)
			}
		})
	}
}

// TestStartSkipsHooksForNonNativeDriver asserts a fully configured manager still
// leaves a launch unwired when the driver does not advertise NativeHooks.
func TestStartSkipsHooksForNonNativeDriver(t *testing.T) {
	drv := newCaptureDriver(t, driver.Caps{Headless: true, NativeHooks: false})
	cfg := Config{
		HookCommand:   "/x/grove hook",
		DaemonURL:     "http://127.0.0.1:7433",
		MintHookToken: func(core.NodeID) string { return "tok" },
	}
	m, _, node := newFixtureWithDriver(t, cfg, drv)
	if _, err := m.Start(t.Context(), node, core.ModeHeadless, "go", ""); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if spec := drv.spec(); spec == nil || spec.Hooks != nil {
		t.Fatalf("expected no hook wiring for a non-native driver, got spec %+v", spec)
	}
}
