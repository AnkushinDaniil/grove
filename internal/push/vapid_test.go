package push

import (
	"path/filepath"
	"testing"

	"github.com/AnkushinDaniil/grove/internal/store"
)

func TestGenerateOrLoadKeysIdempotent(t *testing.T) {
	st := newTestStore(t)

	k1, err := GenerateOrLoadKeys(t.Context(), st)
	if err != nil {
		t.Fatalf("GenerateOrLoadKeys: %v", err)
	}
	if k1.Private == "" || k1.Public == "" {
		t.Fatalf("keys = %+v, want both non-empty", k1)
	}

	k2, err := GenerateOrLoadKeys(t.Context(), st)
	if err != nil {
		t.Fatalf("GenerateOrLoadKeys (second load): %v", err)
	}
	if k1 != k2 {
		t.Fatalf("second load = %+v, want the same keys as the first %+v", k2, k1)
	}
}

// TestGenerateOrLoadKeysPersistsAcrossStoreReopen guards the reason the keys
// are generated once, ever: a browser's PushSubscription is bound to the
// public key it was created with, so a fresh keypair on every daemon restart
// would silently orphan every existing subscription.
func TestGenerateOrLoadKeysPersistsAcrossStoreReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "grove.db")

	st1, err := store.Open(t.Context(), path)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	k1, err := GenerateOrLoadKeys(t.Context(), st1)
	if err != nil {
		t.Fatalf("GenerateOrLoadKeys: %v", err)
	}
	if err := st1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	st2, err := store.Open(t.Context(), path)
	if err != nil {
		t.Fatalf("store.Open (reopen): %v", err)
	}
	t.Cleanup(func() { _ = st2.Close() })
	k2, err := GenerateOrLoadKeys(t.Context(), st2)
	if err != nil {
		t.Fatalf("GenerateOrLoadKeys (reopen): %v", err)
	}
	if k1 != k2 {
		t.Fatalf("keys after reopen = %+v, want the persisted %+v", k2, k1)
	}
}

func TestKeysPublicKey(t *testing.T) {
	k := Keys{Public: "abc", Private: "def"}
	if got := k.PublicKey(); got != "abc" {
		t.Errorf("PublicKey() = %q, want %q", got, "abc")
	}
}
