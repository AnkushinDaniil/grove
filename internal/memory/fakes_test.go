package memory

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

// This file provides hermetic fakes for the MemPalace CLI and the Python
// installers, mirroring the compiled-stub approach in internal/testutil/fakeagent.
// Two tiny Go programs are built once per test binary run; per-test bin dirs then
// hold copies plus control files, and Env{PATH, Home} points at them so no real
// mempalace/uv/pipx/pip is ever touched.

var (
	fakesOnce sync.Once
	fakesMemp string
	fakesInst string
	fakesErr  error
)

// buildFakes compiles the fake mempalace and installer binaries once and returns
// their paths. The build dir is intentionally leaked for the test process's life
// (shared via sync.Once); the OS reclaims TMPDIR afterwards.
func buildFakes(t testing.TB) (mempalace, installer string) {
	t.Helper()
	fakesOnce.Do(func() {
		dir, err := os.MkdirTemp("", "memfakes-*")
		if err != nil {
			fakesErr = err
			return
		}
		if fakesMemp, fakesErr = buildProg(dir, "mempalace", fakeMempalaceSrc); fakesErr != nil {
			return
		}
		fakesInst, fakesErr = buildProg(dir, "installer", fakeInstallerSrc)
	})
	if fakesErr != nil {
		t.Fatalf("buildFakes: %v", fakesErr)
	}
	return fakesMemp, fakesInst
}

func buildProg(dir, name, src string) (string, error) {
	if err := os.WriteFile(filepath.Join(dir, name+".go"), []byte(src), 0o600); err != nil {
		return "", err
	}
	bin := filepath.Join(dir, name)
	cmd := exec.Command("go", "build", "-o", bin, name+".go")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("build %s: %w\n%s", name, err, out)
	}
	return bin, nil
}

// fakeOpts configures a per-test fake bin dir.
type fakeOpts struct {
	tools       []string // installer stubs to place (uv|pipx|pip)
	installed   bool     // place a working mempalace stub
	version     string   // versionout control ("" => stub default; "EXIT1" => --version fails)
	mcpmode     string   // mcpmode control (ok|notools|hang|errorinit|badjson)
	installFail bool     // installer stub exits non-zero
}

type fakeBin struct {
	binDir string
	home   string
}

func newFakeBin(t *testing.T, opts fakeOpts) fakeBin {
	t.Helper()
	memp, inst := buildFakes(t)
	fb := fakeBin{binDir: t.TempDir(), home: t.TempDir()}
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(fb.binDir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write control %s: %v", name, err)
		}
	}
	// installer stubs copy this fake mempalace onto PATH when they "install".
	write("installer.src", memp)
	if opts.version != "" {
		write("versionout", opts.version)
	}
	if opts.mcpmode != "" {
		write("mcpmode", opts.mcpmode)
	}
	if opts.installFail {
		write("installer.fail", "1")
	}
	if opts.installed {
		copyBin(t, memp, filepath.Join(fb.binDir, "mempalace"))
	}
	for _, tool := range opts.tools {
		copyBin(t, inst, filepath.Join(fb.binDir, tool))
	}
	return fb
}

func (fb fakeBin) env() Env { return Env{PATH: fb.binDir, Home: fb.home} }

// record returns the installer invocation log ("" if no installer ran).
func (fb fakeBin) record(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(fb.binDir, "record"))
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		t.Fatalf("read record: %v", err)
	}
	return string(b)
}

func copyBin(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read %s: %v", src, err)
	}
	if err := os.WriteFile(dst, data, 0o755); err != nil {
		t.Fatalf("write %s: %v", dst, err)
	}
}

func writePalace(t *testing.T, fb fakeBin) {
	t.Helper()
	dir := filepath.Join(fb.home, palaceDirName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir palace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, palaceMarker), []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write palace marker: %v", err)
	}
}

func writeClaudeSettings(t *testing.T, fb fakeBin, content string) {
	t.Helper()
	dir := filepath.Join(fb.home, ".claude")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(content), 0o600); err != nil {
		t.Fatalf("write settings: %v", err)
	}
}

