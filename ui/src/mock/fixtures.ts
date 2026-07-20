import type { Event, EventID, Node, NodeID, Session, UsageWindow, UsageWindowKind } from "../gen/types";

/**
 * A demo tree: one workspace, two projects, tasks/subtasks nested up to
 * three levels deep, spanning every NodeStatus and every Attention value
 * that the M1 event contract can actually produce (see NOTE below on
 * "review"). Timestamps are generated relative to load time so relative-time
 * labels ("2m ago") always look sensible.
 *
 * NOTE on `attention: "review"` (task-stripe below): internal/core/event.go's
 * AttentionFor() only maps awaiting_input/turn_done/error events to
 * attention; nothing in the frozen M1 EventType vocabulary produces
 * "review" (per Attention's own doc comment it's for "dirty worktree, PR
 * comments" -- worktree/github integration, which is M2/M3). We still model
 * it on a Node (it's a valid enum value the UI must render), but we don't
 * synthesize a matching inbox Event for it, since no real event would
 * exist yet either.
 */

function ago(ms: number): string {
  return new Date(Date.now() - ms).toISOString();
}

const MIN = 60_000;
const HOUR = 60 * MIN;

const SEC = 1000;

let eventSeq = 0;
function eventId(): EventID {
  eventSeq += 1;
  return `evt-${eventSeq}`;
}

function node(partial: Omit<Node, "meta" | "attention_reason"> & { meta?: Record<string, unknown>; attention_reason?: string }): Node {
  return {
    attention_reason: "",
    meta: {},
    ...partial,
  };
}

export const ROOT_ID: NodeID = "ws-daniil";
export const PROJECT_GROVE_ID: NodeID = "proj-grove";
export const PROJECT_BILLING_ID: NodeID = "proj-billing";
export const TASK_UI_ID: NodeID = "task-ui";
export const TASK_UI_TREE_ID: NodeID = "task-ui-tree";
export const TASK_UI_TERM_ID: NodeID = "task-ui-term";
export const TASK_UI_PALETTE_ID: NodeID = "task-ui-palette";
export const TASK_FLAKE_ID: NodeID = "task-flake";
export const TASK_GOLDEN_ID: NodeID = "task-golden";
export const TASK_STRIPE_ID: NodeID = "task-stripe";
export const TASK_PG17_ID: NodeID = "task-pg17";
export const TASK_SLOW_QUERY_ID: NodeID = "task-slow-query";
export const TASK_INDEX_ID: NodeID = "task-index";

export const SESSION_TERM_ID = "sess-term";
export const SESSION_FLAKE_ID = "sess-flake";
export const SESSION_GOLDEN_ID = "sess-golden";

