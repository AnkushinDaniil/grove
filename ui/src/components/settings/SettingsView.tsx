import { useCallback, useEffect, useState } from "react";
import { AlertTriangle } from "lucide-react";
import clsx from "clsx";
import { Switch } from "../common/Switch";
import { FOCUS_RING } from "../../lib/constants";
import { disablePush, enablePush, getPushStatus, showTestNotification } from "../../state/push";
import type { PushStatus } from "../../state/push";

const STATUS_MESSAGE: Record<PushStatus, string> = {
  unsupported: "Push notifications aren't supported in this browser.",
  insecure: "Open grove over your Tailscale HTTPS address to enable push on this device.",
  denied: "Notifications are blocked for grove -- enable them in your browser's site settings.",
  default: "Get notified when a node needs your attention, even when this tab is closed.",
  subscribed: "You'll get a push notification whenever a node needs your attention.",
};

/**
 * Device and browser preferences (GET /push/key, POST /push/subscribe,
 * POST /push/unsubscribe via state/push.ts). Currently just the push
 * notifications toggle; a natural home for more device-local settings later.
 * Owns its own fetch + mutation state, same convention as ProfilesView --
 * there is no cross-view settings store to share.
 */
export function SettingsView() {
  const [status, setStatus] = useState<PushStatus | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [testMessage, setTestMessage] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setStatus(await getPushStatus());
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  async function toggle() {
    if (busy || status === null) return;
    setBusy(true);
    setError(null);
    setTestMessage(null);
    try {
      if (status === "subscribed") {
        await disablePush();
      } else {
        await enablePush();
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      await refresh();
      setBusy(false);
    }
  }

  async function runTest() {
    setError(null);
    setTestMessage(null);
    try {
      await showTestNotification();
      setTestMessage("Test notification sent -- check your notification tray.");
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  const loading = status === null;
  const locked = status === "unsupported" || status === "insecure" || status === "denied";
  const checked = status === "subscribed";

  return (
    <div className="h-full overflow-y-auto px-5 py-4">
      <div className="mx-auto max-w-2xl space-y-4">
        <div>
          <h1 className="font-sans text-sm font-medium text-ink">Settings</h1>
          <p className="mt-0.5 font-sans text-2xs text-ink-faint">
            Device and browser preferences for this grove client.
          </p>
        </div>

        <section className="space-y-3 rounded-md border border-border bg-canvas px-3 py-3">
          <h2 className="text-2xs font-medium tracking-wide text-ink-faint uppercase">Notifications</h2>

          <div className="flex items-start justify-between gap-3">
            <div className="min-w-0">
              <p className="font-sans text-xs font-medium text-ink">Push notifications</p>
              <p className="mt-1 max-w-sm font-sans text-2xs text-ink-faint">
                {status === null ? "Checking this browser's push support…" : STATUS_MESSAGE[status]}
              </p>
            </div>
            <Switch
              checked={checked}
              disabled={loading || locked || busy}
              onChange={() => void toggle()}
              label="Enable push notifications"
            />
          </div>

          {error && (
            <div
              role="alert"
              className="flex items-start gap-2 rounded-md border border-status-failed/40 bg-status-failed/10 px-2.5 py-1.5 text-xs text-status-failed"
            >
              <AlertTriangle size={12} className="mt-0.5 shrink-0" />
              <span className="min-w-0 flex-1 break-words">{error}</span>
            </div>
          )}

          {checked && (
            <div className="flex flex-wrap items-center gap-2 border-t border-border pt-3">
              <button
                type="button"
                onClick={() => void runTest()}
                className={clsx(
                  "rounded-md border border-border-strong px-2.5 py-1.5 text-xs text-ink-muted hover:bg-hover hover:text-ink",
                  FOCUS_RING,
                )}
              >
                Send test notification
              </button>
              <span className="text-2xs text-ink-disabled">Local only -- doesn't test server delivery.</span>
            </div>
          )}
          {testMessage && <p className="text-2xs text-status-running">{testMessage}</p>}
        </section>
      </div>
    </div>
  );
}
