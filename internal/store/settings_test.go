package store

import "testing"

func TestSettingGetMissingReturnsFalse(t *testing.T) {
	s := newTestStore(t)
	value, ok, err := s.GetSetting(t.Context(), "unset-key")
	if err != nil {
		t.Fatalf("GetSetting: %v", err)
	}
	if ok {
		t.Errorf("GetSetting(unset) ok = true, want false")
	}
	if value != "" {
		t.Errorf("GetSetting(unset) value = %q, want empty", value)
	}
}

func TestSettingSetAndGet(t *testing.T) {
	s := newTestStore(t)
	if err := s.SetSetting(t.Context(), "theme", "dark"); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}

	value, ok, err := s.GetSetting(t.Context(), "theme")
	if err != nil {
		t.Fatalf("GetSetting: %v", err)
	}
	if !ok {
		t.Fatal("GetSetting ok = false, want true")
	}
	if value != "dark" {
		t.Errorf("GetSetting value = %q, want %q", value, "dark")
	}
}

func TestSettingSetOverwrites(t *testing.T) {
	s := newTestStore(t)
	if err := s.SetSetting(t.Context(), "theme", "dark"); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	if err := s.SetSetting(t.Context(), "theme", "light"); err != nil {
		t.Fatalf("SetSetting (overwrite): %v", err)
	}

	value, ok, err := s.GetSetting(t.Context(), "theme")
	if err != nil {
		t.Fatalf("GetSetting: %v", err)
	}
	if !ok || value != "light" {
		t.Errorf("GetSetting = (%q, %v), want (%q, true)", value, ok, "light")
	}
}