export function buildFixtureNodes(): Node[] {
  return [
    node({
      id: ROOT_ID,
      parent_id: "",
      kind: "workspace",
      title: "daniil",
      brief: "",
      status: "idle",
      attention: "none",
      driver: "claude",
      profile_id: "",
      current_session_id: "",
      workspace_dir: "",
      position: 0,
      created_at: ago(30 * HOUR),
      updated_at: ago(1 * MIN),
    }),
    node({
      id: PROJECT_GROVE_ID,
      parent_id: ROOT_ID,
      kind: "project",
      title: "grove",
      brief: "Tree-of-agents manager -- the daemon and UI you're looking at.",
      status: "idle",
      attention: "none",
      driver: "",
      profile_id: "",
      current_session_id: "",
      workspace_dir: "~/code/grove",
      position: 0,
      created_at: ago(30 * HOUR),
      updated_at: ago(2 * MIN),
    }),
    node({
      id: TASK_UI_ID,
      parent_id: PROJECT_GROVE_ID,
      kind: "task",
      title: "Build UI foundation",
      brief: "Vite + React + xterm.js web UI per docs/API.md and docs/DESIGN.md.",
      status: "running",
      attention: "none",
      driver: "",
      profile_id: "",
      current_session_id: "",
      workspace_dir: "~/.grove/worktrees/a1b2c3d4-build-ui-foundation",
      position: 1,
      created_at: ago(6 * HOUR),
      updated_at: ago(20 * SEC),
    }),
    node({
      id: TASK_UI_TREE_ID,
      parent_id: TASK_UI_ID,
      kind: "task",
      title: "Tree rail + keyboard nav",
      brief: "Recursive tree, status dots, j/k/h/l navigation, collapse persistence.",
      status: "done",
      attention: "done",
      attention_since: ago(4 * MIN),
      driver: "",
      profile_id: "",
      current_session_id: "",
      workspace_dir: "~/.grove/worktrees/e5f6a7b8-tree-rail",
      position: 0,
      created_at: ago(5 * HOUR),
      updated_at: ago(4 * MIN),
    }),
    node({
      id: TASK_UI_TERM_ID,
      parent_id: TASK_UI_ID,
      kind: "task",
      title: "Terminal attach (xterm)",
      brief: "Replay + live attach over /ws/term, WebGL on focus, LRU mount pool.",
      status: "running",
      attention: "none",
      driver: "",
      profile_id: "",
      current_session_id: SESSION_TERM_ID,
      workspace_dir: "~/.grove/worktrees/c9d0e1f2-terminal-attach",
      position: 1,
      created_at: ago(3 * HOUR),
      updated_at: ago(5 * SEC),
    }),
    node({
      id: TASK_UI_PALETTE_ID,
      parent_id: TASK_UI_ID,
      kind: "task",
      title: "Command palette",
      brief: "cmdk-based Cmd+K: fuzzy node jump + quick actions.",
      status: "idle",
      attention: "none",
      driver: "",
      profile_id: "",
      current_session_id: "",
      workspace_dir: "",
      position: 2,
      created_at: ago(3 * HOUR),
      updated_at: ago(3 * HOUR),
    }),
    node({
      id: TASK_FLAKE_ID,
      parent_id: PROJECT_GROVE_ID,
      kind: "task",
      title: "Fix rev-gap reconnect flake",
      brief: "Occasional double-reconnect under fast network flap in ws test harness.",
      status: "awaiting_input",
      attention: "permission",
      attention_reason: "Allow running `git worktree prune`?",
      attention_since: ago(90 * SEC),
      driver: "",
      profile_id: "",
      current_session_id: SESSION_FLAKE_ID,
      workspace_dir: "~/.grove/worktrees/11223344-rev-gap-flake",
      position: 2,
      created_at: ago(2 * HOUR),
      updated_at: ago(90 * SEC),
    }),
    node({
      id: TASK_GOLDEN_ID,
      parent_id: PROJECT_GROVE_ID,
      kind: "task",
      title: "Write driver golden tests",
      brief: "Table-tested fixtures for claude driver JSONL -> normalized events.",
      status: "failed",
      attention: "error",
      attention_reason: "3 golden fixture mismatches in claude driver",
      attention_since: ago(11 * MIN),
      driver: "",
      profile_id: "",
      current_session_id: SESSION_GOLDEN_ID,
      workspace_dir: "~/.grove/worktrees/55667788-golden-tests",
      position: 3,
      created_at: ago(4 * HOUR),
      updated_at: ago(11 * MIN),
    }),
    node({
      id: PROJECT_BILLING_ID,
      parent_id: ROOT_ID,
      kind: "project",
      title: "billing-service",
      brief: "Internal billing API (Go + Postgres).",
      status: "idle",
      attention: "none",
      driver: "",
      profile_id: "",
      current_session_id: "",
      workspace_dir: "~/code/billing-service",
      position: 1,
      created_at: ago(28 * HOUR),
      updated_at: ago(40 * MIN),
    }),
    node({
      id: TASK_STRIPE_ID,
      parent_id: PROJECT_BILLING_ID,
      kind: "task",
      title: "Add Stripe webhook handler",
      brief: "Handle invoice.paid / invoice.payment_failed, idempotent by event id.",
      status: "done",
      attention: "review",
      attention_reason: "Worktree has 1 open PR comment thread",
      attention_since: ago(40 * MIN),
      driver: "",
      profile_id: "",
      current_session_id: "",
      workspace_dir: "~/.grove/worktrees/99aabbcc-stripe-webhooks",
      meta: { pr_url: "https://github.com/daniil/billing-service/pull/142" },
      position: 0,
      created_at: ago(20 * HOUR),
      updated_at: ago(40 * MIN),
    }),
    node({
      id: TASK_PG17_ID,
      parent_id: PROJECT_BILLING_ID,
      kind: "task",
      title: "Migrate to Postgres 17",
      brief: "Bump server version, verify logical replication compatibility.",
      status: "interrupted",
      attention: "none",
      driver: "",
      profile_id: "",
      current_session_id: "",
      workspace_dir: "~/.grove/worktrees/ddeeff00-pg17",
      position: 1,
      created_at: ago(15 * HOUR),
      updated_at: ago(3 * HOUR),
    }),
    node({
      id: TASK_SLOW_QUERY_ID,
      parent_id: PROJECT_BILLING_ID,
      kind: "task",
      title: "Investigate slow query on /invoices",
      brief: "P99 latency regression since the customer_id filter shipped.",
      status: "idle",
      attention: "none",
      driver: "",
      profile_id: "",
      current_session_id: "",
      workspace_dir: "",
      position: 2,
      created_at: ago(10 * HOUR),
      updated_at: ago(10 * HOUR),
    }),
    node({
      id: TASK_INDEX_ID,
      parent_id: TASK_SLOW_QUERY_ID,
      kind: "task",
      title: "Add index on invoices.customer_id",
      brief: "",
      status: "idle",
      attention: "none",
      driver: "",
      profile_id: "",
      current_session_id: "",
      workspace_dir: "",
      position: 0,
      created_at: ago(9 * HOUR),
      updated_at: ago(9 * HOUR),
    }),
  ];
}

