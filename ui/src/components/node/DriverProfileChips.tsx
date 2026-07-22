import { useEffect, useMemo, useState } from "react";
import clsx from "clsx";
import { Pill } from "../common/Pill";
import { ProfilePickerPopover } from "./ProfilePickerPopover";
import { useResolvedDriverProfile } from "../../hooks/useResolvedDriverProfile";
import { useProfiles } from "../../hooks/useProfiles";
import { FOCUS_RING } from "../../lib/constants";
import type { NodeID } from "../../gen/types";

interface DriverProfileChipsProps {
  nodeId: NodeID;
}

export function DriverProfileChips({ nodeId }: DriverProfileChipsProps) {
  const { driver, profileId } = useResolvedDriverProfile(nodeId);
  const { profiles } = useProfiles();
  const [pickerOpen, setPickerOpen] = useState(false);

  // Collapse the picker when navigating to another node.
  useEffect(() => {
    setPickerOpen(false);
  }, [nodeId]);

  // Show the resolved profile's friendly name once the list has loaded; fall
  // back to the raw resolved value (a profile id, or the "default" placeholder).
  const profileLabel = useMemo(() => {
    const match = profiles?.find((p) => p.id === profileId.value);
    return match ? match.name : profileId.value;
  }, [profiles, profileId.value]);

  return (
    <>
      <Pill
        tone={driver.inherited ? "muted" : "neutral"}
        title={driver.inherited ? "Inherited from an ancestor" : "Set on this node"}
      >
        {driver.value}
        {driver.inherited && <span className="text-ink-disabled">inherited</span>}
      </Pill>
      <span className="relative inline-flex">
        <button
          type="button"
          onClick={() => setPickerOpen((v) => !v)}
          aria-label="Profile"
          aria-expanded={pickerOpen}
          title={
            profileId.inherited
              ? "Inherited from an ancestor — click to set a profile on this node"
              : "Click to change this node's profile"
          }
          className={clsx("rounded-md", FOCUS_RING)}
        >
          <Pill tone={profileId.inherited ? "muted" : "neutral"}>
            {profileLabel}
            {profileId.inherited && <span className="text-ink-disabled">inherited</span>}
          </Pill>
        </button>
        {pickerOpen && (
          <ProfilePickerPopover
            nodeId={nodeId}
            profiles={profiles}
            selectedId={profileId.inherited ? "" : profileId.value}
            onClose={() => setPickerOpen(false)}
          />
        )}
      </span>
    </>
  );
}
