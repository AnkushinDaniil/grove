import { useState } from "react";
import clsx from "clsx";
import { ChevronRight, FileDiff } from "lucide-react";
import { DiffHunk } from "./DiffHunk";
import { FOCUS_RING } from "../../lib/constants";
import type { ActiveComposerTarget } from "./ReviewWorkspace";
import type { DraftComment, PRReviewFile, ReviewCommentSide, ReviewThread } from "../../gen/types";

const STATUS_LABEL: Record<PRReviewFile["status"], string> = {
  modified: "M",
  added: "A",
  removed: "D",
  renamed: "R",
};

const STATUS_CLASS: Record<PRReviewFile["status"], string> = {
  modified: "text-status-done",
  added: "text-diff-add",
  removed: "text-status-failed",
  renamed: "text-ink-muted",
};

interface DiffFileProps {
  file: PRReviewFile;
  dir: string;
  pr: number;
  threads: ReviewThread[];
  drafts: DraftComment[];
  activeComposer: ActiveComposerTarget | null;
  onOpenComposer: (side: ReviewCommentSide, line: number) => void;
  onCloseComposer: () => void;
}

/** One changed file: a collapsible header (status, path, +/- diffstat) and
 *  its hunks. Binary files, and files with no diff body at all (e.g. one
 *  `gh` declined to expand for size), fall back to a "view on GitHub" note
 *  instead of an empty hunk list. */
export function DiffFile({ file, dir, pr, threads, drafts, activeComposer, onOpenComposer, onCloseComposer }: DiffFileProps) {
  const [expanded, setExpanded] = useState(true);
  const unrenderable = file.binary || file.hunks.length === 0;
  const isActiveFile = activeComposer?.path === file.path;
  const renamed = file.status === "renamed" && file.old_path && file.old_path !== file.path;

  return (
    <section className="border-b border-border">
      <button
        type="button"
        onClick={() => setExpanded((v) => !v)}
        aria-expanded={expanded}
        className={clsx(
          "flex w-full items-center gap-2 border-b border-border bg-surface-2/60 px-5 py-2 text-left hover:bg-surface-2",
          FOCUS_RING,
        )}
      >
        <ChevronRight
          size={12}
          className={clsx("shrink-0 text-ink-faint transition-transform", expanded && "rotate-90")}
        />
        <span className={clsx("shrink-0 font-mono text-2xs font-semibold", STATUS_CLASS[file.status])} title={file.status}>
          {STATUS_LABEL[file.status]}
        </span>
        <span className="min-w-0 flex-1 truncate font-mono text-xs text-ink" title={file.path}>
          {renamed ? (
            <>
              <span className="text-ink-faint">{file.old_path}</span> → {file.path}
            </>
          ) : (
            file.path
          )}
        </span>
        <span className="shrink-0 font-mono text-2xs">
          <span className="text-diff-add">+{file.additions}</span> <span className="text-status-failed">-{file.deletions}</span>
        </span>
      </button>

      {expanded && (
        <div className="overflow-x-auto">
          {unrenderable ? (
            <div className="flex items-center gap-2 px-5 py-4 text-xs text-ink-faint">
              <FileDiff size={14} className="shrink-0" />
              {file.binary ? "Binary" : "Diff not available"} — view on GitHub to see the full contents.
            </div>
          ) : (
            file.hunks.map((hunk, i) => (
              <DiffHunk
                key={i}
                hunk={hunk}
                path={file.path}
                dir={dir}
                pr={pr}
                threads={threads}
                drafts={drafts}
                activeComposer={isActiveFile ? activeComposer : null}
                onOpenComposer={onOpenComposer}
                onCloseComposer={onCloseComposer}
              />
            ))
          )}
        </div>
      )}
    </section>
  );
}
