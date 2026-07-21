import { useEffect, useMemo } from "react";
import { useNavigate } from "react-router";
import { useShallow } from "zustand/react/shallow";
import { Inbox as InboxIcon } from "lucide-react";
import { selectInboxEvents, useInboxStore } from "../../state/inbox";
import { useTreeStore } from "../../state/tree";
import { apiClient } from "../../state/api";
import { InboxGroup } from "./InboxGroup";
import { EmptyState } from "../common/EmptyState";
import { UsageMeter } from "../shell/UsageMeter";
import type { Event, NodeID } from "../../gen/types";

interface Group {
  nodeId: NodeID;
  events: Event[];
  latest: string;
}

export function InboxView() {
  // See CommandPalette's identical usage for why useShallow is required here.
  const events = useInboxStore(useShallow(selectInboxEvents)); // already newest-first
  const nodesById = useTreeStore((s) => s.nodesById);
  const navigate = useNavigate();

  const groups = useMemo<Group[]>(() => {
    const byNode = new Map<NodeID, Event[]>();
    for (const e of events) {
      const list = byNode.get(e.node_id) ?? [];
      list.push(e);
      byNode.set(e.node_id, list);
    }
    return [...byNode.entries()]
      .map(([nodeId, evs]) => ({ nodeId, events: evs, latest: evs[0].created_at }))
      .sort((a, b) => b.latest.localeCompare(a.latest));
  }, [events]);

  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      if (e.target instanceof HTMLElement && (e.target.tagName === "INPUT" || e.target.tagName === "TEXTAREA")) {
        return;
      }
      if (e.metaKey || e.ctrlKey || e.altKey) return;
      const n = Number(e.key);
      if (Number.isInteger(n) && n >= 1 && n <= 9) {
        const el = document.getElementById(`inbox-group-${n - 1}`);
        if (el) {
          e.preventDefault();
          el.scrollIntoView({ behavior: "smooth", block: "start" });
        }
      }
    }
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, []);

  async function ackNode(nodeId: NodeID) {
    // Clear the badge instantly; the authoritative acked events arrive over
    // the WS moments later and confirm it.
    useInboxStore.getState().ackNodeOptimistic(nodeId);
    await apiClient.ackNode(nodeId).catch(() => {
      // Best-effort; a failed ack is reconciled on the next snapshot.
    });
  }

  // Statusbar owns UsageMeter on desktop (hidden there); mobile has no
  // statusbar, so the meter lives here instead, at the top of Inbox.
  const mobileUsageBar = <UsageMeter className="border-b border-border px-4 py-2 md:hidden" />;

  if (groups.length === 0) {
    return (
      <div className="flex h-full flex-col">
        {mobileUsageBar}
        <EmptyState
          icon={<InboxIcon size={28} strokeWidth={1.5} />}
          title="Inbox zero"
          description="Nothing needs your attention right now."
        />
      </div>
    );
  }

  return (
    <div className="h-full overflow-y-auto">
      {mobileUsageBar}
      <div className="sticky top-0 z-10 border-b border-border bg-canvas/90 px-5 py-3 backdrop-blur-sm">
        <h1 className="font-sans text-sm font-medium text-ink">Inbox</h1>
        <p className="mt-0.5 font-sans text-2xs text-ink-faint">
          {events.length} unacked across {groups.length} node{groups.length === 1 ? "" : "s"}
        </p>
      </div>
      <div className="divide-y divide-border">
        {groups.map((group, i) => (
          <InboxGroup
            key={group.nodeId}
            id={`inbox-group-${i}`}
            index={i}
            node={nodesById[group.nodeId]}
            events={group.events}
            onGo={() => navigate(`/n/${group.nodeId}`)}
            onAck={() => void ackNode(group.nodeId)}
          />
        ))}
      </div>
    </div>
  );
}
