import { useEffect, useState } from "react";
import { useParams } from "react-router";
import { AlertTriangle } from "lucide-react";
import { loadReviewWorkspace, useReviewWorkspaceStore } from "../../state/reviewWorkspace";
import { EmptyState } from "../common/EmptyState";
import { ReviewHeader } from "./ReviewHeader";
import { DiffFile } from "./DiffFile";
import { DraftsRail } from "./DraftsRail";
import { SubmitBar } from "./SubmitBar";
import type { ReviewCommentSide } from "../../gen/types";

/** The line a new (not-yet-posted) comment composer is currently open on --
 *  at most one at a time, app-wide, to keep the diff from turning into a
 *  wall of open textareas. */
export interface ActiveComposerTarget {
  path: string;
  side: ReviewCommentSide;
  line: number;
}

/** One PR = one review workspace (docs/API.md "Interactive review
 *  workspace"): the PR diff rendered with inline comment threads,
 *  LLM-assisted drafting, and batch submit. Route: /review/:dir/:pr, `dir`
 *  URL-encoded by the caller (react-router's useParams already fully
 *  decodes it back -- see PRRow's openWorkspace). */
export function ReviewWorkspace() {
  const { dir, pr: prParam } = useParams<{ dir: string; pr: string }>();
  const pr = prParam ? Number(prParam) : NaN;
  const validParams = Boolean(dir) && Number.isFinite(pr);

  const review = useReviewWorkspaceStore((s) => s.review);
  const drafts = useReviewWorkspaceStore((s) => s.drafts);
  const loading = useReviewWorkspaceStore((s) => s.loading);
  const loaded = useReviewWorkspaceStore((s) => s.loaded);
  const error = useReviewWorkspaceStore((s) => s.error);

  const [activeComposer, setActiveComposer] = useState<ActiveComposerTarget | null>(null);

  useEffect(() => {
    if (!validParams || !dir) return;
    void loadReviewWorkspace(dir, pr);
    return () => useReviewWorkspaceStore.getState().reset();
  }, [dir, pr, validParams]);

  // The route can go from one PR straight to another without unmounting
  // (same element, new params) -- drop any composer left open on the old PR.
  useEffect(() => {
    setActiveComposer(null);
  }, [dir, pr]);

  if (!validParams || !dir) {
    return <EmptyState title="Invalid review link" description="Missing a repository directory or PR number." />;
  }

  if (!loaded || loading) {
    return <div className="p-5 text-xs text-ink-faint">Loading review…</div>;
  }

  if (error || !review) {
    return (
      <EmptyState
        icon={<AlertTriangle size={28} strokeWidth={1.5} />}
        title="Couldn't load this PR"
        description={error ?? "Unknown error."}
      />
    );
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
      <ReviewHeader review={review} dir={dir} />
      <div className="flex min-h-0 flex-1 flex-col overflow-hidden lg:flex-row">
        <div className="min-h-0 flex-1 overflow-y-auto">
          {review.files.length === 0 && (
            <EmptyState title="No file changes" description="This PR has no diff to review." />
          )}
          {review.files.map((file) => (
            <DiffFile
              key={file.path}
              file={file}
              dir={dir}
              pr={pr}
              threads={review.threads.filter((t) => t.path === file.path)}
              drafts={drafts.filter((d) => d.path === file.path)}
              activeComposer={activeComposer}
              onOpenComposer={(side, line) => setActiveComposer({ path: file.path, side, line })}
              onCloseComposer={() => setActiveComposer(null)}
            />
          ))}
        </div>
        <DraftsRail drafts={drafts} />
      </div>
      <SubmitBar dir={dir} pr={pr} drafts={drafts} />
    </div>
  );
}
