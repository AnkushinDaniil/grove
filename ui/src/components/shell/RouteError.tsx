import { useEffect } from "react";
import { useRouteError } from "react-router";
import { isStaleChunkError, recoverFromStaleChunk } from "../../state/preloadRecovery";

/**
 * RouteError is the router's root error boundary. Its first job is to self-heal
 * the stale-chunk failure that follows a daemon upgrade: a lazy import that
 * fails with "Failed to fetch dynamically imported module" means this tab holds
 * an old bundle, so it reloads to pick up the new one (see state/preloadRecovery
 * -- the window-level `vite:preloadError` handler usually reloads first; this is
 * the fallback when that event does not fire). For any other error it shows a
 * clean message with a manual reload, instead of react-router's default
 * developer-facing page.
 */
export function RouteError() {
  const error = useRouteError();
  const stale = isStaleChunkError(error);

  useEffect(() => {
    // Guarded by the shared cooldown, so this is a no-op if the window handler
    // already reloaded moments ago.
    if (stale) recoverFromStaleChunk();
  }, [stale]);

  if (stale) {
    return (
      <div className="flex h-dvh w-full items-center justify-center bg-canvas text-xs text-ink-faint">
        Updating grove…
      </div>
    );
  }

  const message = error instanceof Error ? error.message : "Something went wrong.";
  return (
    <div className="flex h-dvh w-full flex-col items-center justify-center gap-3 bg-canvas px-6 text-center">
      <p className="text-sm font-medium text-ink">grove hit an error</p>
      <p className="max-w-md font-sans text-xs text-ink-faint break-words">{message}</p>
      <button
        type="button"
        onClick={() => window.location.reload()}
        className="mt-1 rounded-md border border-border-strong bg-surface px-3 py-1.5 text-xs text-ink hover:bg-hover"
      >
        Reload
      </button>
    </div>
  );
}
