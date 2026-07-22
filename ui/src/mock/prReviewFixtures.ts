import type { DiffHunk, PRReview } from "../gen/types";
import { ago, HOUR } from "./fixtures";
import { NETHERMIND_DIR, REVIEW_LOGIN } from "./reviewFixtures";

// The rich demo PR for the interactive review workspace -- deliberately
// reuses docs/API.md's own example PR number (12540) and the Review Radar
// fixture's repo, and is also seeded into that repo's `needs_review` bucket
// (see reviewFixtures.ts) so it's reachable both by direct URL and by
// clicking "Review in grove" from Review Radar, the feature's real entry
// point.
export const HERO_PR_DIR = NETHERMIND_DIR;
export const HERO_PR_NUMBER = 12540;

function hunk(header: string, lines: DiffHunk["lines"]): DiffHunk {
  return { header, lines };
}

export function buildHeroPRReview(): PRReview {
  return {
    number: HERO_PR_NUMBER,
    title: "Fix nonce ordering check in TxPool.Insert and add regression test",
    author: "asdacap",
    url: `https://github.com/NethermindEth/nethermind/pull/${HERO_PR_NUMBER}`,
    state: "OPEN",
    head_sha: "a1b2c3d4e5f6789012345678901234567890abcd",
    base_ref: "master",
    checks: "passing",
    review_decision: "REVIEW_REQUIRED",
    body:
      "Fixes a bug where a stale-nonce rejection happened silently, with no trace of *why* " +
      "the transaction was dropped. Also adds a null guard to `IsKnown` and a regression test.\n\n" +
      "### Testing\n- `dotnet test Nethermind.TxPool.Test`",
    files: [
      {
        path: "src/Nethermind/Nethermind.TxPool/TxPool.cs",
        status: "modified",
        additions: 7,
        deletions: 3,
        binary: false,
        hunks: [
          hunk(
            "@@ -142,7 +142,11 @@ public AcceptTxResult Insert(Transaction tx, TxHandlingOptions handlingOptions)",
            [
              { op: " ", old_line: 142, new_line: 142, text: "        {" },
              { op: " ", old_line: 143, new_line: 143, text: "            if (tx.Nonce < accountNonce)" },
              { op: "-", old_line: 144, new_line: 0, text: "            {" },
              { op: "-", old_line: 145, new_line: 0, text: "                return AcceptTxResult.OldNonce;" },
              { op: "-", old_line: 146, new_line: 0, text: "            }" },
              { op: "+", old_line: 0, new_line: 144, text: "            {" },
              {
                op: "+",
                old_line: 0,
                new_line: 145,
                text: '                if (_logger.IsTrace) _logger.Trace($"Skipping tx {tx.Hash}: nonce {tx.Nonce} < account nonce {accountNonce}");',
              },
              { op: "+", old_line: 0, new_line: 146, text: "                return AcceptTxResult.OldNonce;" },
              { op: "+", old_line: 0, new_line: 147, text: "            }" },
              { op: " ", old_line: 147, new_line: 148, text: "" },
              {
                op: " ",
                old_line: 148,
                new_line: 149,
                text: "            if (tx.Nonce > accountNonce + _maxPendingNonceGap)",
              },
            ],
          ),
          hunk("@@ -210,6 +214,7 @@ private bool IsKnown(Keccak hash)", [
            { op: " ", old_line: 210, new_line: 214, text: "        {" },
            { op: " ", old_line: 211, new_line: 215, text: "            if (hash is null)" },
            { op: "+", old_line: 0, new_line: 216, text: "            {" },
            { op: "+", old_line: 0, new_line: 217, text: "                throw new ArgumentNullException(nameof(hash));" },
            { op: "+", old_line: 0, new_line: 218, text: "            }" },
            { op: " ", old_line: 212, new_line: 219, text: "" },
            { op: " ", old_line: 213, new_line: 220, text: "            return _hashCache.Contains(hash);" },
          ]),
        ],
      },
      {
        path: "src/Nethermind/Nethermind.TxPool.Test/TxPoolTests.cs",
        status: "modified",
        additions: 15,
        deletions: 0,
        binary: false,
        hunks: [
          hunk("@@ -88,6 +88,21 @@ public class TxPoolTests", [
            { op: " ", old_line: 88, new_line: 88, text: "        }" },
            { op: " ", old_line: 89, new_line: 89, text: "" },
            { op: "+", old_line: 0, new_line: 90, text: "        [Test]" },
            { op: "+", old_line: 0, new_line: 91, text: "        public void Insert_rejects_stale_nonce_and_logs_reason()" },
            { op: "+", old_line: 0, new_line: 92, text: "        {" },
            { op: "+", old_line: 0, new_line: 93, text: "            TxPool pool = CreatePool();" },
            { op: "+", old_line: 0, new_line: 94, text: "            Address account = TestItem.AddressA;" },
            { op: "+", old_line: 0, new_line: 95, text: "            EnsureSenderBalance(account, 1.Ether());" },
            { op: "+", old_line: 0, new_line: 96, text: "            SetAccountNonce(account, 5);" },
            { op: "+", old_line: 0, new_line: 97, text: "" },
            {
              op: "+",
              old_line: 0,
              new_line: 98,
              text: "            Transaction tx = Build.A.Transaction.WithNonce(3).SignedAndResolved().TestObject;",
            },
            {
              op: "+",
              old_line: 0,
              new_line: 99,
              text: "            AcceptTxResult result = pool.SubmitTx(tx, TxHandlingOptions.None);",
            },
            { op: "+", old_line: 0, new_line: 100, text: "" },
            { op: "+", old_line: 0, new_line: 101, text: "            result.Should().Be(AcceptTxResult.OldNonce);" },
            {
              op: "+",
              old_line: 0,
              new_line: 102,
              text: '            _logger.TraceEntries.Should().ContainSingle(e => e.Contains("nonce 3"));',
            },
            { op: "+", old_line: 0, new_line: 103, text: "        }" },
            { op: "+", old_line: 0, new_line: 104, text: "" },
            { op: " ", old_line: 90, new_line: 105, text: "        [Test]" },
            { op: " ", old_line: 91, new_line: 106, text: "        public void Insert_accepts_valid_nonce()" },
          ]),
        ],
      },
      {
        path: "src/Nethermind/Nethermind.TxPool/TxPoolConfig.cs",
        status: "modified",
        additions: 1,
        deletions: 1,
        binary: false,
        hunks: [
          hunk("@@ -18,7 +18,7 @@ public class TxPoolConfig : ITxPoolConfig", [
            { op: " ", old_line: 18, new_line: 18, text: "        public int GasLimit { get; set; } = 10_000_000;" },
            { op: " ", old_line: 19, new_line: 19, text: "" },
            { op: "-", old_line: 20, new_line: 0, text: "        public int Size { get; set; } = 2048;" },
            { op: "+", old_line: 0, new_line: 20, text: "        public int Size { get; set; } = 4096;" },
            { op: " ", old_line: 21, new_line: 21, text: "" },
            { op: " ", old_line: 22, new_line: 22, text: "        public bool BlobsSupport { get; set; } = true;" },
          ]),
        ],
      },
    ],
    threads: [
      {
        id: "PRRT_hero_1",
        path: "src/Nethermind/Nethermind.TxPool/TxPool.cs",
        line: 146,
        side: "RIGHT",
        is_resolved: false,
        diff_hunk:
          "@@ -142,7 +142,11 @@ public AcceptTxResult Insert(...)\n             ...\n                return AcceptTxResult.OldNonce;",
        comments: [
          {
            id: "PRRC_hero_1",
            author: "LukaszRozmej",
            body:
              "Should we also check `tx.GasBottleneck` before rejecting here, or is that handled " +
              "upstream in `ValidateAndFilter`?",
            created_at: ago(3 * HOUR),
            is_mine: false,
          },
        ],
      },
      {
        id: "PRRT_hero_2",
        path: "src/Nethermind/Nethermind.TxPool/TxPool.cs",
        line: 217,
        side: "RIGHT",
        is_resolved: true,
        diff_hunk:
          "@@ -210,6 +214,7 @@ private bool IsKnown(Keccak hash)\n             ...\n                throw new ArgumentNullException(nameof(hash));",
        comments: [
          {
            id: "PRRC_hero_2",
            author: REVIEW_LOGIN,
            body: "Nit: could inline this as `ArgumentNullException.ThrowIfNull(hash)` -- we're already targeting net8.",
            created_at: ago(2 * HOUR),
            is_mine: true,
          },
          {
            id: "PRRC_hero_3",
            author: "asdacap",
            body: "Good call, done.",
            created_at: ago(1 * HOUR),
            is_mine: false,
          },
        ],
      },
    ],
  };
}
