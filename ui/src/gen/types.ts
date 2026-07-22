// Hand-written mirror of docs/API.md (the frozen contract). These will move
// to codegen later (see Makefile `gen` target); until then, this file IS the
// source of truth on the UI side and must track API.md exactly.
//
// Wire format: JSON everywhere, snake_case fields, timestamps as RFC 3339
// strings (omitted when zero — modeled here as optional).

export type NodeID = string;
export type SessionID = string;
export type EventID = string;
export type ProfileID = string;

export type NodeKind = "workspace" | "project" | "task";

export type NodeStatus =
  | "idle"
  | "starting"
  | "running"
  | "awaiting_input"
  | "done"
  | "failed"
  | "interrupted";

export type Attention =
  | "none"
  | "permission"
  | "question"
  | "done"
  | "error"
  | "review";

export type SessionMode = "pty" | "headless";

export type SessionStatus =
  | "starting"
  | "running"
  | "awaiting_input"
  | "exited"
  | "failed"
  | "interrupted";

export type EventType =
  | "session_started"
  | "text"
  | "tool_call"
  | "tool_result"
  | "awaiting_input"
  | "turn_done"
  | "session_ended"
  | "error"
  | "usage";

// Only referenced by AwaitingPayload.reason; not enumerated in API.md's
// entity table but required by internal/core/event.go's AwaitingReason.
export type AwaitingReason = "permission" | "question" | "idle";

export interface Node {
  id: NodeID;
  parent_id: NodeID | ""; // empty for the root workspace
  kind: NodeKind;
  title: string;
  brief: string;
  status: NodeStatus;
  attention: Attention;
  attention_reason: string;
  attention_since?: string; // zero time omitted
  driver: string; // empty = inherited from parent chain
  profile_id: ProfileID; // empty = inherited
  current_session_id: SessionID | "";
  workspace_dir: string;
  work_dir: string; // user-set working directory; empty = inherited from parent chain
  meta: Record<string, unknown>;
  position: number;
  created_at: string;
  updated_at: string;
  archived_at?: string; // zero = live
}

export interface Session {
  id: SessionID;
  node_id: NodeID;
  driver: string;
  profile_id: ProfileID;
  mode: SessionMode;
  driver_session_id: string;
  status: SessionStatus;
  exit_code?: number;
  cwd: string;
  started_at: string;
  ended_at?: string; // zero while live
}

// --- Event payloads (internal/core/payload.go), keyed by EventType ---

export interface SessionStartedPayload {
  driver_session_id: string;
  transcript_path?: string;
  model?: string;
}

export interface TextPayload {
  text: string;
  final?: boolean; // end-of-turn assistant text
  // Distinguishes an injected user prompt ("user") from agent output; absent
  // means assistant. POST /nodes/{id}/prompt echoes the injected text as a
  // "user" event before it reaches the agent (see API.md clarifications).
  role?: "user";
}

export interface ToolCallPayload {
  name: string;
  input_summary?: string;
}

export interface ToolResultPayload {
  name: string;
  ok: boolean;
  summary?: string;
}

export interface AwaitingPayload {
  reason: AwaitingReason;
  detail?: string;
}

export interface TurnDonePayload {
  result_text?: string;
  duration_ms?: number;
}

export interface SessionEndedPayload {
  exit_code: number;
}

export interface ErrorPayload {
  message: string;
  fatal?: boolean;
}

export interface UsagePayload {
  input_tokens: number;
  output_tokens: number;
  cost_usd?: number;
}

interface EventBase {
  id: EventID;
  node_id: NodeID;
  session_id: SessionID | ""; // empty for node-level events
  requires_attention: boolean;
  acked_at?: string; // unset = unacked
  created_at: string;
}

// Discriminated union keyed by `type` so consumers get payload narrowing via
// a switch on event.type.
export type Event =
  | (EventBase & { type: "session_started"; payload: SessionStartedPayload })
  | (EventBase & { type: "text"; payload: TextPayload })
  | (EventBase & { type: "tool_call"; payload: ToolCallPayload })
  | (EventBase & { type: "tool_result"; payload: ToolResultPayload })
  | (EventBase & { type: "awaiting_input"; payload: AwaitingPayload })
  | (EventBase & { type: "turn_done"; payload: TurnDonePayload })
  | (EventBase & { type: "session_ended"; payload: SessionEndedPayload })
  | (EventBase & { type: "error"; payload: ErrorPayload })
  | (EventBase & { type: "usage"; payload: UsagePayload });

// --- REST request/response shapes ---

export interface TreeSnapshot {
  rev: number;
  nodes: Node[];
  sessions: Session[];
}

