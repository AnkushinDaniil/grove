package store

import (
	"testing"

	"github.com/AnkushinDaniil/grove/internal/core"
)

func testProfile(id core.ProfileID, driver, name string) core.Profile {
	return core.Profile{
		ID:        id,
		Driver:    driver,
		Name:      name,
		ConfigDir: "/home/user/.grove/profiles/" + string(id),
		IsDefault: false,
		CreatedAt: msTime(1_700_000_000_000),
	}
}

func TestProfileCRUD(t *testing.T) {
	s := newTestStore(t)
	p := testProfile(core.NewProfileID(), "claude", "personal")
	if err := s.SaveProfile(t.Context(), p); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}

	profiles, err := s.ListProfiles(t.Context())
	if err != nil {
		t.Fatalf("ListProfiles: %v", err)
	}
	if len(profiles) != 1 || profiles[0] != p {
		t.Fatalf("ListProfiles = %+v, want [%+v]", profiles, p)
	}

	p.Name = "personal-renamed"
	p.IsDefault = true
	if err := s.SaveProfile(t.Context(), p); err != nil {
		t.Fatalf("SaveProfile (update): %v", err)
	}
	profiles, err = s.ListProfiles(t.Context())
	if err != nil {
		t.Fatalf("ListProfiles after update: %v", err)
	}
	if len(profiles) != 1 || profiles[0] != p {
		t.Fatalf("ListProfiles after update = %+v, want [%+v]", profiles, p)
	}

	if err := s.DeleteProfile(t.Context(), p.ID); err != nil {
		t.Fatalf("DeleteProfile: %v", err)
	}
	profiles, err = s.ListProfiles(t.Context())
	if err != nil {
		t.Fatalf("ListProfiles after delete: %v", err)
	}
	if len(profiles) != 0 {
		t.Errorf("ListProfiles after delete = %+v, want none", profiles)
	}
}

func TestDeleteProfileMissingIsNotError(t *testing.T) {
	s := newTestStore(t)
	if err := s.DeleteProfile(t.Context(), core.NewProfileID()); err != nil {
		t.Errorf("DeleteProfile(missing): %v", err)
	}
}

func TestProfileUniqueDriverName(t *testing.T) {
	s := newTestStore(t)
	p1 := testProfile(core.NewProfileID(), "claude", "personal")
	if err := s.SaveProfile(t.Context(), p1); err != nil {
		t.Fatalf("SaveProfile p1: %v", err)
	}

	p2 := testProfile(core.NewProfileID(), "claude", "personal") // same (driver, name), different id
	assertUniqueViolation(t, s.SaveProfile(t.Context(), p2))
}
