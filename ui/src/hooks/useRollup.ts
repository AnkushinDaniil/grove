import { useTreeStore } from "../state/tree";
import { computeRollup, type Rollup } from "../state/rollup";
import type { NodeID } from "../gen/types";

export function useRollup(nodeId: NodeID): Rollup {
  const childrenByParent = useTreeStore((s) => s.childrenByParent);
  const nodesById = useTreeStore((s) => s.nodesById);
  const rev = useTreeStore((s) => s.rev);
  return computeRollup(nodeId, childrenByParent, nodesById, rev);
}
