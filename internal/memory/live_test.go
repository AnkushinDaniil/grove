package memory

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AnkushinDaniil/grove/internal/core"
)

// TestLiveRoundTrip exercises the real MemPalace MCP server end to end through
// the grove client: capture a uniquely-tagged drawer, read it back through the
// node-memory endpoint path, then delete it. It is gated on GROVE_LIVE because
// it writes into the user's installed palace; it namespaces everything under a
// per-run grove: wing/source and cleans up via delete_by_source.
//
// Timeouts are generous: on a large or index-degraded palace a single call can
// take a minute or more. Run with:
//
//	GROVE_LIVE=1 go test ./internal/memory/ -run TestLiveRoundTrip -timeout 900s -v
func TestLiveRoundTrip(t *testing.T) {
	if os.Getenv("GROVE_LIVE") == "" {
		t.Skip("set GROVE_LIVE=1 to run against the installed MemPalace")
	}

	stamp := strconv.FormatInt(time.Now().UnixNano(), 36)
	projID := core.NodeID("grove-liveproj-" + stamp)
	taskID := core.NodeID("grove-livetask-" + stamp)
	sentinel := "grove live sentinel " + stamp

	tr := newFakeTree()
	tr.add("ws", "", core.KindWorkspace, "Workspace")
	tr.add(projID, "ws", core.KindProject, "live project")
	// Seed the task title with the sentinel so the endpoint's title-seeded search
	// matches the drawer we write.
	tr.add(taskID, projID, core.KindTask, sentinel)

	c := NewClient(Options{
		Tree:          tr,
		SpoolPath:     filepath.Join(t.TempDir(), "spool", "memory.jsonl"),
		CallTimeout:   4 * time.Minute,
		RecallTimeout: 4 * time.Minute,
	})
	if !c.available() {
		t.Skip("mempalace-mcp not on PATH; run `grove memory install`")
	}
	ctx := context.Background()

	// Always attempt cleanup, even if an assertion fails midway.
	source := sourceRef(taskID)
	t.Cleanup(func() {
		cctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
		defer cancel()
		if _, err := c.callTool(cctx, "mempalace_delete_by_source", map[string]any{
			"source_file": source,
			"dry_run":     false,
		}); err != nil {
			t.Logf("cleanup delete_by_source(%s) failed (may need manual `mempalace` cleanup): %v", source, err)
		} else {
			t.Logf("cleanup ok: deleted drawers for %s", source)
		}
	})

	// 1) Capture a sentinel decision for the task node.
	t0 := time.Now()
	content := sentinel + ": recall injection maps project→wing, node→room."
	c.Capture(ctx, taskID, KindDecision, content, SourceAuto)
	t.Logf("capture took %s", time.Since(t0).Round(time.Millisecond))

	// 2) Read it back through the same path the REST endpoint uses.
	t1 := time.Now()
	res := c.NodeMemory(ctx, taskID, ScopeSelf)
	t.Logf("NodeMemory(self) took %s; healthy=%v backend=%q entries=%d",
		time.Since(t1).Round(time.Millisecond), res.Healthy, res.Backend, len(res.Entries))

	if !res.Healthy || res.Backend != backendName {
		t.Fatalf("expected healthy %q backend, got healthy=%v backend=%q", backendName, res.Healthy, res.Backend)
	}
	var found *Entry
	for i := range res.Entries {
		if strings.Contains(res.Entries[i].Content, stamp) {
			found = &res.Entries[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("captured sentinel %q not found in %d entries", stamp, len(res.Entries))
	}
	if found.Kind != KindDecision || found.Source != SourceAuto {
		t.Errorf("round-tripped entry = kind %q source %q, want decision/auto (header parse)", found.Kind, found.Source)
	}
	if strings.Contains(found.Content, headerPrefix) {
		t.Errorf("entry content still carries grove header: %q", found.Content)
	}
	t.Logf("round-trip OK: id=%s kind=%s source=%s created_at=%s", found.ID, found.Kind, found.Source, found.CreatedAt)
}
