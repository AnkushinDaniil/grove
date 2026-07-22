import type {
  ArchiveResponse,
  CreateNodeRequest,
  CreateSessionRequest,
  DirSuggestions,
  Event,
  Node,
  PatchNodeRequest,
  PromptRequest,
  ResumeTarget,
  ReviewSources,
  ReviewsResponse,
  Session,
  StartReviewRequest,
  TreeSnapshot,
  UsageResponse,
  UsageWindowKind,
  VersionResponse,
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
};

// Mock mode swaps in an in-memory client. The dynamic import keeps src/mock/
// out of production bundles: Vite statically replaces `import.meta.env
// .VITE_MOCK` at build time, so when it's unset this whole branch is
// dead-code-eliminated, including the import() call.
export const apiClient: ApiClient =
  import.meta.env.VITE_MOCK === "1"
    ? await (await import("../mock/mockApi")).createMockApiClient()
    : realApiClient;
