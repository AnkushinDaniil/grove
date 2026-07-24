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

// --- AI review pass (/api/v1/reviews/pr/ai-review) ---

export type AiFindingSeverity = "issue" | "suggestion" | "nit";

// One AI-proposed review comment anchored to a changed line, optionally with a
// single-line code suggestion. Findings are transient (never persisted): the UI
// holds one pass's set and turns each accepted finding into a normal draft, its
// `suggestion` becoming a GitHub ```suggestion block in the draft body.
export interface AiFinding {
  path: string;
  line: number;
  side: ReviewCommentSide;
  severity: AiFindingSeverity;
  body: string;
  /** Replacement text for the anchored line; "" when the finding is comment-only. */
  suggestion: string;
}

export interface AiReviewRequest {
  dir: string;
  pr: number;
}

export type GraphStatus = "ready" | "building" | "off";

export interface AiReviewResponse {
  findings: AiFinding[];
  /** Whether the pass was codebase-aware: "ready" (call-graph context
   *  injected), "building" (graph warming up; this pass was diff-only), or
   *  "off" (no code-review-graph installed). */
  graph_status: GraphStatus;
}

export interface ReviewChatRequest {
  dir: string;
  pr: number;
  message: string;
}

export interface ReviewChatResponse {
  reply: string;
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

// --- Repos (/api/v1/projects/{id}/repos) ---

// A git repository registered on a project node. Once a project has repos, new
// task nodes under it auto-provision a worktree per repo (branch
// grove/<short8>-<slug>), which is what makes worktree review, merge-back and
// PR-from-task work.
export interface Repo {
  id: string;
  project_id: NodeID;
  name: string;
  source_path: string;
  default_base: string; // "" = auto-detect origin/HEAD
  created_at: string;
}

export interface CreateRepoRequest {
  // Defaults to the source_path basename when omitted; must be a plain
  // directory name (used as the worktree subdir).
  name?: string;
  source_path: string;
  default_base?: string;
}

export interface ReposResponse {
  repos: Repo[];
}

// --- Profiles (/api/v1/profiles) ---

// A provider account: an isolated CLI config dir (CLAUDE_CONFIG_DIR/CODEX_HOME)
// selected per node via profile_id (inherited like driver). The `default`
// profile is auto-created and adopts the CLI's own dir (~/.claude) untouched;
// config_dir otherwise defaults to ~/.grove/profiles/<driver>/<name>.
export interface Profile {
  id: ProfileID;
  driver: string;
  name: string;
  config_dir: string;
  is_default: boolean;
  created_at: string;
}

export interface CreateProfileRequest {
  driver: string;
  name: string;
  config_dir?: string;
}

export interface ProfilesResponse {
  profiles: Profile[];
}

// One GET /profiles/{id}/doctor probe: a named health check plus its outcome
// and a human-readable detail (the resolved path, the reason it failed, ...).
export interface DoctorCheck {
  name: string;
  ok: boolean;
  detail: string;
}

export interface DoctorResponse {
  checks: DoctorCheck[];
}

// --- Stats (/api/v1/stats, draft -- additive evolution allowed) ---

export type StatsRange = "24h" | "7d" | "30d";

export interface TokenTotals {
  input: number;
  output: number;
  cache_read: number;
  cost_usd: number;
}

export interface TokenByDay {
  day: string; // "2026-07-20"
  input: number;
  output: number;
  cost_usd: number;
}

export interface TokenByDriver {
  driver: string;
  input: number;
  output: number;
  cost_usd: number;
}

export interface TokenByProfile {
  profile_id: ProfileID;
  name: string;
  input: number;
  output: number;
  cost_usd: number;
}

export interface TokenTopNode {
  node_id: NodeID;
  title: string;
  input: number;
  output: number;
  cost_usd: number;
}

export interface StatsTokens {
  total: TokenTotals;
  by_day: TokenByDay[];
  by_driver: TokenByDriver[];
  by_profile: TokenByProfile[];
  top_nodes: TokenTopNode[];
}

export interface AgentsByDay {
  day: string;
  started: number;
  done: number;
  failed: number;
}

export interface AgentsByDriver {
  driver: string;
  count: number;
}

export interface StatsAgents {
  sessions_active: number;
  sessions_by_day: AgentsByDay[];
  avg_session_minutes: number;
  by_driver: AgentsByDriver[];
}

export interface StatsFlow {
  tasks_created: number;
  tasks_done: number;
  tasks_failed: number;
  median_task_hours: number;
  attention_wait_p50_minutes: number;
  attention_wait_p95_minutes: number;
  prs_opened: number;
  prs_merged: number;
}

// Parsed from tool_call/tool_result event payloads.
export interface StatsTool {
  name: string;
  calls: number;
  errors: number;
}

export interface StatsModel {
  model: string;
  input: number;
  output: number;
  cost_usd: number;
}

// Skill-tool invocations parsed from tool_call payloads (payload.name ===
// "Skill"; see FeedbackKind's "skill" doc comment for the same distinction).
export interface StatsSkill {
  skill: string;
  invocations: number;
}

export interface StatsFeedbackSummary {
  kind: FeedbackKind;
  subject: string;
  open: number;
  total: number;
}

export interface StatsResponse {
  range: StatsRange;
  scope: NodeID | ""; // "" = whole workspace
  tokens: StatsTokens;
  agents: StatsAgents;
  flow: StatsFlow;
  tools: StatsTool[];
  models: StatsModel[];
  skills: StatsSkill[];
  feedback: StatsFeedbackSummary[];
}

// --- Feedback loop (/api/v1/feedback) ---

// User-recorded quality signal about a skill/tool/model/agent turn -- "skill"
// specifically means a Skill-tool invocation (payload.name === "Skill" on a
// tool_call event), distinct from "tool" (any other tool_call).
export type FeedbackKind = "skill" | "tool" | "model" | "agent" | "other";

export type FeedbackStatusFilter = "open" | "resolved" | "all";

export interface Feedback {
  id: string;
  node_id: NodeID;
  session_id: SessionID | "";
  event_id: EventID | "";
  kind: FeedbackKind;
  subject: string;
  comment: string;
  created_at: string;
  resolved_at?: string; // unset = open
  fix_node_id?: NodeID | ""; // set once "Create fix task" links a fix node
}

export interface CreateFeedbackRequest {
  node_id: NodeID;
  session_id?: SessionID;
  event_id?: EventID;
  kind: FeedbackKind;
  subject?: string;
  comment: string;
}

export interface ResolveFeedbackRequest {
  fix_node_id?: NodeID;
}

// --- Node memory (/api/v1/nodes/{id}/memory) ---

// Which slice of a node's tree lineage a memory query covers: the node itself,
// its subtree, or its ancestor chain up to the project that anchors its wing.
export type MemoryScope = "self" | "subtree" | "ancestors";

// How a memory item is classified (for the UI) and who filed it.
export type MemoryKind = "fact" | "decision" | "gotcha" | "convention";
export type MemorySource = "auto" | "agent" | "user";

// One MemPalace-backed memory item mapped into grove's vocabulary. created_at is
// MemPalace's ISO 8601 string (it may lack a timezone).
export interface MemoryEntry {
  id: string;
  kind: MemoryKind;
  content: string;
  source: MemorySource;
  created_at: string;
}

// GET /nodes/{id}/memory. healthy is false with backend:"" when MemPalace is
// unavailable -- the tab shows a "run `grove memory install`" hint, not an error.
export interface MemoryResponse {
  entries: MemoryEntry[];
  backend: string;
  healthy: boolean;
}

// --- Web push (/api/v1/push) ---

// GET /push/key -- the VAPID applicationServerKey (base64url), passed to
// PushManager.subscribe() to authorize this daemon as the push sender.
export interface PushKeyResponse {
  public_key: string;
}

// POST /push/subscribe body -- flattened from the browser's PushSubscription
// (see PushSubscription.toJSON().keys, which carries exactly these two).
export interface PushSubscribeRequest {
  endpoint: string;
  keys: {
    p256dh: string;
    auth: string;
  };
}

// POST /push/unsubscribe body.
export interface PushUnsubscribeRequest {
  endpoint: string;
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
