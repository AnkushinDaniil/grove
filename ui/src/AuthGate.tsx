import { useEffect, useState } from "react";
import type { ReactNode } from "react";
import { apiClient } from "./state/api";
import { extractHashToken, stripHashToken } from "./state/auth";
import { startStateSocket } from "./state/stateSocketBootstrap";
import { startUsagePolling } from "./state/usagePolling";
import { startReviewsPolling } from "./state/reviewsPolling";

type AuthPhase = "checking" | "authed" | "unauthorized" | "error";

interface AuthGateProps {
  children: ReactNode;
}

/**
 * Resolves the token handoff / existing-cookie check from docs/API.md
 * before rendering the app, so REST calls and the state socket never race
 * an unauthenticated first paint. Mock mode skips this entirely.
 */
export function AuthGate({ children }: AuthGateProps) {
  const [phase, setPhase] = useState<AuthPhase>(import.meta.env.VITE_MOCK === "1" ? "authed" : "checking");

  useEffect(() => {
    if (import.meta.env.VITE_MOCK === "1") {
      void startStateSocket();
      startUsagePolling();
      startReviewsPolling();
      return;
    }

    let cancelled = false;
    (async () => {
      try {
        const token = extractHashToken(window.location.hash);
        if (token) {
          await apiClient.authSession(token);
          stripHashToken();
          if (cancelled) return;
          setPhase("authed");
          void startStateSocket();
          startUsagePolling();
          startReviewsPolling();
          return;
        }
        const ok = await apiClient.authMe();
        if (cancelled) return;
        setPhase(ok ? "authed" : "unauthorized");
        if (ok) {
          void startStateSocket();
          startUsagePolling();
          startReviewsPolling();
        }
      } catch {
        if (!cancelled) setPhase("error");
      }
    })();

    return () => {
      cancelled = true;
    };
  }, []);

  if (phase === "checking") {
    return (
      <div className="flex h-dvh w-full items-center justify-center bg-canvas text-xs text-ink-faint">
        Connecting to grove…
      </div>
    );
  }

  if (phase === "unauthorized" || phase === "error") {
    return (
      <div className="flex h-dvh w-full flex-col items-center justify-center gap-2 bg-canvas px-6 text-center">
        <p className="text-sm font-medium text-ink">
          {phase === "unauthorized" ? "Not authenticated" : "Couldn't reach the grove daemon"}
        </p>
        <p className="max-w-sm font-sans text-xs text-ink-faint">
          {phase === "unauthorized"
            ? "Open this page from a fresh `grove serve` link, which includes a one-time token."
            : "Check that the daemon is running and reachable, then reload."}
        </p>
      </div>
    );
  }

  return <>{children}</>;
}
