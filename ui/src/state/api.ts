import type {
  AddReviewDraftRequest,
  AddWorktreeCommentRequest,
  AiDraftRequest,
  AiDraftResponse,
  AiReviewRequest,
  AiReviewResponse,
  ReviewChatRequest,
  ReviewChatResponse,
  ArchiveResponse,
  CreateFeedbackRequest,
  CreateNodeRequest,
  CreateProfileRequest,
  CreateRepoRequest,
  CreateSessionRequest,
  DirSuggestions,
  DoctorResponse,
  DraftComment,
  Event,
  Feedback,
  FeedbackStatusFilter,
  MemoryResponse,
  MemoryScope,
  MergeWorktreeResponse,
  Node,
  NodeID,
  PatchNodeRequest,
  Profile,
  ProfilesResponse,
  PRReview,
  PromptRequest,
  PushKeyResponse,
  PushSubscribeRequest,
  PushUnsubscribeRequest,
  ReplyToThreadRequest,
  Repo,
  ReposResponse,
  ResolveFeedbackRequest,
  ResumeTarget,
  ReviewDraftsResponse,
  ReviewSources,
  ReviewsResponse,
  Session,
  StartReviewRequest,
  StatsRange,
  StatsResponse,
  SubmitReviewRequest,
  SubmitReviewResponse,
  TreeSnapshot,
  UsageResponse,
  UsageWindowKind,
  VersionResponse,
  WorktreeComment,
  WorktreeCommentsResponse,
  WorktreeReview,
} from "../gen/types";
import { CSRF_HEADER } from "./auth";

const API_BASE = "/api/v1";

