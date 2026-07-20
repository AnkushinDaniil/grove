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

export interface AuthSessionRequest {
  token: string;
}

export interface ApiErrorBody {
  error: string;
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
