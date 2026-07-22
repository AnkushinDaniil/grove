package api

import (
	"net/http"
	"testing"
)

const testPushPublicKey = "test-vapid-public-key"

func TestPushKey(t *testing.T) {
	h := newHarness(t, nil)

	var got pushKeyResponse
	h.decode(h.do(http.MethodGet, "/api/v1/push/key", nil), http.StatusOK, &got)
	if got.PublicKey != testPushPublicKey {
		t.Errorf("public_key = %q, want %q", got.PublicKey, testPushPublicKey)
	}
}

func TestPushSubscribeAndUnsubscribe(t *testing.T) {
	h := newHarness(t, nil)
	endpoint := "https://push.example.com/sub/abc123"

	h.decode(h.do(http.MethodPost, "/api/v1/push/subscribe", map[string]any{
		"endpoint": endpoint,
		"keys":     map[string]string{"p256dh": "p256dh-value", "auth": "auth-value"},
	}), http.StatusNoContent, nil)

	subs, err := h.store.ListSubscriptions(t.Context())
	if err != nil {
		t.Fatalf("ListSubscriptions: %v", err)
	}
	if len(subs) != 1 || subs[0].Endpoint != endpoint {
		t.Fatalf("subscriptions = %+v, want one for %s", subs, endpoint)
	}

	h.decode(h.do(http.MethodPost, "/api/v1/push/unsubscribe", map[string]string{
		"endpoint": endpoint,
	}), http.StatusNoContent, nil)

	subs, err = h.store.ListSubscriptions(t.Context())
	if err != nil {
		t.Fatalf("ListSubscriptions after unsubscribe: %v", err)
	}
	if len(subs) != 0 {
		t.Fatalf("subscriptions after unsubscribe = %+v, want none", subs)
	}
}

func TestPushSubscribeRejectsNonHTTPSEndpoint(t *testing.T) {
	h := newHarness(t, nil)

	h.decode(h.do(http.MethodPost, "/api/v1/push/subscribe", map[string]any{
		"endpoint": "http://push.example.com/sub/abc123",
		"keys":     map[string]string{"p256dh": "a", "auth": "b"},
	}), http.StatusBadRequest, nil)

	h.decode(h.do(http.MethodPost, "/api/v1/push/subscribe", map[string]any{
		"endpoint": "not-a-url",
		"keys":     map[string]string{"p256dh": "a", "auth": "b"},
	}), http.StatusBadRequest, nil)
}

func TestPushSubscribeRejectsMissingKeys(t *testing.T) {
	h := newHarness(t, nil)

	h.decode(h.do(http.MethodPost, "/api/v1/push/subscribe", map[string]any{
		"endpoint": "https://push.example.com/sub/abc123",
		"keys":     map[string]string{"p256dh": "", "auth": "b"},
	}), http.StatusBadRequest, nil)
}

func TestPushUnsubscribeUnknownEndpointIsIdempotent(t *testing.T) {
	h := newHarness(t, nil)
	h.decode(h.do(http.MethodPost, "/api/v1/push/unsubscribe", map[string]string{
		"endpoint": "https://push.example.com/does-not-exist",
	}), http.StatusNoContent, nil)
}

func TestPushSubscribeUpsertsExistingEndpoint(t *testing.T) {
	h := newHarness(t, nil)
	endpoint := "https://push.example.com/sub/abc123"

	h.decode(h.do(http.MethodPost, "/api/v1/push/subscribe", map[string]any{
		"endpoint": endpoint, "keys": map[string]string{"p256dh": "old", "auth": "old"},
	}), http.StatusNoContent, nil)
	h.decode(h.do(http.MethodPost, "/api/v1/push/subscribe", map[string]any{
		"endpoint": endpoint, "keys": map[string]string{"p256dh": "new", "auth": "new"},
	}), http.StatusNoContent, nil)

	subs, err := h.store.ListSubscriptions(t.Context())
	if err != nil {
		t.Fatalf("ListSubscriptions: %v", err)
	}
	if len(subs) != 1 || subs[0].P256dh != "new" {
		t.Fatalf("subscriptions = %+v, want one upserted with p256dh=new", subs)
	}
}
