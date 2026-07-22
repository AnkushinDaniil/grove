import { useEffect, useState } from "react";
import clsx from "clsx";
import { Check } from "lucide-react";
import { apiClient } from "../../state/api";
import { FOCUS_RING } from "../../lib/constants";
import type { NodeID, Profile } from "../../gen/types";

interface ProfilePickerPopoverProps {
  nodeId: NodeID;
  /** The full profile list (null while loading), shared from the chip. */
  profiles: Profile[] | null;
  /** The node's own profile_id ("" = inherited from an ancestor). */
  selectedId: string;
  onClose: () => void;
}

/** Popover that sets a node's profile (PATCH /nodes/{id} {profile_id}) or clears
 *  it to inherit from an ancestor. Mirrors WorkDirChip's inline-editor structure;
 *  the chosen profile's config dir is what the node's future sessions run under. */
export function ProfilePickerPopover({
  nodeId,
  profiles,
  selectedId,
  onClose,
}: ProfilePickerPopoverProps) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Escape closes the popover, matching the other node-header editors.
  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
    }
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [onClose]);

  async function pick(profileId: string) {
    if (busy) return;
    setBusy(true);
    setError(null);
    try {
      await apiClient.patchNode(nodeId, { profile_id: profileId });
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      setBusy(false);
    }
  }

  return (
    <div className="absolute top-full left-0 z-20 mt-1 w-60 rounded-lg border border-border-strong bg-surface-2 p-1.5 shadow-panel">
      <p className="px-1.5 py-1 text-2xs font-medium text-ink-faint">Run sessions under…</p>
      <ul className="max-h-56 overflow-y-auto">
        <ProfileOption
          label="Inherit from parent"
          sub="use the nearest ancestor's profile"
          selected={selectedId === ""}
          disabled={busy}
          onClick={() => void pick("")}
        />
        {profiles?.map((p) => (
          <ProfileOption
            key={p.id}
            label={p.name}
            sub={`${p.driver} · ${p.config_dir}`}
            selected={selectedId === p.id}
            disabled={busy}
            onClick={() => void pick(p.id)}
          />
        ))}
      </ul>
      {profiles === null && <p className="px-1.5 py-1 text-2xs text-ink-faint">Loading profiles…</p>}
      {error && <p className="px-1.5 py-1 text-2xs break-words text-status-failed">{error}</p>}
    </div>
  );
}

interface ProfileOptionProps {
  label: string;
  sub: string;
  selected: boolean;
  disabled: boolean;
  onClick: () => void;
}

function ProfileOption({ label, sub, selected, disabled, onClick }: ProfileOptionProps) {
  return (
    <li>
      <button
        type="button"
        onClick={onClick}
        disabled={disabled}
        aria-current={selected}
        className={clsx(
          "flex w-full items-center gap-2 rounded-md px-1.5 py-1 text-left hover:bg-hover disabled:opacity-50",
          FOCUS_RING,
        )}
      >
        <Check
          size={12}
          className={clsx("mt-0.5 shrink-0", selected ? "text-accent" : "text-transparent")}
        />
        <span className="min-w-0 flex-1">
          <span className="block truncate text-xs text-ink">{label}</span>
          <span className="block truncate font-mono text-2xs text-ink-faint">{sub}</span>
        </span>
      </button>
    </li>
  );
}
