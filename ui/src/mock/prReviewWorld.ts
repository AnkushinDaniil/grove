import type {
  AddReviewDraftRequest,
  AiDraftRequest,
  AiDraftResponse,
  DraftComment,
  PRReview,
  ReplyToThreadRequest,
  ReviewThread,
  SubmitReviewRequest,
  SubmitReviewResponse,
  ThreadComment,
} from "../gen/types";
import { ApiError } from "../state/api";
import { buildHeroPRReview, HERO_PR_DIR, HERO_PR_NUMBER } from "./prReviewFixtures";
import { REVIEW_LOGIN } from "./reviewFixtures";

let seq = 0;
function nextId(prefix: string): string {
  seq += 1;
  return `${prefix}-mock-${seq}`;
}

function nowISO(): string {
  return new Date().toISOString();
}

function delay(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function cannedSuggestion(req: AiDraftRequest): string {
  const steer = req.instruction?.trim();
  const steerNote = steer ? ` (steering: "${steer}")` : "";
  if (req.kind === "reply") {
    return `Good point${steerNote} -- I'll follow up with a fix and ping this thread once it's pushed.`;
  }
  if (req.path && req.line) {
    return (
      `Consider logging the rejection reason here so it's visible in trace output${steerNote}; ` +
      "right now this fails silently from the caller's point of view."
    );
  }
  return (
    "Overall this looks solid -- the nonce-ordering fix addresses the silent-rejection bug cleanly " +
    `and the added regression test covers the exact case${steerNote}. One inline nit; nothing blocking.`
  );
}

/**
 * In-memory backing store for the interactive review workspace
 * (/api/v1/reviews/pr), mirroring reviewWorld.ts's role for Review Radar.
 * Only the hero fixture PR (see prReviewFixtures.ts) is pre-registered; any
 * other (dir, pr) 404s, same as a real PR the daemon's `gh` call can't
 * reach -- ReviewWorkspace's error state is exercised by that path rather
 * than by fabricating low-fidelity diffs for every Review Radar fixture PR.
 */
class MockPRReviewWorld {
  private readonly reviews = new Map<string, PRReview>();
  private readonly drafts = new Map<string, DraftComment[]>();

  private key(dir: string, pr: number): string {
    return `${dir}::${pr}`;
  }

  getReview(dir: string, pr: number): PRReview {
    const key = this.key(dir, pr);
    const existing = this.reviews.get(key);
    if (existing) return existing;
    if (dir === HERO_PR_DIR && pr === HERO_PR_NUMBER) {
      const review = buildHeroPRReview();
      this.reviews.set(key, review);
      return review;
    }
    throw new ApiError(404, `PR #${pr} not found in ${dir} (mock)`);
  }

  getDrafts(dir: string, pr: number): DraftComment[] {
    this.getReview(dir, pr); // 404s consistently if the PR itself doesn't exist
    return this.drafts.get(this.key(dir, pr)) ?? [];
  }

  addDraft(body: AddReviewDraftRequest): DraftComment {
    this.getReview(body.dir, body.pr);
    const draft: DraftComment = {
      id: nextId("draft"),
      dir: body.dir,
      pr: body.pr,
      path: body.path,
      line: body.line,
      side: body.side,
      body: body.body,
      created_at: nowISO(),
    };
    const key = this.key(body.dir, body.pr);
    this.drafts.set(key, [...(this.drafts.get(key) ?? []), draft]);
    return draft;
  }

  removeDraft(draftId: string): void {
    for (const [key, list] of this.drafts) {
      const next = list.filter((d) => d.id !== draftId);
      if (next.length !== list.length) {
        this.drafts.set(key, next);
        return;
      }
    }
    throw new ApiError(404, `draft ${draftId} not found (mock)`);
  }

  async aiDraft(req: AiDraftRequest): Promise<AiDraftResponse> {
    this.getReview(req.dir, req.pr);
    await delay(280);
    return { text: cannedSuggestion(req) };
  }

  submitReview(req: SubmitReviewRequest): SubmitReviewResponse {
    const key = this.key(req.dir, req.pr);
    const review = this.getReview(req.dir, req.pr);
    const pending = this.drafts.get(key) ?? [];
    const submitted = pending.filter((d) => req.draft_ids.includes(d.id));
    const now = nowISO();

    const newThreads: ReviewThread[] = submitted.map((draft) => ({
      id: nextId("PRRT"),
      path: draft.path,
      line: draft.line,
      side: draft.side,
      is_resolved: false,
      diff_hunk: "",
      comments: [{ id: nextId("PRRC"), author: REVIEW_LOGIN, body: draft.body, created_at: now, is_mine: true }],
    }));

    const review_decision =
      req.event === "APPROVE"
        ? "APPROVED"
        : req.event === "REQUEST_CHANGES"
          ? "CHANGES_REQUESTED"
          : review.review_decision;

    this.reviews.set(key, { ...review, threads: [...review.threads, ...newThreads], review_decision });
    this.drafts.set(
      key,
      pending.filter((d) => !req.draft_ids.includes(d.id)),
    );
    return { url: `${review.url}#pullrequestreview-${nextId("rev")}` };
  }

  reply(req: ReplyToThreadRequest): void {
    const key = this.key(req.dir, req.pr);
    const review = this.getReview(req.dir, req.pr);
    const comment: ThreadComment = {
      id: nextId("PRRC"),
      author: REVIEW_LOGIN,
      body: req.body,
      created_at: nowISO(),
      is_mine: true,
    };
    let found = false;
    const threads = review.threads.map((t) => {
      if (t.id !== req.thread_id) return t;
      found = true;
      return { ...t, comments: [...t.comments, comment], is_resolved: req.resolve || t.is_resolved };
    });
    if (!found) throw new ApiError(404, `thread ${req.thread_id} not found (mock)`);
    this.reviews.set(key, { ...review, threads });
  }
}

export const prReviewWorld = new MockPRReviewWorld();