export interface CreateNodeRequest {
  parent_id: NodeID;
  kind: NodeKind;
  title: string;
  brief?: string;
  driver?: string;
  profile_id?: string;
  work_dir?: string;
}

export interface PatchNodeRequest {
  title?: string;
  brief?: string;
  driver?: string;
  profile_id?: string;
  work_dir?: string;
  meta?: Record<string, unknown>;
}

export interface ArchiveResponse {
  archived: NodeID[];
}

export interface CreateSessionRequest {
  mode: SessionMode;
  prompt?: string;
  resume_id?: string;
}

export interface PromptRequest {
  text: string;
}

export interface VersionResponse {
  version: string;
  commit: string;
}

export type UsageWindowKind = "5h" | "week";

// One profile's consumption in the requested window. utilization is a 0..1
// estimate against the plan's limit (absent/null = unknown -- render token
// counts instead of a percentage bar). resets_at appears when a rate-limit
// reset time was detected; cooldown_until is set while the profile is
// actively rate-limited.
export interface UsageWindow {
  profile_id: ProfileID;
  name: string;
  driver: string;
  window: UsageWindowKind;
  window_start: string;
  window_end: string;
  input_tokens: number;
  output_tokens: number;
  cache_read_tokens: number;
  cost_usd: number;
  utilization: number | null;
  resets_at?: string;
  cooldown_until?: string;
}

export interface UsageResponse {
  profiles: UsageWindow[];
}

// GET /fs/dirs?prefix=<text> — terminal-style work_dir completion candidates
// (absolute directory paths) plus the daemon user's resolved home directory.
export interface DirSuggestions {
  dirs: string[];
  home: string;
}

// --- Review Radar (/api/v1/reviews) ---

export type ReviewChecksState = "passing" | "failing" | "pending" | "none";

// "" = no review decision yet (e.g. a fresh draft with no reviewers).
export type ReviewDecision = "REVIEW_REQUIRED" | "APPROVED" | "CHANGES_REQUESTED" | "";

export interface PR {
  number: number;
  title: string;
  author: string;
  url: string;
  is_draft: boolean;
  updated_at: string;
  review_decision: ReviewDecision;
  checks: ReviewChecksState;
  additions: number;
  deletions: number;
}

// A PR appears in exactly one bucket -- first match wins, in this order
// (see API.md's Review Radar section for the classification rules).
export interface ReviewBuckets {
  needs_review: PR[];
  re_review: PR[];
  reviewed: PR[];
  mine: PR[];
}

export interface ReviewRepo {
  dir: string;
  name_with_owner: string;
  buckets: ReviewBuckets;
}

export interface ReviewsResponse {
  login: string;
  repos: ReviewRepo[];
  errors: string[];
}

// GET /nodes/{id}/resume-target: whether the node's latest session can be
// resumed (its conversation transcript still exists), and with which id.
export interface ResumeTarget {
  resumable: boolean;
  driver_session_id: string;
  reason: string;
}

// GET /reviews/sources response and POST /reviews/sources request share this
// shape -- the endpoint always replaces the full watched-directory set.
export interface ReviewSources {
  dirs: string[];
}

export interface StartReviewRequest {
  dir: string;
  pr: number;
  title?: string;
}

export interface AuthSessionRequest {
  token: string;
}

export interface ApiErrorBody {
  error: string;
}

// --- Interactive review workspace (/api/v1/reviews/pr) ---

// " " = context, "+" = added, "-" = removed (unified diff op per line).
export type DiffLineOp = " " | "+" | "-";

export interface DiffLine {
  op: DiffLineOp;
  // 0 = not applicable for this op (e.g. old_line is 0 on an added line,
  // new_line is 0 on a removed line) -- never both 0 for a real line.
  old_line: number;
  new_line: number;
  text: string;
}

export interface DiffHunk {
  header: string; // "@@ -a,b +c,d @@ ..."
  lines: DiffLine[];
}

export type PRFileStatus = "modified" | "added" | "removed" | "renamed";

// "" = full contents included; a non-empty reason means original_content/
// modified_content are omitted and the UI renders a placeholder instead
// (see docs/API.md "Diff content for rich rendering (Pierre)").
export type ContentOmittedReason = "" | "binary" | "too_large";

export interface PRReviewFile {
  path: string;
  old_path?: string; // set when status === "renamed" and differs from path
  status: PRFileStatus;
  additions: number;
  deletions: number;
  binary: boolean;
  // Full before/after file text (not patches) so @pierre/diffs can compute
  // its own diff -- "" for added/removed/binary/omitted, as appropriate.
  original_content: string;
  modified_content: string;
  content_omitted: ContentOmittedReason;
  // Kept as a fallback; the UI renders original_content/modified_content via
  // @pierre/diffs rather than walking these directly.
  hunks: DiffHunk[];
}

