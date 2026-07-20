import { create } from "zustand";
import type { Event, EventID, NodeID } from "../gen/types";

// The inbox is "unacked attention events." Acking is a per-node operation
// (POST /nodes/{id}/ack -- there is no per-event ack endpoint), so this
// store optimistically marks every unacked attention event for a node as
// acked in one shot; the real acked_at lands moments later via a WS delta
// that re-publishes the affected event records.
interface InboxState {
  eventsById: Record<EventID, Event>;
  setInitial: (events: Event[]) => void;
  upsertMany: (events: Event[]) => void;
  ackNodeOptimistic: (nodeId: NodeID) => void;
  reset: () => void;
}

export const useInboxStore = create<InboxState>((set) => ({
  eventsById: {},

  setInitial: (events) => {
    const eventsById: Record<EventID, Event> = {};
    for (const e of events) eventsById[e.id] = e;
    set({ eventsById });
  },

  upsertMany: (events) => {
    const relevant = events.filter((e) => e.requires_attention);
    if (relevant.length === 0) return;
    set((state) => {
      const eventsById = { ...state.eventsById };
      for (const e of relevant) eventsById[e.id] = e;
      return { eventsById };
    });
  },

  ackNodeOptimistic: (nodeId) => {
    const ackedAt = new Date().toISOString();
    set((state) => {
      let changed = false;
      const eventsById = { ...state.eventsById };
      for (const [id, e] of Object.entries(eventsById)) {
        if (e.node_id === nodeId && e.requires_attention && !e.acked_at) {
          eventsById[id] = { ...e, acked_at: ackedAt };
          changed = true;
        }
      }
      return changed ? { eventsById } : state;
    });
  },

  reset: () => set({ eventsById: {} }),
}));

/** Unacked attention events, newest first. */
export function selectInboxEvents(state: Pick<InboxState, "eventsById">): Event[] {
  return Object.values(state.eventsById)
    .filter((e) => e.requires_attention && !e.acked_at)
    .sort((a, b) => b.created_at.localeCompare(a.created_at));
}

/** Unacked attention events for one node -- used for the tree rail's
 *  per-node attention badge count. */
export function selectInboxCountForNode(
  state: Pick<InboxState, "eventsById">,
  nodeId: NodeID,
): number {
  let count = 0;
  for (const e of Object.values(state.eventsById)) {
    if (e.node_id === nodeId && e.requires_attention && !e.acked_at) count += 1;
  }
  return count;
}
