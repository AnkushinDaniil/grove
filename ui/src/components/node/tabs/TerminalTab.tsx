import { lazy, Suspense } from "react";
import { RotateCcw, Terminal as TerminalIcon } from "lucide-react";
import { EmptyState } from "../../common/EmptyState";
import { PromptInputBar } from "../PromptInputBar";
import type { Node, Session } from "../../../gen/types";

// xterm + its addons are a meaningful chunk of the bundle and are only ever
// needed once a Terminal tab with a pty actually renders.
const XtermHost = lazy(() => import("../../../terminal/XtermHost").then((m) => ({ default: m.XtermHost })));

interface TerminalTabProps {
  node: Node;
  /** Latest session bound to the node, regardless of state. */
  latestSession: Session | undefined;
  /** The latest session only while it is still live. */
  activeSession: Session | undefined;
  onStartPty: () => void;
  onOpenHeadless: () => void;
  /** Start a new PTY session resuming the given driver conversation. */
  onResume: (driverSessionId: string) => void;
}

const ENDED_LABEL: Record<string, string> = {
  exited: "exited",
  failed: "failed",
  interrupted: "interrupted — the daemon restarted while it was running",
};

export function TerminalTab({ node, latestSession, activeSession, onStartPty, onOpenHeadless, onResume }: TerminalTabProps) {
  if (!latestSession) {
    return (
      <EmptyState
        icon={<TerminalIcon size={28} strokeWidth={1.5} />}
        title="No session yet"
        description="Start a PTY session to attach a live terminal, or run headless with an initial prompt."
        action={
          <div className="flex gap-2">
            <button
              type="button"
              onClick={onStartPty}
              className="rounded-md bg-accent px-3 py-1.5 text-xs font-medium text-accent-ink hover:bg-accent-strong"
            >
              Start PTY session
            </button>
            <button
              type="button"
              onClick={onOpenHeadless}
              className="rounded-md border border-border-strong px-3 py-1.5 text-xs text-ink-muted hover:bg-hover hover:text-ink"
            >
              Start headless…
            </button>
          </div>
        }
      />
    );
  }

  if (activeSession) {
    return (
      <div className="flex h-full min-h-0 flex-col">
        <div className="min-h-0 flex-1">
          {activeSession.mode === "pty" ? (
            <Suspense fallback={<div className="p-3 text-2xs text-ink-faint">Loading terminal…</div>}>
              <XtermHost key={activeSession.id} sessionId={activeSession.id} />
            </Suspense>
          ) : (
            <EmptyState
              icon={<TerminalIcon size={28} strokeWidth={1.5} />}
              title="Headless session running"
              description="No attached terminal for headless sessions -- watch progress in the Events tab."
            />
          )}
        </div>
        {/* PTY sessions have the CLI's own input inside the terminal — a second
            send box is just confusing. The bar is load-bearing for headless. */}
        {activeSession.mode !== "pty" && <PromptInputBar nodeId={node.id} />}
      </div>
    );
  }

  // Ended session: keep the scrollback visible (the terminal socket serves a
  // replay for finished sessions) and offer resume — the CLI conversation
  // survives grove restarts and can continue in a fresh PTY.
  const endedLabel = ENDED_LABEL[latestSession.status] ?? latestSession.status;
  const canResume = latestSession.driver_session_id !== "";
  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex shrink-0 flex-wrap items-center gap-2 border-b border-border bg-raised px-3 py-2 text-xs text-ink-muted">
        <span className="min-w-0 flex-1">
          Session {endedLabel}
          {latestSession.exit_code != null && ` (exit ${latestSession.exit_code})`}
        </span>
        <button
          type="button"
          onClick={() => onResume(latestSession.driver_session_id)}
          disabled={!canResume}
          title={
            canResume
              ? "Start a new terminal continuing this conversation"
              : "This session predates hook wiring, so its conversation id is unknown — run the CLI's own --continue in the working directory instead"
          }
          className="inline-flex items-center gap-1.5 rounded-md bg-accent px-2.5 py-1 text-xs font-medium text-accent-ink hover:bg-accent-strong disabled:pointer-events-none disabled:opacity-40"
        >
          <RotateCcw size={11} />
          Resume session
        </button>
        <button
          type="button"
          onClick={onStartPty}
          className="rounded-md border border-border-strong px-2.5 py-1 text-xs text-ink-muted hover:bg-hover hover:text-ink"
        >
          New session
        </button>
      </div>
      <div className="min-h-0 flex-1">
        {latestSession.mode === "pty" ? (
          <Suspense fallback={<div className="p-3 text-2xs text-ink-faint">Loading scrollback…</div>}>
            <XtermHost key={latestSession.id} sessionId={latestSession.id} />
          </Suspense>
        ) : (
          <EmptyState
            icon={<TerminalIcon size={28} strokeWidth={1.5} />}
            title={`Headless session ${endedLabel}`}
            description="Its output is in the Events tab. Resume continues the same conversation in a live terminal."
          />
        )}
      </div>
    </div>
  );
}
