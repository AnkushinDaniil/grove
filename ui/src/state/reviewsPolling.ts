import { apiClient } from "./api";
import { useReviewsStore } from "./reviews";

const POLL_MS = 120_000;

let pollTimer: ReturnType<typeof setInterval> | null = null;
let started = false;

async function fetchReviews(): Promise<void> {
  const { setLoading, setData, setFetchError } = useReviewsStore.getState();
  setLoading(true);
  try {
    const res = await apiClient.getReviews();
    setData(res);
  } catch (err) {
    setFetchError(err instanceof Error ? err.message : String(err));
  } finally {
    setLoading(false);
  }
}

async function fetchSources(): Promise<void> {
  try {
    const res = await apiClient.getReviewSources();
    useReviewsStore.getState().setSourceDirs(res.dirs);
  } catch {
    // Non-critical for the main list; the "Manage sources" panel surfaces
    // its own load error and lets the user retry by reopening it.
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
  void fetchReviews();
  if (pollTimer === null) {
    pollTimer = setInterval(() => void fetchReviews(), POLL_MS);
  }
}

/**
 * Fetches immediately, then every 120s while the browser tab is visible;
 * paused entirely while backgrounded. Review Radar has no WS push to lean
 * on (unlike the tree/inbox), so this polling loop is its only freshness
 * signal. Started once from AuthGate (mirroring startUsagePolling) so the
 * nav badge stays live from boot instead of going stale the moment the user
 * navigates away from /reviews.
 */
export function startReviewsPolling(): void {
  if (started) return;
  started = true;

  void fetchReviews();
  void fetchSources();
  if (!document.hidden) {
    pollTimer = setInterval(() => void fetchReviews(), POLL_MS);
  }
  document.addEventListener("visibilitychange", onVisibilityChange);
}

export function stopReviewsPolling(): void {
  started = false;
  if (pollTimer !== null) clearInterval(pollTimer);
  pollTimer = null;
  document.removeEventListener("visibilitychange", onVisibilityChange);
}

/** One-off refetch -- ReviewsView mounting and its manual refresh button
 *  both funnel through this instead of waiting for the next ambient tick. */
export function refreshReviews(): void {
  void fetchReviews();
}

/** One-off refetch of the watched-sources list -- ReviewsView mounting. Not
 *  needed after SourcesPanel's own mutations, since POST /reviews/sources
 *  already returns the updated list directly. */
export function refreshReviewSources(): void {
  void fetchSources();
}
