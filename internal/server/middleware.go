package server

import (
	"encoding/json"
	"log/slog"
	"net"
	"net/http"

	"github.com/AnkushinDaniil/grove/internal/api"
)

// allowedHosts are the only Host header hostnames the daemon serves, a
// DNS-rebinding defense (the daemon binds 127.0.0.1 only). Ports are ignored.
var allowedHosts = map[string]bool{"127.0.0.1": true, "localhost": true}

// hostGuard rejects requests whose Host header is not a loopback name, closing
// the DNS-rebinding hole before any handler runs.
func (s *Server) hostGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !hostAllowed(r.Host) {
			writeJSONError(w, s.logger, http.StatusForbidden, "forbidden host")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// hostAllowed reports whether host (with or without a port) is a loopback name.
func hostAllowed(host string) bool {
	name := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		name = h
	}
	return allowedHosts[name]
}

// guard authenticates API and WebSocket requests: the login and hook endpoints
// pass through (they authenticate themselves), everything else requires a valid
// session cookie or bearer token, and mutating requests additionally require the
// CSRF header.
func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case api.PathAuthSession, api.PathAuthMe, api.PathHook:
			next.ServeHTTP(w, r)
			return
		}
		if !s.auth.Authorized(r) {
			writeJSONError(w, s.logger, http.StatusUnauthorized, "unauthenticated")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if r.Header.Get(api.CSRFHeader) != api.CSRFValue {
				writeJSONError(w, s.logger, http.StatusForbidden, "missing csrf header")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// writeJSONError writes a {"error": msg} envelope with the given status, matching
// the REST error shape so clients parse server-level failures the same way.
func writeJSONError(w http.ResponseWriter, logger *slog.Logger, status int, msg string) {
	buf, err := json.Marshal(map[string]string{"error": msg})
	if err != nil {
		logger.Error("marshal error", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(buf)
}
