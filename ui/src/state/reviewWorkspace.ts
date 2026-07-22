import { create } from "zustand";
import { apiClient } from "./api";
import type { DraftComment, PRReview } from "../gen/types";

interface ReviewWorkspaceState {
  dir: string | null;
  pr: number | null;
  review: PRReview | null;
  drafts: DraftComment[];
  loading: boolean;
  /** True once the load for the current (dir, pr) has settled (success or
   *  error) -- distinguishes "still loading" from "loaded, PR not found". */
  loaded: boolean;
  error: string | null;

  startLoad: (dir: string, pr: number) => void;
  setLoaded: (review: PRReview, drafts: DraftComment[]) => void;
  setLoadError: (message: string) => void;
  setReview: (review: PRReview) => void;
  addDraftLocal: (draft: DraftComment) => void;
  removeDraftLocal: (id: string) => void;
  reset: () => void;
}

const initial = {
  dir: null as string | null,
  pr: null as number | null,
  review: null as PRReview | null,
  drafts: [] as DraftComment[],
  loading: false,
  loaded: false,
  error: null as string | null,
};

export const useReviewWorkspaceStore = create<ReviewWorkspaceState>((set) => ({
  ...initial,

  startLoad: (dir, pr) => set({ ...initial, dir, pr, loading: true }),
  setLoaded: (review, drafts) => set({ review, drafts, loaded: true, loading: false }),
  setLoadError: (message) => set({ error: message, loaded: true, loading: false }),
  setReview: (review) => set({ review }),
  addDraftLocal: (draft) => set((s) => ({ drafts: [...s.drafts, draft] })),
  removeDraftLocal: (id) => set((s) => ({ drafts: s.drafts.filter((d) => d.id !== id) })),

  reset: () => set({ ...initial }),
}));

/** Loads a PR's review + its pending drafts together. ReviewWorkspace calls
 *  this once per (dir, pr) route param change; SubmitBar calls it again
 *  after a successful submit to pick up the server's authoritative state
 *  (new threads, cleared drafts, updated review_decision) instead of
 *  guessing it client-side -- submitReview's wire response is just {url}. */
export async function loadReviewWorkspace(dir: string, pr: number): Promise<void> {
  useReviewWorkspaceStore.getState().startLoad(dir, pr);
  try {
    const [review, draftsRes] = await Promise.all([
      apiClient.getPRReview(dir, pr),
      apiClient.getReviewDrafts(dir, pr),
    ]);
    useReviewWorkspaceStore.getState().setLoaded(review, draftsRes.drafts);
  } catch (err) {
    useReviewWorkspaceStore.getState().setLoadError(err instanceof Error ? err.message : String(err));
  }
}
