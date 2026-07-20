package api

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// CSRFHeader is the header the UI sends on every mutating request; the server
// requires it (value "1") for non-GET requests as a same-origin CSRF guard.
const (
	CSRFHeader = "X-Grove-CSRF"
	CSRFValue  = "1"

	// authCookie is the HttpOnly session cookie holding the daemon token.
	authCookie = "grove_auth"
)

// Auth validates the daemon token presented as a session cookie or a bearer
// header. The token is compared in constant time.
type Auth struct {
	token string
}

// NewAuth builds an Auth over the daemon token.
func NewAuth(token string) *Auth { return &Auth{token: token} }

// Matches reports whether candidate equals the daemon token (constant time).
func (a *Auth) Matches(candidate string) bool {
	return subtle.ConstantTimeCompare([]byte(candidate), []byte(a.token)) == 1
}

// Authorized reports whether the request carries a valid session cookie or a
// valid Authorization: Bearer <token> header.
func (a *Auth) Authorized(r *http.Request) bool {
	if c, err := r.Cookie(authCookie); err == nil && a.Matches(c.Value) {
		return true
	}
	if h := r.Header.Get("Authorization"); h != "" {
		if token, ok := strings.CutPrefix(h, "Bearer "); ok && a.Matches(token) {
			return true
		}
	}
	return false
}

// setSessionCookie issues the HttpOnly, SameSite=Strict session cookie.
func (a *Auth) setSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     authCookie,
		Value:    a.token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
}

// handleAuthSession exchanges a token (matching the daemon token) for the
// session cookie. POST /api/v1/auth/session {token} → 204 + cookie.
func (h *Handlers) handleAuthSession(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token string `json:"token"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "invalid request body")
		return
	}
	if !h.auth.Matches(body.Token) {
		writeErrorStatus(w, h.logger, http.StatusUnauthorized, "invalid token")
		return
	}
	h.auth.setSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

// handleAuthMe reports whether the caller is authenticated. GET
// /api/v1/auth/me → 204 (authorized) or 401.
func (h *Handlers) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	if !h.auth.Authorized(r) {
		writeErrorStatus(w, h.logger, http.StatusUnauthorized, "unauthenticated")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
