import { create } from "zustand";
import type { ReviewRepo } from "../gen/types";

interface ReviewsState {
  repos: ReviewRepo[];
  errors: string[];
  login: string;
  loading: boolean;
  /** True once the first getReviews() response has landed -- distinguishes
   *  "no repos yet" from "haven't loaded yet" for empty-state rendering. */
  loaded: boolean;
  /** Transport-level failure of the last getReviews() call itself, distinct
   *  from `errors` (which are per-repo messages the daemon reports inside a
   *  200 response). */
  lastError: string | null;
  dismissedErrors: Set<string>;
  /** Watched directories (GET/POST /reviews/sources). null = not yet loaded. */
  sourceDirs: string[] | null;

  setData: (data: { login: string; repos: ReviewRepo[]; errors: string[] }) => void;
  setLoading: (loading: boolean) => void;
  setFetchError: (message: string | null) => void;
  dismissError: (message: string) => void;
  setSourceDirs: (dirs: string[]) => void;
  reset: () => void;
}

const initial = {
  repos: [] as ReviewRepo[],
  errors: [] as string[],
  login: "",
  loading: false,
  loaded: false,
  lastError: null as string | null,
  sourceDirs: null as string[] | null,
};

export const useReviewsStore = create<ReviewsState>((set) => ({
  ...initial,
  dismissedErrors: new Set<string>(),

  setData: ({ login, repos, errors }) => set({ login, repos, errors, loaded: true, lastError: null }),
  setLoading: (loading) => set({ loading }),
  setFetchError: (message) => set({ lastError: message }),
  dismissError: (message) => set((s) => ({ dismissedErrors: new Set(s.dismissedErrors).add(message) })),
  setSourceDirs: (dirs) => set({ sourceDirs: dirs }),

  reset: () => set({ ...initial, dismissedErrors: new Set() }),
}));

/** Total PRs actionable across every watched repo -- the nav badge count
 *  shown next to the inbox icon in TreeRail and on BottomTabs' Reviews tab. */
export function selectNeedsAttentionCount(state: Pick<ReviewsState, "repos">): number {
  let count = 0;
  for (const repo of state.repos) {
    count += repo.buckets.needs_review.length + repo.buckets.re_review.length;
  }
  return count;
}

/** Server-reported per-repo fetch errors, minus ones the user already
 *  dismissed this session -- a poll that still reports the same message
 *  stays dismissed rather than nagging again every 120s. */
export function selectVisibleErrors(state: Pick<ReviewsState, "errors" | "dismissedErrors">): string[] {
  return state.errors.filter((e) => !state.dismissedErrors.has(e));
}
