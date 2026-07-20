import { create } from "zustand";

const MAX_MOUNTED = 4;

// Tracks which session ids are considered "warm" (MRU order, capped at 4)
// per docs/DESIGN.md's terminal mount-pool economy. In the current
// single-pane-at-a-time Terminal tab (only one node's terminal is ever
// visible), this naturally stays well under the cap; the LRU bookkeeping is
// real and tested so a future multi-pane view can key off it directly.
interface TerminalPoolState {
  mounted: string[];
  touch: (sessionId: string) => void;
  isMounted: (sessionId: string) => boolean;
}

export const useTerminalPoolStore = create<TerminalPoolState>((set, get) => ({
  mounted: [],
  touch: (sessionId) => {
    set((state) => {
      const rest = state.mounted.filter((id) => id !== sessionId);
      return { mounted: [sessionId, ...rest].slice(0, MAX_MOUNTED) };
    });
  },
  isMounted: (sessionId) => get().mounted.includes(sessionId),
}));
