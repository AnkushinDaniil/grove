package session

import (
	"context"
	"testing"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/driver"
	"github.com/AnkushinDaniil/grove/internal/tree"
)

// mapLookup is an in-memory ProfileLookup for wiring assertions.
type mapLookup map[core.ProfileID]core.Profile

func (m mapLookup) Get(_ context.Context, id core.ProfileID) (core.Profile, bool) {
	p, ok := m[id]
	return p, ok
}

// headlessCaps is the capability set the capture driver advertises for these
// profile-wiring launches (mirrors hookwiring_test's usage).
var headlessCaps = driver.Caps{Headless: true, HeadlessStream: true, EmitsSessionID: true}

func TestStartWiresProfileConfigDir(t *testing.T) {
	drv := newCaptureDriver(t, headlessCaps)
	const configDir = "/tmp/grove/profiles/fake/work"
	pid := core.NewProfileID()
	lookup := mapLookup{pid: {ID: pid, Driver: "fake", Name: "work", ConfigDir: configDir}}

	m, tr, taskID := newFixtureWithDriver(t, Config{Profiles: lookup}, drv)
	if _, err := tr.UpdateNode(t.Context(), taskID, tree.Patch{ProfileID: &pid}); err != nil {
		t.Fatalf("UpdateNode profile: %v", err)
	}

	if _, err := m.Start(t.Context(), taskID, core.ModeHeadless, "go", ""); err != nil {
		t.Fatalf("Start: %v", err)
	}
	spec := drv.spec()
	if spec == nil {
		t.Fatal("driver NewCommand was never called")
	}
	if spec.Profile.ConfigDir != configDir {
		t.Fatalf("spec.Profile.ConfigDir = %q, want the resolved profile's dir %q", spec.Profile.ConfigDir, configDir)
	}
}

func TestStartWithoutLookupLeavesProfileEmpty(t *testing.T) {
	drv := newCaptureDriver(t, headlessCaps)
	pid := core.NewProfileID()

	// A profile is selected on the node, but no lookup is configured, so the
	// launch stays on the CLI default (empty config dir).
	m, tr, taskID := newFixtureWithDriver(t, Config{}, drv)
	if _, err := tr.UpdateNode(t.Context(), taskID, tree.Patch{ProfileID: &pid}); err != nil {
		t.Fatalf("UpdateNode profile: %v", err)
	}

	if _, err := m.Start(t.Context(), taskID, core.ModeHeadless, "go", ""); err != nil {
		t.Fatalf("Start: %v", err)
	}
	spec := drv.spec()
	if spec == nil {
		t.Fatal("driver NewCommand was never called")
	}
	if spec.Profile.ConfigDir != "" {
		t.Fatalf("spec.Profile.ConfigDir = %q, want empty without a lookup", spec.Profile.ConfigDir)
	}
}

func TestStoreProfileLookup(t *testing.T) {
	pid := core.NewProfileID()
	lister := listerFunc(func(context.Context) ([]core.Profile, error) {
		return []core.Profile{{ID: pid, Driver: "claude", ConfigDir: "/cfg"}}, nil
	})
	lookup := NewStoreProfileLookup(lister)

	if p, ok := lookup.Get(t.Context(), pid); !ok || p.ConfigDir != "/cfg" {
		t.Errorf("Get(known) = %+v, %v; want the profile", p, ok)
	}
	if _, ok := lookup.Get(t.Context(), core.NewProfileID()); ok {
		t.Error("Get(unknown) = ok, want not found")
	}
	if _, ok := lookup.Get(t.Context(), ""); ok {
		t.Error("Get(empty) = ok, want not found")
	}
}

// listerFunc adapts a func to the ProfileLister interface.
type listerFunc func(context.Context) ([]core.Profile, error)

func (f listerFunc) ListProfiles(ctx context.Context) ([]core.Profile, error) { return f(ctx) }
