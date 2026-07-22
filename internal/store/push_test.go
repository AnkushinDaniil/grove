package store

import "testing"

func testPushSubscription(endpoint string) PushSubscription {
	return PushSubscription{
		Endpoint:  endpoint,
		P256dh:    "p256dh-value",
		Auth:      "auth-value",
		CreatedAt: msTime(1_700_000_000_000),
	}
}

func TestPushSubscriptionCRUD(t *testing.T) {
	s := newTestStore(t)
	sub := testPushSubscription("https://push.example.com/sub/1")

	if err := s.SaveSubscription(t.Context(), sub); err != nil {
		t.Fatalf("SaveSubscription: %v", err)
	}

	subs, err := s.ListSubscriptions(t.Context())
	if err != nil {
		t.Fatalf("ListSubscriptions: %v", err)
	}
	if len(subs) != 1 || subs[0] != sub {
		t.Fatalf("ListSubscriptions = %+v, want [%+v]", subs, sub)
	}

	if err := s.DeleteSubscription(t.Context(), sub.Endpoint); err != nil {
		t.Fatalf("DeleteSubscription: %v", err)
	}
	subs, err = s.ListSubscriptions(t.Context())
	if err != nil {
		t.Fatalf("ListSubscriptions after delete: %v", err)
	}
	if len(subs) != 0 {
		t.Errorf("ListSubscriptions after delete = %+v, want none", subs)
	}
}

func TestSaveSubscriptionUpsertsKeys(t *testing.T) {
	s := newTestStore(t)
	endpoint := "https://push.example.com/sub/1"

	original := testPushSubscription(endpoint)
	if err := s.SaveSubscription(t.Context(), original); err != nil {
		t.Fatalf("SaveSubscription: %v", err)
	}

	updated := original
	updated.P256dh = "new-p256dh"
	updated.Auth = "new-auth"
	if err := s.SaveSubscription(t.Context(), updated); err != nil {
		t.Fatalf("SaveSubscription (update): %v", err)
	}

	subs, err := s.ListSubscriptions(t.Context())
	if err != nil {
		t.Fatalf("ListSubscriptions: %v", err)
	}
	if len(subs) != 1 || subs[0].P256dh != "new-p256dh" || subs[0].Auth != "new-auth" {
		t.Fatalf("ListSubscriptions after upsert = %+v, want one row with refreshed keys", subs)
	}
}

func TestDeleteSubscriptionMissingIsNotError(t *testing.T) {
	s := newTestStore(t)
	if err := s.DeleteSubscription(t.Context(), "https://push.example.com/does-not-exist"); err != nil {
		t.Errorf("DeleteSubscription(missing): %v", err)
	}
}

func TestListSubscriptionsOrderedByCreatedAt(t *testing.T) {
	s := newTestStore(t)
	older := testPushSubscription("https://push.example.com/sub/older")
	older.CreatedAt = msTime(1_700_000_000_000)
	newer := testPushSubscription("https://push.example.com/sub/newer")
	newer.CreatedAt = msTime(1_700_000_001_000)

	if err := s.SaveSubscription(t.Context(), newer); err != nil {
		t.Fatalf("SaveSubscription newer: %v", err)
	}
	if err := s.SaveSubscription(t.Context(), older); err != nil {
		t.Fatalf("SaveSubscription older: %v", err)
	}

	subs, err := s.ListSubscriptions(t.Context())
	if err != nil {
		t.Fatalf("ListSubscriptions: %v", err)
	}
	if len(subs) != 2 || subs[0].Endpoint != older.Endpoint || subs[1].Endpoint != newer.Endpoint {
		t.Fatalf("ListSubscriptions = %+v, want [older, newer]", subs)
	}
}
