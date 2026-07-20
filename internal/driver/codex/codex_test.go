package codex

import (
	"context"
	"errors"
	"testing"

	"github.com/AnkushinDaniil/grove/internal/driver"
)

func TestNewIdentity(t *testing.T) {
	d := New()
	if got := d.ID(); got != "codex" {
		t.Errorf("ID() = %q, want %q", got, "codex")
	}
	want := driver.Caps{
		Interactive:    true,
		Headless:       true,
		HeadlessStream: false,
		Resume:         true,
		Fork:           false,
		EmitsSessionID: true,
		NativeHooks:    true,
		MCP:            true,
	}
	if got := d.Capabilities(); got != want {
		t.Errorf("Capabilities() = %+v, want %+v", got, want)
	}
}

func TestRecoverSessionIDUnsupported(t *testing.T) {
	d := New()
	id, err := d.RecoverSessionID(context.Background(), driver.SessionInfo{})
	if id != "" {
		t.Errorf("RecoverSessionID() id = %q, want empty", id)
	}
	if !errors.Is(err, driver.ErrUnsupported) {
		t.Errorf("RecoverSessionID() error = %v, want %v", err, driver.ErrUnsupported)
	}
}

func TestRegistryIntegration(t *testing.T) {
	reg, err := driver.NewRegistry(New())
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	got, ok := reg.Get("codex")
	if !ok {
		t.Fatal(`Get("codex") ok = false, want true`)
	}
	if got.ID() != "codex" {
		t.Errorf("Get(\"codex\").ID() = %q, want %q", got.ID(), "codex")
	}
	if ids := reg.IDs(); len(ids) != 1 || ids[0] != "codex" {
		t.Errorf("IDs() = %v, want [codex]", ids)
	}
}
