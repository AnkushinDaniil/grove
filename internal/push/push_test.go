package push

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"

	"github.com/AnkushinDaniil/grove/internal/notify"
	"github.com/AnkushinDaniil/grove/internal/store"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "grove.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func testKeys(t *testing.T) Keys {
	t.Helper()
	priv, pub, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		t.Fatalf("GenerateVAPIDKeys: %v", err)
	}
	return Keys{Private: priv, Public: pub}
}

// testSubscriptionKeys generates a real P-256 ECDH keypair and auth secret,
// shaped like a browser's PushSubscription.getKey() output, so webpush-go's
// real RFC 8291 encryption succeeds against it end to end.
func testSubscriptionKeys(t *testing.T) (p256dh, auth string) {
	t.Helper()
	key, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ecdh key: %v", err)
	}
	secret := make([]byte, 16)
	if _, err := rand.Read(secret); err != nil {
		t.Fatalf("generate auth secret: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()), base64.RawURLEncoding.EncodeToString(secret)
}

func saveTestSubscription(t *testing.T, st *store.Store, endpoint string) store.PushSubscription {
	t.Helper()
	p256dh, auth := testSubscriptionKeys(t)
	sub := store.PushSubscription{Endpoint: endpoint, P256dh: p256dh, Auth: auth, CreatedAt: time.Now()}
	if err := st.SaveSubscription(t.Context(), sub); err != nil {
		t.Fatalf("SaveSubscription: %v", err)
	}
	return sub
}

// capturedPush is one push POST recorded by a fake push endpoint.
type capturedPush struct {
	body            []byte
	contentEncoding string
}

// fakePushServer stands in for a browser's push service: an HTTPS test server
// (Dispatcher's endpoint validation and real-world push services both require
// https) that records every request it receives and replies with status.
func fakePushServer(t *testing.T, status int) (*httptest.Server, chan capturedPush) {
	t.Helper()
	captured := make(chan capturedPush, 8)
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		captured <- capturedPush{body: body, contentEncoding: r.Header.Get("Content-Encoding")}
		w.WriteHeader(status)
	}))
	t.Cleanup(ts.Close)
	return ts, captured
}

func TestDispatcherSendsEncryptedPush(t *testing.T) {
	ts, captured := fakePushServer(t, http.StatusCreated)
	st := newTestStore(t)
	saveTestSubscription(t, st, ts.URL)

	d := New(Config{Store: st, Keys: testKeys(t), Client: ts.Client()})
	d.dispatch(notify.Notification{
		NodeID: "n1", Title: "grove: Fix bug", Body: "needs you",
		URL: "http://127.0.0.1:7433/n/n1",
	})

	select {
	case got := <-captured:
		if got.contentEncoding != "aes128gcm" {
			t.Errorf("Content-Encoding = %q, want aes128gcm", got.contentEncoding)
		}
		if len(got.body) == 0 {
			t.Error("push body is empty")
		}
		if strings.Contains(string(got.body), "Fix bug") || strings.Contains(string(got.body), "needs you") {
			t.Error("push body contains plaintext, want it RFC 8291 encrypted")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("push endpoint never received a request")
	}
}

func TestDispatcherPrunesOnGoneStatuses(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		wantPruned bool
	}{
		{"404 not found prunes", http.StatusNotFound, true},
		{"410 gone prunes", http.StatusGone, true},
		{"500 server error does not prune", http.StatusInternalServerError, false},
		{"400 bad request does not prune", http.StatusBadRequest, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts, captured := fakePushServer(t, tt.status)
			st := newTestStore(t)
			sub := saveTestSubscription(t, st, ts.URL)

			d := New(Config{Store: st, Keys: testKeys(t), Client: ts.Client()})
			d.dispatch(notify.Notification{NodeID: "n1", Title: "t", Body: "b"})

			select {
			case <-captured:
			case <-time.After(5 * time.Second):
				t.Fatal("push endpoint never received a request")
			}

			subs, err := st.ListSubscriptions(t.Context())
			if err != nil {
				t.Fatalf("ListSubscriptions: %v", err)
			}
			pruned := len(subs) == 0
			if pruned != tt.wantPruned {
				t.Errorf("pruned = %v, want %v (endpoint %s, subscriptions left: %+v)", pruned, tt.wantPruned, sub.Endpoint, subs)
			}
		})
	}
}

func TestDispatcherNoSubscriptionsIsNoop(t *testing.T) {
	st := newTestStore(t)
	d := New(Config{Store: st, Keys: testKeys(t)})
	d.dispatch(notify.Notification{NodeID: "n1", Title: "t", Body: "b"}) // must not panic or hang
}

// TestNotifyDoesNotBlockCaller pins the Sink contract (see notify.Sink's
// doc): even a push endpoint that hangs indefinitely must not make Notify
// block its caller (the coalescer, on the runner's dispatch path).
func TestNotifyDoesNotBlockCaller(t *testing.T) {
	block := make(chan struct{})
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-block
		w.WriteHeader(http.StatusCreated)
	}))
	defer func() { close(block); ts.Close() }()

	st := newTestStore(t)
	saveTestSubscription(t, st, ts.URL)

	d := New(Config{Store: st, Keys: testKeys(t), Client: ts.Client()})

	done := make(chan struct{})
	go func() {
		d.Notify(notify.Notification{NodeID: "n1", Title: "t", Body: "b"})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Notify blocked on a hung push endpoint")
	}
}
