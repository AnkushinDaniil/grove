package claude

import (
	"context"
	"errors"
	"testing"

	"github.com/AnkushinDaniil/grove/internal/driver"
)

func TestNewIdentity(t *testing.T) {
	d := New()
	if got := d.ID(); got != "claude" {
		t.Errorf("ID() = %q, want %q", got, "claude")
	}
	want := driver.Caps{
		Interactive:    true,
		Headless:       true,
		HeadlessStream: true,
		Resume:         true,
		Fork:           true,
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
	got, ok := reg.Get("claude")
	if !ok {
		t.Fatal(`Get("claude") ok = false, want true`)
	}
	if got.ID() != "claude" {
		t.Errorf("Get(\"claude\").ID() = %q, want %q", got.ID(), "claude")
	}
	if ids := reg.IDs(); len(ids) != 1 || ids[0] != "claude" {
		t.Errorf("IDs() = %v, want [claude]", ids)
	}
}
