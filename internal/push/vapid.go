package push

import (
	"context"
	"fmt"

	webpush "github.com/SherClockHolmes/webpush-go"

	"github.com/AnkushinDaniil/grove/internal/store"
)

// Settings keys the VAPID keypair is persisted under (docs/API.md "Web push").
const (
	settingVAPIDPrivate = "vapid_private"
	settingVAPIDPublic  = "vapid_public"
)

// Keys is the daemon's VAPID keypair (RFC 8292). It is generated once and
// persisted in settings: a browser's PushSubscription is bound to the public
// key (applicationServerKey) it was created with, so rotating the pair would
// silently orphan every existing subscription.
type Keys struct {
	Private string
	Public  string
}

// PublicKey returns the base64url-encoded VAPID public key — the
// applicationServerKey a browser's PushManager.subscribe() needs, served at
// GET /api/v1/push/key.
func (k Keys) PublicKey() string { return k.Public }

// GenerateOrLoadKeys loads the daemon's persisted VAPID keypair, generating
// and storing a fresh one on first run. Later calls (including across daemon
// restarts) reload the same persisted pair.
func GenerateOrLoadKeys(ctx context.Context, st *store.Store) (Keys, error) {
	keys, ok, err := loadKeys(ctx, st)
	if err != nil {
		return Keys{}, err
	}
	if ok {
		return keys, nil
	}

	priv, pub, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		return Keys{}, fmt.Errorf("generate vapid keys: %w", err)
	}
	if err := st.SetSetting(ctx, settingVAPIDPrivate, priv); err != nil {
		return Keys{}, fmt.Errorf("persist vapid private key: %w", err)
	}
	if err := st.SetSetting(ctx, settingVAPIDPublic, pub); err != nil {
		return Keys{}, fmt.Errorf("persist vapid public key: %w", err)
	}
	return Keys{Private: priv, Public: pub}, nil
}

// loadKeys reads a previously persisted keypair. ok is false when either half
// is missing (a fresh install), signaling the caller to generate one.
func loadKeys(ctx context.Context, st *store.Store) (Keys, bool, error) {
	priv, ok, err := st.GetSetting(ctx, settingVAPIDPrivate)
	if err != nil {
		return Keys{}, false, fmt.Errorf("load vapid private key: %w", err)
	}
	if !ok {
		return Keys{}, false, nil
	}
	pub, ok, err := st.GetSetting(ctx, settingVAPIDPublic)
	if err != nil {
		return Keys{}, false, fmt.Errorf("load vapid public key: %w", err)
	}
	if !ok {
		return Keys{}, false, nil
	}
	return Keys{Private: priv, Public: pub}, true, nil
}