export function buildFixtureSessions(): Session[] {
  return [
    {
      id: SESSION_TERM_ID,
      node_id: TASK_UI_TERM_ID,
      driver: "claude",
      profile_id: "",
      mode: "pty",
      driver_session_id: "claude-sess-3f9a7c",
      status: "running",
      cwd: "~/.grove/worktrees/c9d0e1f2-terminal-attach",
      started_at: ago(3 * MIN),
    },
    {
      id: SESSION_FLAKE_ID,
      node_id: TASK_FLAKE_ID,
      driver: "claude",
      profile_id: "",
      mode: "pty",
      driver_session_id: "claude-sess-8b21e0",
      status: "awaiting_input",
      cwd: "~/.grove/worktrees/11223344-rev-gap-flake",
      started_at: ago(4 * MIN),
    },
    {
      id: SESSION_GOLDEN_ID,
      node_id: TASK_GOLDEN_ID,
      driver: "claude",
      profile_id: "",
      mode: "headless",
      driver_session_id: "claude-sess-1c44d9",
      status: "failed",
      exit_code: 1,
      cwd: "~/.grove/worktrees/55667788-golden-tests",
      started_at: ago(14 * MIN),
      ended_at: ago(11 * MIN),
    },
  ];
}

function textEvent(nodeId: NodeID, sessionId: string, text: string, ageMs: number, final = false): Event {
  return {
    id: eventId(),
    node_id: nodeId,
    session_id: sessionId,
    type: "text",
    payload: { text, final },
    requires_attention: false,
    created_at: ago(ageMs),
  };
}

function toolEvents(nodeId: NodeID, sessionId: string, name: string, summary: string, ageMs: number): Event[] {
  return [
    {
      id: eventId(),
      node_id: nodeId,
      session_id: sessionId,
      type: "tool_call",
      payload: { name, input_summary: summary },
      requires_attention: false,
      created_at: ago(ageMs),
    },
    {
      id: eventId(),
      node_id: nodeId,
      session_id: sessionId,
      type: "tool_result",
      payload: { name, ok: true, summary: "done" },
      requires_attention: false,
      created_at: ago(ageMs - 2 * SEC),
    },
  ];
}

