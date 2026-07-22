package profile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/AnkushinDaniil/grove/internal/core"
)

// Check is one profile health-probe result surfaced by GET /profiles/{id}/doctor.
type Check struct {
	Name   string
	OK     bool
	Detail string
}

// configDirEnv maps a driver family to the env var that points its CLI at an
// isolated config dir. Drivers absent from the map have no isolation wired yet.
var configDirEnv = map[string]string{
	"claude": "CLAUDE_CONFIG_DIR",
	"codex":  "CODEX_HOME",
}

// driverBinary maps a driver family to the CLI binary doctor probes for a
// version response under the profile env.
var driverBinary = map[string]string{
	"claude":   "claude",
	"codex":    "codex",
	"gemini":   "gemini",
	"opencode": "opencode",
}

// Prober abstracts the OS interactions Doctor performs so tests can drive it
// with fakes. A nil Prober passed to Doctor uses the real OS (osProber).
type Prober interface {
	// Resolve confirms dir exists as a directory and resolves symlinks,
	// returning the resolved absolute path.
	Resolve(dir string) (string, error)
	// ReadSettings returns the profile's settings.json bytes and whether the
	// file exists. A missing file is (nil, false, nil).
	ReadSettings(configDir string) ([]byte, bool, error)
	// RunVersion runs "<bin> --version" with <envVar>=<configDir> added to the
	// environment, returning an error only if the CLI could not be invoked.
	RunVersion(ctx context.Context, bin, envVar, configDir string) error
}

// Doctor runs the profile health probes in a stable order and returns their
// results. It never reads or stores credentials. A nil prober uses the real OS.
func Doctor(ctx context.Context, p core.Profile, prober Prober) []Check {
	if prober == nil {
		prober = osProber{}
	}
	return []Check{
		resolveCheck(p, prober),
		apiKeyCheck(p, prober),
		cliCheck(ctx, p, prober),
	}
}

// resolveCheck verifies the profile config dir exists and resolves.
func resolveCheck(p core.Profile, prober Prober) Check {
	const name = "config dir resolvable"
	resolved, err := prober.Resolve(p.ConfigDir)
	if err != nil {
		return Check{Name: name, OK: false, Detail: fmt.Sprintf("%s: %v", p.ConfigDir, err)}
	}
	return Check{Name: name, OK: true, Detail: resolved}
}

// settingsEnv is the slice of a CLI settings.json doctor inspects: the env block
// that can smuggle an ANTHROPIC_API_KEY overriding subscription auth.
type settingsEnv struct {
	Env map[string]string `json:"env"`
}

// apiKeyCheck flags an ANTHROPIC_API_KEY set in the profile's settings.json env
// block — it silently overrides subscription auth, which defeats the point of a
// per-account profile. Only the presence of the key is inspected; its value is
// never read into the result.
func apiKeyCheck(p core.Profile, prober Prober) Check {
	const name = "no ANTHROPIC_API_KEY in settings"
	data, ok, err := prober.ReadSettings(p.ConfigDir)
	if err != nil {
		return Check{Name: name, OK: false, Detail: fmt.Sprintf("read settings.json: %v", err)}
	}
	if !ok {
		return Check{Name: name, OK: true, Detail: "no settings.json"}
	}
	var s settingsEnv
	if err := json.Unmarshal(data, &s); err != nil {
		return Check{Name: name, OK: false, Detail: fmt.Sprintf("parse settings.json: %v", err)}
	}
	if _, present := s.Env["ANTHROPIC_API_KEY"]; present {
		return Check{Name: name, OK: false, Detail: "settings.json sets ANTHROPIC_API_KEY, which overrides subscription auth"}
	}
	return Check{Name: name, OK: true, Detail: ""}
}

// cliCheck runs the driver CLI's version under the profile env to confirm the
// binary is installed and accepts the isolated config dir.
func cliCheck(ctx context.Context, p core.Profile, prober Prober) Check {
	name := fmt.Sprintf("%s CLI runs under profile env", p.Driver)
	envVar, ok := configDirEnv[p.Driver]
	if !ok {
		return Check{Name: name, OK: false, Detail: fmt.Sprintf("no config-dir isolation wired for driver %q", p.Driver)}
	}
	bin, ok := driverBinary[p.Driver]
	if !ok {
		return Check{Name: name, OK: false, Detail: fmt.Sprintf("no CLI binary known for driver %q", p.Driver)}
	}
	if err := prober.RunVersion(ctx, bin, envVar, p.ConfigDir); err != nil {
		return Check{Name: name, OK: false, Detail: fmt.Sprintf("%s --version: %v", bin, err)}
	}
	return Check{Name: name, OK: true, Detail: fmt.Sprintf("%s=%s", envVar, p.ConfigDir)}
}

// osProber is the production Prober backed by the real filesystem and exec.
type osProber struct{}

func (osProber) Resolve(dir string) (string, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("not a directory")
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", err
	}
	return resolved, nil
}

func (osProber) ReadSettings(configDir string) ([]byte, bool, error) {
	data, err := os.ReadFile(filepath.Join(configDir, "settings.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}

func (osProber) RunVersion(ctx context.Context, bin, envVar, configDir string) error {
	cmd := exec.CommandContext(ctx, bin, "--version")
	cmd.Env = append(os.Environ(), envVar+"="+configDir)
	return cmd.Run()
}
