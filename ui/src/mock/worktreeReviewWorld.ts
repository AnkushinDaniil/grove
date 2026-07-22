import type { AddWorktreeCommentRequest, MergeWorktreeResponse, WorktreeComment, WorktreeReview } from "../gen/types";
import { ApiError } from "../state/api";
import { world } from "./world";
import { buildHeroWorktreeComment, buildHeroWorktreeReview, WORKTREE_REPO, WORKTREE_REVIEW_NODE_ID } from "./worktreeReviewFixtures";

function nowISO(): string {
  return new Date().toISOString();
}

/**
 * In-memory backing store for the worktree review tab (/api/v1/reviews/
 * worktree), mirroring prReviewWorld.ts's role for PR review. Only
 * WORKTREE_REVIEW_NODE_ID (task-stripe) has real fixture content; any other
 * known node reports a clean, empty worktree (no changes yet) rather than
 * 404ing, since a task with a workspace_dir legitimately has a worktree even
 * before it has produced a diff -- the Review tab's "no worktree changes to
 * review" empty state exercises that path.
 */
class MockWorktreeReviewWorld {
  private readonly mergedKeys = new Set<string>();
  private readonly commentsByKey = new Map<string, WorktreeComment[]>();
  private seq = 0;

  constructor() {
    this.commentsByKey.set(this.key(WORKTREE_REVIEW_NODE_ID, WORKTREE_REPO), [
      buildHeroWorktreeComment(WORKTREE_REPO),
    ]);
  }

  private key(node: string, repo: string): string {
    return `${node}::${repo}`;
  }

  getReview(node: string, repo: string): WorktreeReview {
    const effectiveRepo = repo || WORKTREE_REPO;
    const found = world.nodesById.get(node);
    if (!found) throw new ApiError(404, `node ${node} not found (mock)`);

    if (node === WORKTREE_REVIEW_NODE_ID && !this.mergedKeys.has(this.key(node, effectiveRepo))) {
      return buildHeroWorktreeReview(effectiveRepo);
    }
    return {
      node_id: node,
      repo: effectiveRepo,
      worktree_path: found.workspace_dir,
      branch: `grove/${node}`,
      base_ref: "master",
      has_uncommitted: false,
      files: [],
    };
  }

  getComments(node: string, repo: string): WorktreeComment[] {
    return this.commentsByKey.get(this.key(node, repo || WORKTREE_REPO)) ?? [];
  }

  addComment(body: AddWorktreeCommentRequest): WorktreeComment {
    this.getReview(body.node, body.repo); // 404s consistently if the node itself doesn't exist
    const comment: WorktreeComment = {
      id: `wtc-mock-${(this.seq += 1)}`,
      node_id: body.node,
      repo: body.repo,
      path: body.path,
      line: body.line,
      side: body.side,
      body: body.body,
      created_at: nowISO(),
    };
    const key = this.key(body.node, body.repo);
    this.commentsByKey.set(key, [...(this.commentsByKey.get(key) ?? []), comment]);
    return comment;
  }

  removeComment(id: string): void {
    for (const [key, list] of this.commentsByKey) {
      const next = list.filter((c) => c.id !== id);
      if (next.length !== list.length) {
        this.commentsByKey.set(key, next);
        return;
      }
    }
    throw new ApiError(404, `comment ${id} not found (mock)`);
  }

  /** Squash-merge simulation: once "merged", the worktree reports clean
   *  (files: []) until it accrues new changes again -- mirrors the real
   *  endpoint's "requires a clean tree" precondition closely enough to demo
   *  the merged/not-mergeable states. */
  merge(node: string, repo: string): MergeWorktreeResponse {
    const review = this.getReview(node, repo);
    if (review.files.length === 0) {
      return { merged: false, message: "Nothing to merge -- the worktree has no changes." };
    }
    this.mergedKeys.add(this.key(node, review.repo));
    return { merged: true, message: `Merged into ${review.base_ref}.` };
  }
}

export const worktreeReviewWorld = new MockWorktreeReviewWorld();
