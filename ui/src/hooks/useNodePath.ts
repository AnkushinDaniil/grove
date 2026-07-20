import { useTreeStore } from "../state/tree";
import type { Node, NodeID } from "../gen/types";

/** Ancestor chain from the workspace root down to (and including) nodeId,
 *  for breadcrumbs. Returns [] if the node is unknown. */
export function useNodePath(nodeId: NodeID | undefined): Node[] {
  const nodesById = useTreeStore((s) => s.nodesById);
  if (!nodeId) return [];

  const path: Node[] = [];
  let current: Node | undefined = nodesById[nodeId];
  const seen = new Set<NodeID>();
  while (current && !seen.has(current.id)) {
    seen.add(current.id); // guards against a corrupt/cyclic parent chain
    path.unshift(current);
    current = current.parent_id ? nodesById[current.parent_id] : undefined;
  }
  return path;
}
