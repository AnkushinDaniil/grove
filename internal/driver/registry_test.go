package driver

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/AnkushinDaniil/grove/internal/core"
)

// fakeDriver is a minimal Driver stub for exercising Registry in isolation
// from any real driver implementation.
type fakeDriver struct{ id string }

func (f fakeDriver) ID() string { return f.id }

func (fakeDriver) Capabilities() Caps { return Caps{} }

func (fakeDriver) NewCommand(LaunchSpec) (ExecSpec, error) { return ExecSpec{}, nil }

func (fakeDriver) NewParser() Parser { return nil }

func (fakeDriver) FormatPrompt(string) ([]byte, error) { return nil, ErrUnsupported }

func (fakeDriver) RecoverSessionID(context.Context, SessionInfo) (string, error) {
	return "", ErrUnsupported
}

func TestNewRegistryDuplicateID(t *testing.T) {
	_, err := NewRegistry(fakeDriver{id: "x"}, fakeDriver{id: "x"})
	if !errors.Is(err, core.ErrInvalid) {
		t.Fatalf("NewRegistry() error = %v, want ErrInvalid", err)
	}
}

func TestNewRegistryEmptyID(t *testing.T) {
	_, err := NewRegistry(fakeDriver{id: ""})
	if !errors.Is(err, core.ErrInvalid) {
		t.Fatalf("NewRegistry() error = %v, want ErrInvalid", err)
	}
}

func TestNewRegistryEmpty(t *testing.T) {
	reg, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	if ids := reg.IDs(); len(ids) != 0 {
		t.Errorf("IDs() = %v, want empty", ids)
	}
	if _, ok := reg.Get("anything"); ok {
		t.Error("Get(anything) ok = true, want false")
	}
}

func TestRegistryGetAndIDs(t *testing.T) {
	reg, err := NewRegistry(fakeDriver{id: "beta"}, fakeDriver{id: "alpha"})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	if ids := reg.IDs(); !slices.Equal(ids, []string{"alpha", "beta"}) {
		t.Errorf("IDs() = %v, want [alpha beta]", ids)
	}
	got, ok := reg.Get("alpha")
	if !ok || got.ID() != "alpha" {
		t.Errorf("Get(alpha) = %v, %v, want id alpha, true", got, ok)
	}
	if _, ok := reg.Get("missing"); ok {
		t.Error("Get(missing) ok = true, want false")
	}
}
