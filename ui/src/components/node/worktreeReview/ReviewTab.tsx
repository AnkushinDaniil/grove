import { lazy, Suspense, useEffect, useState } from "react";
import { GitBranch } from "lucide-react";
import { loadWorktreeReview, useWorktreeReviewStore } from "../../../state/worktreeReview";
import { EmptyState } from "../../common/EmptyState";
import { WorktreeActionBar } from "./WorktreeActionBar";
import { WorktreeCommentCard } from "./WorktreeCommentCard";
import { WorktreeCommentComposer } from "./WorktreeCommentComposer";
import { RepoSwitcher } from "./RepoSwitcher";
import type { DiffViewComment, DiffViewComposerTarget } from "../../diff/types";
import type { Node } from "../../../gen/types";

// Same lazy split as ReviewWorkspace -- @pierre/diffs + shiki only load once
// a diff actually renders.
const DiffView = lazy(() => import("../../diff/DiffView").then((m) => ({ default: m.DiffView })));

// The frozen API contract's `repo` query param has no enumeration endpoint,
// so "which repos does this node span" isn't derivable from anything in
// docs/API.md. Reading an optional `meta.repos: string[]` is a legitimate,
// additive way for the daemon to hint it without violating the contract
// (Node.meta is documented as free-form JSON) -- absent, this node has
// exactly one (default) repo and the switcher never renders. Flagged as a
// contract ambiguity in the handoff notes.
function reposFor(node: Node): string[] {
  const raw = node.meta?.repos;
  if (Array.isArray(raw) && raw.length > 0 && raw.every((r) => typeof r === "string")) return raw as string[];
  return [""];
}

interface ReviewTabProps {
  node: Node;
  /** NodeView owns the tab strip -- after "Address with agent" starts a
   *  session, this switches the view to Terminal so the user watches it
   *  work, the same as clicking the tab by hand. */
  onAddressed: () => void;
}

/** Review tab for task nodes with a worktree: the node's local diff (working
 *  tree vs. merge-base) via the same DiffView PR review uses, local
 *  comments with AI-draft assist, and merge/address actions. Grove's own
 *  loop -- worktree per task -> review -> merge/PR -- happening entirely
 *  inside the tree, no GitHub round-trip required. */
export function ReviewTab({ node, onAddressed }: ReviewTabProps) {
  const repos = reposFor(node);
  const [repo, setRepo] = useState(repos[0]);
  const [activeComposer, setActiveComposer] = useState<DiffViewComposerTarget | null>(null);

  const review = useWorktreeReviewStore((s) => s.review);
  const comments = useWorktreeReviewStore((s) => s.comments);
  const loading = useWorktreeReviewStore((s) => s.loading);
  const loaded = useWorktreeReviewStore((s) => s.loaded);
  const error = useWorktreeReviewStore((s) => s.error);

  useEffect(() => {
    void loadWorktreeReview(node.id, repo);
    return () => useWorktreeReviewStore.getState().reset();
  }, [node.id, repo]);

  useEffect(() => {
    setActiveComposer(null);
  }, [node.id, repo]);

  if (!loaded || loading) {
    return <div className="p-5 text-xs text-ink-faint">Loading worktree diff…</div>;
  }

  if (error || !review) {
    return (
      <EmptyState
        icon={<GitBranch size={28} strokeWidth={1.5} />}
        title="Couldn't load this worktree"
        description={error ?? "Unknown error."}
      />
    );
  }

  if (review.files.length === 0) {
    return (
      <div className="flex h-full min-h-0 flex-col">
        {repos.length > 1 && <RepoSwitcher repos={repos} active={repo} onChange={setRepo} />}
        <EmptyState
          icon={<GitBranch size={28} strokeWidth={1.5} />}
          title="No worktree changes to review"
          description="Nothing to review yet -- changes will show up here once the agent edits files."
        />
      </div>
    );
  }

  const diffComments: DiffViewComment[] = comments.map((c) => ({
    id: c.id,
    path: c.path,
    side: c.side,
    line: c.line,
    content: <WorktreeCommentCard key={c.id} comment={c} />,
  }));

  function renderComposer(target: DiffViewComposerTarget) {
    return (
      <WorktreeCommentComposer
        node={node.id}
        repo={repo}
        worktreePath={review!.worktree_path}
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
      {repos.length > 1 && <RepoSwitcher repos={repos} active={repo} onChange={setRepo} />}
      <div className="min-h-0 flex-1 overflow-hidden">
        <Suspense fallback={<div className="p-5 text-xs text-ink-faint">Loading diff viewer…</div>}>
          <DiffView
            files={review.files}
            comments={diffComments}
            viewedScopeKey={`worktree:${node.id}:${repo}`}
            activeComposer={activeComposer}
            onOpenComposer={setActiveComposer}
            renderComposer={renderComposer}
          />
        </Suspense>
      </div>
      <WorktreeActionBar
        node={node.id}
        repo={repo}
        hasChanges={review.files.length > 0}
        commentCount={comments.length}
        onAddressed={onAddressed}
      />
    </div>
  );
}
