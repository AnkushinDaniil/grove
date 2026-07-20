package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"time"

	"github.com/AnkushinDaniil/grove/internal/config"
)

// dialTimeout bounds the reachability probe against the daemon.
const dialTimeout = 500 * time.Millisecond

// runOpen opens the web UI in a browser, handing the daemon token to the login
// page via the URL fragment. If the daemon is not reachable it prints
// instructions instead of failing silently.
func runOpen(args []string) error {
	fs := flag.NewFlagSet("open", flag.ContinueOnError)
	port := fs.Int("port", defaultPort, "daemon port")
	home := fs.String("home", "", "grove state directory (default ~/.grove or $GROVE_HOME)")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse open flags: %w", err)
	}

	layout, err := resolveLayout(*home)
	if err != nil {
		return err
	}
	token, err := config.LoadOrCreateToken(layout.TokenPath)
	if err != nil {
		return fmt.Errorf("load daemon token: %w", err)
	}

	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	url := fmt.Sprintf("http://%s/auth#t=%s", addr, token)

	if !daemonReachable(addr) {
		fmt.Printf("grove daemon is not reachable at %s.\n", addr)
		fmt.Printf("Start it with `grove serve`, then open:\n  %s\n", url)
		return nil
	}
	if err := openBrowser(url); err != nil {
		fmt.Printf("Open this URL to sign in:\n  %s\n", url)
	}
	return nil
}

// daemonReachable reports whether a TCP connection to addr succeeds quickly.
func daemonReachable(addr string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// openBrowser launches the platform's URL opener.
func openBrowser(url string) error {
	var name string
	switch runtime.GOOS {
	case "darwin":
		name = "open"
	case "linux":
		name = "xdg-open"
	default:
		return fmt.Errorf("unsupported platform %q", runtime.GOOS)
	}
	//nolint:gosec // G204: opener is a fixed literal; url is built from the local token file, not external input.
	cmd := exec.CommandContext(context.Background(), name, url)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("launch browser: %w", err)
	}
	return nil
}
