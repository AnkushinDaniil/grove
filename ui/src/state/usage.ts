import { create } from "zustand";
import type { UsageWindow, UsageWindowKind } from "../gen/types";

interface UsageState {
  window: UsageWindowKind;
  profiles: UsageWindow[];
  loading: boolean;
  setWindow: (window: UsageWindowKind) => void;
  setProfiles: (profiles: UsageWindow[]) => void;
  setLoading: (loading: boolean) => void;
}

export const useUsageStore = create<UsageState>((set) => ({
  window: "5h",
  profiles: [],
  loading: false,
  setWindow: (window) => set({ window }),
  setProfiles: (profiles) => set({ profiles }),
  setLoading: (loading) => set({ loading }),
}));
