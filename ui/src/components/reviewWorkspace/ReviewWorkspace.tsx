import { lazy, Suspense, useEffect, useState } from "react";
import { useParams } from "react-router";
import { AlertTriangle } from "lucide-react";
import { loadReviewWorkspace, useReviewWorkspaceStore } from "../../state/reviewWorkspace";
import { EmptyState } from "../common/EmptyState";
import { ReviewHeader } from "./ReviewHeader";
import { ThreadCard } from "./ThreadCard";
import { DraftPendingCard } from "./DraftPendingCard";
import { CommentComposer } from "./CommentComposer";
import { DraftsRail } from "./DraftsRail";
import { AiFindingsPanel } from "./AiFindingsPanel";
import { AiFindingCard } from "./AiFindingCard";
import { SubmitBar } from "./SubmitBar";
import type { LocalFinding } from "../../state/reviewWorkspace";
import type { DiffViewComment, DiffViewComposerTarget } from "../diff/types";

/** DOM id of a finding's inline card, used to jump to its exact line. */
function findingDomId(id: string): string {
  return `ai-finding:${id}`;
}

// @pierre/diffs + shiki are a meaningful chunk of the bundle (syntax
// highlighting, worker pool) -- lazy-load so they only load once a diff
// actually renders, mirroring TerminalTab's XtermHost split.
const DiffView = lazy(() => import("../diff/DiffView").then((m) => ({ default: m.DiffView })));

/** One PR = one review workspace (docs/API.md "Interactive review
 *  workspace"): the PR diff rendered with inline comment threads,
 *  LLM-assisted drafting, and batch submit. Route: /review/:dir/:pr, `dir`
 *  URL-encoded by the caller (see PRRow's openWorkspace). */
export function ReviewWorkspace() {
  const { dir, pr: prParam } = useParams<{ dir: string; pr: string }>();
  const pr = prParam ? Number(prParam) : NaN;
  const validParams = Boolean(dir) && Number.isFinite(pr);

  const review = useReviewWorkspaceStore((s) => s.review);
  const drafts = useReviewWorkspaceStore((s) => s.drafts);
  const aiFindings = useReviewWorkspaceStore((s) => s.aiFindings);
  const loading = useReviewWorkspaceStore((s) => s.loading);
  const loaded = useReviewWorkspaceStore((s) => s.loaded);
  const error = useReviewWorkspaceStore((s) => s.error);

  const [activeComposer, setActiveComposer] = useState<DiffViewComposerTarget | null>(null);

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

  // Existing GitHub threads and pending drafts both anchor at path+side+line
  // -- DiffView doesn't need to know GitHub threads and local drafts are
  // different things, just where each pre-rendered card belongs.
  const comments: DiffViewComment[] = [
    ...review.threads.map((t) => ({
      id: t.id,
      path: t.path,
      side: t.side,
      line: t.line,
      content: <ThreadCard key={t.id} thread={t} dir={dir} pr={pr} />,
    })),
    ...drafts.map((d) => ({
      id: d.id,
      path: d.path,
      side: d.side,
      line: d.line,
      content: <DraftPendingCard key={d.id} draft={d} />,
    })),
    // AI findings render inline at their exact anchor line -- the same
    // annotation mechanism as threads/drafts, so a finding sits on the line it
    // is about. The wrapper id lets the panel jump straight to it.
    ...aiFindings.map((f) => ({
      id: `aifinding:${f.id}`,
      path: f.path,
      side: f.side,
      line: f.line,
      content: (
        <div key={f.id} id={findingDomId(f.id)}>
          <AiFindingCard finding={f} dir={dir!} pr={pr} inline />
        </div>
      ),
    })),
  ];

  // Jump the diff to a finding's inline card at its exact line (the anchored
  // node above). Falls back to the file top when the card isn't in the DOM --
  // e.g. its file is collapsed (marked viewed). The brief inline outline uses a
  // style, not a Tailwind class the JIT might not have generated.
  function highlight(el: HTMLElement) {
    el.style.transition = "box-shadow 0.15s ease";
    el.style.boxShadow = "inset 0 0 0 2px var(--color-accent)";
    window.setTimeout(() => {
      el.style.boxShadow = "";
    }, 1200);
  }

  function scrollToFinding(f: LocalFinding) {
    const el = document.getElementById(findingDomId(f.id));
    if (el) {
      el.scrollIntoView({ behavior: "smooth", block: "center" });
      highlight(el);
      return;
    }
    const fileEl = document.getElementById(`diff-file:pr:${dir}:${pr}:${f.path}`);
    if (!fileEl) return;
    fileEl.scrollIntoView({ behavior: "smooth", block: "start" });
    highlight(fileEl);
  }

  function renderComposer(target: DiffViewComposerTarget) {
    return (
      <CommentComposer
        mode="new"
        dir={dir!}
        pr={pr}
        path={target.path}
        side={target.side}
        line={target.line}
        onAdded={() => setActiveComposer(null)}
        onCancel={() => setActiveComposer(null)}
      />
    );
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
      <ReviewHeader review={review} dir={dir} />
      <div className="flex min-h-0 flex-1 flex-col overflow-hidden lg:flex-row">
        <div className="min-h-0 flex-1 overflow-hidden">
          <Suspense fallback={<div className="p-5 text-xs text-ink-faint">Loading diff viewer…</div>}>
            <DiffView
              files={review.files}
              comments={comments}
              viewedScopeKey={`pr:${dir}:${pr}`}
              activeComposer={activeComposer}
              onOpenComposer={setActiveComposer}
              renderComposer={renderComposer}
              emptyDescription="This PR has no diff to review."
            />
          </Suspense>
        </div>
        <aside className="flex max-h-[60vh] shrink-0 flex-col border-t border-border bg-surface lg:max-h-none lg:w-72 lg:border-t-0 lg:border-l">
          <AiFindingsPanel dir={dir} pr={pr} onSelect={scrollToFinding} />
          <DraftsRail drafts={drafts} />
        </aside>
      </div>
      <SubmitBar dir={dir} pr={pr} drafts={drafts} />
    </div>
  );
}
