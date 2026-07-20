// Command grove is the single binary: daemon, browser opener, and the hook
// helper the CLI agents call back into.
package main

import (
	"fmt"
	"os"
)

// Set via -ldflags at release time.
var (
	version = "dev"
	commit  = "none"
)

type command struct {
	name    string
	summary string
	run     func(args []string) error
}

func main() {
	commands := []command{
		{"serve", "run the grove daemon", runServe},
		{"open", "open the web UI in a browser", runOpen},
		{"hook", "internal: forward a CLI hook payload to the daemon", runHook},
		{"version", "print version information", runVersion},
	}
	if len(os.Args) < 2 {
		usage(commands)
		os.Exit(2)
	}
	name := os.Args[1]
	for _, c := range commands {
		if c.name == name {
			if err := c.run(os.Args[2:]); err != nil {
				fmt.Fprintf(os.Stderr, "grove %s: %v\n", name, err)
				os.Exit(1)
			}
			return
		}
	}
	fmt.Fprintf(os.Stderr, "grove: unknown command %q\n\n", name)
	usage(commands)
	os.Exit(2)
}

func usage(commands []command) {
	fmt.Fprintln(os.Stderr, "Usage: grove <command> [flags]")
	fmt.Fprintln(os.Stderr, "")
	for _, c := range commands {
		fmt.Fprintf(os.Stderr, "  %-10s %s\n", c.name, c.summary)
	}
}

func runVersion([]string) error {
	fmt.Printf("grove %s (%s)\n", version, commit)
	return nil
}