// fakeMempalaceSrc is a stand-in for the mempalace CLI: it answers --version,
// creates a palace on `init`, and speaks just enough newline JSON-RPC on `mcp`.
// Behavior is driven by control files next to its own executable, so no test
// environment variables leak between cases.
const fakeMempalaceSrc = `package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func ctlDir() string {
	p, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(p)
}

func ctl(name, def string) string {
	b, err := os.ReadFile(filepath.Join(ctlDir(), name))
	if err != nil {
		return def
	}
	return strings.TrimSpace(string(b))
}

func main() {
	args := os.Args[1:]
	for _, a := range args {
		if a == "--version" || a == "-V" {
			v := ctl("versionout", "9.9.9")
			if v == "EXIT1" {
				fmt.Fprintln(os.Stderr, "version unavailable")
				os.Exit(1)
			}
			fmt.Printf("mempalace %s\n", v)
			return
		}
	}
	if len(args) > 0 && args[0] == "init" {
		dir := filepath.Join(os.Getenv("HOME"), ".mempalace")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		_ = os.WriteFile(filepath.Join(dir, "config.json"), []byte("{}\n"), 0o600)
		return
	}
	if len(args) > 0 && args[0] == "mcp" {
		runMCP(ctl("mcpmode", "ok"))
	}
}

func runMCP(mode string) {
	if mode == "hang" {
		time.Sleep(30 * time.Second)
		return
	}
	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		var m map[string]any
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			continue
		}
		id, hasID := m["id"]
		if !hasID {
			continue
		}
		method, _ := m["method"].(string)
		switch method {
		case "initialize":
			if mode == "errorinit" {
				writeErr(out, id, -32000, "init failed")
			} else if mode == "badjson" {
				fmt.Fprintln(out, "this is not json")
				out.Flush()
				return
			} else {
				writeResult(out, id, map[string]any{
					"protocolVersion": "2025-06-18",
					"serverInfo":      map[string]any{"name": "mempalace", "version": "9.9.9"},
					"capabilities":    map[string]any{"tools": map[string]any{}},
				})
			}
		case "tools/list":
			var tools []map[string]any
			if mode != "notools" {
				for _, n := range toolNames() {
					tools = append(tools, map[string]any{"name": n, "description": n})
				}
			}
			writeResult(out, id, map[string]any{"tools": tools})
		default:
			writeErr(out, id, -32601, "method not found: "+method)
		}
		out.Flush()
	}
}

func toolNames() []string {
	return []string{
		"mempalace_status", "mempalace_list_wings", "mempalace_list_rooms",
		"mempalace_search", "mempalace_add_drawer", "mempalace_delete_drawer",
		"mempalace_kg_query",
	}
}

func writeResult(w *bufio.Writer, id, result any) {
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
	_, _ = w.Write(append(b, '\n'))
}

func writeErr(w *bufio.Writer, id any, code int, msg string) {
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": msg}})
	_, _ = w.Write(append(b, '\n'))
}
`

// fakeInstallerSrc is a stand-in for uv/pipx/pip: it records its argv and, unless
// installer.fail is present, drops the fake mempalace binary onto PATH (into its
// own dir) to simulate a successful install.
const fakeInstallerSrc = `package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func ctlDir() string {
	p, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(p)
}

func main() {
	name := filepath.Base(os.Args[0])
	if f, err := os.OpenFile(filepath.Join(ctlDir(), "record"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
		fmt.Fprintf(f, "%s %s\n", name, strings.Join(os.Args[1:], " "))
		_ = f.Close()
	}
	if _, err := os.Stat(filepath.Join(ctlDir(), "installer.fail")); err == nil {
		fmt.Fprintln(os.Stderr, name+": simulated install failure")
		os.Exit(1)
	}
	src, _ := os.ReadFile(filepath.Join(ctlDir(), "installer.src"))
	if s := strings.TrimSpace(string(src)); s != "" {
		if err := copyExe(s, filepath.Join(ctlDir(), "mempalace")); err != nil {
			fmt.Fprintln(os.Stderr, "drop mempalace:", err)
			os.Exit(1)
		}
	}
	fmt.Printf("%s: installed mempalace (fake)\n", name)
}

func copyExe(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
`
