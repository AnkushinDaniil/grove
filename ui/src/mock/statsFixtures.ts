import type {
  AgentsByDay,
  StatsAgents,
  StatsFlow,
  StatsModel,
  StatsRange,
  StatsResponse,
  StatsSkill,
  StatsTokens,
  StatsTool,
  TokenByDay,
  TokenTopNode,
} from "../gen/types";
import { HOUR, PROJECT_BILLING_ID, PROJECT_GROVE_ID, TASK_FLAKE_ID, TASK_GOLDEN_ID, TASK_PG17_ID, TASK_STRIPE_ID, TASK_UI_ID } from "./fixtures";
import { feedbackWorld } from "./feedbackWorld";
import { world } from "./world";

const DAY = 24 * HOUR;

function daysFor(range: StatsRange): number {
  switch (range) {
    case "24h":
      return 2;
    case "7d":
      return 7;
    case "30d":
      return 30;
  }
}

// Deterministic pseudo-random in [0, 1) from a string seed, so fixture
// numbers are stable across re-renders/polls instead of jittering on every
// refetch (a jittery demo dashboard reads as broken, not "live").
function seeded(seed: string): number {
  let h = 0;
  for (let i = 0; i < seed.length; i++) h = (h * 31 + seed.charCodeAt(i)) | 0;
  return (Math.abs(h) % 10_000) / 10_000;
}

function dayKey(daysAgo: number): string {
  return new Date(Date.now() - daysAgo * DAY).toISOString().slice(0, 10);
}

function buildTokensByDay(range: StatsRange): TokenByDay[] {
  const n = daysFor(range);
  const days: TokenByDay[] = [];
  for (let i = n - 1; i >= 0; i--) {
    const key = dayKey(i);
    const weekday = new Date(Date.now() - i * DAY).getDay();
    const weekendDip = weekday === 0 || weekday === 6 ? 0.45 : 1;
    const noise = 0.7 + seeded(`tok-${key}`) * 0.6;
    const input = Math.round(180_000 * weekendDip * noise);
    const output = Math.round(input * (0.11 + seeded(`out-${key}`) * 0.03));
    const cost_usd = Number((input * 0.000003 + output * 0.000015).toFixed(2));
    days.push({ day: key, input, output, cost_usd });
  }
  return days;
}

function buildSessionsByDay(range: StatsRange): AgentsByDay[] {
  const n = daysFor(range);
  const days: AgentsByDay[] = [];
  for (let i = n - 1; i >= 0; i--) {
    const key = dayKey(i);
    const started = 3 + Math.round(seeded(`start-${key}`) * 9);
    const failed = Math.round(seeded(`fail-${key}`) * Math.min(2, started));
    const stillOpen = i === 0 ? 1 : 0; // today has one session not yet resolved
    const done = Math.max(0, started - failed - stillOpen);
    days.push({ day: key, started, done, failed });
  }
  return days;
}

// Raw (unscaled) per-node token/cost figures -- reuses real fixture node ids
// so "top nodes by cost" click-through lands on an actual node in the tree.
const ALL_TOP_NODES: TokenTopNode[] = [
  { node_id: PROJECT_GROVE_ID, title: "grove", input: 2_140_000, output: 268_000, cost_usd: 26.4 },
  { node_id: TASK_UI_ID, title: "Build UI foundation", input: 812_000, output: 96_000, cost_usd: 9.8 },
  { node_id: PROJECT_BILLING_ID, title: "billing-service", input: 610_000, output: 74_000, cost_usd: 7.4 },
  { node_id: TASK_STRIPE_ID, title: "Add Stripe webhook handler", input: 410_000, output: 58_000, cost_usd: 5.6 },
  { node_id: TASK_GOLDEN_ID, title: "Write driver golden tests", input: 340_000, output: 41_000, cost_usd: 4.1 },
  { node_id: TASK_FLAKE_ID, title: "Fix rev-gap reconnect flake", input: 190_000, output: 22_000, cost_usd: 2.3 },
  { node_id: TASK_PG17_ID, title: "Migrate to Postgres 17", input: 95_000, output: 11_000, cost_usd: 1.2 },
];

function isWithinScope(nodeId: string, scope: string): boolean {
  if (nodeId === scope) return true;
  let cur = world.nodesById.get(nodeId);
  while (cur?.parent_id) {
    if (cur.parent_id === scope) return true;
    cur = world.nodesById.get(cur.parent_id);
  }
  return false;
}

function scaleRound(n: number, scale: number): number {
  return Math.round(n * scale);
}

/** Builds a full StatsResponse for mock mode. When `scope` narrows to a
 *  subtree, top_nodes is genuinely filtered to that subtree and every other
 *  additive count/total scales by the scoped nodes' share of total cost, so
 *  switching scope visibly moves the whole dashboard together -- without a
 *  real per-node event join, which is out of reach for a UI-only mock.
 *  Rate/average fields (median task hours, attention wait, avg session
 *  minutes) are left unscaled since an average doesn't shrink with the
 *  denominator. feedback is always workspace-wide: it's a quality signal,
 *  not a cost metric. */
