// Package memory provides zero-touch lifecycle management for MemPalace, the
// local-first memory backend grove uses for cross-driver, tree-scoped agent
// memory (see docs/ORCHESTRATION.md §8).
//
// This package implements phase 1 of that integration: detect an existing
// MemPalace install, install it automatically from the best available channel,
// initialize the on-disk palace, and health-probe the MCP server. The daemon's
// active use of MemPalace as an MCP client (recall injection, auto-capture) is
// phase 2 and lives elsewhere.
//
// MemPalace ground truth (researched 2026-07-21):
//   - PyPI package "mempalace", console-script entry point "mempalace".
//     https://pypi.org/project/mempalace/  https://github.com/MemPalace/mempalace
//   - MCP server speaks newline-delimited JSON-RPC over stdio, launched via
//     `mempalace mcp`. https://zhanghandong.github.io/mempalace-book/en/ch19-mcp-server.html
//   - Data lives under ~/.mempalace/ (config.json, palace/, wal/, ...).
//     https://www.mempalace.net/setup  https://www.mempalace.tech/guides/setup
//   - `mempalace init` creates the palace structure. `mempalace --version`
//     reports the installed version.
package memory

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	// BinaryName is the MemPalace CLI executable (PyPI console-script entry point).
	BinaryName = "mempalace"

	// MCPBinaryName is the dedicated MCP server executable shipped since v3.6
	// alongside the CLI (the older `mempalace mcp` subcommand is the fallback).
	MCPBinaryName = "mempalace-mcp"

	// PinnedVersion is the known-good MemPalace release grove installs by default.
	// This is the latest on PyPI at research time (2026-07-21); bump deliberately.
	// https://pypi.org/project/mempalace/
	PinnedVersion = "3.6.0"

	// palaceDirName is the MemPalace data directory under the user's home dir.
	palaceDirName = ".mempalace"

	// palaceMarker is the file `mempalace init` writes; its presence under the
	// data dir means a palace already exists and must not be re-initialized.
	palaceMarker = "config.json"
)

const (
	versionProbeTimeout = 5 * time.Second
	initTimeout         = 60 * time.Second
	probeTimeout        = 10 * time.Second
	installTimeout      = 5 * time.Minute
)

// Status is the observed state of the MemPalace install on this machine.
type Status struct {
	Installed    bool   // mempalace resolves on PATH
	Path         string // absolute path to the resolved binary
	Version      string // reported by `mempalace --version`, best-effort
	Channel      string // installer that owns the binary (uv|pipx|pip), best-effort
	PalaceExists bool   // the on-disk palace has been initialized
	PalacePath   string // the palace data directory (~/.mempalace)
}

// Env resolves the ambient dependencies memory operations run against. The zero
// value targets the real process PATH and the user's home directory; tests set
// PATH and Home to point at fake binaries and a scratch home for hermetic runs.
type Env struct {
	PATH string // executable search path; "" => os.Getenv("PATH")
	Home string // home dir for palace + Claude settings; "" => os.UserHomeDir()
}

// Detect reports the MemPalace install state. A missing binary is a normal
// status, not an error — callers decide whether to install.
func Detect(ctx context.Context) (Status, error) { return Env{}.Detect(ctx) }

// Install ensures the mempalace CLI is present, picking the best available
// channel. See Env.Install.
func Install(ctx context.Context, opts InstallOptions) error { return Env{}.Install(ctx, opts) }

// InitPalace initializes the on-disk palace if absent (idempotent).
func InitPalace(ctx context.Context) error { return Env{}.InitPalace(ctx) }

// Probe health-checks the MCP server via a JSON-RPC handshake.
func Probe(ctx context.Context) (ProbeReport, error) { return Env{}.Probe(ctx) }

// Doctor runs the full diagnostic pass, printing ✓/✗ lines to w, and reports
// whether the backend is healthy.
func Doctor(ctx context.Context, w io.Writer) bool { return Env{}.Doctor(ctx, w) }

// Detect reports the MemPalace install state for this environment.
func (e Env) Detect(ctx context.Context) (Status, error) {
	palace, err := e.palaceDir()
	if err != nil {
		return Status{}, err
	}
	st := Status{
		PalacePath:   palace,
		PalaceExists: fileExists(filepath.Join(palace, palaceMarker)),
	}
	path, err := e.lookPath(BinaryName)
	if err != nil {
		// Not installed is a normal status, not a failure.
		return st, nil //nolint:nilerr // absence is reported via Status, not error.
	}
	st.Installed = true
	st.Path = path
	st.Version = e.detectVersion(ctx, path)
	st.Channel = detectChannel(path)
	return st, nil
}

