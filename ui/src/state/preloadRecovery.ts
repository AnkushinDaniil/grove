// Recovers from stale-chunk failures that follow a daemon upgrade.
//
// grove fingerprints its lazy JS chunks (e.g. XtermHost-<hash>.js). When the
// daemon is upgraded while a tab stays open, that tab keeps running the old
// bundle, whose chunk hashes the upgraded daemon no longer serves. The first
// lazy import after the upgrade -- opening a terminal or a diff -- then fails
// with "Failed to fetch dynamically imported module", and react-router shows
// its developer-facing default error page.
//
// Vite dispatches a `vite:preloadError` event on window for exactly this
// failure. Reloading pulls the fresh index.html (served no-cache) and its new
// chunk graph, so the same import succeeds on the retry. A short sessionStorage
// cooldown stops the page from looping when a chunk is genuinely gone (a broken
// deploy) rather than merely stale.

const RELOAD_MARKER = "grove:preload-reloaded-at";
const RELOAD_COOLDOWN_MS = 10_000;

/**
 * isStaleChunkError reports whether err looks like a failed dynamic import of a
 * fingerprinted chunk -- the signature of a stale tab after an upgrade. It
 * matches the three phrasings browsers use (Chromium, Firefox, Safari).
 */
export function isStaleChunkError(err: unknown): boolean {
  const msg = err instanceof Error ? err.message : typeof err === "string" ? err : "";
  return (
    msg.includes("Failed to fetch dynamically imported module") ||
    msg.includes("error loading dynamically imported module") ||
    msg.includes("Importing a module script failed")
  );
}

/**
 * shouldReload decides whether a stale-chunk failure at `now` warrants a
 * reload, given the last reload timestamp. It returns false within the cooldown
 * so a chunk that is genuinely missing cannot loop the page indefinitely.
 */
export function shouldReload(lastReloadAt: number, now: number): boolean {
  if (!Number.isFinite(lastReloadAt) || lastReloadAt <= 0) return true;
  return now - lastReloadAt >= RELOAD_COOLDOWN_MS;
}

/**
 * recoverFromStaleChunk reloads once (respecting the cooldown) and reports
 * whether it initiated a reload. The router error boundary shares this helper
 * with the window-level handler so both honor the same guard.
 */
export function recoverFromStaleChunk(win: Window = window): boolean {
  const last = Number(win.sessionStorage.getItem(RELOAD_MARKER) ?? "0");
  const now = Date.now();
  if (!shouldReload(last, now)) return false;
  win.sessionStorage.setItem(RELOAD_MARKER, String(now));
  win.location.reload();
  return true;
}

/**
 * installPreloadRecovery wires the window-level `vite:preloadError` handler.
 * Call once at startup, before rendering, so a stale lazy import self-heals
 * without ever surfacing an error to the user.
 */
export function installPreloadRecovery(win: Window = window): void {
  win.addEventListener("vite:preloadError", ((event: Event) => {
    // Suppress Vite's default rethrow; recovery is a reload, not an error.
    event.preventDefault();
    recoverFromStaleChunk(win);
  }) as EventListener);
}
