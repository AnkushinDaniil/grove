import { useTreeStore } from "../state/tree";
import type { NodeID } from "../gen/types";

export interface ResolvedField {
  value: string;
  /** true if resolved from an ancestor rather than set on this node. */
  inherited: boolean;
}

export interface ResolvedDriverProfile {
  driver: ResolvedField;
  profileId: ResolvedField;
}

const FALLBACK_DRIVER = "claude";
const FALLBACK_PROFILE = "default";

/** Walks the ancestor chain to resolve driver/profile_id per API.md's
 *  "empty = inherited" rule -- inheritance is derived on demand, never
 *  stored (see docs/DESIGN.md), so the UI has to do this walk itself. */
export function useResolvedDriverProfile(nodeId: NodeID | undefined): ResolvedDriverProfile {
  const nodesById = useTreeStore((s) => s.nodesById);

  if (!nodeId || !nodesById[nodeId]) {
    return {
      driver: { value: FALLBACK_DRIVER, inherited: true },
      profileId: { value: FALLBACK_PROFILE, inherited: true },
    };
  }

  const node = nodesById[nodeId];
  let driverValue = node.driver || undefined;
  let profileValue = node.profile_id || undefined;

  let cursor = node.parent_id || undefined;
  const seen = new Set<NodeID>();
  while ((driverValue === undefined || profileValue === undefined) && cursor && !seen.has(cursor)) {
    seen.add(cursor);
    const ancestor = nodesById[cursor];
    if (!ancestor) break;
    if (driverValue === undefined && ancestor.driver) driverValue = ancestor.driver;
    if (profileValue === undefined && ancestor.profile_id) profileValue = ancestor.profile_id;
    cursor = ancestor.parent_id || undefined;
  }

  return {
    driver: { value: driverValue ?? FALLBACK_DRIVER, inherited: !node.driver },
    profileId: { value: profileValue ?? FALLBACK_PROFILE, inherited: !node.profile_id },
  };
}
