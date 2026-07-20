import { apiClient } from "./api";
import { useUsageStore } from "./usage";
import { useConnectionStore } from "./connection";

const POLL_MS = 60_000;

let pollTimer: ReturnType<typeof setInterval> | null = null;
let unsubscribeConnection: (() => void) | null = null;
let started = false;

async function fetchNow(): Promise<void> {
  const { window, setProfiles, setLoading } = useUsageStore.getState();
  setLoading(true);
  try {
    const res = await apiClient.getUsage(window);
    setProfiles(res.profiles);
  } catch {
    // Non-critical widget: keep the last-known profiles rather than
    // clearing them on a transient fetch failure.
  } finally {
    setLoading(false);
  }
}

/** Fetches immediately, then every 60s, and again on every reconnect (a
 *  transition to "open") so the meter recovers promptly after a network
 *  blip instead of waiting out the rest of the poll interval. */
export function startUsagePolling(): void {
  if (started) return;
  started = true;

  void fetchNow();
  pollTimer = setInterval(() => void fetchNow(), POLL_MS);

  unsubscribeConnection = useConnectionStore.subscribe((state, prevState) => {
    if (state.status === "open" && prevState.status !== "open") void fetchNow();
  });
}

export function stopUsagePolling(): void {
  started = false;
  if (pollTimer !== null) clearInterval(pollTimer);
  pollTimer = null;
  unsubscribeConnection?.();
  unsubscribeConnection = null;
}

/** Cycles 5h <-> week and refetches immediately, rather than waiting for
 *  the next scheduled poll tick. */
export function cycleUsageWindow(): void {
  const next = useUsageStore.getState().window === "5h" ? "week" : "5h";
  useUsageStore.getState().setWindow(next);
  void fetchNow();
}
