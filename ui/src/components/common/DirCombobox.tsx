import { useCallback, useEffect, useRef, useState, type KeyboardEvent } from "react";
import clsx from "clsx";
import { apiClient } from "../../state/api";
import { decideTab, ensureTrailingSlash, nextIndex, prevIndex } from "../../lib/completion";
import { FOCUS_RING } from "../../lib/constants";

/** Debounce for the completion fetch on free typing; explicit navigation (Tab,
 *  Enter, clicking a row) refetches immediately instead. */
const FETCH_DEBOUNCE_MS = 150;

/** Final path segment of an absolute suggestion (the daemon returns paths
 *  without a trailing slash). */
function baseName(path: string): string {
  const i = path.lastIndexOf("/");
  return i >= 0 ? path.slice(i + 1) : path;
}

export interface DirComboboxProps {
  /** Prefixes every id this component renders (input/listbox/options), so
   *  multiple instances can coexist on one page without collisions. */
  idPrefix: string;
  value: string;
  onChange: (value: string) => void;
  /** Enter pressed with no suggestion row highlighted -- the caller decides
   *  what "commit" means (patch a node's work_dir, add a watched source, ...). */
  onCommit?: (value: string) => void;
  /** Escape pressed while the suggestion list is already closed (Escape
   *  dismisses the list first, terminal-style; the caller decides what a
   *  second Escape means -- e.g. closing its own popover). */
  onEscape?: () => void;
  placeholder?: string;
  autoFocus?: boolean;
  disabled?: boolean;
  className?: string;
}

/**
 * Terminal-style directory completion combobox backed by GET /fs/dirs: free
 * typing debounces a completion fetch, Tab completes a single match (or
 * extends to the suggestions' common prefix) or cycles the highlighted row,
 * and clicking/Enter-ing a row descends into it. The pure keyboard/prefix
 * logic lives in ../../lib/completion so it stays unit-testable without a
 * DOM; this component owns the fetch + DOM wiring around it. `value` is
 * controlled by the caller so different call sites (WorkDirChip's node
 * work_dir editor, ReviewsView's watched-sources editor) can each own their
 * own submit/error semantics around the same completion mechanics.
 */
export function DirCombobox({
  idPrefix,
  value,
  onChange,
  onCommit,
  onEscape,
  placeholder = "/absolute/path",
  autoFocus = true,
  disabled = false,
  className,
}: DirComboboxProps) {
  const [suggestions, setSuggestions] = useState<string[]>([]);
  const [selected, setSelected] = useState(-1);
  const [listboxOpen, setListboxOpen] = useState(false);

  const inputRef = useRef<HTMLInputElement | null>(null);
  const selectedRef = useRef<HTMLLIElement | null>(null);
  const timerRef = useRef<number | null>(null);
  // Monotonic request token so a slow response can't overwrite a newer one.
  const reqRef = useRef(0);
  // Captures the value at mount time only, for the initial fetch effect below.
  const initialValueRef = useRef(value);

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

  // Focus, select, and fetch completions for the starting value once, on
  // mount -- this component is always given a fresh instance per popover/row
  // open (WorkDirChip conditionally mounts it, ReviewsView remounts it via a
  // key bump after each add), so "on mount" already means "on open".
  useEffect(() => {
    if (autoFocus) {
      inputRef.current?.focus();
      inputRef.current?.select();
    }
    fetchImmediate(initialValueRef.current);
    return clearTimer;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    selectedRef.current?.scrollIntoView({ block: "nearest" });
  }, [selected]);

  function handleChange(next: string) {
    onChange(next);
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
    onChange(next);
    fetchImmediate(next);
  }

  function handleKeyDown(e: KeyboardEvent<HTMLInputElement>) {
    if (disabled) return;
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
          onChange(decision.value);
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
        } else {
          onCommit?.(value);
        }
        return;
      }
      case "Escape": {
        e.preventDefault();
        if (listboxOpen && suggestions.length > 0) {
          setListboxOpen(false);
        } else {
          onEscape?.();
        }
        return;
      }
    }
  }

  const showList = listboxOpen && suggestions.length > 0;
  const inputId = `${idPrefix}-input`;
  const listboxId = `${idPrefix}-listbox`;

  return (
    <div className={className}>
      <input
        id={inputId}
        ref={inputRef}
        role="combobox"
        aria-expanded={showList}
        aria-controls={listboxId}
        aria-autocomplete="list"
        aria-activedescendant={selected >= 0 ? `${idPrefix}-option-${selected}` : undefined}
        value={value}
        onChange={(e) => handleChange(e.target.value)}
        onKeyDown={handleKeyDown}
        placeholder={placeholder}
        disabled={disabled}
        spellCheck={false}
        autoComplete="off"
        className={clsx(
          "w-full rounded-md border border-border bg-canvas px-2 py-1.5 font-mono text-xs text-ink placeholder:text-ink-faint disabled:opacity-50",
          FOCUS_RING,
        )}
      />
      {showList && (
        <ul
          id={listboxId}
          role="listbox"
          className="mt-1 max-h-48 overflow-y-auto rounded-md border border-border bg-canvas py-1"
        >
          {suggestions.map((s, idx) => {
            const isSelected = idx === selected;
            return (
              <li
                key={s}
                id={`${idPrefix}-option-${idx}`}
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
    </div>
  );
}
