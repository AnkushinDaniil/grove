-- 0007_push_subscriptions.sql: Web Push subscriptions (docs/API.md "Web push").
--
-- One row per browser subscription registered via POST /push/subscribe. The
-- endpoint is the push service URL from the browser's PushSubscription object;
-- it is unique per subscribed browser/device, so it doubles as the primary
-- key — re-subscribing with the same endpoint just refreshes its keys.
-- p256dh/auth are the base64 values from PushSubscription.getKey(), used to
-- encrypt each payload per RFC 8291.
--
-- Timestamps are unix milliseconds, matching every other timestamp column
-- (see internal/store/scan.go).

CREATE TABLE push_subscriptions (
    endpoint    TEXT PRIMARY KEY,
    p256dh      TEXT NOT NULL,
    auth        TEXT NOT NULL,
    created_at  INTEGER NOT NULL
);
