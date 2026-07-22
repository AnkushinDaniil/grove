package api

import "net/http"

// versionResponse is the GET /version body.
type versionResponse struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
}

// handleVersion returns the daemon build identifiers.
func (h *Handlers) handleVersion(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, h.logger, http.StatusOK, versionResponse{Version: h.version, Commit: h.commit})
}
