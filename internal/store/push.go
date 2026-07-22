package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// PushSubscription is one browser's Web Push registration (docs/API.md "Web
// push"). Endpoint is the push service URL handed back by the browser's
// PushManager.subscribe() and uniquely identifies the subscription; P256dh
// and Auth are the base64 keys from PushSubscription.getKey(), used to
// encrypt each payload per RFC 8291.
type PushSubscription struct {
	Endpoint  string
	P256dh    string
	Auth      string
	CreatedAt time.Time
}

const upsertPushSubscriptionSQL = `
INSERT INTO push_subscriptions (endpoint, p256dh, auth, created_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(endpoint) DO UPDATE SET p256dh = excluded.p256dh, auth = excluded.auth
`

// SaveSubscription upserts a subscription: re-subscribing with the same
// endpoint refreshes its keys rather than erroring, matching how a browser
// re-registers (e.g. after the site data resets but the push endpoint is
// reused).
func (s *Store) SaveSubscription(ctx context.Context, sub PushSubscription) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, upsertPushSubscriptionSQL,
			sub.Endpoint, sub.P256dh, sub.Auth, msFromTime(sub.CreatedAt),
		)
		if err != nil {
			return fmt.Errorf("upsert push subscription: %w", err)
		}
		return nil
	})
}

const deletePushSubscriptionSQL = `DELETE FROM push_subscriptions WHERE endpoint = ?`

// DeleteSubscription removes a subscription by endpoint. Deleting an unknown
// endpoint is not an error (idempotent, matching the unsubscribe endpoint's
// always-204 contract and the dispatcher's prune-on-410 path).
func (s *Store) DeleteSubscription(ctx context.Context, endpoint string) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, deletePushSubscriptionSQL, endpoint); err != nil {
			return fmt.Errorf("delete push subscription: %w", err)
		}
		return nil
	})
}

const selectPushSubscriptionsSQL = `
SELECT endpoint, p256dh, auth, created_at FROM push_subscriptions ORDER BY created_at ASC
`

// ListSubscriptions returns every registered subscription, oldest first.
func (s *Store) ListSubscriptions(ctx context.Context) ([]PushSubscription, error) {
	rows, err := s.db.QueryContext(ctx, selectPushSubscriptionsSQL)
	if err != nil {
		return nil, fmt.Errorf("list push subscriptions: %w", err)
	}
	out, err := collect(rows, scanPushSubscription)
	if err != nil {
		return nil, fmt.Errorf("list push subscriptions: %w", err)
	}
	return out, nil
}

// scanPushSubscription scans one row shaped like selectPushSubscriptionsSQL
// into a PushSubscription.
func scanPushSubscription(row rowScanner) (PushSubscription, error) {
	var (
		sub       PushSubscription
		createdAt int64
	)
	if err := row.Scan(&sub.Endpoint, &sub.P256dh, &sub.Auth, &createdAt); err != nil {
		return PushSubscription{}, fmt.Errorf("scan push subscription row: %w", err)
	}
	sub.CreatedAt = timeFromMS(createdAt)
	return sub, nil
}
