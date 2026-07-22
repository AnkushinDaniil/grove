import { create } from "zustand";
import type { NodeID, StatsRange, StatsResponse } from "../gen/types";

interface StatsState {
  range: StatsRange;
  scope: NodeID | ""; // "" = whole workspace
  data: StatsResponse | null;
  loading: boolean;
  /** True once the first getStats() response has landed -- distinguishes
   *  "still loading" from "loaded but genuinely empty" for skeleton vs.
   *  empty-state rendering. */
  loaded: boolean;
  lastError: string | null;

  setRange: (range: StatsRange) => void;
  setScope: (scope: NodeID | "") => void;
  setData: (data: StatsResponse) => void;
  setLoading: (loading: boolean) => void;
  setError: (message: string | null) => void;
  reset: () => void;
}

const initial = {
  range: "7d" as StatsRange,
  scope: "" as NodeID | "",
  data: null as StatsResponse | null,
  loading: false,
  loaded: false,
  lastError: null as string | null,
};

export const useStatsStore = create<StatsState>((set) => ({
  ...initial,

  setRange: (range) => set({ range }),
  setScope: (scope) => set({ scope }),
  setData: (data) => set({ data, loaded: true, lastError: null }),
  setLoading: (loading) => set({ loading }),
  setError: (lastError) => set({ lastError }),

  reset: () => set({ ...initial }),
}));
