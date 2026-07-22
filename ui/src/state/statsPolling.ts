import { apiClient } from "./api";
import { useStatsStore } from "./stats";
import type { NodeID, StatsRange } from "../gen/types";

const POLL_MS = 60_000;

let pollTimer: ReturnType<typeof setInterval> | null = null;
// Monotonic request token so a slow response for a since-abandoned
// scope/range can't clobber a newer one that resolved first -- same guard
// DirCombobox uses for its completion fetch.
let reqSeq = 0;

async function fetchNow(): Promise<void> {
  const { scope, range, setLoading, setData, setError } = useStatsStore.getState();
  const token = (reqSeq += 1);
  setLoading(true);
  try {
    const res = await apiClient.getStats(scope || undefined, range);
    if (reqSeq !== token) return;
    setData(res);
  } catch (err) {
    if (reqSeq !== token) return;
    setError(err instanceof Error ? err.message : String(err));
  } finally {
    if (reqSeq === token) setLoading(false);
  }
}

function onVisibilityChange(): void {
  if (document.hidden) {
    if (pollTimer !== null) {
      clearInterval(pollTimer);
      pollTimer = null;
    }
    return;
  }
  // Coming back into focus: refresh immediately, then resume the interval.
  void fetchNow();
  if (pollTimer === null) pollTimer = setInterval(() => void fetchNow(), POLL_MS);
}

/** Fetches immediately, then every 60s while the tab is visible; paused
 *  while backgrounded. Scoped to the Stats view's mount lifecycle (unlike
 *  usage/reviews polling, which run app-wide for their nav badges) --
 *  idempotent-safe to call again after stopStatsPolling, so StrictMode's
 *  dev-mode mount/unmount/mount doesn't double up intervals. */
export function startStatsPolling(): void {
  stopStatsPolling();
  void fetchNow();
  pollTimer = document.hidden ? null : setInterval(() => void fetchNow(), POLL_MS);
  document.addEventListener("visibilitychange", onVisibilityChange);
}

export function stopStatsPolling(): void {
  if (pollTimer !== null) clearInterval(pollTimer);
  pollTimer = null;
  document.removeEventListener("visibilitychange", onVisibilityChange);
}

/** One-off refetch against whatever scope/range is currently in the store. */
export function refreshStats(): void {
  void fetchNow();
}

/** Sets the range and refetches immediately, rather than waiting for the
 *  next scheduled poll tick -- mirrors cycleUsageWindow's "set + refetch
 *  together" idiom. */
export function setStatsRange(range: StatsRange): void {
  useStatsStore.getState().setRange(range);
  refreshStats();
}

export function setStatsScope(scope: NodeID | ""): void {
  useStatsStore.getState().setScope(scope);
  refreshStats();
}