// The diff side a thread/draft/comment anchors to: RIGHT = new-file line
// number, LEFT = old-file line number (GitHub review-comment vocabulary).
export type ReviewCommentSide = "LEFT" | "RIGHT";

export interface ThreadComment {
  id: string;
  author: string;
  body: string;
  created_at: string;
  is_mine: boolean;
}

export interface ReviewThread {
  id: string;
  path: string;
  line: number;
  side: ReviewCommentSide;
  is_resolved: boolean;
  diff_hunk: string;
  comments: ThreadComment[];
}

// GitHub PR state as surfaced by `gh` -- API.md's example only shows "OPEN"
// but a real `gh pr view` can report any of these three.
export type PRReviewState = "OPEN" | "CLOSED" | "MERGED";

export interface PRReview {
  number: number;
  title: string;
  author: string;
  url: string;
  state: PRReviewState;
  head_sha: string;
  base_ref: string;
  checks: ReviewChecksState;
  review_decision: ReviewDecision;
  body: string; // PR description (markdown)
  files: PRReviewFile[];
  threads: ReviewThread[];
}

// A pending review comment held in grove until submit.
export interface DraftComment {
  id: string;
  dir: string;
  pr: number;
  path: string;
  line: number;
  side: ReviewCommentSide;
  body: string;
  created_at: string;
}

export interface ReviewDraftsResponse {
  drafts: DraftComment[];
}

export interface AddReviewDraftRequest {
  dir: string;
  pr: number;
  path: string;
  line: number;
  side: ReviewCommentSide;
  body: string;
}

export type AiDraftKind = "comment" | "reply";

export interface AiDraftRequest {
  dir: string;
  pr: number;
  kind: AiDraftKind;
  path?: string;
  line?: number;
  thread_id?: string;
  /** Optional steering text -- typically whatever the user already typed
   *  into the composer before requesting a draft. */
  instruction?: string;
}

export interface AiDraftResponse {
  text: string;
}

export type SubmitReviewEvent = "APPROVE" | "REQUEST_CHANGES" | "COMMENT";

export interface SubmitReviewRequest {
  dir: string;
  pr: number;
  event: SubmitReviewEvent;
  body: string;
  draft_ids: string[];
}

export interface SubmitReviewResponse {
  url: string;
}

export interface ReplyToThreadRequest {
  dir: string;
  pr: number;
  thread_id: string;
  body: string;
  resolve: boolean;
}

// --- Worktree review (/api/v1/reviews/worktree) ---

// Same content-bearing shape as PRReviewFile (see docs/API.md's "Worktree
// review" section) -- both carry full before/after file text for
// @pierre/diffs rather than patches.
export type WorktreeFile = PRReviewFile;

export interface WorktreeReview {
  node_id: NodeID;
  repo: string;
  worktree_path: string;
  branch: string;
  base_ref: string;
  has_uncommitted: boolean;
  files: WorktreeFile[];
}

// A local review note keyed to (node, repo, path, line) -- not a GitHub
// entity; "Address with agent" composes these into a prompt for a fix
// session rather than posting them anywhere.
export interface WorktreeComment {
  id: string;
  node_id: NodeID;
  repo: string;
  path: string;
  line: number;
  side: ReviewCommentSide;
  body: string;
  created_at: string;
}

export interface WorktreeCommentsResponse {
  comments: WorktreeComment[];
}

export interface AddWorktreeCommentRequest {
  node: NodeID;
  repo: string;
  path: string;
  line: number;
  side: ReviewCommentSide;
  body: string;
}

export interface MergeWorktreeRequest {
  node: NodeID;
  repo: string;
}

export interface MergeWorktreeResponse {
  merged: boolean;
  message: string;
}

export interface AddressWorktreeRequest {
  node: NodeID;
  repo: string;
}

// --- WebSocket /ws/state (JSON text frames, server-push) ---

export interface WSHello {
  t: "hello";
  rev: number;
  nodes: Node[];
  sessions: Session[];
  inbox: Event[];
}

export interface WSDelta {
  t: "delta";
  rev: number;
  nodes?: Node[];
  sessions?: Session[];
  events?: Event[];
}

export type WSStateMessage = WSHello | WSDelta;

// --- WebSocket /ws/term/{session_id} (binary frames + JSON control frames) ---

export interface TermResizeMessage {
  t: "resize";
  cols: number;
  rows: number;
}

export interface TermLiveMessage {
  t: "live";
}

export interface TermExitMessage {
  t: "exit";
  code: number;
}

// Server -> client text frames.
export type TermControlMessage = TermLiveMessage | TermExitMessage;
