import type { MemoryEntry, MemoryResponse, MemoryScope } from "../gen/types";

// Fixture memory for VITE_MOCK: a small, believable spread of kinds and sources
// whose visible set changes with the scope switcher, so the node memory tab can
// be exercised without a live MemPalace.

function entry(
  id: string,
  kind: MemoryEntry["kind"],
  source: MemoryEntry["source"],
  content: string,
  minutesAgo: number,
): MemoryEntry {
  return { id, kind, source, content, created_at: new Date(Date.now() - minutesAgo * 60_000).toISOString() };
}

// The node's own room: what this task decided and tripped over.
const SELF: MemoryEntry[] = [
  entry(
    "m-self-1",
    "decision",
    "auto",
    "Chose Postgres over SQLite for the task store — concurrent writers and row-level locking justify the operational cost.",
    28,
  ),
  entry(
    "m-self-2",
    "gotcha",
    "auto",
    "The initial migration takes an ACCESS EXCLUSIVE lock on tasks; run it in a maintenance window, not during peak traffic.",
    92,
  ),
];

// What the subtree (children) learned, folded in under the subtree scope.
const SUBTREE_EXTRA: MemoryEntry[] = [
  entry(
    "m-sub-1",
    "fact",
    "agent",
    "Child task wired the auth middleware; left a TODO to rate-limit POST /login before shipping.",
    18,
  ),
  entry("m-sub-2", "gotcha", "agent", "bcrypt cost 12 adds ~250ms per login — acceptable, but keep an eye on the p95.", 44),
];

// Project-level knowledge inherited from ancestor rooms.
const ANCESTOR_EXTRA: MemoryEntry[] = [
  entry(
    "m-anc-1",
    "convention",
    "user",
    "Every API handler returns the { data, error } envelope — never a bare payload — so clients parse one shape.",
    610,
  ),
  entry(
    "m-anc-2",
    "decision",
    "auto",
    "Project-wide: wrap errors with %w for context and never surface store internals to the client.",
    740,
  ),
];

export function buildFixtureMemory(_nodeId: string, scope: MemoryScope): MemoryResponse {
  let entries = SELF;
  if (scope === "subtree") entries = [...SELF, ...SUBTREE_EXTRA];
  else if (scope === "ancestors") entries = [...SELF, ...ANCESTOR_EXTRA];
  return { entries, backend: "mempalace", healthy: true };
}
