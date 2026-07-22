// grove service worker: web push notifications + click-to-open deep links.
//
// Plain JS, no bundler, no dependencies -- this file is copied verbatim to
// dist/sw.js by `vite build` (public/ is copied to the dist root) and served
// at /sw.js by the daemon. See docs/API.md "Web push (/api/v1/push)": the
// daemon dispatches an encrypted push carrying {title, body, url, tag} to
// each subscription when a node raises attention.

const APP_ICON = "/icons/icon-192.png";
const APP_BADGE = "/icons/badge-96.png";
const DEFAULT_TAG = "grove-attention";

self.addEventListener("install", () => {
  // Take over from any previous version immediately -- this worker has no
  // versioned cache to worry about invalidating, it only ever handles
  // push/notificationclick, so there's nothing to lose by skipping the wait.
  self.skipWaiting();
});

self.addEventListener("activate", (event) => {
  event.waitUntil(self.clients.claim());
});

self.addEventListener("push", (event) => {
  let data = {};
  if (event.data) {
    try {
      data = event.data.json();
    } catch {
      // Not JSON (or unparseable) -- fall back to a fully generic
      // notification rather than dropping the push.
      data = {};
    }
  }

  const title = data.title || "grove";
  const body = data.body || "A node needs your attention.";
  const tag = data.tag || DEFAULT_TAG;
  const url = data.url || "/";

  event.waitUntil(
    self.registration.showNotification(title, {
      body,
      tag,
      icon: APP_ICON,
      badge: APP_BADGE,
      data: { url },
      // Re-alert (sound/vibrate) even if a notification with this tag is
      // already showing -- attention events sharing a tag shouldn't go
      // unnoticed just because an earlier one is still on screen.
      renotify: true,
    }),
  );
});

self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  const url = (event.notification.data && event.notification.data.url) || "/";
  const targetUrl = new URL(url, self.location.origin).href;

  event.waitUntil(
    (async () => {
      const allClients = await self.clients.matchAll({ type: "window", includeUncontrolled: true });
      const existing = allClients[0];
      if (existing) {
        await existing.focus();
        // Hand the deep link to the already-loaded SPA's own router (see
        // state/push.ts's listenForPushNavigation) rather than
        // WindowClient.navigate(), which isn't supported everywhere (e.g.
        // Safari) -- postMessage is the reliable common denominator.
        existing.postMessage({ type: "push-navigate", url: targetUrl });
        return;
      }
      await self.clients.openWindow(targetUrl);
    })(),
  );
});

self.addEventListener("pushsubscriptionchange", (event) => {
  // Best-effort re-subscription when the push service invalidates the old
  // endpoint (key rotation, expiry). There's no UI to report failure to from
  // inside the service worker; if this doesn't manage to re-subscribe, the
  // next visit to Settings self-heals it (getPushStatus() will read back as
  // "default", not "subscribed").
  event.waitUntil(
    (async () => {
      try {
        const oldOptions = event.oldSubscription && event.oldSubscription.options;
        const newOptions = event.newSubscription && event.newSubscription.options;
        const applicationServerKey =
          (oldOptions && oldOptions.applicationServerKey) || (newOptions && newOptions.applicationServerKey);
        if (!applicationServerKey) return;

        const subscription = await self.registration.pushManager.subscribe({
          userVisibleOnly: true,
          applicationServerKey,
        });
        const json = subscription.toJSON();
        if (!json.endpoint || !json.keys || !json.keys.p256dh || !json.keys.auth) return;

        await fetch("/api/v1/push/subscribe", {
          method: "POST",
          headers: { "Content-Type": "application/json", "X-Grove-CSRF": "1" },
          credentials: "include",
          body: JSON.stringify({
            endpoint: json.endpoint,
            keys: { p256dh: json.keys.p256dh, auth: json.keys.auth },
          }),
        });
      } catch {
        // Best effort; nothing more actionable from inside the SW.
      }
    })(),
  );
});
