package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/AnkushinDaniil/grove/internal/config"
)

// runService manages a login service for the daemon so grove is always
// running: launchd on macOS, a systemd user unit on Linux.
func runService(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: grove service <install|uninstall|status> [--port N] [--home DIR]")
	}
	action := args[0]

	fs := flag.NewFlagSet("service "+action, flag.ContinueOnError)
	port := fs.Int("port", defaultPort, "daemon port")
	home := fs.String("home", "", "grove state directory (default ~/.grove or $GROVE_HOME)")
	if err := fs.Parse(args[1:]); err != nil {
		return fmt.Errorf("parse service flags: %w", err)
	}
	layout, err := resolveLayout(*home)
	if err != nil {
		return err
	}
	if err := layout.Ensure(); err != nil {
		return fmt.Errorf("ensure state layout: %w", err)
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve grove executable: %w", err)
	}

	switch runtime.GOOS {
	case "darwin":
		return launchdService(action, exe, layout, *port)
	case "linux":
		return systemdService(action, exe, layout, *port)
	default:
		return fmt.Errorf("grove service is not supported on %s", runtime.GOOS)
	}
}

const launchdLabel = "dev.grove.daemon"

// launchdPlist renders the LaunchAgent definition.
func launchdPlist(exe string, layout config.Layout, port int) string {
	logPath := filepath.Join(layout.Logs, "daemon.log")
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key><string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>serve</string>
		<string>--port</string><string>%d</string>
		<string>--home</string><string>%s</string>
	</array>
	<key>RunAtLoad</key><true/>
	<key>KeepAlive</key><true/>
	<key>StandardOutPath</key><string>%s</string>
	<key>StandardErrorPath</key><string>%s</string>
</dict>
</plist>
`, launchdLabel, exe, port, layout.Home, logPath, logPath)
}

func launchdService(action, exe string, layout config.Layout, port int) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve user home: %w", err)
	}
	plistPath := filepath.Join(homeDir, "Library", "LaunchAgents", launchdLabel+".plist")

	switch action {
	case "install":
		if err := os.MkdirAll(filepath.Dir(plistPath), 0o750); err != nil {
			return fmt.Errorf("create LaunchAgents dir: %w", err)
		}
		if err := os.WriteFile(plistPath, []byte(launchdPlist(exe, layout, port)), 0o600); err != nil {
			return fmt.Errorf("write %s: %w", plistPath, err)
		}
		_ = run("launchctl", "unload", "-w", plistPath) // reload cleanly if it was present
		if err := run("launchctl", "load", "-w", plistPath); err != nil {
			return fmt.Errorf("launchctl load: %w", err)
		}
		fmt.Printf("Installed and started %s (port %d).\nThe daemon now starts at login; open the UI with `grove open`.\n", launchdLabel, port)
		return nil
	case "uninstall":
		_ = run("launchctl", "unload", "-w", plistPath)
		if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove %s: %w", plistPath, err)
		}
		fmt.Println("Service removed.")
		return nil
	case "status":
		if err := run("launchctl", "list", launchdLabel); err != nil {
			fmt.Println("not loaded")
			return nil //nolint:nilerr // "not loaded" is a valid status outcome, not a failure.
		}
		return nil
	default:
		return fmt.Errorf("unknown service action %q (want install|uninstall|status)", action)
	}
}

// systemdUnit renders the user service definition.
func systemdUnit(exe string, layout config.Layout, port int) string {
	return fmt.Sprintf(`[Unit]
Description=grove daemon — tree-of-agents manager

[Service]
ExecStart=%s serve --port %d --home %s
Restart=on-failure
RestartSec=2

[Install]
WantedBy=default.target
`, exe, port, layout.Home)
}

func systemdService(action, exe string, layout config.Layout, port int) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve user home: %w", err)
	}
	unitPath := filepath.Join(homeDir, ".config", "systemd", "user", "grove.service")

	switch action {
	case "install":
		if err := os.MkdirAll(filepath.Dir(unitPath), 0o750); err != nil {
			return fmt.Errorf("create systemd user dir: %w", err)
		}
		if err := os.WriteFile(unitPath, []byte(systemdUnit(exe, layout, port)), 0o600); err != nil {
			return fmt.Errorf("write %s: %w", unitPath, err)
		}
		if err := run("systemctl", "--user", "daemon-reload"); err != nil {
			return fmt.Errorf("systemctl daemon-reload: %w", err)
		}
		if err := run("systemctl", "--user", "enable", "--now", "grove.service"); err != nil {
			return fmt.Errorf("systemctl enable --now: %w", err)
		}
		fmt.Printf("Installed and started grove.service (port %d).\nThe daemon now starts at login; open the UI with `grove open`.\n", port)
		return nil
	case "uninstall":
		_ = run("systemctl", "--user", "disable", "--now", "grove.service")
		if err := os.Remove(unitPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove %s: %w", unitPath, err)
		}
		_ = run("systemctl", "--user", "daemon-reload")
		fmt.Println("Service removed.")
		return nil
	case "status":
		return run("systemctl", "--user", "status", "--no-pager", "grove.service")
	default:
		return fmt.Errorf("unknown service action %q (want install|uninstall|status)", action)
	}
}

// run executes a system command, streaming its output to the user.
func run(name string, args ...string) error {
	//nolint:gosec // G204: fixed service-manager binaries with constructed-in-code arguments.
	cmd := exec.CommandContext(context.Background(), name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
