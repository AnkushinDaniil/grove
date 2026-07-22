// Web push subscription lifecycle: registers the service worker
// (public/sw.js -> served as /sw.js), requests Notification permission, and
// syncs the resulting PushSubscription with the daemon (GET /push/key,
// POST /push/subscribe, POST /push/unsubscribe -- see docs/API.md "Web push
// (/api/v1/push)"). Every direct navigator/ServiceWorker/Notification call
// in the app lives here; SettingsView only reads getPushStatus() and calls
// enablePush/disablePush/showTestNotification.
import { apiClient } from "./api";
import type { PushSubscribeRequest } from "../gen/types";

export type PushStatus =
  | "unsupported" // no Push API in this browser
  | "insecure" // Push API exists, but this origin isn't a secure context (needs https or localhost)
  | "denied" // the user has blocked notifications for this origin
  | "default" // supported and not blocked, but not currently subscribed
  | "subscribed";

const SW_URL = "/sw.js";

// Truthiness checks rather than `"x" in navigator/window`: real browsers
// either omit these globals entirely or provide a real object, but a
// defensive check should also treat an explicitly falsy value as absent.
function hasPushSupport(): boolean {
  return Boolean(navigator.serviceWorker) && Boolean(window.PushManager) && typeof Notification !== "undefined";
}

/** Feature support, permission, and subscription state, checked cheapest
 *  first. isSecureContext is checked before feature detection because
 *  browsers commonly hide `navigator.serviceWorker` entirely on insecure
 *  origins -- checking it first is what lets an http (non-localhost) origin
 *  report the more actionable "insecure" instead of a generic
 *  "unsupported". */
export async function getPushStatus(): Promise<PushStatus> {
  if (!window.isSecureContext) return "insecure";
  if (!hasPushSupport()) return "unsupported";
  if (Notification.permission === "denied") return "denied";

  try {
    const registration = await navigator.serviceWorker.getRegistration();
    const subscription = await registration?.pushManager.getSubscription();
    return subscription ? "subscribed" : "default";
  } catch {
    return "default";
  }
}

/** Registers the service worker (idempotent -- re-registering the same
 *  script+scope is a browser no-op), requests Notification permission if
 *  not already decided, subscribes via PushManager using the daemon's VAPID
 *  key, and posts the subscription to the daemon. Throws a user-facing
 *  message on failure; SettingsView surfaces it inline. */
export async function enablePush(): Promise<void> {
  if (!window.isSecureContext) {
    throw new Error("Push notifications need a secure origin (https, or localhost during development).");
  }
  if (!hasPushSupport()) {
    throw new Error("Push notifications aren't supported in this browser.");
  }

  // Kicks off registration; .ready below (rather than this call's return
  // value) is what actually resolves once the worker is active, which
  // PushManager.subscribe() needs.
  await navigator.serviceWorker.register(SW_URL);

  const permission =
    Notification.permission === "granted" ? "granted" : await Notification.requestPermission();
  if (permission !== "granted") {
    throw new Error("Notification permission was not granted.");
  }

  const ready = await navigator.serviceWorker.ready;
  const { public_key } = await apiClient.getPushKey();
  const subscription = await ready.pushManager.subscribe({
    userVisibleOnly: true,
    applicationServerKey: urlBase64ToUint8Array(public_key),
  });

  await apiClient.pushSubscribe(toSubscribeRequest(subscription));
}

/** Unsubscribes the active PushManager subscription (if any) and tells the
 *  daemon to drop it. No-ops quietly when push was never enabled on this
 *  device -- toggling "off" should never surface an error for that. */
export async function disablePush(): Promise<void> {
  if (!navigator.serviceWorker) return;
  const registration = await navigator.serviceWorker.getRegistration();
  const subscription = await registration?.pushManager.getSubscription();
  if (!subscription) return;
  await subscription.unsubscribe();
  await apiClient.pushUnsubscribe(subscription.endpoint);
}

/** Local-only smoke test: asks the active service worker to show a
 *  notification directly, without a round trip through a push service or
 *  the daemon. Confirms permission + SW wiring are correct on this device;
 *  it does not exercise server -> browser delivery (there is no
 *  `/push/test` endpoint -- see docs/API.md -- real delivery can only be
 *  observed by triggering an actual attention event). */
export async function showTestNotification(): Promise<void> {
  const registration = await navigator.serviceWorker.getRegistration();
  if (!registration) {
    throw new Error("No active service worker -- enable push notifications first.");
  }
  await registration.showNotification("grove", {
    body: "Test notification -- push is wired up on this device.",
    tag: "grove-test",
    icon: "/icons/icon-192.png",
    badge: "/icons/badge-96.png",
  });
}

/** Extracts the {endpoint, keys} shape the daemon expects from a raw
 *  PushSubscription. Exported standalone since payload shaping is the one
 *  part of this flow that's meaningfully unit-testable without a real push
 *  service (see state/push.test.ts). */
export function toSubscribeRequest(subscription: PushSubscription): PushSubscribeRequest {
  const json = subscription.toJSON();
  const endpoint = json.endpoint ?? subscription.endpoint;
  const p256dh = json.keys?.p256dh;
  const auth = json.keys?.auth;
  if (!endpoint || !p256dh || !auth) {
    throw new Error("Push subscription response is missing required fields.");
  }
  return { endpoint, keys: { p256dh, auth } };
}

/** The daemon's VAPID applicationServerKey arrives as base64url (per the Web
 *  Push spec); PushManager.subscribe() wants raw bytes. Explicitly
 *  parameterized as Uint8Array<ArrayBuffer> (rather than bare `Uint8Array`,
 *  which defaults to the wider `Uint8Array<ArrayBufferLike>`) so this
 *  satisfies PushSubscriptionOptionsInit's BufferSource without a cast. */
export function urlBase64ToUint8Array(base64Url: string): Uint8Array<ArrayBuffer> {
  const padding = "=".repeat((4 - (base64Url.length % 4)) % 4);
  const base64 = (base64Url + padding).replace(/-/g, "+").replace(/_/g, "/");
  const raw = atob(base64);
  const bytes = new Uint8Array(raw.length);
  for (let i = 0; i < raw.length; i++) bytes[i] = raw.charCodeAt(i);
  return bytes;
}

/** Structural subset of react-router's data router -- just enough to
 *  imperatively navigate from outside the React tree, without coupling this
 *  module to react-router's internal router type. */
interface Navigable {
  navigate: (to: string) => unknown;
}

/** Wires the service worker's postMessage (sent from notificationclick, see
 *  public/sw.js) to the app's own router. This is what makes a notification
 *  click deep-link to /n/<node_id> even when a grove tab is already open and
 *  focused (docs/API.md: "a notification whose click deep-links to
 *  /n/<node_id>") -- postMessage + the SPA's own router is used instead of
 *  WindowClient.navigate(), which isn't reliably supported everywhere (e.g.
 *  Safari). Returns an unsubscribe function; call once at startup (see
 *  main.tsx). */
export function listenForPushNavigation(router: Navigable): () => void {
  if (!navigator.serviceWorker) return () => {};
  function onMessage(event: MessageEvent) {
    const data = event.data as { type?: unknown; url?: unknown } | null | undefined;
    if (data && data.type === "push-navigate" && typeof data.url === "string") {
      void router.navigate(data.url);
    }
  }
  navigator.serviceWorker.addEventListener("message", onMessage);
  return () => navigator.serviceWorker.removeEventListener("message", onMessage);
}
