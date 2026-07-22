import type { PR, ReviewRepo } from "../gen/types";
import { ago, HOUR, MIN } from "./fixtures";

// A single demo repo so `npm run dev:mock` (VITE_MOCK=1) can show off every
// bucket, the draft tag, the failing-checks pill, and the diff-stat colors
// with zero backend. Mirrors the "daniil" persona already used by the rest
// of the mock fixtures (see fixtures.ts's billing-service PR meta).
export const REVIEW_LOGIN = "daniil";
export const NETHERMIND_DIR = "/Users/daniil/code/nethermind";
const NETHERMIND_NAME = "NethermindEth/nethermind";

function pr(partial: Omit<PR, "url">): PR {
  return { url: `https://github.com/${NETHERMIND_NAME}/pull/${partial.number}`, ...partial };
}

export function buildReviewFixtureRepos(): ReviewRepo[] {
  return [
    {
      dir: NETHERMIND_DIR,
      name_with_owner: NETHERMIND_NAME,
      buckets: {
        needs_review: [
          pr({
            number: 8421,
            title: "Fix null reference in trie pruning during snap sync",
            author: "asdacap",
            is_draft: false,
            updated_at: ago(2 * HOUR),
            review_decision: "REVIEW_REQUIRED",
            checks: "failing",
            additions: 142,
            deletions: 37,
          }),
          pr({
            number: 8430,
            title: "Add Verkle tree witness generation benchmark",
            author: "LukaszRozmej",
            is_draft: false,
            updated_at: ago(5 * HOUR),
            review_decision: "REVIEW_REQUIRED",
            checks: "passing",
            additions: 310,
            deletions: 12,
          }),
          // Also the hero fixture for the interactive review workspace (see
          // mock/prReviewFixtures.ts) -- reachable both by direct URL and by
          // clicking "Review in grove" here, the feature's real entry point.
          pr({
            number: 12540,
            title: "Fix nonce ordering check in TxPool.Insert and add regression test",
            author: "asdacap",
            is_draft: false,
            updated_at: ago(15 * MIN),
            review_decision: "REVIEW_REQUIRED",
            checks: "passing",
            additions: 23,
            deletions: 4,
          }),
        ],
        re_review: [
          pr({
            number: 8390,
            title: "Optimize state sync batch size heuristics",
            author: "kamilchodola",
            is_draft: false,
            updated_at: ago(30 * MIN),
            review_decision: "CHANGES_REQUESTED",
            checks: "pending",
            additions: 58,
            deletions: 20,
          }),
        ],
        reviewed: [
          pr({
            number: 8355,
            title: "Refactor RLP decoder allocations",
            author: "rubo",
            is_draft: false,
            updated_at: ago(24 * HOUR),
            review_decision: "APPROVED",
            checks: "passing",
            additions: 89,
            deletions: 64,
          }),
        ],
        mine: [
          // Also doubles as the "draft" demo PR -- a WIP of your own is the
          // most natural place a draft shows up in Review Radar.
          pr({
            number: 8440,
            title: "WIP: Discovery v5 rate limiting",
            author: REVIEW_LOGIN,
            is_draft: true,
            updated_at: ago(10 * MIN),
            review_decision: "",
            checks: "pending",
            additions: 210,
            deletions: 5,
          }),
        ],
      },
    },
  ];
}
