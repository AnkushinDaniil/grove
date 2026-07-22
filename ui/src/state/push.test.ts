import { afterEach, describe, expect, it, vi } from "vitest";
import { apiClient } from "./api";
import {
  disablePush,
  enablePush,
  getPushStatus,
  listenForPushNavigation,
  showTestNotification,
  toSubscribeRequest,
  urlBase64ToUint8Array,
} from "./push";

// jsdom implements none of the Push API (ServiceWorker/PushManager/
// Notification are all absent), so every test below wires up its own
// minimal fakes rather than exercising a real browser. isSecureContext and
// navigator.serviceWorker are patched directly via defineProperty (not
// vi.stubGlobal, which would swap out the whole window/navigator object)
// since both are normally read-only on the real objects jsdom provides.

const originalServiceWorker = Object.getOwnPropertyDescriptor(navigator, "serviceWorker");
const originalIsSecureContext = Object.getOwnPropertyDescriptor(window, "isSecureContext");

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  if (originalServiceWorker) Object.defineProperty(navigator, "serviceWorker", originalServiceWorker);
  if (originalIsSecureContext) Object.defineProperty(window, "isSecureContext", originalIsSecureContext);
});

function stubSecureContext(secure: boolean) {
  Object.defineProperty(window, "isSecureContext", { value: secure, configurable: true });
}

function stubNotification(permission: NotificationPermission, requestResult?: NotificationPermission) {
  const requestPermission = vi.fn(async () => requestResult ?? permission);
  vi.stubGlobal("Notification", { permission, requestPermission });
  return { requestPermission };
}

interface FakeSubscription {
  endpoint: string;
  unsubscribe: ReturnType<typeof vi.fn>;
  toJSON: () => { endpoint: string; keys: { p256dh: string; auth: string } };
}

function makeFakeSubscription(endpoint = "https://push.example/ep-1"): FakeSubscription {
  return {
    endpoint,
    unsubscribe: vi.fn(async () => true),
    toJSON: () => ({ endpoint, keys: { p256dh: "p256dh-value", auth: "auth-value" } }),
  };
}

/** Installs a fake navigator.serviceWorker + window.PushManager so
 *  hasPushSupport() passes. `subscription` seeds what getSubscription()
 *  resolves to (null = "no active subscription"); `subscribe` lets a test
 *  control what PushManager.subscribe() itself returns. */
function stubServiceWorker(
  opts: { subscription?: FakeSubscription | null; subscribe?: ReturnType<typeof vi.fn> } = {},
) {
  const subscribe = opts.subscribe ?? vi.fn(async () => makeFakeSubscription());
  const getSubscription = vi.fn(async () => opts.subscription ?? null);
  const registration = {
    pushManager: { subscribe, getSubscription },
    showNotification: vi.fn(async () => undefined),
  };
  const container = {
    register: vi.fn(async () => registration),
    getRegistration: vi.fn(async () => registration),
    ready: Promise.resolve(registration),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  };
  Object.defineProperty(navigator, "serviceWorker", { value: container, configurable: true });
  vi.stubGlobal("PushManager", class {});
  return { container, registration, subscribe, getSubscription };
}

/** A secure, feature-complete browser that has simply never registered a
 *  service worker (getRegistration() -> undefined). Used for the "no active
 *  registration" branches of disablePush/showTestNotification. */
function stubNoRegistration() {
  const container = {
    register: vi.fn(async () => {
      throw new Error("register() should not be reached in this scenario");
    }),
    getRegistration: vi.fn(async () => undefined),
    ready: new Promise(() => {}),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  };
  Object.defineProperty(navigator, "serviceWorker", { value: container, configurable: true });
  vi.stubGlobal("PushManager", class {});
  return container;
}

