// Command fakeagent is a scripted stand-in for a real CLI agent, used by
// session integration tests. It replays a JSON script from the file named by
// the FAKEAGENT_SCRIPT environment variable: emitting lines, sleeping, waiting
// for a stdin line, or exiting with a code.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// step is one scripted action. Exactly one field is meaningful per step.
type step struct {
	Emit          string `json:"emit"`
	SleepMS       int    `json:"sleep_ms"`
	WaitStdinLine bool   `json:"wait_stdin_line"`
	ExitCode      *int   `json:"exit_code"`
}

type script struct {
	Steps []step `json:"steps"`
}

func main() { os.Exit(run()) }

// run replays the script and returns the process exit code.
func run() int {
	path := os.Getenv("FAKEAGENT_SCRIPT")
	if path == "" {
		fmt.Fprintln(os.Stderr, "fakeagent: FAKEAGENT_SCRIPT is not set")
		return 1
	}
	//nolint:gosec // G703: script path is provided by the trusted test harness.
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fakeagent: read script:", err)
		return 1
	}
	var sc script
	if err := json.Unmarshal(data, &sc); err != nil {
		fmt.Fprintln(os.Stderr, "fakeagent: parse script:", err)
		return 1
	}

	stdin := bufio.NewReader(os.Stdin)
	out := bufio.NewWriter(os.Stdout)
	for _, s := range sc.Steps {
		switch {
		case s.Emit != "":
			// The write error surfaces on the following Flush.
			_, _ = fmt.Fprintln(out, s.Emit)
			if err := out.Flush(); err != nil {
				return 1
			}
		case s.SleepMS > 0:
			time.Sleep(time.Duration(s.SleepMS) * time.Millisecond)
		case s.WaitStdinLine:
			if err := out.Flush(); err != nil {
				return 1
			}
			if _, err := stdin.ReadString('\n'); err != nil {
				fmt.Fprintln(os.Stderr, "fakeagent: read stdin:", err)
				return 1
			}
		case s.ExitCode != nil:
			if err := out.Flush(); err != nil {
				return 1
			}
			return *s.ExitCode
		}
	}
	if err := out.Flush(); err != nil {
		return 1
	}
	return 0
}
