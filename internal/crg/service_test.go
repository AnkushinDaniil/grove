package crg

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestServiceStatusOffWhenUnavailable(t *testing.T) {
	svc := NewService(&Runner{bin: "", graphDir: t.TempDir()}, discardLogger())
	if got := svc.Status("/repo"); got != StatusOff {
		t.Errorf("Status = %q, want off", got)
	}
	// Nil runner is tolerated too.
	if got := NewService(nil, discardLogger()).Status("/repo"); got != StatusOff {
		t.Errorf("nil-runner Status = %q, want off", got)
	}
}

func TestServiceStatusReadyWhenGraphExists(t *testing.T) {
	r := fakeRunner(t, nil)
	dir := r.dataDir("/repo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "graph.db"), []byte("db"), 0o600); err != nil {
		t.Fatal(err)
	}
	svc := NewService(r, discardLogger())
	if got := svc.Status("/repo"); got != StatusReady {
		t.Errorf("Status = %q, want ready", got)
	}
}

func TestServiceBuildingIsDeduped(t *testing.T) {
	var mu sync.Mutex
	builds := 0
	block := make(chan struct{})
	r := fakeRunner(t, func(_ context.Context, _ string, _ string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "build" {
			mu.Lock()
			builds++
			mu.Unlock()
			<-block // hold the "build" so both Status calls see it in flight
		}
		return []byte("{}"), nil
	})
	svc := NewService(r, discardLogger())

	// No graph yet -> Status starts a build and reports building.
	if got := svc.Status("/repo"); got != StatusBuilding {
		t.Fatalf("first Status = %q, want building", got)
	}
	// A concurrent second call must not start a second build.
	if got := svc.Status("/repo"); got != StatusBuilding {
		t.Fatalf("second Status = %q, want building", got)
	}
	close(block)

	// Let the background build goroutine settle.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := builds
		mu.Unlock()
		if n >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if builds != 1 {
		t.Errorf("builds = %d, want exactly 1 (deduped)", builds)
	}
}
