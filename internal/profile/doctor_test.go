package profile

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/AnkushinDaniil/grove/internal/core"
)

// fakeProber drives Doctor with canned filesystem/exec results.
type fakeProber struct {
	resolveErr    error
	settings      []byte
	settingsOK    bool
	settingsErr   error
	runErr        error
	lastRunBin    string
	lastRunEnvVar string
}

func (p *fakeProber) Resolve(dir string) (string, error) {
	if p.resolveErr != nil {
		return "", p.resolveErr
	}
	return dir, nil
}

func (p *fakeProber) ReadSettings(string) ([]byte, bool, error) {
	return p.settings, p.settingsOK, p.settingsErr
}

func (p *fakeProber) RunVersion(_ context.Context, bin, envVar, _ string) error {
	p.lastRunBin = bin
	p.lastRunEnvVar = envVar
	return p.runErr
}

// checkByName finds a check result by name; fails the test if it is absent.
func checkByName(t *testing.T, checks []Check, name string) Check {
	t.Helper()
	for _, c := range checks {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no check named %q in %+v", name, checks)
	return Check{}
}

func claudeProfile() core.Profile {
	return core.Profile{ID: "p1", Driver: "claude", Name: "work", ConfigDir: "/cfg/claude/work"}
}

func TestDoctorAllHealthy(t *testing.T) {
	prober := &fakeProber{settingsOK: false} // no settings.json
	checks := Doctor(t.Context(), claudeProfile(), prober)

	for _, c := range checks {
		if !c.OK {
			t.Errorf("check %q not ok: %s", c.Name, c.Detail)
		}
	}
	if prober.lastRunBin != "claude" || prober.lastRunEnvVar != "CLAUDE_CONFIG_DIR" {
		t.Errorf("RunVersion invoked with bin=%q env=%q, want claude/CLAUDE_CONFIG_DIR",
			prober.lastRunBin, prober.lastRunEnvVar)
	}
}

func TestDoctorFlagsAPIKeyInSettings(t *testing.T) {
	prober := &fakeProber{
		settingsOK: true,
		settings:   []byte(`{"env":{"ANTHROPIC_API_KEY":"sk-should-not-be-here"}}`),
	}
	checks := Doctor(t.Context(), claudeProfile(), prober)

	c := checkByName(t, checks, "no ANTHROPIC_API_KEY in settings")
	if c.OK {
		t.Error("ANTHROPIC_API_KEY in settings.json should fail the doctor check")
	}
}

func TestDoctorSettingsWithoutAPIKeyPasses(t *testing.T) {
	prober := &fakeProber{
		settingsOK: true,
		settings:   []byte(`{"env":{"FOO":"bar"},"model":"claude-sonnet-5"}`),
	}
	checks := Doctor(t.Context(), claudeProfile(), prober)

	c := checkByName(t, checks, "no ANTHROPIC_API_KEY in settings")
	if !c.OK {
		t.Errorf("settings without ANTHROPIC_API_KEY should pass, got: %s", c.Detail)
	}
}

func TestDoctorResolveFailure(t *testing.T) {
	prober := &fakeProber{resolveErr: errors.New("no such file or directory")}
	checks := Doctor(t.Context(), claudeProfile(), prober)

	c := checkByName(t, checks, "config dir resolvable")
	if c.OK {
		t.Error("unresolvable config dir should fail the doctor check")
	}
}

func TestDoctorCLIFailure(t *testing.T) {
	prober := &fakeProber{runErr: errors.New("exec: \"claude\": not found in $PATH")}
	checks := Doctor(t.Context(), claudeProfile(), prober)

	c := checkByName(t, checks, "claude CLI runs under profile env")
	if c.OK {
		t.Error("a CLI that will not run should fail the doctor check")
	}
}

func TestOSProberResolve(t *testing.T) {
	dir := t.TempDir()
	resolved, err := (osProber{}).Resolve(dir)
	if err != nil {
		t.Fatalf("Resolve(dir): %v", err)
	}
	if resolved == "" {
		t.Error("Resolve returned an empty path")
	}

	file := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(file, []byte("{}"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := (osProber{}).Resolve(file); err == nil {
		t.Error("Resolve(file) succeeded, want a not-a-directory error")
	}
	if _, err := (osProber{}).Resolve(filepath.Join(dir, "missing")); err == nil {
		t.Error("Resolve(missing) succeeded, want an error")
	}
}

func TestOSProberReadSettings(t *testing.T) {
	dir := t.TempDir()

	data, ok, err := (osProber{}).ReadSettings(dir)
	if err != nil || ok || data != nil {
		t.Fatalf("ReadSettings(no file) = %q, %v, %v; want nil, false, nil", data, ok, err)
	}

	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"env":{}}`), 0o600); err != nil {
		t.Fatalf("write settings.json: %v", err)
	}
	data, ok, err = (osProber{}).ReadSettings(dir)
	if err != nil || !ok || len(data) == 0 {
		t.Fatalf("ReadSettings(present) = %q, %v, %v; want bytes, true, nil", data, ok, err)
	}
}

func TestOSProberRunVersion(t *testing.T) {
	// `true` ignores its args and exits 0, standing in for a CLI that runs.
	if err := (osProber{}).RunVersion(t.Context(), "true", "SOME_CONFIG_DIR", t.TempDir()); err != nil {
		t.Errorf("RunVersion(true) = %v, want nil", err)
	}
	if err := (osProber{}).RunVersion(t.Context(), "grove-no-such-binary-xyz", "X", t.TempDir()); err == nil {
		t.Error("RunVersion(missing binary) = nil, want an error")
	}
}

func TestDoctorUnsupportedDriverIsolation(t *testing.T) {
	prober := &fakeProber{}
	p := core.Profile{ID: "p2", Driver: "gemini", Name: "x", ConfigDir: "/cfg/gemini/x"}
	checks := Doctor(t.Context(), p, prober)

	// gemini has no config-dir env wired yet, so the CLI check reports that
	// rather than shelling out.
	c := checkByName(t, checks, "gemini CLI runs under profile env")
	if c.OK {
		t.Error("driver without config-dir isolation should fail the CLI check")
	}
	if prober.lastRunBin != "" {
		t.Errorf("RunVersion should not be called for an unwired driver, got bin=%q", prober.lastRunBin)
	}
}
