import { useEffect, useRef, useState } from "react";
import clsx from "clsx";
import { Folder } from "lucide-react";
import { Pill } from "../common/Pill";
import { useResolvedWorkDir } from "../../hooks/useResolvedWorkDir";
import { apiClient } from "../../state/api";
import { FOCUS_RING } from "../../lib/constants";
import type { NodeID } from "../../gen/types";

interface WorkDirChipProps {
  nodeId: NodeID;
}

const MAX_LEN = 36;

/** Middle-truncates a path so both the mount root and the leaf stay readable
 *  (the full path is always available via the title tooltip). */
function middleTruncate(path: string, max: number): string {
  if (path.length <= max) return path;
  const side = Math.floor((max - 1) / 2);
  return `${path.slice(0, side)}…${path.slice(path.length - side)}`;
}

/** The working-directory chip in the node header: shows the effective work dir
 *  (muted when inherited, neutral when set on this node) and opens an inline
 *  editor to set or clear the node's own override. */
export function WorkDirChip({ nodeId }: WorkDirChipProps) {
  const { value, inherited } = useResolvedWorkDir(nodeId);
  const [open, setOpen] = useState(false);

  // Collapse the editor when navigating to another node.
  useEffect(() => {
    setOpen(false);
  }, [nodeId]);

  return (
    <span className="relative inline-flex">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        title={inherited ? `Inherited from an ancestor: ${value}` : value}
        className={clsx("rounded-md", FOCUS_RING)}
      >
        <Pill tone={inherited ? "muted" : "neutral"}>
          <Folder size={11} />
          {middleTruncate(value, MAX_LEN)}
          {inherited && <span className="text-ink-disabled">inherited</span>}
        </Pill>
      </button>
      {open && (
        <WorkDirPopover
          nodeId={nodeId}
          // The editor prefills with the effective absolute path so the user
          // edits from the inherited value; the home placeholder is not a real
          // path, so it starts empty instead.
          initial={value === "~" ? "" : value}
          onClose={() => setOpen(false)}
        />
      )}
    </span>
  );
}

interface WorkDirPopoverProps {
  nodeId: NodeID;
  initial: string;
  onClose: () => void;
}

/** Inline editor mirroring StartHeadlessPopover's structure. Owns its own error
 *  state (the daemon's 400 for a bad path is user-actionable) rather than
 *  routing through NodeView's action error. */
function WorkDirPopover({ nodeId, initial, onClose }: WorkDirPopoverProps) {
  const [value, setValue] = useState(initial);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const inputRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    inputRef.current?.focus();
    inputRef.current?.select();
  }, []);

  async function submit() {
    setBusy(true);
    setError(null);
    try {
      // A trimmed empty string clears the override (falls back to inheritance).
      await apiClient.patchNode(nodeId, { work_dir: value.trim() });
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="absolute top-full left-0 z-20 mt-1 w-72 rounded-lg border border-border-strong bg-surface-2 p-3 shadow-panel">
      <label className="mb-1.5 block text-2xs font-medium text-ink-faint" htmlFor="work-dir-input">
        Working directory
      </label>
      <input
        id="work-dir-input"
        ref={inputRef}
        value={value}
        onChange={(e) => setValue(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter") {
            e.preventDefault();
            if (!busy) void submit();
          } else if (e.key === "Escape") {
            onClose();
          }
        }}
        placeholder="/absolute/path"
        spellCheck={false}
        autoComplete="off"
        className={clsx(
          "w-full rounded-md border border-border bg-canvas px-2 py-1.5 font-mono text-xs text-ink placeholder:text-ink-faint",
          FOCUS_RING,
        )}
      />
      <p className="mt-1.5 text-2xs text-ink-disabled">Absolute path; children inherit it. Leave empty to inherit.</p>
      {error && <p className="mt-1.5 text-2xs break-words text-status-failed">{error}</p>}
      <div className="mt-2 flex items-center justify-end gap-2">
        <button
          type="button"
          onClick={onClose}
          className={clsx(
            "rounded-md px-2.5 py-1.5 text-xs text-ink-muted hover:bg-hover hover:text-ink",
            FOCUS_RING,
          )}
        >
          Cancel
        </button>
        <button
          type="button"
          disabled={busy}
          onClick={() => void submit()}
          className={clsx(
            "rounded-md bg-accent px-2.5 py-1.5 text-xs font-medium text-accent-ink hover:bg-accent-strong disabled:opacity-40",
            FOCUS_RING,
          )}
        >
          Set
        </button>
      </div>
    </div>
  );
}