export function buildFixtureStats(scope: string | undefined, range: StatsRange): StatsResponse {
  const rootId = [...world.nodesById.values()].find((n) => n.kind === "workspace" && !n.archived_at)?.id;
  const scoped = Boolean(scope && scope !== rootId);
  const topNodes = scoped ? ALL_TOP_NODES.filter((n) => isWithinScope(n.node_id, scope!)) : ALL_TOP_NODES;

  const totalCost = ALL_TOP_NODES.reduce((s, n) => s + n.cost_usd, 0);
  const scopedCost = topNodes.reduce((s, n) => s + n.cost_usd, 0);
  const scale = scoped ? Math.max(0.05, scopedCost / totalCost) : 1;

  const byDay = buildTokensByDay(range).map((d) => ({
    day: d.day,
    input: scaleRound(d.input, scale),
    output: scaleRound(d.output, scale),
    cost_usd: Number((d.cost_usd * scale).toFixed(2)),
  }));
  const totalInput = byDay.reduce((s, d) => s + d.input, 0);
  const totalOutput = byDay.reduce((s, d) => s + d.output, 0);
  const totalCostUsd = Number(byDay.reduce((s, d) => s + d.cost_usd, 0).toFixed(2));

  const tokens: StatsTokens = {
    total: { input: totalInput, output: totalOutput, cache_read: Math.round(totalInput * 2.6), cost_usd: totalCostUsd },
    by_day: byDay,
    by_driver: [
      { driver: "claude", input: Math.round(totalInput * 0.62), output: Math.round(totalOutput * 0.6), cost_usd: Number((totalCostUsd * 0.6).toFixed(2)) },
      { driver: "codex", input: Math.round(totalInput * 0.24), output: Math.round(totalOutput * 0.26), cost_usd: Number((totalCostUsd * 0.26).toFixed(2)) },
      { driver: "gemini", input: Math.round(totalInput * 0.14), output: Math.round(totalOutput * 0.14), cost_usd: Number((totalCostUsd * 0.14).toFixed(2)) },
    ],
    by_profile: [
      { profile_id: "profile-personal", name: "personal", input: Math.round(totalInput * 0.55), output: Math.round(totalOutput * 0.55), cost_usd: Number((totalCostUsd * 0.55).toFixed(2)) },
      { profile_id: "profile-work", name: "work", input: Math.round(totalInput * 0.45), output: Math.round(totalOutput * 0.45), cost_usd: Number((totalCostUsd * 0.45).toFixed(2)) },
    ],
    top_nodes: [...topNodes].sort((a, b) => b.cost_usd - a.cost_usd),
  };

  const sessionsByDay = buildSessionsByDay(range).map((d) => ({
    day: d.day,
    started: scaleRound(d.started, scale),
    done: scaleRound(d.done, scale),
    failed: scaleRound(d.failed, scale),
  }));
  const agents: StatsAgents = {
    sessions_active: scaleRound(3, scale),
    sessions_by_day: sessionsByDay,
    avg_session_minutes: 14.3,
    by_driver: [
      { driver: "claude", count: scaleRound(28, scale) },
      { driver: "codex", count: scaleRound(9, scale) },
      { driver: "gemini", count: scaleRound(5, scale) },
    ],
  };

  const flow: StatsFlow = {
    tasks_created: scaleRound(22, scale),
    tasks_done: scaleRound(15, scale),
    tasks_failed: scaleRound(3, scale),
    median_task_hours: 4.2,
    attention_wait_p50_minutes: 6.5,
    attention_wait_p95_minutes: 38.4,
    prs_opened: scaleRound(6, scale),
    prs_merged: scaleRound(4, scale),
  };

  const tools: StatsTool[] = [
    { name: "Read", calls: scaleRound(480, scale), errors: 0 },
    { name: "Bash", calls: scaleRound(340, scale), errors: scaleRound(6, scale) },
    { name: "Edit", calls: scaleRound(210, scale), errors: scaleRound(2, scale) },
    { name: "Grep", calls: scaleRound(96, scale), errors: scaleRound(1, scale) },
    { name: "WebFetch", calls: scaleRound(42, scale), errors: scaleRound(9, scale) },
  ];

  const models: StatsModel[] = [
    { model: "claude-sonnet-5", input: Math.round(totalInput * 0.6), output: Math.round(totalOutput * 0.6), cost_usd: Number((totalCostUsd * 0.55).toFixed(2)) },
    { model: "claude-opus-4-8", input: Math.round(totalInput * 0.4), output: Math.round(totalOutput * 0.4), cost_usd: Number((totalCostUsd * 0.45).toFixed(2)) },
  ];

  const skills: StatsSkill[] = [
    { skill: "code-review", invocations: scaleRound(14, scale) },
    { skill: "tdd-guide", invocations: scaleRound(8, scale) },
    { skill: "planner", invocations: scaleRound(6, scale) },
    { skill: "dataviz", invocations: scaleRound(3, scale) },
  ];

  return {
    range,
    scope: scope ?? "",
    tokens,
    agents,
    flow,
    tools,
    models,
    skills,
    feedback: feedbackWorld.summary(),
  };
}
