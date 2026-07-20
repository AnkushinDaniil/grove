package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/AnkushinDaniil/grove/internal/config"
)

const readyTimeout = 8 * time.Second

// runUp is the one-command entry point: start the daemon in the background if
// it is not already running, then open the web UI. Running `grove` with no
// arguments lands here.
func runUp(args []string) error {
	fs := flag.NewFlagSet("up", flag.ContinueOnError)
	port := fs.Int("port", defaultPort, "daemon port")
	home := fs.String("home", "", "grove state directory (default ~/.grove or $GROVE_HOME)")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse up flags: %w", err)
	}

	layout, err := resolveLayout(*home)
	if err != nil {
		return err
	}
	if err := layout.Ensure(); err != nil {
		return fmt.Errorf("ensure state layout: %w", err)
	}
	token, err := config.LoadOrCreateToken(layout.TokenPath)
	if err != nil {
		return fmt.Errorf("load daemon token: %w", err)
	}

	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	if !daemonReachable(addr) {
		if err := spawnDaemon(layout, *port); err != nil {
			return err
		}
		if err := waitReady(addr, readyTimeout); err != nil {
			return fmt.Errorf("%w — check %s", err, filepath.Join(layout.Logs, "daemon.log"))
		}
		fmt.Printf("grove daemon started on http://%s (logs: %s)\n", addr, filepath.Join(layout.Logs, "daemon.log"))
	}

	url := fmt.Sprintf("http://%s/auth#t=%s", addr, token)
	if err := openBrowser(url); err != nil {
		fmt.Printf("Open this URL to sign in:\n  %s\n", url)
	}
	return nil
}

// spawnDaemon starts `grove serve` detached in its own session, with output
// appended to the daemon log file. The parent returns immediately.
func spawnDaemon(layout config.Layout, port int) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve grove executable: %w", err)
	}
	logPath := filepath.Join(layout.Logs, "daemon.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open daemon log: %w", err)
	}
	defer func() { _ = logFile.Close() }() // the child holds its own descriptor

	//nolint:gosec,noctx // G204: argv is the grove binary itself with fixed flags.
	// noctx: the daemon is a detached long-lived child that must outlive any
	// context this short-lived launcher could carry.
	cmd := exec.Command(exe, "serve",
		"--port", strconv.Itoa(port),
		"--home", layout.Home,
	)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // survive this process and its terminal
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start daemon: %w", err)
	}
	// Detach: the daemon is intentionally not waited on. Reap-avoidance is the
	// init system's job once this parent exits.
	return nil
}

// waitReady polls addr until the daemon accepts connections or the timeout expires.
func waitReady(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if daemonReachable(addr) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("daemon did not become ready on %s within %s", addr, timeout)
}
