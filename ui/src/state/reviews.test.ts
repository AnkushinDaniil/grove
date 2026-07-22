import { beforeEach, describe, expect, it } from "vitest";
import { selectNeedsAttentionCount, selectVisibleErrors, useReviewsStore } from "./reviews";
import type { PR, ReviewRepo } from "../gen/types";

function makePR(number: number): PR {
  return {
    number,
    title: `PR ${number}`,
    author: "someone",
    url: `https://github.com/acme/widgets/pull/${number}`,
    is_draft: false,
    updated_at: "2026-07-20T00:00:00Z",
    review_decision: "REVIEW_REQUIRED",
    checks: "passing",
    additions: 1,
    deletions: 1,
  };
}

function makeRepo(dir: string, counts: Partial<Record<keyof ReviewRepo["buckets"], number>>): ReviewRepo {
  let n = 0;
  const bucket = (count = 0) => Array.from({ length: count }, () => makePR(++n));
  return {
    dir,
    name_with_owner: `acme/${dir}`,
    buckets: {
      needs_review: bucket(counts.needs_review),
      re_review: bucket(counts.re_review),
      reviewed: bucket(counts.reviewed),
      mine: bucket(counts.mine),
    },
  };
}

describe("reviews store selectors", () => {
  beforeEach(() => useReviewsStore.getState().reset());

  it("sums needs_review + re_review across every repo for the nav badge count", () => {
    const repos = [
      makeRepo("a", { needs_review: 2, re_review: 1, reviewed: 5 }),
      makeRepo("b", { needs_review: 1, mine: 3 }),
    ];
    expect(selectNeedsAttentionCount({ repos })).toBe(4); // (2+1) + (1+0)
  });

  it("ignores reviewed/mine entirely -- they never count toward the badge", () => {
    const repos = [makeRepo("a", { reviewed: 9, mine: 9 })];
    expect(selectNeedsAttentionCount({ repos })).toBe(0);
  });

  it("returns 0 for no repos", () => {
    expect(selectNeedsAttentionCount({ repos: [] })).toBe(0);
  });

  it("dismissing an error hides it until a genuinely new message arrives", () => {
    useReviewsStore.getState().setData({ login: "x", repos: [], errors: ["couldn't reach /a: boom"] });
    expect(selectVisibleErrors(useReviewsStore.getState())).toEqual(["couldn't reach /a: boom"]);

    useReviewsStore.getState().dismissError("couldn't reach /a: boom");
    expect(selectVisibleErrors(useReviewsStore.getState())).toEqual([]);

    // A later poll still reporting the same message stays dismissed...
    useReviewsStore.getState().setData({ login: "x", repos: [], errors: ["couldn't reach /a: boom"] });
    expect(selectVisibleErrors(useReviewsStore.getState())).toEqual([]);

    // ...but a genuinely different message still surfaces.
    useReviewsStore
      .getState()
      .setData({ login: "x", repos: [], errors: ["couldn't reach /a: boom", "couldn't reach /b: nope"] });
    expect(selectVisibleErrors(useReviewsStore.getState())).toEqual(["couldn't reach /b: nope"]);
  });
});
