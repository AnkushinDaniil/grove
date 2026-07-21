import { useEffect, useState } from "react";
import { useParams } from "react-router";
import clsx from "clsx";
import { AlertTriangle } from "lucide-react";
import { useTreeStore } from "../../state/tree";
import { useRollup } from "../../hooks/useRollup";
import { apiClient } from "../../state/api";
import { ATTENTION_LABEL, FOCUS_RING } from "../../lib/constants";
import { Breadcrumb } from "./Breadcrumb";
import { NodeTitle } from "./NodeTitle";
import { StatusChip } from "./StatusChip";
import { DriverProfileChips } from "./DriverProfileChips";
import { WorkDirChip } from "./WorkDirChip";
import { ActionsRow } from "./ActionsRow";
import { StartHeadlessPopover } from "./StartHeadlessPopover";
import { TerminalTab } from "./tabs/TerminalTab";
import { EventsTab } from "./tabs/EventsTab";
import { ChildrenTab } from "./tabs/ChildrenTab";
import { EmptyState } from "../common/EmptyState";
import type { NodeID, SessionStatus } from "../../gen/types";

type TabId = "terminal" | "events" | "children";

const TABS: { id: TabId; label: string }[] = [
  { id: "terminal", label: "Terminal" },
  { id: "events", label: "Events" },
  { id: "children", label: "Children" },
];

function isSessionTerminal(status: SessionStatus): boolean {
  return status === "exited" || status === "failed" || status === "interrupted";
}

export function NodeView() {
  const { id } = useParams<{ id: NodeID }>();
  const node = useTreeStore((s) => (id ? s.nodesById[id] : undefined));
  const sessionsById = useTreeStore((s) => s.sessionsById);
  const loaded = useTreeStore((s) => s.loaded);
  const rollup = useRollup(id ?? "");

  const [tab, setTab] = useState<TabId>("terminal");
  const [headlessOpen, setHeadlessOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);

  useEffect(() => {
    setTab("terminal");
    setHeadlessOpen(false);
    setActionError(null);
  }, [id]);

  // Viewing is seeing: once the node has been open for a beat, its attention
  // badge auto-clears. The status chip (e.g. awaiting_input) still tells the
  // truth about the session — attention only tracks "unseen".
  const attention = node?.attention ?? "none";
  useEffect(() => {
    if (!id || attention === "none") return;
    const timer = setTimeout(() => {
      void apiClient.ackNode(id).catch(() => {
        // Best-effort: a failed auto-ack just leaves the badge until the next view.
      });
    }, 1200);
    return () => clearTimeout(timer);
  }, [id, attention]);

  if (!id) return null;

  if (!node) {
    return (
      <EmptyState
        title={loaded ? "Node not found" : "Loading…"}
        description={loaded ? "It may have been archived, or the link is stale." : undefined}
      />
    );
  }

  const session = node.current_session_id ? sessionsById[node.current_session_id] : undefined;
  const activeSession = session && !isSessionTerminal(session.status) ? session : undefined;

  // Every user action funnels through this wrapper so a failed request is
  // always surfaced — a silently swallowed rejection reads as a dead button.
  async function runAction(fn: () => Promise<unknown>) {
    setBusy(true);
    setActionError(null);
    try {
      await fn();
    } catch (err) {
      setActionError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  async function startPty() {
    await runAction(() => apiClient.createSession(node!.id, { mode: "pty" }));
  }

  async function startHeadless(prompt: string) {
    try {
      await runAction(() => apiClient.createSession(node!.id, { mode: "headless", prompt }));
    } finally {
      setHeadlessOpen(false);
    }
  }

  async function stopSession() {
    if (!activeSession) return;
    await runAction(() => apiClient.stopSession(activeSession.id));
  }

  async function resumePty(driverSessionId: string) {
    await runAction(() => apiClient.createSession(node!.id, { mode: "pty", resume_id: driverSessionId }));
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="shrink-0 space-y-2.5 border-b border-border px-5 py-3">
        <Breadcrumb nodeId={id} />
        <div className="flex flex-wrap items-center gap-2">
          <NodeTitle node={node} />
          <StatusChip status={node.status} />
          {node.attention !== "none" && (
            <span className="inline-flex items-center gap-1.5 rounded-md border border-accent/30 bg-accent-soft px-2 py-1 text-2xs font-medium text-accent">
              <AlertTriangle size={11} />
              {ATTENTION_LABEL[node.attention]}
            </span>
          )}
          <DriverProfileChips nodeId={id} />
          <WorkDirChip nodeId={id} />
          {rollup.total > 0 && (
            <span className="text-2xs text-ink-faint">
              {rollup.total} descendant{rollup.total === 1 ? "" : "s"}
              {rollup.attentionCount > 0 && ` · ${rollup.attentionCount} need attention`}
            </span>
          )}
        </div>
        {node.brief && <p className="max-w-2xl font-sans text-xs text-ink-muted">{node.brief}</p>}
        <ActionsRow
          node={node}
          activeSession={activeSession}
          busy={busy}
          onStartPty={() => void startPty()}
          onOpenHeadless={() => setHeadlessOpen(true)}
          onStopSession={() => void stopSession()}
        />
        {headlessOpen && (
          <StartHeadlessPopover onStart={(prompt) => void startHeadless(prompt)} onCancel={() => setHeadlessOpen(false)} />
        )}
        {actionError && (
          <div className="flex items-center gap-2 rounded-md border border-status-failed/40 bg-status-failed/10 px-2.5 py-1.5 text-xs text-status-failed">
            <AlertTriangle size={12} className="shrink-0" />
            <span className="min-w-0 flex-1 break-words">{actionError}</span>
            <button
              type="button"
              onClick={() => setActionError(null)}
              className="shrink-0 text-2xs text-ink-faint hover:text-ink"
            >
              dismiss
            </button>
          </div>
        )}
      </div>

      <div className="flex shrink-0 items-center gap-1 border-b border-border px-3 pt-1.5">
        {TABS.map((t) => (
          <button
            key={t.id}
            type="button"
            onClick={() => setTab(t.id)}
            className={clsx(
              "min-h-11 rounded-t-md px-3 py-1.5 text-xs font-medium transition-colors md:min-h-0",
              tab === t.id
                ? "border-b-2 border-accent text-ink"
                : "border-b-2 border-transparent text-ink-faint hover:text-ink",
              FOCUS_RING,
            )}
          >
            {t.label}
          </button>
        ))}
      </div>

      <div className="min-h-0 flex-1">
        {tab === "terminal" && (
          <TerminalTab
            node={node}
            latestSession={session}
            activeSession={activeSession}
            onStartPty={() => void startPty()}
            onOpenHeadless={() => setHeadlessOpen(true)}
            onResume={(id) => void resumePty(id)}
          />
        )}
        {tab === "events" && <EventsTab nodeId={id} />}
        {tab === "children" && <ChildrenTab node={node} />}
      </div>
    </div>
  );
}
