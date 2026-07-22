import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  installPreloadRecovery,
  isStaleChunkError,
  recoverFromStaleChunk,
  shouldReload,
} from "./preloadRecovery";

describe("isStaleChunkError", () => {
  it("matches the browser phrasings of a failed dynamic import", () => {
    expect(isStaleChunkError(new Error("Failed to fetch dynamically imported module: /assets/X.js"))).toBe(true);
    expect(isStaleChunkError(new Error("error loading dynamically imported module"))).toBe(true);
    expect(isStaleChunkError(new Error("Importing a module script failed."))).toBe(true);
    expect(isStaleChunkError("Failed to fetch dynamically imported module")).toBe(true);
  });

  it("ignores unrelated errors and non-error values", () => {
    expect(isStaleChunkError(new Error("network request failed"))).toBe(false);
    expect(isStaleChunkError(null)).toBe(false);
    expect(isStaleChunkError({ message: "Failed to fetch dynamically imported module" })).toBe(false);
  });
});

describe("shouldReload", () => {
  it("reloads on a first failure (no prior reload recorded)", () => {
    expect(shouldReload(0, 1_000)).toBe(true);
    expect(shouldReload(Number.NaN, 1_000)).toBe(true);
  });

  it("suppresses a reload within the 10s cooldown, allows it after", () => {
    expect(shouldReload(1_000, 5_000)).toBe(false);
    expect(shouldReload(1_000, 10_999)).toBe(false);
    expect(shouldReload(1_000, 11_000)).toBe(true);
  });
});

/** fakeWindow is a minimal Window stand-in for the recovery helpers. */
function fakeWindow(lastReloadAt?: number) {
  const store = new Map<string, string>();
  if (lastReloadAt !== undefined) store.set("grove:preload-reloaded-at", String(lastReloadAt));
  const reload = vi.fn();
  const listeners = new Map<string, EventListener>();
  const win = {
    sessionStorage: {
      getItem: (k: string) => store.get(k) ?? null,
      setItem: (k: string, v: string) => void store.set(k, v),
    },
    location: { reload },
    addEventListener: (type: string, cb: EventListener) => void listeners.set(type, cb),
  } as unknown as Window;
  return { win, reload, store, listeners };
}

describe("recoverFromStaleChunk", () => {
  beforeEach(() => vi.spyOn(Date, "now"));
  afterEach(() => vi.restoreAllMocks());

  it("reloads and records the timestamp on a fresh failure", () => {
    vi.mocked(Date.now).mockReturnValue(50_000);
    const { win, reload, store } = fakeWindow();
    expect(recoverFromStaleChunk(win)).toBe(true);
    expect(reload).toHaveBeenCalledOnce();
    expect(store.get("grove:preload-reloaded-at")).toBe("50000");
  });

  it("does not reload again within the cooldown", () => {
    vi.mocked(Date.now).mockReturnValue(52_000);
    const { win, reload } = fakeWindow(50_000);
    expect(recoverFromStaleChunk(win)).toBe(false);
    expect(reload).not.toHaveBeenCalled();
  });
});

describe("installPreloadRecovery", () => {
  beforeEach(() => vi.spyOn(Date, "now").mockReturnValue(1_000));
  afterEach(() => vi.restoreAllMocks());

  it("reloads when a vite:preloadError fires", () => {
    const { win, reload, listeners } = fakeWindow();
    installPreloadRecovery(win);
    const handler = listeners.get("vite:preloadError");
    expect(handler).toBeDefined();

    const preventDefault = vi.fn();
    handler?.({ preventDefault } as unknown as Event);
    expect(preventDefault).toHaveBeenCalledOnce();
    expect(reload).toHaveBeenCalledOnce();
  });
});
