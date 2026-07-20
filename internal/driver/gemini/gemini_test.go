package gemini

import (
	"testing"

	"github.com/AnkushinDaniil/grove/internal/driver"
)

func TestNewIdentity(t *testing.T) {
	d := New()
	if got := d.ID(); got != "gemini" {
		t.Errorf("ID() = %q, want %q", got, "gemini")
	}
	want := driver.Caps{
		Interactive:    true,
		Headless:       true,
		HeadlessStream: false,
		Resume:         true,
		Fork:           false,
		EmitsSessionID: false,
		NativeHooks:    false,
		MCP:            true,
	}
	if got := d.Capabilities(); got != want {
		t.Errorf("Capabilities() = %+v, want %+v", got, want)
	}
}

func TestRegistryIntegration(t *testing.T) {
	reg, err := driver.NewRegistry(New())
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	got, ok := reg.Get("gemini")
	if !ok {
		t.Fatal(`Get("gemini") ok = false, want true`)
	}
	if got.ID() != "gemini" {
		t.Errorf("Get(\"gemini\").ID() = %q, want %q", got.ID(), "gemini")
	}
	if ids := reg.IDs(); len(ids) != 1 || ids[0] != "gemini" {
		t.Errorf("IDs() = %v, want [gemini]", ids)
	}
}
