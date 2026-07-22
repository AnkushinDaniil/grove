package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/AnkushinDaniil/grove/internal/store"
)

// profileHarness is a minimal API stack for the profile endpoints: a temp store
// plus temp home/profiles dirs wired through Config. It stays independent of the
// shared newHarness so its home/profiles seams don't perturb other suites.
type profileHarness struct {
	t           *testing.T
	store       *store.Store
	ts          *httptest.Server
	home        string
	profilesDir string
}

func newProfileHarness(t *testing.T) *profileHarness {
	t.Helper()

	st, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "grove.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	home := t.TempDir()
	profilesDir := filepath.Join(t.TempDir(), "profiles")

	h := New(Config{
		Store:       st,
		Auth:        NewAuth(testToken),
		HookTokens:  NewHookTokens(),
		ProfilesDir: profilesDir,
		Home:        func() (string, error) { return home, nil },
	})
	ts := httptest.NewServer(h.Routes())
	t.Cleanup(ts.Close)

	return &profileHarness{t: t, store: st, ts: ts, home: home, profilesDir: profilesDir}
}

func (h *profileHarness) do(method, path string, body any) response {
	h.t.Helper()
	var r io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			h.t.Fatalf("marshal request: %v", err)
		}
		r = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(h.t.Context(), method, h.ts.URL+path, r)
	if err != nil {
		h.t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := h.ts.Client().Do(req)
	if err != nil {
		h.t.Fatalf("do request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		h.t.Fatalf("read body: %v", err)
	}
	return response{status: resp.StatusCode, body: b}
}

func (h *profileHarness) decode(resp response, wantStatus int, v any) {
	h.t.Helper()
	if resp.status != wantStatus {
		h.t.Fatalf("status = %d, want %d (body: %s)", resp.status, wantStatus, resp.body)
	}
	if v != nil {
		if err := json.Unmarshal(resp.body, v); err != nil {
			h.t.Fatalf("decode body %q: %v", resp.body, err)
		}
	}
}

func (h *profileHarness) listProfiles() profilesResponse {
	h.t.Helper()
	var out profilesResponse
	h.decode(h.do(http.MethodGet, "/api/v1/profiles", nil), http.StatusOK, &out)
	return out
}

// TestListProfilesSeedsDefault verifies GET /profiles auto-creates the default
// claude profile on first read (adopting <home>/.claude, marked is_default).
func TestListProfilesSeedsDefault(t *testing.T) {
	h := newProfileHarness(t)

	profiles := h.listProfiles().Profiles
	if len(profiles) != 1 {
		t.Fatalf("profiles = %+v, want exactly the seeded default", profiles)
	}
	def := profiles[0]
	if !def.IsDefault || def.Driver != "claude" || def.Name != "default" {
		t.Errorf("default = %+v, want is_default claude/default", def)
	}
	if want := filepath.Join(h.home, ".claude"); def.ConfigDir != want {
		t.Errorf("default config_dir = %q, want %q", def.ConfigDir, want)
	}

	// A second read does not seed a duplicate.
	if again := h.listProfiles().Profiles; len(again) != 1 {
		t.Fatalf("second GET /profiles = %+v, want the same single default", again)
	}
}

func TestCreateProfileDefaultsConfigDir(t *testing.T) {
	h := newProfileHarness(t)

	var created profileDTO
	h.decode(h.do(http.MethodPost, "/api/v1/profiles", map[string]any{
		"driver": "claude", "name": "work",
	}), http.StatusCreated, &created)

	want := filepath.Join(h.profilesDir, "claude", "work")
	if created.ConfigDir != want {
		t.Errorf("config_dir = %q, want default %q", created.ConfigDir, want)
	}
	if created.IsDefault {
		t.Error("a created profile should not be the default")
	}
	if info, err := os.Stat(want); err != nil || !info.IsDir() {
		t.Errorf("config dir not provisioned: stat %q -> %v", want, err)
	}

	// It joins the listing alongside the seeded default.
	names := map[string]bool{}
	for _, p := range h.listProfiles().Profiles {
		names[p.Name] = true
	}
	if !names["work"] || !names["default"] {
		t.Errorf("listing names = %v, want both work and default", names)
	}
}

func TestCreateProfileAcceptsExplicitConfigDir(t *testing.T) {
	h := newProfileHarness(t)
	explicit := filepath.Join(t.TempDir(), "my-claude")

	var created profileDTO
	h.decode(h.do(http.MethodPost, "/api/v1/profiles", map[string]any{
		"driver": "claude", "name": "custom", "config_dir": explicit,
	}), http.StatusCreated, &created)
	if created.ConfigDir != explicit {
		t.Errorf("config_dir = %q, want explicit %q", created.ConfigDir, explicit)
	}
}

func TestCreateProfileValidation(t *testing.T) {
	h := newProfileHarness(t)

	// Unknown driver -> 400.
	h.decode(h.do(http.MethodPost, "/api/v1/profiles", map[string]any{
		"driver": "cursor", "name": "x",
	}), http.StatusBadRequest, nil)

	// Empty name -> 400.
	h.decode(h.do(http.MethodPost, "/api/v1/profiles", map[string]any{
		"driver": "claude", "name": "   ",
	}), http.StatusBadRequest, nil)

	// Relative config_dir -> 400.
	h.decode(h.do(http.MethodPost, "/api/v1/profiles", map[string]any{
		"driver": "claude", "name": "x", "config_dir": "relative/dir",
	}), http.StatusBadRequest, nil)
}

func TestCreateProfileRejectsDuplicate(t *testing.T) {
	h := newProfileHarness(t)

	h.decode(h.do(http.MethodPost, "/api/v1/profiles", map[string]any{
		"driver": "claude", "name": "dup",
	}), http.StatusCreated, nil)
	// Same (driver, name) again -> 409.
	h.decode(h.do(http.MethodPost, "/api/v1/profiles", map[string]any{
		"driver": "claude", "name": "dup",
	}), http.StatusConflict, nil)
	// A different driver with the same name is allowed (unique is per driver).
	h.decode(h.do(http.MethodPost, "/api/v1/profiles", map[string]any{
		"driver": "codex", "name": "dup",
	}), http.StatusCreated, nil)
}

func TestDeleteProfile(t *testing.T) {
	h := newProfileHarness(t)

	var created profileDTO
	h.decode(h.do(http.MethodPost, "/api/v1/profiles", map[string]any{
		"driver": "claude", "name": "temp",
	}), http.StatusCreated, &created)

	h.decode(h.do(http.MethodDelete, "/api/v1/profiles/"+created.ID, nil), http.StatusNoContent, nil)
	// Deleting again is idempotent.
	h.decode(h.do(http.MethodDelete, "/api/v1/profiles/"+created.ID, nil), http.StatusNoContent, nil)

	for _, p := range h.listProfiles().Profiles {
		if p.ID == created.ID {
			t.Fatalf("profile %s still listed after delete", created.ID)
		}
	}
}

func TestProfileDoctor(t *testing.T) {
	h := newProfileHarness(t)
	explicit := filepath.Join(t.TempDir(), "claude-cfg")

	var created profileDTO
	h.decode(h.do(http.MethodPost, "/api/v1/profiles", map[string]any{
		"driver": "claude", "name": "doc", "config_dir": explicit,
	}), http.StatusCreated, &created)

	var doc doctorResponse
	h.decode(h.do(http.MethodGet, "/api/v1/profiles/"+created.ID+"/doctor", nil), http.StatusOK, &doc)
	if len(doc.Checks) == 0 {
		t.Fatal("doctor returned no checks")
	}
	// The provisioned config dir resolves cleanly regardless of whether the CLI
	// is installed on the test host.
	var sawResolve bool
	for _, c := range doc.Checks {
		if c.Name == "config dir resolvable" {
			sawResolve = true
			if !c.OK {
				t.Errorf("config dir check not ok: %s", c.Detail)
			}
		}
	}
	if !sawResolve {
		t.Errorf("doctor checks missing the config-dir probe: %+v", doc.Checks)
	}

	// Doctor on an unknown id -> 404.
	h.decode(h.do(http.MethodGet, "/api/v1/profiles/does-not-exist/doctor", nil), http.StatusNotFound, nil)
}
