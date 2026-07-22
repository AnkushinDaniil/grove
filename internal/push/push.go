// Package push delivers Web Push notifications (RFC 8291) to browsers
// subscribed via POST /api/v1/push/subscribe, so attention alerts reach a
// phone even when its browser tab is closed. Dispatcher implements
// internal/notify.Sink, so it plugs into the daemon's notification runner
// alongside the macOS sink: the same attention-transition policy drives both,
// this is just another fan-out target.
//
// Like every notify.Sink, delivery is best-effort: a push service that is
// slow, unreachable, or returns an error must never block or fail the
// daemon, so every send is bounded, concurrent, and swallows its own errors
// (debug-logged).
package push

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"

	"github.com/AnkushinDaniil/grove/internal/notify"
	"github.com/AnkushinDaniil/grove/internal/store"
)

const (
	// sendTimeout bounds one subscription's delivery so a hung push service
	// can never pin a goroutine indefinitely.
	sendTimeout = 10 * time.Second
	// maxConcurrentSends bounds how many subscriptions are dispatched to at
	// once per notification, so a large subscriber list cannot spawn an
	// unbounded number of goroutines.
	maxConcurrentSends = 8
	// pushTTLSeconds is the Web Push TTL header: how long a push service
	// should hold an undelivered message for an offline device. The library's
	// zero-value default tells the push service to drop the message
	// immediately if the phone is unreachable, defeating the point of an
	// attention alert sent while the user is away.
	pushTTLSeconds = 24 * 60 * 60
	// vapidSubscriber identifies grove to push services in the VAPID JWT's
	// "sub" claim (RFC 8292) — a contact URL that push services do not verify
	// for reachability, only well-formedness.
	vapidSubscriber = "https://github.com/AnkushinDaniil/grove"
)

// Dispatcher fans a notify.Notification out to every registered browser
// subscription as an encrypted Web Push.
type Dispatcher struct {
	store  *store.Store
	keys   Keys
	client *http.Client // nil defaults to webpush-go's own *http.Client
	logger *slog.Logger
}

// Config carries the Dispatcher's dependencies.
type Config struct {
	Store *store.Store
	Keys  Keys
	// Client overrides the HTTP client used to deliver pushes. Nil defaults to
	// webpush-go's own client; tests point it at a fake push endpoint's
	// client (e.g. an httptest.Server's, to trust its TLS certificate).
	Client *http.Client
	Logger *slog.Logger
}

// New builds a Dispatcher from cfg.
func New(cfg Config) *Dispatcher {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Dispatcher{store: cfg.Store, keys: cfg.Keys, client: cfg.Client, logger: logger}
}

// pushPayload is the JSON body delivered to the service worker (/sw.js),
// which reads it to render the notification and its deep link.
type pushPayload struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url"`
	Tag   string `json:"tag"`
}

// Notify implements notify.Sink: it dispatches asynchronously and never
// blocks or fails the caller, matching every other Sink.
func (d *Dispatcher) Notify(n notify.Notification) {
	go d.dispatch(n)
}

// dispatch sends n to every subscription concurrently, bounded by
// maxConcurrentSends.
func (d *Dispatcher) dispatch(n notify.Notification) {
	ctx := context.Background()
	subs, err := d.store.ListSubscriptions(ctx)
	if err != nil {
		d.logger.Debug("push list subscriptions", "err", err)
		return
	}
	if len(subs) == 0 {
		return
	}

	msg, err := json.Marshal(pushPayload{Title: n.Title, Body: n.Body, URL: n.URL, Tag: string(n.NodeID)})
	if err != nil {
		d.logger.Debug("push marshal payload", "err", err)
		return
	}

	sem := make(chan struct{}, maxConcurrentSends)
	var wg sync.WaitGroup
	for _, sub := range subs {
		wg.Add(1)
		sem <- struct{}{}
		go func(sub store.PushSubscription) {
			defer wg.Done()
			defer func() { <-sem }()
			d.send(ctx, sub, msg)
		}(sub)
	}
	wg.Wait()
}

// send delivers msg to one subscription, pruning it on a 404/410 response
// (the push service reporting the endpoint is gone) and debug-logging any
// other failure.
func (d *Dispatcher) send(ctx context.Context, sub store.PushSubscription, msg []byte) {
	sendCtx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()

	opts := &webpush.Options{
		Subscriber:      vapidSubscriber,
		TTL:             pushTTLSeconds,
		VAPIDPublicKey:  d.keys.Public,
		VAPIDPrivateKey: d.keys.Private,
	}
	if d.client != nil {
		opts.HTTPClient = d.client
	}

	resp, err := webpush.SendNotificationWithContext(sendCtx, msg, &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys:     webpush.Keys{Auth: sub.Auth, P256dh: sub.P256dh},
	}, opts)
	if err != nil {
		d.logger.Debug("push send", "endpoint", sub.Endpoint, "err", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusNotFound, http.StatusGone:
		if err := d.store.DeleteSubscription(ctx, sub.Endpoint); err != nil {
			d.logger.Debug("push prune subscription", "endpoint", sub.Endpoint, "err", err)
		}
	default:
		if resp.StatusCode >= 300 {
			d.logger.Debug("push send non-2xx", "endpoint", sub.Endpoint, "status", resp.StatusCode)
		}
	}
}
