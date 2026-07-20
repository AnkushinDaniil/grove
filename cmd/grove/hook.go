package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const (
	hookBodyLimit = 1 << 20 // hook payloads are small; 1 MiB is generous
	hookTimeout   = 5 * time.Second
)

// runHook is invoked BY the CLI agents' hook configs (Claude --settings,
// Codex notify). It forwards the JSON payload from stdin to the daemon and
// never fails the calling agent: delivery problems surface in the daemon's
// logs, not as hook errors inside the user's session.
func runHook(args []string) error {
	fs := flag.NewFlagSet("hook", flag.ContinueOnError)
	var (
		driver = fs.String("driver", "", "driver id that produced the payload")
		node   = fs.String("node", "", "grove node id")
		token  = fs.String("token", "", "per-node hook token")
		daemon = fs.String("daemon", "http://127.0.0.1:7433", "daemon base URL")
		event  = fs.String("event", "", "hook event name override (Codex notify)")
	)
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse hook flags: %w", err)
	}
	if *node == "" || *token == "" {
		return fmt.Errorf("hook requires --node and --token")
	}

	payload, err := io.ReadAll(io.LimitReader(os.Stdin, hookBodyLimit))
	if err != nil {
		return fmt.Errorf("read hook payload: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), hookTimeout)
	defer cancel()
	url := fmt.Sprintf("%s/api/v1/internal/hook?node=%s&driver=%s&event=%s", *daemon, *node, *driver, *event)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build hook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Grove-Hook-Token", *token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// Deliberately swallow delivery errors (daemon restarting, etc.) so a
		// hook hiccup never breaks the user's agent session.
		fmt.Fprintf(os.Stderr, "grove hook: delivery failed: %v\n", err)
		return nil
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, hookBodyLimit))
	return nil
}
