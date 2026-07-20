import { useCallback, useEffect, useRef, useState, type KeyboardEvent } from "react";
import clsx from "clsx";
import { Folder } from "lucide-react";
import { Pill } from "../common/Pill";
import { useResolvedWorkDir } from "../../hooks/useResolvedWorkDir";
import { apiClient } from "../../state/api";
import { FOCUS_RING } from "../../lib/constants";
import { decideTab, ensureTrailingSlash, nextIndex, prevIndex } from "../../lib/completion";
import type { NodeID } from "../../gen/types";

interface WorkDirChipProps {
  nodeId: NodeID;
}

const MAX_LEN = 36;

/** Debounce for the completion fetch on free typing; explicit navigation (Tab,
 *  Enter, clicking a row) refetches immediately instead. */
const FETCH_DEBOUNCE_MS = 150;

/** Middle-truncates a path so both the mount root and the leaf stay readable
 *  (the full path is always available via the title tooltip). */
function middleTruncate(path: string, max: number): string {
  if (path.length <= max) return path;
  const side = Math.floor((max - 1) / 2);
  return `${path.slice(0, side)}…${path.slice(path.length - side)}`;
}

/** Final path segment of an absolute suggestion (the daemon returns paths
 *  without a trailing slash). */
function baseName(path: string): string {
  const i = path.lastIndexOf("/");
  return i >= 0 ? path.slice(i + 1) : path;
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

/** Inline editor mirroring StartHeadlessPopover's structure, upgraded to a
 *  terminal-style completing combobox. Owns its own error state (the daemon's
 *  400 for a bad path is user-actionable) rather than routing through NodeView's
 *  action error. The pure keyboard logic lives in ../../lib/completion; server
 *  validation stays the source of truth on commit. */
function WorkDirPopover({ nodeId, initial, onClose }: WorkDirPopoverProps) {
  const [value, setValue] = useState(initial);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [suggestions, setSuggestions] = useState<string[]>([]);
  const [selected, setSelected] = useState(-1);
  const [listboxOpen, setListboxOpen] = useState(false);

  const inputRef = useRef<HTMLInputElement | null>(null);
  const selectedRef = useRef<HTMLLIElement | null>(null);
  const timerRef = useRef<number | null>(null);
  // Monotonic request token so a slow response can't overwrite a newer one.
  const reqRef = useRef(0);

  const runFetch = useCallback(async (prefix: string) => {
    const token = (reqRef.current += 1);
    try {
      const res = await apiClient.suggestDirs(prefix);
      if (reqRef.current !== token) return;
      setSuggestions(res.dirs);
      setSelected(-1);
      setListboxOpen(true);
    } catch {
      if (reqRef.current !== token) return;
      setSuggestions([]);
      setSelected(-1);
    }
  }, []);

  const clearTimer = useCallback(() => {
    if (timerRef.current !== null) {
      window.clearTimeout(timerRef.current);
      timerRef.current = null;
    }
  }, []);

  const fetchImmediate = useCallback(
    (prefix: string) => {
      clearTimer();
      void runFetch(prefix);
    },
    [clearTimer, runFetch],
  );

  // Focus, select, and fetch completions for the current value on open.
  useEffect(() => {
    inputRef.current?.focus();
    inputRef.current?.select();
    fetchImmediate(initial);
    return clearTimer;
  }, [initial, fetchImmediate, clearTimer]);

  // Keep the highlighted row visible without wrapping the whole popover.
  useEffect(() => {
    selectedRef.current?.scrollIntoView({ block: "nearest" });
  }, [selected]);

  function handleChange(next: string) {
    setValue(next);
    setError(null);
    clearTimer();
    timerRef.current = window.setTimeout(() => {
      timerRef.current = null;
      void runFetch(next);
    }, FETCH_DEBOUNCE_MS);
  }

  /** Fill the input with a directory and descend into it (terminal-style),
   *  refetching its children immediately. */
  function descend(path: string) {
    const next = ensureTrailingSlash(path);
    setValue(next);
    setError(null);
    fetchImmediate(next);
  }

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

  function handleKeyDown(e: KeyboardEvent<HTMLInputElement>) {
    switch (e.key) {
      case "Tab": {
        e.preventDefault();
        if (suggestions.length === 0) return;
        if (e.shiftKey) {
          setSelected((c) => prevIndex(c, suggestions.length));
          setListboxOpen(true);
          return;
        }
        const decision = decideTab(value, suggestions);
        if (decision.kind === "cycle") {
          setSelected((c) => nextIndex(c, suggestions.length));
          setListboxOpen(true);
        } else {
          // complete (single match, trailing slash) or extend to common prefix.
          setValue(decision.value);
          setError(null);
          fetchImmediate(decision.value);
        }
        return;
      }
      case "ArrowDown": {
        e.preventDefault();
        if (suggestions.length === 0) return;
        setListboxOpen(true);
        setSelected((c) => nextIndex(c, suggestions.length));
        return;
      }
      case "ArrowUp": {
        e.preventDefault();
        if (suggestions.length === 0) return;
        setListboxOpen(true);
        setSelected((c) => prevIndex(c, suggestions.length));
        return;
      }
      case "Enter": {
        e.preventDefault();
        if (selected >= 0 && selected < suggestions.length) {
          descend(suggestions[selected]);
        } else if (!busy) {
          void submit();
        }
        return;
      }
      case "Escape": {
        e.preventDefault();
        // Dismiss the suggestion list first, the popover second.
        if (listboxOpen && suggestions.length > 0) {
          setListboxOpen(false);
        } else {
          onClose();
        }
        return;
      }
    }
  }

  const showList = listboxOpen && suggestions.length > 0;

  return (
    <div className="absolute top-full left-0 z-20 mt-1 w-72 rounded-lg border border-border-strong bg-surface-2 p-3 shadow-panel">
      <label className="mb-1.5 block text-2xs font-medium text-ink-faint" htmlFor="work-dir-input">
        Working directory
      </label>
      <input
        id="work-dir-input"
        ref={inputRef}
        role="combobox"
        aria-expanded={showList}
        aria-controls="work-dir-listbox"
        aria-autocomplete="list"
        aria-activedescendant={selected >= 0 ? `work-dir-option-${selected}` : undefined}
        value={value}
        onChange={(e) => handleChange(e.target.value)}
        onKeyDown={handleKeyDown}
        placeholder="/absolute/path"
        spellCheck={false}
        autoComplete="off"
        className={clsx(
          "w-full rounded-md border border-border bg-canvas px-2 py-1.5 font-mono text-xs text-ink placeholder:text-ink-faint",
          FOCUS_RING,
        )}
      />
      {showList && (
        <ul
          id="work-dir-listbox"
          role="listbox"
          className="mt-1 max-h-48 overflow-y-auto rounded-md border border-border bg-canvas py-1"
        >
          {suggestions.map((s, idx) => {
            const isSelected = idx === selected;
            return (
              <li
                key={s}
                id={`work-dir-option-${idx}`}
                role="option"
                aria-selected={isSelected}
                ref={isSelected ? selectedRef : null}
                // Keep focus in the input so typing continues after a click.
                onMouseDown={(e) => {
                  e.preventDefault();
                  descend(s);
                }}
                className={clsx(
                  "flex cursor-pointer items-baseline gap-2 px-2 py-1 font-mono text-2xs",
                  isSelected ? "bg-hover" : "hover:bg-hover",
                )}
              >
                <span className="shrink-0 font-medium text-ink">{baseName(s)}</span>
                <span className="truncate text-ink-faint">{s}</span>
              </li>
            );
          })}
        </ul>
      )}
      <p className="mt-1.5 text-2xs text-ink-disabled">
        Absolute path; children inherit it. Tab to complete. Leave empty to inherit.
      </p>
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