export class ApiError extends Error {
  readonly status: number;
  constructor(status: number, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

export interface ApiClient {
  getTree(): Promise<TreeSnapshot>;
  createNode(body: CreateNodeRequest): Promise<Node>;
  patchNode(id: string, body: PatchNodeRequest): Promise<Node>;
  archiveNode(id: string): Promise<ArchiveResponse>;
  ackNode(id: string): Promise<Node>;
  createSession(nodeId: string, body: CreateSessionRequest): Promise<Session>;
  sendPrompt(nodeId: string, text: string): Promise<void>;
  stopSession(sessionId: string): Promise<void>;
  getEvents(nodeId: string, after?: string, limit?: number): Promise<Event[]>;
  getInbox(): Promise<Event[]>;
  getVersion(): Promise<VersionResponse>;
  getUsage(window: UsageWindowKind): Promise<UsageResponse>;
  /** Directory completion candidates for a partial work_dir path (terminal
   *  tab-completion). An empty prefix lists the user's home directory. */
  suggestDirs(prefix: string): Promise<DirSuggestions>;
  authSession(token: string): Promise<void>;
  /** Resolves true if a valid session cookie is already present. */
  authMe(): Promise<boolean>;
  /** Whether a node's latest session can be resumed, and with which id. */
  resumeTarget(nodeId: string): Promise<ResumeTarget>;
  /** Review Radar: open PRs across watched repos, classified into buckets. */
  getReviews(): Promise<ReviewsResponse>;
  getReviewSources(): Promise<ReviewSources>;
  /** Replaces the full watched-directory set (not a merge/append). */
  setReviewSources(dirs: string[]): Promise<ReviewSources>;
  /** Spawns a read-only review task node for a PR; the caller navigates to it. */
  startReview(dir: string, pr: number, title?: string): Promise<Node>;

  // --- Interactive review workspace (/api/v1/reviews/pr) ---
  /** One PR's diff + inline comment threads. */
  getPRReview(dir: string, pr: number): Promise<PRReview>;
  getReviewDrafts(dir: string, pr: number): Promise<ReviewDraftsResponse>;
  addReviewDraft(body: AddReviewDraftRequest): Promise<DraftComment>;
  deleteReviewDraft(id: string): Promise<void>;
  /** Runs a headless claude pass over the diff/thread context to suggest
   *  comment or reply text; always human-reviewed/edited before it becomes
   *  a draft or a posted reply. */
  aiDraft(req: AiDraftRequest): Promise<AiDraftResponse>;
  /** Runs a headless claude pass over the whole PR diff and returns
   *  line-anchored findings (proposed comments, some with a code suggestion).
   *  Nothing is posted; the reviewer accepts/dismisses each finding. */
  aiReview(req: AiReviewRequest): Promise<AiReviewResponse>;
  /** Continues the review conversation by resuming the session the last
   *  ai-review pass created, so the reviewer answers with the PR + findings in
   *  context (e.g. a question about one finding). */
  reviewChat(req: ReviewChatRequest): Promise<ReviewChatResponse>;
  /** Posts one batch review (event + body + the referenced drafts) and
   *  clears those drafts. */
  submitReview(req: SubmitReviewRequest): Promise<SubmitReviewResponse>;
  /** Posts an immediate reply to an existing thread, optionally resolving it. */
  replyToThread(req: ReplyToThreadRequest): Promise<void>;

  // --- Worktree review (/api/v1/reviews/worktree) ---
  /** A task node's worktree diff (working tree vs. merge-base with base_ref). */
  getWorktreeReview(node: string, repo: string): Promise<WorktreeReview>;
  getWorktreeComments(node: string, repo: string): Promise<WorktreeCommentsResponse>;
  addWorktreeComment(body: AddWorktreeCommentRequest): Promise<WorktreeComment>;
  deleteWorktreeComment(id: string): Promise<void>;
  /** Squash-merges the worktree into its parent; requires a clean tree. */
  mergeWorktree(node: string, repo: string): Promise<MergeWorktreeResponse>;
  /** Composes the worktree's comments into a prompt and starts a PTY session
   *  on the node so the agent fixes them -- navigate to the node's terminal
   *  to watch. */
  addressWorktree(node: string, repo: string): Promise<Session>;

  // --- Repos (/api/v1/projects/{id}/repos) ---
  /** Git repos registered on a project node. Registering one makes every task
   *  created under the project afterwards auto-provision a worktree per repo. */
  getRepos(projectId: string): Promise<ReposResponse>;
  addRepo(projectId: string, body: CreateRepoRequest): Promise<Repo>;
  /** Idempotent: removing a repo only affects tasks created afterwards;
   *  existing task worktrees are untouched. */
  deleteRepo(repoId: string): Promise<void>;

  // --- Profiles (/api/v1/profiles) ---
  /** Provider accounts. Reading seeds the default claude profile on first run. */
  getProfiles(): Promise<ProfilesResponse>;
  addProfile(body: CreateProfileRequest): Promise<Profile>;
  /** Idempotent; the default profile is re-seeded on the next getProfiles. */
  deleteProfile(id: string): Promise<void>;
  /** Health probes for one profile (config dir resolves, no API-key override,
   *  the CLI runs under the profile env). */
  profileDoctor(id: string): Promise<DoctorResponse>;

  // --- Stats (/api/v1/stats) ---
  /** Aggregated token/agent/flow/tool/feedback stats over a scope subtree
   *  (undefined scope = whole workspace) and a time range. */
  getStats(scope?: NodeID, range?: StatsRange): Promise<StatsResponse>;

  // --- Feedback loop (/api/v1/feedback) ---
  listFeedback(status?: FeedbackStatusFilter): Promise<Feedback[]>;
  createFeedback(body: CreateFeedbackRequest): Promise<Feedback>;
  /** Marks feedback resolved, optionally linking the fix task node that
   *  closes the loop (see "Create fix task" in docs/API.md). */
  resolveFeedback(id: string, fixNodeId?: NodeID): Promise<Feedback>;

  // --- Node memory (/api/v1/nodes/{id}/memory) ---
  /** A node's MemPalace-backed memory in the given scope (default self).
   *  Read-only; agents write memory via MCP. healthy:false means MemPalace is
   *  unavailable -- the tab shows an install hint rather than erroring. */
  getNodeMemory(nodeId: string, scope?: MemoryScope): Promise<MemoryResponse>;

  // --- Web push (/api/v1/push) ---
  /** The daemon's VAPID applicationServerKey, passed to
   *  PushManager.subscribe() to authorize it as the push sender. */
  getPushKey(): Promise<PushKeyResponse>;
  /** Registers a browser PushSubscription so the daemon can push attention
   *  notifications to it (see state/push.ts for the full enable flow). */
  pushSubscribe(body: PushSubscribeRequest): Promise<void>;
  /** Removes a previously registered subscription by endpoint. */
  pushUnsubscribe(endpoint: string): Promise<void>;
}

function isErrorBody(v: unknown): v is { error: string } {
  return typeof v === "object" && v !== null && typeof (v as { error?: unknown }).error === "string";
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const method = init?.method ?? "GET";
  const headers = new Headers(init?.headers);
  if (method !== "GET") headers.set(CSRF_HEADER, "1");
  if (init?.body !== undefined && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }

  const res = await fetch(API_BASE + path, {
    ...init,
    method,
    headers,
    credentials: "include",
  });

  if (res.status === 204) return undefined as T;

  const text = await res.text();
  const data: unknown = text ? JSON.parse(text) : undefined;

  if (!res.ok) {
    const message = isErrorBody(data) ? data.error : res.statusText || `HTTP ${res.status}`;
    throw new ApiError(res.status, message);
  }
  return data as T;
}

function qs(params: Record<string, string | number | undefined>): string {
  const usp = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined) usp.set(k, String(v));
  }
  const s = usp.toString();
  return s ? `?${s}` : "";
}

