package api

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/profile"
)

// doctorTimeout bounds the profile doctor probe so a hung CLI --version cannot
// stall the request indefinitely.
const doctorTimeout = 5 * time.Second

// profileDTO is the wire representation of a core.Profile (docs/API.md "Profiles").
type profileDTO struct {
	ID        string `json:"id"`
	Driver    string `json:"driver"`
	Name      string `json:"name"`
	ConfigDir string `json:"config_dir"`
	IsDefault bool   `json:"is_default"`
	CreatedAt string `json:"created_at"`
}

// profileToDTO maps a core.Profile to its wire shape.
func profileToDTO(p core.Profile) profileDTO {
	return profileDTO{
		ID:        string(p.ID),
		Driver:    p.Driver,
		Name:      p.Name,
		ConfigDir: p.ConfigDir,
		IsDefault: p.IsDefault,
		CreatedAt: rfc3339(p.CreatedAt),
	}
}

// profilesResponse is the GET /profiles body.
type profilesResponse struct {
	Profiles []profileDTO `json:"profiles"`
}

// handleListProfiles returns every registered profile. It seeds the default
// claude profile on first read (lazy EnsureDefault) so a fresh install always
// reports the account it is already using without a startup hook in serve.go.
func (h *Handlers) handleListProfiles(w http.ResponseWriter, r *http.Request) {
	if err := h.ensureDefaultProfile(r.Context()); err != nil {
		writeError(w, h.logger, err)
		return
	}
	profiles, err := h.store.ListProfiles(r.Context())
	if err != nil {
		writeError(w, h.logger, err)
		return
	}
	out := make([]profileDTO, 0, len(profiles))
	for _, p := range profiles {
		out = append(out, profileToDTO(p))
	}
	writeJSON(w, h.logger, http.StatusOK, profilesResponse{Profiles: out})
}

// ensureDefaultProfile seeds the default profile against the daemon user's home.
func (h *Handlers) ensureDefaultProfile(ctx context.Context) error {
	home, err := h.home()
	if err != nil {
		return fmt.Errorf("resolve home for default profile: %w", err)
	}
	if _, err := profile.EnsureDefault(ctx, h.store, home); err != nil {
		return err
	}
	return nil
}

// createProfileRequest is the POST /profiles body.
type createProfileRequest struct {
	Driver    string `json:"driver"`
	Name      string `json:"name"`
	ConfigDir string `json:"config_dir"`
}

// handleCreateProfile registers a provider account. config_dir defaults to
// ~/.grove/profiles/<driver>/<name> and is provisioned (0700); a duplicate
// (driver, name) is rejected with 409.
func (h *Handlers) handleCreateProfile(w http.ResponseWriter, r *http.Request) {
	var req createProfileRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "invalid request body")
		return
	}
	driverID := strings.TrimSpace(req.Driver)
	if !profile.IsSupportedDriver(driverID) {
		writeErrorStatus(w, h.logger, http.StatusBadRequest,
			"driver must be one of claude, codex, gemini, opencode")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "name must not be empty")
		return
	}
	configDir := strings.TrimSpace(req.ConfigDir)
	if configDir != "" && !filepath.IsAbs(configDir) {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "config_dir must be an absolute path")
		return
	}

	p, err := profile.Create(driverID, name, configDir, h.profilesDir, time.Now())
	if err != nil {
		writeError(w, h.logger, err)
		return
	}
	if err := h.store.SaveProfile(r.Context(), p); err != nil {
		if isUniqueViolation(err) {
			writeErrorStatus(w, h.logger, http.StatusConflict,
				fmt.Sprintf("a %s profile named %q already exists", driverID, name))
			return
		}
		writeError(w, h.logger, err)
		return
	}
	writeJSON(w, h.logger, http.StatusCreated, profileToDTO(p))
}

// handleDeleteProfile removes a profile by id. Deletion is idempotent: an
// unknown id still returns 204 (the default profile is re-seeded on the next
// GET /profiles, so this never leaves the daemon without one).
func (h *Handlers) handleDeleteProfile(w http.ResponseWriter, r *http.Request) {
	id := core.ProfileID(r.PathValue("id"))
	if id == "" {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "profile id is required")
		return
	}
	if err := h.store.DeleteProfile(r.Context(), id); err != nil {
		writeError(w, h.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// doctorCheckDTO is one probe result in the GET /profiles/{id}/doctor body.
type doctorCheckDTO struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

// doctorResponse is the GET /profiles/{id}/doctor body.
type doctorResponse struct {
	Checks []doctorCheckDTO `json:"checks"`
}

// handleProfileDoctor runs the profile health probes and returns their results.
func (h *Handlers) handleProfileDoctor(w http.ResponseWriter, r *http.Request) {
	id := core.ProfileID(r.PathValue("id"))
	p, ok, err := h.findProfile(r.Context(), id)
	if err != nil {
		writeError(w, h.logger, err)
		return
	}
	if !ok {
		writeErrorStatus(w, h.logger, http.StatusNotFound, "profile not found")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), doctorTimeout)
	defer cancel()
	checks := profile.Doctor(ctx, p, nil)
	out := make([]doctorCheckDTO, 0, len(checks))
	for _, c := range checks {
		out = append(out, doctorCheckDTO{Name: c.Name, OK: c.OK, Detail: c.Detail})
	}
	writeJSON(w, h.logger, http.StatusOK, doctorResponse{Checks: out})
}

// findProfile looks a profile up by id via the store's list. Profiles are few,
// so a scan avoids adding a keyed query the rest of the store does not need.
func (h *Handlers) findProfile(ctx context.Context, id core.ProfileID) (core.Profile, bool, error) {
	profiles, err := h.store.ListProfiles(ctx)
	if err != nil {
		return core.Profile{}, false, err
	}
	for _, p := range profiles {
		if p.ID == id {
			return p, true, nil
		}
	}
	return core.Profile{}, false, nil
}
