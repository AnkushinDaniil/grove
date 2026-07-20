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

// handleUsage returns the usage stub. Per docs/API.md the aggregator is not yet
// implemented, so the endpoint always reports an empty profile list.
func (h *Handlers) handleUsage(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, h.logger, http.StatusOK, map[string][]any{"profiles": {}})
}

// handleStats reports the draft stats endpoint as not implemented (501); the
// aggregation lands in M3.
func (h *Handlers) handleStats(w http.ResponseWriter, _ *http.Request) {
	writeErrorStatus(w, h.logger, http.StatusNotImplemented, "stats endpoint not implemented")
}