export const realApiClient: ApiClient = {
  getTree: () => request("/tree"),

  createNode: (body) => request("/nodes", { method: "POST", body: JSON.stringify(body) }),

  patchNode: (id, body) =>
    request(`/nodes/${encodeURIComponent(id)}`, { method: "PATCH", body: JSON.stringify(body) }),

  archiveNode: (id) => request(`/nodes/${encodeURIComponent(id)}/archive`, { method: "POST" }),

  ackNode: (id) => request(`/nodes/${encodeURIComponent(id)}/ack`, { method: "POST" }),

  createSession: (nodeId, body) =>
    request(`/nodes/${encodeURIComponent(nodeId)}/sessions`, {
      method: "POST",
      body: JSON.stringify(body),
    }),

  sendPrompt: (nodeId, text) =>
    request(`/nodes/${encodeURIComponent(nodeId)}/prompt`, {
      method: "POST",
      body: JSON.stringify({ text } satisfies PromptRequest),
    }),

  stopSession: (sessionId) =>
    request(`/sessions/${encodeURIComponent(sessionId)}/stop`, { method: "POST" }),

  getEvents: (nodeId, after, limit) =>
    request(`/nodes/${encodeURIComponent(nodeId)}/events${qs({ after, limit })}`),

  getInbox: () => request("/inbox"),

  getVersion: () => request("/version"),

  getUsage: (window) => request(`/usage${qs({ window })}`),

  suggestDirs: (prefix) => request(`/fs/dirs${qs({ prefix })}`),

  authSession: (token) =>
    request("/auth/session", { method: "POST", body: JSON.stringify({ token }) }),

  authMe: async () => {
    try {
      await request("/auth/me");
      return true;
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) return false;
      throw err;
    }
  },

  resumeTarget: (nodeId) => request(`/nodes/${encodeURIComponent(nodeId)}/resume-target`),
  getReviews: () => request("/reviews"),

  getReviewSources: () => request("/reviews/sources"),

  setReviewSources: (dirs) =>
    request("/reviews/sources", { method: "POST", body: JSON.stringify({ dirs } satisfies ReviewSources) }),

  startReview: (dir, pr, title) =>
    request("/reviews/start", {
      method: "POST",
      body: JSON.stringify({ dir, pr, title } satisfies StartReviewRequest),
    }),

  getPRReview: (dir, pr) => request(`/reviews/pr${qs({ dir, pr })}`),

  getReviewDrafts: (dir, pr) => request(`/reviews/pr/drafts${qs({ dir, pr })}`),

  addReviewDraft: (body) =>
    request("/reviews/pr/drafts", { method: "POST", body: JSON.stringify(body) }),

  deleteReviewDraft: (id) =>
    request(`/reviews/pr/drafts/${encodeURIComponent(id)}`, { method: "DELETE" }),

  aiDraft: (req) => request("/reviews/pr/ai-draft", { method: "POST", body: JSON.stringify(req) }),

  aiReview: (req) => request("/reviews/pr/ai-review", { method: "POST", body: JSON.stringify(req) }),

  reviewChat: (req) => request("/reviews/pr/chat", { method: "POST", body: JSON.stringify(req) }),

  submitReview: (req) => request("/reviews/pr/submit", { method: "POST", body: JSON.stringify(req) }),

  replyToThread: (req) => request("/reviews/pr/reply", { method: "POST", body: JSON.stringify(req) }),

  getWorktreeReview: (node, repo) => request(`/reviews/worktree${qs({ node, repo })}`),

  getWorktreeComments: (node, repo) => request(`/reviews/worktree/comments${qs({ node, repo })}`),

  addWorktreeComment: (body) =>
    request("/reviews/worktree/comments", { method: "POST", body: JSON.stringify(body) }),

  deleteWorktreeComment: (id) =>
    request(`/reviews/worktree/comments/${encodeURIComponent(id)}`, { method: "DELETE" }),

  mergeWorktree: (node, repo) =>
    request("/reviews/worktree/merge", { method: "POST", body: JSON.stringify({ node, repo }) }),

  addressWorktree: (node, repo) =>
    request("/reviews/worktree/address", { method: "POST", body: JSON.stringify({ node, repo }) }),

  getRepos: (projectId) => request(`/projects/${encodeURIComponent(projectId)}/repos`),

  addRepo: (projectId, body) =>
    request(`/projects/${encodeURIComponent(projectId)}/repos`, {
      method: "POST",
      body: JSON.stringify(body),
    }),

  deleteRepo: (repoId) => request(`/repos/${encodeURIComponent(repoId)}`, { method: "DELETE" }),

  getProfiles: () => request("/profiles"),

  addProfile: (body) => request("/profiles", { method: "POST", body: JSON.stringify(body) }),

  deleteProfile: (id) => request(`/profiles/${encodeURIComponent(id)}`, { method: "DELETE" }),

  profileDoctor: (id) => request(`/profiles/${encodeURIComponent(id)}/doctor`),

  getStats: (scope, range) => request(`/stats${qs({ scope, range })}`),

  listFeedback: (status) => request(`/feedback${qs({ status })}`),

  createFeedback: (body) => request("/feedback", { method: "POST", body: JSON.stringify(body) }),

  resolveFeedback: (id, fixNodeId) =>
    request(`/feedback/${encodeURIComponent(id)}/resolve`, {
      method: "POST",
      body: JSON.stringify({ fix_node_id: fixNodeId } satisfies ResolveFeedbackRequest),
    }),

  getNodeMemory: (nodeId, scope) =>
    request(`/nodes/${encodeURIComponent(nodeId)}/memory${qs({ scope })}`),

  getPushKey: () => request("/push/key"),

  pushSubscribe: (body) => request("/push/subscribe", { method: "POST", body: JSON.stringify(body) }),

  pushUnsubscribe: (endpoint) =>
    request("/push/unsubscribe", {
      method: "POST",
      body: JSON.stringify({ endpoint } satisfies PushUnsubscribeRequest),
    }),
};

// Mock mode swaps in an in-memory client. The dynamic import keeps src/mock/
// out of production bundles: Vite statically replaces `import.meta.env
// .VITE_MOCK` at build time, so when it's unset this whole branch is
// dead-code-eliminated, including the import() call.
export const apiClient: ApiClient =
  import.meta.env.VITE_MOCK === "1"
    ? await (await import("../mock/mockApi")).createMockApiClient()
    : realApiClient;
