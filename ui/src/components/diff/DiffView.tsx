import { useState } from "react";
import type { ReactNode } from "react";
import { WorkerPoolContextProvider } from "@pierre/diffs/react";
import { FileDiff } from "lucide-react";
import { EmptyState } from "../common/EmptyState";
import { selectViewedCount, useDiffViewedStore } from "../../state/diffViewed";
import { createDiffsWorker, DIFFS_WORKER_SUPPORTED } from "./pierreTheme";
import { DiffToolbar } from "./DiffToolbar";
import { DiffFileCard } from "./DiffFileCard";
import "./diffView.css";
import type { DiffStyle, DiffViewComment, DiffViewComposerTarget, DiffViewFile } from "./types";
import type { ReviewCommentSide } from "../../gen/types";

// Stable reference -- passed to WorkerPoolContextProvider, which re-creates
// its worker pool whenever poolOptions changes identity.
const WORKER_POOL_OPTIONS = { workerFactory: createDiffsWorker };
const HIGHLIGHTER_OPTIONS = {};

function fileDomId(scopeKey: string, path: string): string {
  return `diff-file:${scopeKey}:${path}`;
}

function wrapIndex(i: number, len: number): number {
  return ((i % len) + len) % len;
}

export interface DiffViewProps {
  files: DiffViewFile[];
  comments: DiffViewComment[];
  /** Opaque cache key scoping viewed-file tracking, e.g. `pr:${dir}:${pr}`
   *  or `worktree:${node}:${repo}` -- keeps unrelated diffs' viewed state
   *  from bleeding into each other. */
  viewedScopeKey: string;
  /** The (path, side, line) a comment-input composer is currently open on,
   *  app-wide -- at most one at a time, matching the PR review workspace's
   *  existing convention. */
  activeComposer: DiffViewComposerTarget | null;
  onOpenComposer: (target: DiffViewComposerTarget) => void;
  /** Caller renders the actual composer (grove's CommentComposer for PR
   *  review, a lighter one for worktree notes) -- DiffView only decides
   *  where it anchors in the diff. */
  renderComposer: (target: DiffViewComposerTarget) => ReactNode;
  emptyTitle?: string;
  emptyDescription?: string;
}

/** Reusable @pierre/diffs-backed diff renderer for a list of files, shared
 *  by the PR review workspace and the worktree review tab. Owns the
 *  worker pool, split/unified + expand-unchanged + ignore-whitespace
 *  toggles, and viewed-file tracking; comment rendering and the
 *  comment-input composer are supplied by the caller so this component
 *  stays agnostic to GitHub threads vs. local worktree notes. */
export function DiffView({
  files,
  comments,
  viewedScopeKey,
  activeComposer,
  onOpenComposer,
  renderComposer,
  emptyTitle = "No file changes",
  emptyDescription,
}: DiffViewProps) {
  const [diffStyle, setDiffStyle] = useState<DiffStyle>("unified");
  const [expandUnchanged, setExpandUnchanged] = useState(false);
  const [ignoreWhitespace, setIgnoreWhitespace] = useState(false);
  // "Current" file for prev/next-unviewed purposes. Advances on explicit
  // navigation and on marking the current file viewed -- a scope-proportional
  // stand-in for scroll-position tracking (see treeterm's IntersectionObserver
  // approach, which grove's typically-small diffs don't need).
  const [focusIndex, setFocusIndex] = useState(0);

  const paths = files.map((f) => f.path);
  const viewedCount = useDiffViewedStore((s) => selectViewedCount(s, viewedScopeKey, paths));
  const viewedSet = useDiffViewedStore((s) => s.viewedByScope[viewedScopeKey]);
  const toggleViewed = useDiffViewedStore((s) => s.toggleViewed);

  if (files.length === 0) {
    return (
      <EmptyState
        icon={<FileDiff size={28} strokeWidth={1.5} />}
        title={emptyTitle}
        description={emptyDescription}
        className="py-10"
      />
    );
  }

  function findNextUnviewed(from: number, dir: 1 | -1): number | null {
    for (let step = 1; step <= files.length; step++) {
      const idx = wrapIndex(from + dir * step, files.length);
      if (!viewedSet?.has(files[idx].path)) return idx;
    }
    return null;
  }

  function scrollToFile(path: string) {
    document.getElementById(fileDomId(viewedScopeKey, path))?.scrollIntoView({ behavior: "smooth", block: "start" });
  }

  function goToUnviewed(dir: 1 | -1) {
    const target = findNextUnviewed(focusIndex, dir);
    if (target === null) return;
    setFocusIndex(target);
    scrollToFile(files[target].path);
  }

  function handleToggleViewed(index: number) {
    const file = files[index];
    const willBeViewed = !viewedSet?.has(file.path);
    toggleViewed(viewedScopeKey, file.path);
    if (!willBeViewed) return;
    const next = findNextUnviewed(index, 1);
    if (next !== null) {
      setFocusIndex(next);
      scrollToFile(files[next].path);
    }
  }

  function handleOpenComposer(path: string, side: ReviewCommentSide, line: number) {
    onOpenComposer({ path, side, line });
  }

  const content = (
    <div className="grove-diff-view flex h-full min-h-0 flex-col">
      <DiffToolbar
        diffStyle={diffStyle}
        onToggleDiffStyle={() => setDiffStyle((s) => (s === "split" ? "unified" : "split"))}
        expandUnchanged={expandUnchanged}
        onToggleExpandUnchanged={() => setExpandUnchanged((v) => !v)}
        ignoreWhitespace={ignoreWhitespace}
        onToggleIgnoreWhitespace={() => setIgnoreWhitespace((v) => !v)}
        viewedCount={viewedCount}
        totalFiles={files.length}
        onPrevUnviewed={() => goToUnviewed(-1)}
        onNextUnviewed={() => goToUnviewed(1)}
        hasUnviewed={viewedCount < files.length}
      />
      <div className="min-h-0 flex-1 overflow-y-auto">
        {files.map((file, index) => (
          <DiffFileCard
            key={file.path}
            file={file}
            domId={fileDomId(viewedScopeKey, file.path)}
            comments={comments.filter((c) => c.path === file.path)}
            activeComposer={activeComposer?.path === file.path ? activeComposer : null}
            onOpenComposer={(side, line) => handleOpenComposer(file.path, side, line)}
            renderComposer={renderComposer}
            diffStyle={diffStyle}
            expandUnchanged={expandUnchanged}
            ignoreWhitespace={ignoreWhitespace}
            viewed={viewedSet?.has(file.path) ?? false}
            onToggleViewed={() => handleToggleViewed(index)}
            disableWorkerPool={!DIFFS_WORKER_SUPPORTED}
          />
        ))}
      </div>
    </div>
  );

  // Only pay for the worker pool where it actually works (see
  // pierreTheme.ts's DIFFS_WORKER_SUPPORTED) -- mounting the provider when
  // the underlying Worker isn't usable crashes on its first render.
  if (!DIFFS_WORKER_SUPPORTED) return content;
  return (
    <WorkerPoolContextProvider poolOptions={WORKER_POOL_OPTIONS} highlighterOptions={HIGHLIGHTER_OPTIONS}>
      {content}
    </WorkerPoolContextProvider>
  );
}
