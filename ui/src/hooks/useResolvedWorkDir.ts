import { useTreeStore } from "../state/tree";
import type { NodeID } from "../gen/types";

export interface ResolvedWorkDir {
  /** Effective working directory: the resolved absolute path, or "~" (the
   *  user's home) when neither the node nor any ancestor sets one. */
  value: string;
  /** true if the value was resolved from an ancestor (or the home fallback)
   *  rather than being set on this node. */
  inherited: boolean;
}

/** Placeholder for "no work dir set anywhere": sessions then start in the
 *  user's home directory (see internal/session workingDir). */
const HOME_FALLBACK = "~";

/** Walks the ancestor chain to resolve work_dir per API.md's "empty =
 *  inherited" rule (nearest non-empty ancestor wins) -- inheritance is derived
 *  on demand, never stored, so the UI does the walk itself, mirroring
 *  useResolvedDriverProfile. */
export function useResolvedWorkDir(nodeId: NodeID | undefined): ResolvedWorkDir {
  const nodesById = useTreeStore((s) => s.nodesById);

  if (!nodeId || !nodesById[nodeId]) {
    return { value: HOME_FALLBACK, inherited: true };
  }

  const node = nodesById[nodeId];
  let value = node.work_dir || undefined;

  let cursor = node.parent_id || undefined;
  const seen = new Set<NodeID>();
  while (value === undefined && cursor && !seen.has(cursor)) {
    seen.add(cursor);
    const ancestor = nodesById[cursor];
    if (!ancestor) break;
    if (ancestor.work_dir) value = ancestor.work_dir;
    cursor = ancestor.parent_id || undefined;
  }

  return {
    value: value ?? HOME_FALLBACK,
    inherited: !node.work_dir,
  };
}
