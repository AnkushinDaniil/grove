package api

import (
	"net/http"
	"net/url"
	"time"

	"github.com/AnkushinDaniil/grove/internal/store"
)

// pushKeyResponse is the GET /push/key body (docs/API.md "Web push").
type pushKeyResponse struct {
	PublicKey string `json:"public_key"`
}

// handlePushKey returns the daemon's VAPID public key, the
// applicationServerKey a browser's PushManager.subscribe() needs.
func (h *Handlers) handlePushKey(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, h.logger, http.StatusOK, pushKeyResponse{PublicKey: h.pushPublicKey})
}

// pushSubscribeRequest is the POST /push/subscribe body: the wire shape of a
// browser's PushSubscription object.
type pushSubscribeRequest struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

// handlePushSubscribe registers (or refreshes) a browser's push subscription.
func (h *Handlers) handlePushSubscribe(w http.ResponseWriter, r *http.Request) {
	var req pushSubscribeRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "invalid request body")
		return
	}
	if !validPushEndpoint(req.Endpoint) {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "endpoint must be an absolute https URL")
		return
	}
	if req.Keys.P256dh == "" || req.Keys.Auth == "" {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "keys.p256dh and keys.auth are required")
		return
	}

	sub := store.PushSubscription{
		Endpoint:  req.Endpoint,
		P256dh:    req.Keys.P256dh,
		Auth:      req.Keys.Auth,
		CreatedAt: time.Now(),
	}
	if err := h.store.SaveSubscription(r.Context(), sub); err != nil {
		writeError(w, h.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// pushUnsubscribeRequest is the POST /push/unsubscribe body.
type pushUnsubscribeRequest struct {
	Endpoint string `json:"endpoint"`
}

// handlePushUnsubscribe removes a browser's push subscription. An unknown
// endpoint is not an error (idempotent, matching the store's delete and the
// browser's own unregister-then-notify-server flow).
func (h *Handlers) handlePushUnsubscribe(w http.ResponseWriter, r *http.Request) {
	var req pushUnsubscribeRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.store.DeleteSubscription(r.Context(), req.Endpoint); err != nil {
		writeError(w, h.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// validPushEndpoint reports whether endpoint is an absolute https URL.
func validPushEndpoint(endpoint string) bool {
	u, err := url.Parse(endpoint)
	return err == nil && u.Scheme == "https" && u.Host != ""
}
