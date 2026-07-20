import { useState } from "react";
import clsx from "clsx";
import { ChevronDown, ChevronUp, Search, X } from "lucide-react";
import { FOCUS_RING } from "../lib/constants";

interface TerminalSearchBarProps {
  onClose: () => void;
  onNext: (query: string) => void;
  onPrev: (query: string) => void;
}

/** Inline find-in-terminal bar, opened by Cmd/Ctrl+F while a terminal pane
 *  has focus (see XtermHost). Backed by @xterm/addon-search. */
export function TerminalSearchBar({ onClose, onNext, onPrev }: TerminalSearchBarProps) {
  const [query, setQuery] = useState("");

  return (
    <div className="absolute right-3 top-3 z-20 flex items-center gap-1 rounded-md border border-border-strong bg-surface-2 px-2 py-1.5 shadow-popover">
      <Search size={13} className="shrink-0 text-ink-faint" />
      <input
        autoFocus
        value={query}
        onChange={(e) => {
          setQuery(e.target.value);
          onNext(e.target.value);
        }}
        onKeyDown={(e) => {
          if (e.key === "Enter") {
            e.preventDefault();
            if (e.shiftKey) onPrev(query);
            else onNext(query);
          } else if (e.key === "Escape") {
            e.preventDefault();
            onClose();
          }
        }}
        placeholder="Find in terminal"
        className={clsx(
          "w-40 bg-transparent text-2xs text-ink placeholder:text-ink-faint",
          FOCUS_RING,
        )}
      />
      <button
        type="button"
        onClick={() => onPrev(query)}
        className={clsx("rounded p-0.5 text-ink-faint hover:bg-hover hover:text-ink", FOCUS_RING)}
        aria-label="Previous match"
      >
        <ChevronUp size={13} />
      </button>
      <button
        type="button"
        onClick={() => onNext(query)}
        className={clsx("rounded p-0.5 text-ink-faint hover:bg-hover hover:text-ink", FOCUS_RING)}
        aria-label="Next match"
      >
        <ChevronDown size={13} />
      </button>
      <button
        type="button"
        onClick={onClose}
        className={clsx("rounded p-0.5 text-ink-faint hover:bg-hover hover:text-ink", FOCUS_RING)}
        aria-label="Close search"
      >
        <X size={13} />
      </button>
    </div>
  );
}