export function buildFixtureEvents(): Event[] {
  const events: Event[] = [];

  // task-ui-tree: finished, unacked -- lands in the inbox.
  events.push({
    id: eventId(),
    node_id: TASK_UI_TREE_ID,
    session_id: "",
    type: "turn_done",
    payload: {
      result_text: "Implemented recursive tree rail with keyboard nav; collapse state persists to localStorage.",
      duration_ms: 96_000,
    },
    requires_attention: true,
    created_at: ago(4 * MIN),
  });

  // task-ui-term: a believable in-flight session history.
  events.push({
    id: eventId(),
    node_id: TASK_UI_TERM_ID,
    session_id: SESSION_TERM_ID,
    type: "session_started",
    payload: { driver_session_id: "claude-sess-3f9a7c", model: "claude-opus-4-8" },
    requires_attention: false,
    created_at: ago(3 * MIN),
  });
  events.push(...toolEvents(TASK_UI_TERM_ID, SESSION_TERM_ID, "Read", "src/terminal/XtermHost.tsx", 2 * MIN + 40 * SEC));
  events.push(
    textEvent(
      TASK_UI_TERM_ID,
      SESSION_TERM_ID,
      "Wiring the fit addon to a ResizeObserver now so the pty stays in sync with the pane size.",
      2 * MIN,
    ),
  );
  events.push(...toolEvents(TASK_UI_TERM_ID, SESSION_TERM_ID, "Edit", "src/terminal/XtermHost.tsx", 80 * SEC));
  events.push({
    id: eventId(),
    node_id: TASK_UI_TERM_ID,
    session_id: SESSION_TERM_ID,
    type: "usage",
    payload: { input_tokens: 48_213, output_tokens: 6_104, cost_usd: 0.87 },
    requires_attention: false,
    created_at: ago(70 * SEC),
  });

  // task-flake: awaiting a permission decision -- unacked, in the inbox.
  events.push({
    id: eventId(),
    node_id: TASK_FLAKE_ID,
    session_id: SESSION_FLAKE_ID,
    type: "awaiting_input",
    payload: { reason: "permission", detail: "Allow running `git worktree prune`?" },
    requires_attention: true,
    created_at: ago(90 * SEC),
  });

  // task-golden: a fatal error -- unacked, in the inbox.
  events.push({
    id: eventId(),
    node_id: TASK_GOLDEN_ID,
    session_id: SESSION_GOLDEN_ID,
    type: "session_ended",
    payload: { exit_code: 1 },
    requires_attention: false,
    created_at: ago(11 * MIN),
  });
  events.push({
    id: eventId(),
    node_id: TASK_GOLDEN_ID,
    session_id: SESSION_GOLDEN_ID,
    type: "error",
    payload: { message: "3 golden fixture mismatches in claude driver: tool_call summary truncation differs", fatal: true },
    requires_attention: true,
    created_at: ago(11 * MIN),
  });

  // task-stripe: history only -- current "review" attention isn't event-backed (see file header).
  events.push({
    id: eventId(),
    node_id: TASK_STRIPE_ID,
    session_id: "",
    type: "turn_done",
    payload: { result_text: "PR #142 opened: Stripe webhook handler with idempotency key table.", duration_ms: 212_000 },
    requires_attention: true,
    acked_at: ago(39 * MIN),
    created_at: ago(40 * MIN),
  });

  return events.sort((a, b) => a.created_at.localeCompare(b.created_at));
}

// Two claude profiles: "personal" comfortably under its 5h window, "work"
// currently rate-limited (cooldown_until set) -- per team-lead's spec, so
// UsageMeter is demoable in both the normal-bar and cooldown states.
export function buildFixtureUsage(window: UsageWindowKind): UsageWindow[] {
  const now = Date.now();
  if (window === "5h") {
    return [
      {
        profile_id: "profile-personal",
        name: "personal",
        driver: "claude",
        window: "5h",
        window_start: ago(2 * HOUR + 10 * MIN),
        window_end: new Date(now + 2 * HOUR + 50 * MIN).toISOString(),
        input_tokens: 812_400,
        output_tokens: 94_300,
        cache_read_tokens: 2_140_000,
        cost_usd: 14.82,
        utilization: 0.83,
        resets_at: new Date(now + 2 * HOUR + 50 * MIN).toISOString(),
      },
      {
        profile_id: "profile-work",
        name: "work",
        driver: "claude",
        window: "5h",
        window_start: ago(3 * HOUR + 40 * MIN),
        window_end: new Date(now + 1 * HOUR + 20 * MIN).toISOString(),
        input_tokens: 1_980_000,
        output_tokens: 241_000,
        cache_read_tokens: 5_760_000,
        cost_usd: 41.07,
        utilization: 0.98,
        resets_at: new Date(now + 12 * MIN).toISOString(),
        cooldown_until: new Date(now + 12 * MIN).toISOString(),
      },
    ];
  }
  return [
    {
      profile_id: "profile-personal",
      name: "personal",
      driver: "claude",
      window: "week",
      window_start: ago(4 * 24 * HOUR),
      window_end: new Date(now + 3 * 24 * HOUR).toISOString(),
      input_tokens: 9_240_000,
      output_tokens: 1_105_000,
      cache_read_tokens: 24_600_000,
      cost_usd: 168.31,
      utilization: 0.46,
      resets_at: new Date(now + 3 * 24 * HOUR).toISOString(),
    },
    {
      profile_id: "profile-work",
      name: "work",
      driver: "claude",
      window: "week",
      window_start: ago(1 * 24 * HOUR),
      window_end: new Date(now + 6 * 24 * HOUR).toISOString(),
      input_tokens: 18_900_000,
      output_tokens: 2_260_000,
      cache_read_tokens: 51_200_000,
      cost_usd: 349.6,
      utilization: 0.61,
      resets_at: new Date(now + 6 * 24 * HOUR).toISOString(),
    },
  ];
}
