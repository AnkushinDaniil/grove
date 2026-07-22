import { create } from "zustand";
import { apiClient } from "./api";
import type { WorktreeComment, WorktreeReview } from "../gen/types";

interface WorktreeReviewState {
  node: string | null;
  repo: string | null;
  review: WorktreeReview | null;
  comments: WorktreeComment[];
  loading: boolean;
  /** True once the load for the current (node, repo) has settled (success or
   *  error) -- distinguishes "still loading" from "loaded, nothing there". */
  loaded: boolean;
  error: string | null;

  startLoad: (node: string, repo: string) => void;
  setLoaded: (review: WorktreeReview, comments: WorktreeComment[]) => void;
  setLoadError: (message: string) => void;
  addCommentLocal: (comment: WorktreeComment) => void;
  removeCommentLocal: (id: string) => void;
  reset: () => void;
}

const initial = {
  node: null as string | null,
  repo: null as string | null,
  review: null as WorktreeReview | null,
  comments: [] as WorktreeComment[],
  loading: false,
  loaded: false,
  error: null as string | null,
};

export const useWorktreeReviewStore = create<WorktreeReviewState>((set) => ({
  ...initial,

  startLoad: (node, repo) => set({ ...initial, node, repo, loading: true }),
  setLoaded: (review, comments) => set({ review, comments, loaded: true, loading: false }),
  setLoadError: (message) => set({ error: message, loaded: true, loading: false }),
  addCommentLocal: (comment) => set((s) => ({ comments: [...s.comments, comment] })),
  removeCommentLocal: (id) => set((s) => ({ comments: s.comments.filter((c) => c.id !== id) })),

  reset: () => set({ ...initial }),
}));

/** Loads a node's worktree diff + its local comments together. Call again
 *  after a merge to pick up the daemon's authoritative post-merge state
 *  (an empty diff once the tree is clean). */
export async function loadWorktreeReview(node: string, repo: string): Promise<void> {
  useWorktreeReviewStore.getState().startLoad(node, repo);
  try {
    const [review, commentsRes] = await Promise.all([
      apiClient.getWorktreeReview(node, repo),
      apiClient.getWorktreeComments(node, repo),
    ]);
    useWorktreeReviewStore.getState().setLoaded(review, commentsRes.comments);
  } catch (err) {
    useWorktreeReviewStore.getState().setLoadError(err instanceof Error ? err.message : String(err));
  }
}
