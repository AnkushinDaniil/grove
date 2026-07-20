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

  useEffect(() => {
    setTab("terminal");
    setHeadlessOpen(false);
  }, [id]);

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

  async function startPty() {
    setBusy(true);
    try {
      await apiClient.createSession(node!.id, { mode: "pty" });
    } finally {
      setBusy(false);
    }
  }

  async function startHeadless(prompt: string) {
    setBusy(true);
    try {
      await apiClient.createSession(node!.id, { mode: "headless", prompt });
    } finally {
      setBusy(false);
      setHeadlessOpen(false);
    }
  }

  async function stopSession() {
    if (!activeSession) return;
    setBusy(true);
    try {
      await apiClient.stopSession(activeSession.id);
    } finally {
      setBusy(false);
    }
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
            session={activeSession}
            onStartPty={() => void startPty()}
            onOpenHeadless={() => setHeadlessOpen(true)}
          />
        )}
        {tab === "events" && <EventsTab nodeId={id} />}
        {tab === "children" && <ChildrenTab node={node} />}
      </div>
    </div>
  );
}
