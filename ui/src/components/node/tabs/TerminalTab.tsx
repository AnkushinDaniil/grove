import { lazy, Suspense } from "react";
import { Terminal as TerminalIcon } from "lucide-react";
import { EmptyState } from "../../common/EmptyState";
import { PromptInputBar } from "../PromptInputBar";
import type { Node, Session } from "../../../gen/types";

// xterm + its addons are a meaningful chunk of the bundle and are only ever
// needed once a Terminal tab with a live pty actually renders.
const XtermHost = lazy(() => import("../../../terminal/XtermHost").then((m) => ({ default: m.XtermHost })));

interface TerminalTabProps {
  node: Node;
  session: Session | undefined;
  onStartPty: () => void;
  onOpenHeadless: () => void;
}

export function TerminalTab({ node, session, onStartPty, onOpenHeadless }: TerminalTabProps) {
  if (!session) {
    return (
      <EmptyState
        icon={<TerminalIcon size={28} strokeWidth={1.5} />}
        title="No active session"
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

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="min-h-0 flex-1">
        {session.mode === "pty" ? (
          <Suspense fallback={<div className="p-3 text-2xs text-ink-faint">Loading terminal…</div>}>
            <XtermHost key={session.id} sessionId={session.id} />
          </Suspense>
        ) : (
          <EmptyState
            icon={<TerminalIcon size={28} strokeWidth={1.5} />}
            title="Headless session running"
            description="No attached terminal for headless sessions -- watch progress in the Events tab."
          />
        )}
      </div>
      <PromptInputBar nodeId={node.id} />
    </div>
  );
}
