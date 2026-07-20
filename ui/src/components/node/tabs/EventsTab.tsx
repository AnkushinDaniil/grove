import { useEffect, useMemo, useState } from "react";
import { History } from "lucide-react";
import { apiClient } from "../../../state/api";
import { useLiveEventsStore } from "../../../state/events";
import { EventRow } from "./EventRow";
import { EmptyState } from "../../common/EmptyState";
import type { Event, NodeID } from "../../../gen/types";

const RENDER_CAP = 500;

interface EventsTabProps {
  nodeId: NodeID;
}

export function EventsTab({ nodeId }: EventsTabProps) {
  const [history, setHistory] = useState<Event[] | null>(null);
  const live = useLiveEventsStore((s) => s.byNode[nodeId]) ?? [];

  useEffect(() => {
    let cancelled = false;
    setHistory(null);
    apiClient
      .getEvents(nodeId, undefined, RENDER_CAP)
      .then((events) => {
        if (!cancelled) setHistory(events);
      })
      .catch(() => {
        if (!cancelled) setHistory([]);
      });
    return () => {
      cancelled = true;
    };
  }, [nodeId]);

  const merged = useMemo(() => {
    if (!history) return [];
    const byId = new Map<string, Event>();
    for (const e of history) byId.set(e.id, e);
    for (const e of live) byId.set(e.id, e); // live entries win (e.g. fresher acked_at)
    return [...byId.values()].sort((a, b) => b.created_at.localeCompare(a.created_at)).slice(0, RENDER_CAP);
  }, [history, live]);

  if (history === null) {
    return <div className="p-4 text-2xs text-ink-faint">Loading events…</div>;
  }

  if (merged.length === 0) {
    return (
      <EmptyState
        icon={<History size={28} strokeWidth={1.5} />}
        title="No events yet"
        description="Activity will appear here once a session starts."
      />
    );
  }

  return (
    <div className="h-full overflow-y-auto p-2">
      <ul className="space-y-0.5">
        {merged.map((event) => (
          <EventRow key={event.id} event={event} />
        ))}
      </ul>
    </div>
  );
}
