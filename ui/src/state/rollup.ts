import type { Node, NodeID, NodeStatus } from "../gen/types";

export interface Rollup {
  total: number;
  byStatus: Record<NodeStatus, number>;
  attentionCount: number;
}

function emptyByStatus(): Record<NodeStatus, number> {
  return {
    idle: 0,
    starting: 0,
    running: 0,
    awaiting_input: 0,
    done: 0,
    failed: 0,
    interrupted: 0,
  };
}

// Module-level memo cache keyed by rev: a new tree rev invalidates every
// node's rollup at once (cheap check), so repeated reads within one rev
// (many rows in the tree rail, rollup mini-bars, etc.) share one computation
// instead of re-walking descendants per render.
let cacheRev = -1;
let cache = new Map<NodeID, Rollup>();

/**
 * Descendant status counts + attention count for a node, memoized per rev.
 * Pure function of the tree snapshot (no store dependency) so it works in
 * tests and outside React.
 */
export function computeRollup(
  nodeId: NodeID,
  childrenByParent: Record<NodeID, NodeID[]>,
  nodesById: Record<NodeID, Node>,
  rev: number,
): Rollup {
  if (rev !== cacheRev) {
    cacheRev = rev;
    cache = new Map();
  }
  const cached = cache.get(nodeId);
  if (cached) return cached;

  const byStatus = emptyByStatus();
  let total = 0;
  let attentionCount = 0;

  const stack = [...(childrenByParent[nodeId] ?? [])];
  while (stack.length > 0) {
    const id = stack.pop();
    if (id === undefined) continue;
    const node = nodesById[id];
    if (!node) continue;
    total += 1;
    byStatus[node.status] += 1;
    if (node.attention !== "none") attentionCount += 1;
    const kids = childrenByParent[id];
    if (kids && kids.length > 0) stack.push(...kids);
  }

  const rollup: Rollup = { total, byStatus, attentionCount };
  cache.set(nodeId, rollup);
  return rollup;
}

/** Test-only: the memo cache is module-level (by design, see above), so
 *  tests that construct multiple independent fixtures at the same rev need
 *  to clear it between cases to avoid cross-test contamination. */
export function _resetRollupCacheForTests(): void {
  cacheRev = -1;
  cache = new Map();
}