describe("getPushStatus", () => {
  it("reports insecure on a non-secure origin, before any feature check", async () => {
    stubSecureContext(false);
    expect(await getPushStatus()).toBe("insecure");
  });

  it("reports unsupported when the Push API is missing on a secure origin", async () => {
    stubSecureContext(true);
    Object.defineProperty(navigator, "serviceWorker", { value: undefined, configurable: true });
    vi.stubGlobal("PushManager", undefined);
    expect(await getPushStatus()).toBe("unsupported");
  });

  it("reports denied when Notification.permission is denied", async () => {
    stubSecureContext(true);
    stubServiceWorker();
    stubNotification("denied");
    expect(await getPushStatus()).toBe("denied");
  });

  it("reports default when permission is granted but there's no active subscription", async () => {
    stubSecureContext(true);
    stubServiceWorker({ subscription: null });
    stubNotification("granted");
    expect(await getPushStatus()).toBe("default");
  });

  it("reports subscribed when an active subscription exists", async () => {
    stubSecureContext(true);
    stubServiceWorker({ subscription: makeFakeSubscription() });
    stubNotification("granted");
    expect(await getPushStatus()).toBe("subscribed");
  });
});

describe("enablePush", () => {
  it("registers the service worker, requests permission, subscribes, and posts the subscription", async () => {
    stubSecureContext(true);
    const fakeSub = makeFakeSubscription("https://push.example/new");
    const { container, subscribe } = stubServiceWorker({ subscribe: vi.fn(async () => fakeSub) });
    stubNotification("default", "granted");

    const getPushKeySpy = vi.spyOn(apiClient, "getPushKey");
    const pushSubscribeSpy = vi.spyOn(apiClient, "pushSubscribe").mockResolvedValue(undefined);

    await enablePush();

    expect(container.register).toHaveBeenCalledWith("/sw.js");
    expect(getPushKeySpy).toHaveBeenCalled();
    expect(subscribe).toHaveBeenCalledWith(
      expect.objectContaining({ userVisibleOnly: true, applicationServerKey: expect.any(Uint8Array) }),
    );
    expect(pushSubscribeSpy).toHaveBeenCalledWith({
      endpoint: "https://push.example/new",
      keys: { p256dh: "p256dh-value", auth: "auth-value" },
    });
  });

  it("throws without registering when the origin is insecure", async () => {
    stubSecureContext(false);
    const container = stubNoRegistration();
    await expect(enablePush()).rejects.toThrow(/secure/i);
    expect(container.register).not.toHaveBeenCalled();
  });

  it("throws when the user denies the permission prompt, without subscribing", async () => {
    stubSecureContext(true);
    const { subscribe } = stubServiceWorker();
    stubNotification("default", "denied");

    await expect(enablePush()).rejects.toThrow(/permission/i);
    expect(subscribe).not.toHaveBeenCalled();
  });

  it("skips the permission prompt when already granted", async () => {
    stubSecureContext(true);
    stubServiceWorker();
    const { requestPermission } = stubNotification("granted");
    vi.spyOn(apiClient, "getPushKey");
    vi.spyOn(apiClient, "pushSubscribe").mockResolvedValue(undefined);

    await enablePush();

    expect(requestPermission).not.toHaveBeenCalled();
  });
});

describe("disablePush", () => {
  it("unsubscribes and notifies the daemon when a subscription exists", async () => {
    stubSecureContext(true);
    const fakeSub = makeFakeSubscription("https://push.example/existing");
    stubServiceWorker({ subscription: fakeSub });
    const pushUnsubscribeSpy = vi.spyOn(apiClient, "pushUnsubscribe").mockResolvedValue(undefined);

    await disablePush();

    expect(fakeSub.unsubscribe).toHaveBeenCalled();
    expect(pushUnsubscribeSpy).toHaveBeenCalledWith("https://push.example/existing");
  });

  it("no-ops quietly when there is no active subscription", async () => {
    stubServiceWorker({ subscription: null });
    const pushUnsubscribeSpy = vi.spyOn(apiClient, "pushUnsubscribe").mockResolvedValue(undefined);

    await expect(disablePush()).resolves.toBeUndefined();
    expect(pushUnsubscribeSpy).not.toHaveBeenCalled();
  });

  it("no-ops when serviceWorker isn't supported at all", async () => {
    Object.defineProperty(navigator, "serviceWorker", { value: undefined, configurable: true });
    await expect(disablePush()).resolves.toBeUndefined();
  });
});