// InitPalace creates the MemPalace data structure if absent. Idempotent: an
// existing palace (marker file present) is left untouched — grove never
// re-initializes a user's real palace. https://www.mempalace.net/setup
func (e Env) InitPalace(ctx context.Context) error {
	palace, err := e.palaceDir()
	if err != nil {
		return err
	}
	if fileExists(filepath.Join(palace, palaceMarker)) {
		return nil
	}
	path, err := e.lookPath(BinaryName)
	if err != nil {
		return fmt.Errorf("cannot initialize palace: %w", err)
	}
	ictx, cancel := context.WithTimeout(ctx, initTimeout)
	defer cancel()
	out, err := e.output(ictx, path, "init")
	if err != nil {
		return fmt.Errorf("mempalace init: %w\n%s", err, out)
	}
	if !fileExists(filepath.Join(palace, palaceMarker)) {
		return fmt.Errorf("mempalace init ran but %s was not created", filepath.Join(palace, palaceMarker))
	}
	return nil
}

// detectVersion runs `mempalace --version` best-effort; empty on any failure.
// It takes the already-resolved binary path because exec resolves a bare name
// against the process PATH, not the caller-supplied env.
func (e Env) detectVersion(ctx context.Context, path string) string {
	vctx, cancel := context.WithTimeout(ctx, versionProbeTimeout)
	defer cancel()
	out, err := e.output(vctx, path, "--version")
	if err != nil {
		return ""
	}
	return parseVersion(out)
}

// parseVersion pulls the first dotted-numeric token out of `--version` output,
// tolerating prefixes like "mempalace, version 3.6.0" or a bare "3.6.0".
func parseVersion(out []byte) string {
	for f := range strings.FieldsSeq(string(out)) {
		f = strings.Trim(f, ",vV")
		if looksLikeVersion(f) {
			return f
		}
	}
	return strings.TrimSpace(string(out))
}

func looksLikeVersion(s string) bool {
	if s == "" {
		return false
	}
	dots := 0
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
		case r == '.':
			dots++
		default:
			return false
		}
	}
	return dots >= 1
}

// detectChannel guesses which installer owns the binary from its (symlink-
// resolved) path. Best-effort only: uv and pipx symlink tool binaries into a
// bin dir from their own store; anything else is reported as "".
func detectChannel(path string) string {
	resolved := path
	if real, err := filepath.EvalSymlinks(path); err == nil {
		resolved = real
	}
	switch {
	case strings.Contains(resolved, "uv/tools"):
		return "uv"
	case strings.Contains(resolved, "pipx"):
		return "pipx"
	default:
		return ""
	}
}

func (e Env) home() (string, error) {
	if e.Home != "" {
		return e.Home, nil
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home: %w", err)
	}
	return h, nil
}

func (e Env) palaceDir() (string, error) {
	h, err := e.home()
	if err != nil {
		return "", err
	}
	return filepath.Join(h, palaceDirName), nil
}

func (e Env) searchPath() string {
	if e.PATH != "" {
		return e.PATH
	}
	return os.Getenv("PATH")
}

// lookPath resolves file against e's PATH, mirroring exec.LookPath but honoring
// a caller-supplied PATH instead of the process environment (grove targets unix).
func (e Env) lookPath(file string) (string, error) {
	for _, dir := range filepath.SplitList(e.searchPath()) {
		if dir == "" {
			dir = "."
		}
		full := filepath.Join(dir, file)
		if isExecutable(full) {
			return full, nil
		}
	}
	return "", fmt.Errorf("%q not found on PATH", file)
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode()&0o111 != 0
}

// childEnv is the environment for spawned processes: the process environment
// with PATH and HOME overridden to match e (so MemPalace, which keys its data
// dir off $HOME, and installers resolve against the same view the code does).
func (e Env) childEnv() []string {
	base := os.Environ()
	out := make([]string, 0, len(base)+2)
	for _, kv := range base {
		if strings.HasPrefix(kv, "PATH=") || strings.HasPrefix(kv, "HOME=") {
			continue
		}
		out = append(out, kv)
	}
	out = append(out, "PATH="+e.searchPath())
	if h, err := e.home(); err == nil {
		out = append(out, "HOME="+h)
	}
	return out
}

// output runs name+args to completion and returns combined stdout+stderr.
func (e Env) output(ctx context.Context, name string, args ...string) ([]byte, error) {
	//nolint:gosec // G204: name is a PATH-resolved fixed tool, args constructed in-code.
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = e.childEnv()
	return cmd.CombinedOutput()
}

// stream runs name+args, streaming combined output to w.
func (e Env) stream(ctx context.Context, w io.Writer, name string, args ...string) error {
	if w == nil {
		w = io.Discard
	}
	//nolint:gosec // G204: name is a PATH-resolved fixed tool, args constructed in-code.
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = e.childEnv()
	cmd.Stdout = w
	cmd.Stderr = w
	return cmd.Run()
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func orUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}
