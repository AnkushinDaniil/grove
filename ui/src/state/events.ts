import { create } from "zustand";
import type { Event, NodeID } from "../gen/types";

const MAX_PER_NODE = 200;

/** Bounded per-node ring buffer of events observed live over /ws/state,
 *  newest first. The Events tab fetches history via REST (which supports
 *  proper pagination/limit) and merges anything newer than its last-seen id
 *  from here, so this store only needs to cover "since the tab was last
 *  open," not full history. */
interface LiveEventsState {
  byNode: Record<NodeID, Event[]>;
  publish: (events: Event[]) => void;
  reset: () => void;
}

export const useLiveEventsStore = create<LiveEventsState>((set) => ({
  byNode: {},

  publish: (events) => {
    if (events.length === 0) return;
    set((state) => {
      const byNode = { ...state.byNode };
      for (const e of events) {
        const existing = byNode[e.node_id] ?? [];
        if (existing.some((x) => x.id === e.id)) continue;
        byNode[e.node_id] = [e, ...existing].slice(0, MAX_PER_NODE);
      }
      return { byNode };
    });
  },

  reset: () => set({ byNode: {} }),
}));
