package memory

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectNotInstalled(t *testing.T) {
	fb := newFakeBin(t, fakeOpts{})
	st, err := fb.env().Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if st.Installed {
		t.Errorf("Installed = true, want false")
	}
	if st.PalaceExists {
		t.Errorf("PalaceExists = true, want false")
	}
	if filepath.Base(st.PalacePath) != palaceDirName {
		t.Errorf("PalacePath = %q, want basename %q", st.PalacePath, palaceDirName)
	}
}

func TestDetectInstalled(t *testing.T) {
	fb := newFakeBin(t, fakeOpts{installed: true, version: "3.6.0"})
	st, err := fb.env().Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if !st.Installed {
		t.Fatalf("Installed = false, want true")
	}
	if st.Version != "3.6.0" {
		t.Errorf("Version = %q, want 3.6.0", st.Version)
	}
	if st.Path == "" {
		t.Errorf("Path is empty, want resolved binary path")
	}
}

func TestDetectPalaceExists(t *testing.T) {
	fb := newFakeBin(t, fakeOpts{})
	writePalace(t, fb)
	st, err := fb.env().Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if !st.PalaceExists {
		t.Errorf("PalaceExists = false, want true")
	}
}

func TestInitPalaceCreates(t *testing.T) {
	fb := newFakeBin(t, fakeOpts{installed: true})
	if err := fb.env().InitPalace(context.Background()); err != nil {
		t.Fatalf("InitPalace: %v", err)
	}
	st, err := fb.env().Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if !st.PalaceExists {
		t.Errorf("palace not created by InitPalace")
	}
}

func TestInitPalaceIdempotent(t *testing.T) {
	fb := newFakeBin(t, fakeOpts{installed: true})
	// Pre-existing palace with sentinel content must be left untouched.
	dir := filepath.Join(fb.home, palaceDirName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dir, palaceMarker)
	if err := os.WriteFile(marker, []byte("SENTINEL"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := fb.env().InitPalace(context.Background()); err != nil {
		t.Fatalf("InitPalace: %v", err)
	}
	got, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "SENTINEL" {
		t.Errorf("marker = %q, want untouched SENTINEL", got)
	}
}

func TestInitPalaceNotInstalled(t *testing.T) {
	fb := newFakeBin(t, fakeOpts{})
	if err := fb.env().InitPalace(context.Background()); err == nil {
		t.Fatal("InitPalace succeeded without a binary, want error")
	}
}

func TestParseVersion(t *testing.T) {
	cases := map[string]string{
		"3.6.0":                    "3.6.0",
		"mempalace 3.6.0":          "3.6.0",
		"mempalace, version 3.6.0": "3.6.0",
		"v3.6.0":                   "3.6.0",
		"mempalace 3.6.0\n":        "3.6.0",
	}
	for in, want := range cases {
		if got := parseVersion([]byte(in)); got != want {
			t.Errorf("parseVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestPackageLevelSmoke exercises the package-level Detect/Doctor wrappers
// against the real environment. Both are read-only, so this never mutates state.
func TestPackageLevelSmoke(t *testing.T) {
	ctx := context.Background()
	st, err := Detect(ctx)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if !strings.Contains(st.PalacePath, palaceDirName) {
		t.Errorf("PalacePath = %q, want it to contain %q", st.PalacePath, palaceDirName)
	}
	// Doctor is diagnostic-only; discard its report and its healthy/unhealthy bool.
	_ = Doctor(ctx, io.Discard)
}
