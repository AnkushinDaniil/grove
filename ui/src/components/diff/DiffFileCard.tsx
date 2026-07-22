import { useState } from "react";
import type { ReactNode } from "react";
import clsx from "clsx";
import { ChevronRight, FileDiff } from "lucide-react";
import { MultiFileDiff } from "@pierre/diffs/react";
import type { AnnotationSide, DiffLineAnnotation, OnDiffLineClickProps } from "@pierre/diffs";
import { FOCUS_RING } from "../../lib/constants";
import { GROVE_DIFF_THEME } from "./pierreTheme";
import type { DiffStyle, DiffViewComment, DiffViewComposerTarget, DiffViewFile } from "./types";
import type { ReviewCommentSide } from "../../gen/types";

const STATUS_LABEL: Record<DiffViewFile["status"], string> = {
  modified: "M",
  added: "A",
  removed: "D",
  renamed: "R",
};

const STATUS_CLASS: Record<DiffViewFile["status"], string> = {
  modified: "text-status-done",
  added: "text-diff-add",
  removed: "text-status-failed",
  renamed: "text-ink-muted",
};

const OMITTED_LABEL: Record<"binary" | "too_large", string> = {
  binary: "Binary file — contents not shown.",
  too_large: "File too large to render (over ~512 KB) — contents not shown.",
};

interface AnnotationMeta {
  comments: DiffViewComment[];
  composerTarget?: DiffViewComposerTarget;
}

function toAnnotationSide(side: ReviewCommentSide): AnnotationSide {
  return side === "LEFT" ? "deletions" : "additions";
}

function fromAnnotationSide(side: AnnotationSide): ReviewCommentSide {
  return side === "deletions" ? "LEFT" : "RIGHT";
}

/** Groups a file's comments by side+line into Pierre's lineAnnotations
 *  shape, folding in a synthetic entry for the open composer (if any) --
 *  mirrors treeterm's PierreDiffViewer grouping. */
function buildLineAnnotations(
  comments: DiffViewComment[],
  activeComposer: DiffViewComposerTarget | null,
): DiffLineAnnotation<AnnotationMeta>[] {
  const groups = new Map<string, DiffViewComment[]>();
  for (const c of comments) {
    const key = `${c.side}:${c.line}`;
    const list = groups.get(key);
    if (list) list.push(c);
    else groups.set(key, [c]);
  }

  const annotations: DiffLineAnnotation<AnnotationMeta>[] = [...groups.entries()].map(([key, list]) => {
    const [side, lineStr] = key.split(":") as [ReviewCommentSide, string];
    return { side: toAnnotationSide(side), lineNumber: Number(lineStr), metadata: { comments: list } };
  });

  if (activeComposer) {
    const side = toAnnotationSide(activeComposer.side);
    const existingIdx = annotations.findIndex((a) => a.side === side && a.lineNumber === activeComposer.line);
    if (existingIdx >= 0) {
      const existing = annotations[existingIdx];
      annotations[existingIdx] = { ...existing, metadata: { ...existing.metadata, composerTarget: activeComposer } };
    } else {
      annotations.push({ side, lineNumber: activeComposer.line, metadata: { comments: [], composerTarget: activeComposer } });
    }
  }

  return annotations;
}

interface DiffFileCardProps {
  file: DiffViewFile;
  domId: string;
  comments: DiffViewComment[];
  /** Non-null only when the open composer belongs to this file. */
  activeComposer: DiffViewComposerTarget | null;
  onOpenComposer: (side: ReviewCommentSide, line: number) => void;
  renderComposer: (target: DiffViewComposerTarget) => ReactNode;
  diffStyle: DiffStyle;
  expandUnchanged: boolean;
  ignoreWhitespace: boolean;
  viewed: boolean;
  onToggleViewed: () => void;
  /** True when @pierre/diffs' worker pool isn't usable in this runtime --
   *  see pierreTheme.ts's DIFFS_WORKER_SUPPORTED. */
  disableWorkerPool: boolean;
}

/** One changed file: a collapsible header (status, path, +/- diffstat,
 *  viewed checkbox) and its @pierre/diffs body. Binary/too-large files (per
 *  content_omitted) fall back to a placeholder instead of an empty diff. */
export function DiffFileCard({
  file,
  domId,
  comments,
  activeComposer,
  onOpenComposer,
  renderComposer,
  diffStyle,
  expandUnchanged,
  ignoreWhitespace,
  viewed,
  onToggleViewed,
  disableWorkerPool,
}: DiffFileCardProps) {
  const [expanded, setExpanded] = useState(!viewed);

  // Auto-collapse on the viewed transition false -> true (e.g. from "mark
  // all above"), without fighting the user's own expand/collapse clicks --
  // React's sanctioned "adjust state during render" pattern for derived
  // resets (see treeterm's FileDiffSection, which this mirrors).
  const [prevViewed, setPrevViewed] = useState(viewed);
  if (viewed !== prevViewed) {
    setPrevViewed(viewed);
    if (viewed) setExpanded(false);
  }

  const renamed = file.status === "renamed" && file.old_path && file.old_path !== file.path;

  function handleLineNumberClick(props: OnDiffLineClickProps) {
    onOpenComposer(fromAnnotationSide(props.annotationSide), props.lineNumber);
  }

  function renderAnnotation(annotation: DiffLineAnnotation<AnnotationMeta>): ReactNode {
    const { metadata } = annotation;
    return (
      <div className="space-y-2 px-3 py-2">
        {metadata.comments.map((c) => (
          <div key={c.id}>{c.content}</div>
        ))}
        {metadata.composerTarget && renderComposer(metadata.composerTarget)}
      </div>
    );
  }

  return (
    <section id={domId} className="border-b border-border">
      <div className="flex items-center gap-2 border-b border-border bg-surface-2/60 px-5 py-2">
        <button
          type="button"
          onClick={() => setExpanded((v) => !v)}
          aria-expanded={expanded}
          className={clsx("flex min-w-0 flex-1 items-center gap-2 text-left", FOCUS_RING)}
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
        </button>
        <span className="shrink-0 font-mono text-2xs">
          <span className="text-diff-add">+{file.additions}</span> <span className="text-status-failed">-{file.deletions}</span>
        </span>
        <label
          className={clsx(
            "flex shrink-0 items-center gap-1.5 text-2xs",
            viewed ? "text-accent" : "text-ink-faint hover:text-ink-muted",
          )}
          title="Mark as viewed"
        >
          <input type="checkbox" checked={viewed} onChange={onToggleViewed} className="accent-accent" />
          Viewed
        </label>
      </div>

      {expanded &&
        (file.content_omitted !== "" ? (
          <div className="flex items-center gap-2 px-5 py-4 text-xs text-ink-faint">
            <FileDiff size={14} className="shrink-0" />
            {OMITTED_LABEL[file.content_omitted]}
          </div>
        ) : (
          <div className="overflow-x-auto">
            <MultiFileDiff<AnnotationMeta>
              oldFile={{ name: file.old_path || file.path, contents: file.original_content }}
              newFile={{ name: file.path, contents: file.modified_content }}
              lineAnnotations={buildLineAnnotations(comments, activeComposer)}
              renderAnnotation={renderAnnotation}
              disableWorkerPool={disableWorkerPool}
              options={{
                diffStyle,
                expandUnchanged,
                parseDiffOptions: { ignoreWhitespace },
                theme: GROVE_DIFF_THEME,
                themeType: "system",
                disableFileHeader: true,
                overflow: "wrap",
                onLineNumberClick: handleLineNumberClick,
              }}
            />
          </div>
        ))}
    </section>
  );
}
