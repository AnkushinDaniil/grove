import { Pill } from "../common/Pill";
import { useResolvedDriverProfile } from "../../hooks/useResolvedDriverProfile";
import type { NodeID } from "../../gen/types";

interface DriverProfileChipsProps {
  nodeId: NodeID;
}

export function DriverProfileChips({ nodeId }: DriverProfileChipsProps) {
  const { driver, profileId } = useResolvedDriverProfile(nodeId);
  return (
    <>
      <Pill
        tone={driver.inherited ? "muted" : "neutral"}
        title={driver.inherited ? "Inherited from an ancestor" : "Set on this node"}
      >
        {driver.value}
        {driver.inherited && <span className="text-ink-disabled">inherited</span>}
      </Pill>
      <Pill
        tone={profileId.inherited ? "muted" : "neutral"}
        title={profileId.inherited ? "Inherited from an ancestor" : "Set on this node"}
      >
        {profileId.value}
        {profileId.inherited && <span className="text-ink-disabled">inherited</span>}
      </Pill>
    </>
  );
}