describe("showTestNotification", () => {
  it("calls showNotification on the active registration", async () => {
    const { registration } = stubServiceWorker();
    await showTestNotification();
    expect(registration.showNotification).toHaveBeenCalledWith("grove", expect.objectContaining({ tag: "grove-test" }));
  });

  it("throws when there is no active registration", async () => {
    stubNoRegistration();
    await expect(showTestNotification()).rejects.toThrow(/enable push/i);
  });
});

describe("toSubscribeRequest", () => {
  it("extracts endpoint + keys from a subscription's toJSON()", () => {
    const sub = makeFakeSubscription("https://push.example/x");
    expect(toSubscribeRequest(sub as unknown as PushSubscription)).toEqual({
      endpoint: "https://push.example/x",
      keys: { p256dh: "p256dh-value", auth: "auth-value" },
    });
  });

  it("throws when keys are missing from the JSON payload", () => {
    const sub = {
      endpoint: "https://push.example/x",
      toJSON: () => ({ endpoint: "https://push.example/x", keys: undefined }),
    };
    expect(() => toSubscribeRequest(sub as unknown as PushSubscription)).toThrow(/missing/i);
  });
});

describe("urlBase64ToUint8Array", () => {
  it("decodes a base64url VAPID key into raw bytes", () => {
    // "AAECAw" (no padding needed) is the base64url form of bytes [0,1,2,3].
    expect(Array.from(urlBase64ToUint8Array("AAECAw"))).toEqual([0, 1, 2, 3]);
  });

  it("handles the -/_ substitution and missing padding that base64url implies", () => {
    // Bytes [0xfb, 0xff] are "-_8" in base64url vs "+/8=" in standard base64.
    expect(Array.from(urlBase64ToUint8Array("-_8"))).toEqual([0xfb, 0xff]);
  });
});

describe("listenForPushNavigation", () => {
  it("routes push-navigate messages from the service worker to the router", () => {
    const { container } = stubServiceWorker();
    const navigate = vi.fn();
    listenForPushNavigation({ navigate });

    expect(container.addEventListener).toHaveBeenCalledWith("message", expect.any(Function));
    const handler = container.addEventListener.mock.calls[0]?.[1] as (e: MessageEvent) => void;
    handler({ data: { type: "push-navigate", url: "/n/abc" } } as MessageEvent);

    expect(navigate).toHaveBeenCalledWith("/n/abc");
  });

  it("ignores messages that aren't push-navigate", () => {
    const { container } = stubServiceWorker();
    const navigate = vi.fn();
    listenForPushNavigation({ navigate });

    const handler = container.addEventListener.mock.calls[0]?.[1] as (e: MessageEvent) => void;
    handler({ data: { type: "something-else" } } as MessageEvent);
    handler({ data: null } as MessageEvent);

    expect(navigate).not.toHaveBeenCalled();
  });

  it("unsubscribe() removes the message listener", () => {
    const { container } = stubServiceWorker();
    const unsubscribe = listenForPushNavigation({ navigate: vi.fn() });
    unsubscribe();
    expect(container.removeEventListener).toHaveBeenCalledWith("message", expect.any(Function));
  });

  it("returns a no-op unsubscribe when serviceWorker isn't supported", () => {
    Object.defineProperty(navigator, "serviceWorker", { value: undefined, configurable: true });
    expect(() => listenForPushNavigation({ navigate: vi.fn() })()).not.toThrow();
  });
});
