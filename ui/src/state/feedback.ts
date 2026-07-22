import { create } from "zustand";
import type { Feedback, FeedbackStatusFilter } from "../gen/types";

interface FeedbackState {
  items: Feedback[];
  filter: FeedbackStatusFilter;
  loading: boolean;
  /** True once the first listFeedback() response for the current filter has
   *  landed -- distinguishes "loading" from "loaded, genuinely empty". */
  loaded: boolean;
  lastError: string | null;

  setItems: (items: Feedback[]) => void;
  /** Inserts or replaces one item in place -- used for optimistic local
   *  updates after create/resolve, mirroring addDraftLocal/addCommentLocal
   *  elsewhere in the app. */
  upsert: (item: Feedback) => void;
  setFilter: (filter: FeedbackStatusFilter) => void;
  setLoading: (loading: boolean) => void;
  setError: (message: string | null) => void;
  reset: () => void;
}

const initial = {
  items: [] as Feedback[],
  filter: "open" as FeedbackStatusFilter,
  loading: false,
  loaded: false,
  lastError: null as string | null,
};

export const useFeedbackStore = create<FeedbackState>((set) => ({
  ...initial,

  setItems: (items) => set({ items, loaded: true, lastError: null }),
  upsert: (item) => set((s) => ({ items: [item, ...s.items.filter((f) => f.id !== item.id)] })),
  setFilter: (filter) => set({ filter }),
  setLoading: (loading) => set({ loading }),
  setError: (lastError) => set({ lastError }),

  reset: () => set({ ...initial }),
}));

/** Client-side filter over the store's items -- lets an optimistic upsert
 *  (e.g. resolving an item while viewing "open") disappear from the visible
 *  list immediately, without waiting for a refetch. */
export function selectVisibleFeedback(state: Pick<FeedbackState, "items" | "filter">): Feedback[] {
  const { items, filter } = state;
  if (filter === "all") return items;
  return items.filter((f) => (filter === "open" ? !f.resolved_at : Boolean(f.resolved_at)));
}
