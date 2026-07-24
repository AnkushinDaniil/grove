import { create } from "zustand";
import { apiClient } from "./api";
import type { AiFinding, DraftComment, GraphStatus, PRReview } from "../gen/types";

/** An AI-review finding held client-side. Findings are never persisted, so a
 *  local id keys the list and drives dismiss; the AiFinding fields are the
 *  server's proposal (the card may edit body/suggestion before accepting). */
export interface LocalFinding extends AiFinding {
  id: string;
}

/** One turn in the review conversation (the resumable session the ai-review
 *  pass created). */
export interface ChatMessage {
  role: "user" | "assistant";
  text: string;
}

interface ReviewWorkspaceState {
  dir: string | null;
  pr: number | null;
  review: PRReview | null;
  drafts: DraftComment[];
  /** Proposals from the last "Review with AI" pass, awaiting accept/dismiss. */
  aiFindings: LocalFinding[];
  aiReviewing: boolean;
  aiReviewError: string | null;
  /** True once a pass has completed for the current PR, so the panel can tell
   *  "clean, nothing found" from "not run yet". */
  aiReviewRan: boolean;
  /** Whether the last pass was codebase-aware (call graph injected), null
   *  before the first pass. */
  aiGraphStatus: GraphStatus | null;
  /** The review conversation (chat with the session the pass created). */
  chatMessages: ChatMessage[];
  chatSending: boolean;
  chatError: string | null;
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
  setAiFindings: (findings: LocalFinding[], graphStatus: GraphStatus) => void;
  removeFinding: (id: string) => void;
  setAiReviewing: (v: boolean) => void;
  setAiReviewError: (message: string | null) => void;
  pushChat: (message: ChatMessage) => void;
  setChatSending: (v: boolean) => void;
  setChatError: (message: string | null) => void;
  reset: () => void;
}

const initial = {
  dir: null as string | null,
  pr: null as number | null,
  review: null as PRReview | null,
  drafts: [] as DraftComment[],
  aiFindings: [] as LocalFinding[],
  aiReviewing: false,
  aiReviewError: null as string | null,
  aiReviewRan: false,
  aiGraphStatus: null as GraphStatus | null,
  chatMessages: [] as ChatMessage[],
  chatSending: false,
  chatError: null as string | null,
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
  setAiFindings: (findings, graphStatus) =>
    set({ aiFindings: findings, aiReviewError: null, aiReviewRan: true, aiGraphStatus: graphStatus }),
  removeFinding: (id) => set((s) => ({ aiFindings: s.aiFindings.filter((f) => f.id !== id) })),
  setAiReviewing: (v) => set({ aiReviewing: v }),
  setAiReviewError: (message) => set({ aiReviewError: message }),
  pushChat: (message) => set((s) => ({ chatMessages: [...s.chatMessages, message] })),
  setChatSending: (v) => set({ chatSending: v }),
  setChatError: (message) => set({ chatError: message }),

  reset: () => set({ ...initial }),
}));

// Monotonic id source for client-only findings -- unique within and across
// passes (a new pass replaces the whole set), and deterministic for tests.
let findingSeq = 0;
function nextFindingId(): string {
  findingSeq += 1;
  return `finding-${findingSeq}`;
}

/** Runs one "Review with AI" pass and populates the findings panel. Kept
 *  separate from the store like loadReviewWorkspace so the async lifecycle
 *  (busy flag, error) lives in one place. */
export async function runAIReview(dir: string, pr: number): Promise<void> {
  const store = useReviewWorkspaceStore.getState();
  store.setAiReviewing(true);
  store.setAiReviewError(null);
  try {
    const { findings, graph_status } = await apiClient.aiReview({ dir, pr });
    useReviewWorkspaceStore
      .getState()
      .setAiFindings(findings.map((f) => ({ ...f, id: nextFindingId() })), graph_status);
  } catch (err) {
    useReviewWorkspaceStore.getState().setAiReviewError(err instanceof Error ? err.message : String(err));
  } finally {
    useReviewWorkspaceStore.getState().setAiReviewing(false);
  }
}

/** Sends one chat turn to the review session and appends the reply. The user
 *  message is shown immediately; a failure surfaces as chatError without losing
 *  the transcript. */
export async function sendReviewChat(dir: string, pr: number, message: string): Promise<void> {
  const store = useReviewWorkspaceStore.getState();
  store.pushChat({ role: "user", text: message });
  store.setChatSending(true);
  store.setChatError(null);
  try {
    const { reply } = await apiClient.reviewChat({ dir, pr, message });
    useReviewWorkspaceStore.getState().pushChat({ role: "assistant", text: reply });
  } catch (err) {
    useReviewWorkspaceStore.getState().setChatError(err instanceof Error ? err.message : String(err));
  } finally {
    useReviewWorkspaceStore.getState().setChatSending(false);
  }
}

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
