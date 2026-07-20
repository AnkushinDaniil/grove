import type { NodeID } from "../gen/types";

/** Depth-first, collapse-aware flattening of the tree starting at rootId --
 *  the order j/k keyboard navigation walks through. Pure so it's reusable
 *  (and testable) outside the tree rail component itself. */
export function flattenVisible(
  rootId: NodeID,
  childrenByParent: Record<NodeID, NodeID[]>,
  isCollapsed: (id: NodeID) => boolean,
): NodeID[] {
  const result: NodeID[] = [];
  function walk(id: NodeID) {
    result.push(id);
    if (isCollapsed(id)) return;
    for (const child of childrenByParent[id] ?? []) walk(child);
  }
  walk(rootId);
  return result;
}
