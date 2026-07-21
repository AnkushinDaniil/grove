package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/AnkushinDaniil/grove/internal/memory"
)

// runMemory installs and health-checks the MemPalace memory backend
// (docs/ORCHESTRATION.md §8). Subcommands: status (Detect pretty-print), doctor
// (full diagnostic pass), install (Install + InitPalace + Probe).
func runMemory(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: grove memory <install|doctor|status> [flags]")
	}
	action := args[0]
	switch action {
	case "status":
		return memoryStatus(args[1:])
	case "doctor":
		return memoryDoctor(args[1:])
	case "install":
		return memoryInstall(args[1:])
	default:
		return fmt.Errorf("unknown memory action %q (want install|doctor|status)", action)
	}
}

func memoryStatus(args []string) error {
	fs := flag.NewFlagSet("memory status", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse status flags: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	st, err := memory.Detect(ctx)
	if err != nil {
		return err
	}
	fmt.Println("MemPalace status")
	fmt.Printf("  installed: %v\n", st.Installed)
	if st.Installed {
		fmt.Printf("  binary:    %s\n", st.Path)
		fmt.Printf("  version:   %s\n", orDash(st.Version))
		if st.Channel != "" {
			fmt.Printf("  channel:   %s\n", st.Channel)
		}
	}
	fmt.Printf("  palace:    %s (exists: %v)\n", st.PalacePath, st.PalaceExists)
	return nil
}

func memoryDoctor(args []string) error {
	fs := flag.NewFlagSet("memory doctor", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse doctor flags: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if ok := memory.Doctor(ctx, os.Stdout); !ok {
		return fmt.Errorf("memory backend not healthy (run: grove memory install)")
	}
	return nil
}

func memoryInstall(args []string) error {
	fs := flag.NewFlagSet("memory install", flag.ContinueOnError)
	upgrade := fs.Bool("upgrade", false, "reinstall/upgrade even if already present")
	version := fs.String("version", "", "mempalace version to install (default: pinned known-good)")
	channel := fs.String("channel", "", "force install channel: uv|pipx|pip")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse install flags: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()

	if err := memory.Install(ctx, memory.InstallOptions{
		Out:     os.Stdout,
		Upgrade: *upgrade,
		Version: *version,
		Channel: *channel,
	}); err != nil {
		return fmt.Errorf("install mempalace: %w", err)
	}
	if err := memory.InitPalace(ctx); err != nil {
		return fmt.Errorf("initialize palace: %w", err)
	}
	fmt.Println("Verifying MCP server...")
	rep, err := memory.Probe(ctx)
	if err != nil {
		return fmt.Errorf("probe mempalace: %w", err)
	}
	fmt.Printf("✓ MCP server OK: %d tools — %s\n", rep.ToolCount, rep.Note)
	fmt.Println("MemPalace memory backend is ready.")
	return nil
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
