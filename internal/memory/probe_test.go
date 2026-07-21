package memory

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestProbeOK(t *testing.T) {
	fb := newFakeBin(t, fakeOpts{installed: true, mcpmode: "ok"})
	rep, err := fb.env().Probe(context.Background())
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if !rep.OK {
		t.Fatalf("OK = false, want true (note: %s)", rep.Note)
	}
	if rep.ToolCount != 7 {
		t.Errorf("ToolCount = %d, want 7", rep.ToolCount)
	}
	if rep.ServerName != "mempalace" {
		t.Errorf("ServerName = %q, want mempalace", rep.ServerName)
	}
	if !hasTool(rep.ToolNames, "mempalace_search") {
		t.Errorf("ToolNames = %v, want to include mempalace_search", rep.ToolNames)
	}
	if !strings.Contains(rep.Note, "roundtrip skipped") {
		t.Errorf("Note = %q, want it to mention the skipped roundtrip", rep.Note)
	}
	if rep.RoundTrip {
		t.Errorf("RoundTrip = true, want false (must not write to the live palace)")
	}
}

func TestProbeNoTools(t *testing.T) {
	fb := newFakeBin(t, fakeOpts{installed: true, mcpmode: "notools"})
	rep, err := fb.env().Probe(context.Background())
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if rep.OK {
		t.Errorf("OK = true, want false for zero tools")
	}
	if rep.ToolCount != 0 {
		t.Errorf("ToolCount = %d, want 0", rep.ToolCount)
	}
}

func TestProbeInitError(t *testing.T) {
	fb := newFakeBin(t, fakeOpts{installed: true, mcpmode: "errorinit"})
	_, err := fb.env().Probe(context.Background())
	if err == nil || !strings.Contains(err.Error(), "initialize") {
		t.Fatalf("err = %v, want an initialize error", err)
	}
}

func TestProbeBadJSON(t *testing.T) {
	fb := newFakeBin(t, fakeOpts{installed: true, mcpmode: "badjson"})
	_, err := fb.env().Probe(context.Background())
	if err == nil {
		t.Fatal("Probe succeeded on malformed handshake, want error")
	}
}

func TestProbeTimeout(t *testing.T) {
	fb := newFakeBin(t, fakeOpts{installed: true, mcpmode: "hang"})
	start := time.Now()
	_, err := fb.env().probe(context.Background(), 300*time.Millisecond)
	if err == nil {
		t.Fatal("Probe succeeded against a hanging server, want timeout error")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("probe took %s, want it bounded by the timeout", elapsed)
	}
}

func TestProbeNotInstalled(t *testing.T) {
	fb := newFakeBin(t, fakeOpts{})
	_, err := fb.env().Probe(context.Background())
	if err == nil || !strings.Contains(err.Error(), "cannot probe") {
		t.Fatalf("err = %v, want 'cannot probe'", err)
	}
}
