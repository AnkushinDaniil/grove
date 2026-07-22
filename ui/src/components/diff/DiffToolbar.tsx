import type { ReactNode } from "react";
import clsx from "clsx";
import { AlignJustify, ChevronDown, ChevronUp, Columns2, FoldVertical, ListChecks, UnfoldVertical, WholeWord } from "lucide-react";
import { FOCUS_RING } from "../../lib/constants";
import type { DiffStyle } from "./types";

interface ToggleButtonProps {
  active: boolean;
  onClick: () => void;
  title: string;
  children: ReactNode;
}

function ToggleButton({ active, onClick, title, children }: ToggleButtonProps) {
  return (
    <button
      type="button"
      onClick={onClick}
      title={title}
      aria-pressed={active}
      className={clsx(
        "flex min-h-8 items-center gap-1.5 rounded-md border px-2 py-1 text-2xs font-medium whitespace-nowrap transition-colors",
        active
          ? "border-accent/40 bg-accent-soft text-accent"
          : "border-border-strong text-ink-muted hover:bg-hover hover:text-ink",
        FOCUS_RING,
      )}
    >
      {children}
    </button>
  );
}

interface DiffToolbarProps {
  diffStyle: DiffStyle;
  onToggleDiffStyle: () => void;
  expandUnchanged: boolean;
  onToggleExpandUnchanged: () => void;
  ignoreWhitespace: boolean;
  onToggleIgnoreWhitespace: () => void;
  viewedCount: number;
  totalFiles: number;
  onPrevUnviewed: () => void;
  onNextUnviewed: () => void;
  hasUnviewed: boolean;
}

/** Sticky control strip above the file list: viewed-file progress + prev/
 *  next-unviewed navigation on the left, render-option toggles on the
 *  right. Mirrors treeterm's DiffToolbar, restyled to grove's dense
 *  control-room idiom (bordered pill buttons, not bare icon buttons). */
export function DiffToolbar({
  diffStyle,
  onToggleDiffStyle,
  expandUnchanged,
  onToggleExpandUnchanged,
  ignoreWhitespace,
  onToggleIgnoreWhitespace,
  viewedCount,
  totalFiles,
  onPrevUnviewed,
  onNextUnviewed,
  hasUnviewed,
}: DiffToolbarProps) {
  return (
    <div className="flex shrink-0 flex-wrap items-center gap-1.5 border-b border-border bg-surface px-3 py-1.5">
      <span
        className="mr-1 flex shrink-0 items-center gap-1.5 text-2xs text-ink-faint"
        title={`${viewedCount} of ${totalFiles} files viewed`}
      >
        <ListChecks size={12} />
        {viewedCount}/{totalFiles} viewed
      </span>
      <button
        type="button"
        onClick={onPrevUnviewed}
        disabled={!hasUnviewed}
        title="Previous unviewed file"
        className={clsx(
          "flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-ink-faint hover:bg-hover hover:text-ink disabled:pointer-events-none disabled:opacity-30",
          FOCUS_RING,
        )}
      >
        <ChevronUp size={13} />
      </button>
      <button
        type="button"
        onClick={onNextUnviewed}
        disabled={!hasUnviewed}
        title="Next unviewed file"
        className={clsx(
          "flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-ink-faint hover:bg-hover hover:text-ink disabled:pointer-events-none disabled:opacity-30",
          FOCUS_RING,
        )}
      >
        <ChevronDown size={13} />
      </button>

      <div className="ml-auto flex flex-wrap items-center gap-1.5">
        <ToggleButton
          active={diffStyle === "split"}
          onClick={onToggleDiffStyle}
          title={diffStyle === "split" ? "Switch to unified view" : "Switch to split view"}
        >
          {diffStyle === "split" ? <Columns2 size={12} /> : <AlignJustify size={12} />}
          {diffStyle === "split" ? "Split" : "Unified"}
        </ToggleButton>
        <ToggleButton
          active={expandUnchanged}
          onClick={onToggleExpandUnchanged}
          title={expandUnchanged ? "Collapse unchanged regions" : "Expand unchanged regions"}
        >
          {expandUnchanged ? <FoldVertical size={12} /> : <UnfoldVertical size={12} />}
          Expand unchanged
        </ToggleButton>
        <ToggleButton
          active={ignoreWhitespace}
          onClick={onToggleIgnoreWhitespace}
          title={ignoreWhitespace ? "Show whitespace changes" : "Ignore whitespace changes"}
        >
          <WholeWord size={12} />
          Ignore whitespace
        </ToggleButton>
      </div>
    </div>
  );
}
