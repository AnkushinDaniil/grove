import type { PRReview } from "../gen/types";
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

const ORIGINAL_TX_POOL_CS = `using System;
using Nethermind.Core;
using Nethermind.Core.Crypto;
using Nethermind.Logging;

namespace Nethermind.TxPool
{
    public class TxPool : ITxPool
    {
        private readonly ILogger _logger;
        private readonly HashSet<Keccak> _hashCache = new();
        private readonly int _maxPendingNonceGap;

        public TxPool(ILogManager logManager, int maxPendingNonceGap)
        {
            _logger = logManager.GetClassLogger();
            _maxPendingNonceGap = maxPendingNonceGap;
        }

        public AcceptTxResult Insert(Transaction tx, TxHandlingOptions handlingOptions)
        {
            UInt256 accountNonce = GetAccountNonce(tx.SenderAddress);

            if (tx.Nonce < accountNonce)
            {
                return AcceptTxResult.OldNonce;
            }

            if (tx.Nonce > accountNonce + _maxPendingNonceGap)
            {
                return AcceptTxResult.NonceGapTooWide;
            }

            _pending.Add(tx.Hash, tx);
            return AcceptTxResult.Accepted;
        }

        private bool IsKnown(Keccak hash)
        {
            return _hashCache.Contains(hash);
        }

        private UInt256 GetAccountNonce(Address address)
        {
            return _stateReader.GetNonce(address);
        }
    }
}
`;

const MODIFIED_TX_POOL_CS = `using System;
using Nethermind.Core;
using Nethermind.Core.Crypto;
using Nethermind.Logging;

namespace Nethermind.TxPool
{
    public class TxPool : ITxPool
    {
        private readonly ILogger _logger;
        private readonly HashSet<Keccak> _hashCache = new();
        private readonly int _maxPendingNonceGap;

        public TxPool(ILogManager logManager, int maxPendingNonceGap)
        {
            _logger = logManager.GetClassLogger();
            _maxPendingNonceGap = maxPendingNonceGap;
        }

        public AcceptTxResult Insert(Transaction tx, TxHandlingOptions handlingOptions)
        {
            UInt256 accountNonce = GetAccountNonce(tx.SenderAddress);

            if (tx.Nonce < accountNonce)
            {
                if (_logger.IsTrace) _logger.Trace($"Skipping tx {tx.Hash}: nonce {tx.Nonce} < account nonce {accountNonce}");
                return AcceptTxResult.OldNonce;
            }

            if (tx.Nonce > accountNonce + _maxPendingNonceGap)
            {
                return AcceptTxResult.NonceGapTooWide;
            }

            _pending.Add(tx.Hash, tx);
            return AcceptTxResult.Accepted;
        }

        private bool IsKnown(Keccak hash)
        {
            if (hash is null)
            {
                throw new ArgumentNullException(nameof(hash));
            }

            return _hashCache.Contains(hash);
        }

        private UInt256 GetAccountNonce(Address address)
        {
            return _stateReader.GetNonce(address);
        }
    }
}
`;

const ORIGINAL_TX_POOL_TESTS_CS = `using NUnit.Framework;
using FluentAssertions;
using Nethermind.Core.Test.Builders;

namespace Nethermind.TxPool.Test
{
    [TestFixture]
    public class TxPoolTests
    {
        [Test]
        public void Insert_accepts_valid_nonce()
        {
            TxPool pool = CreatePool();
            Address account = TestItem.AddressA;
            EnsureSenderBalance(account, 1.Ether());
            SetAccountNonce(account, 5);

            Transaction tx = Build.A.Transaction.WithNonce(5).SignedAndResolved().TestObject;
            AcceptTxResult result = pool.SubmitTx(tx, TxHandlingOptions.None);

            result.Should().Be(AcceptTxResult.Accepted);
        }

        [Test]
        public void Insert_rejects_nonce_gap_too_wide()
        {
            TxPool pool = CreatePool();
            Address account = TestItem.AddressA;
            EnsureSenderBalance(account, 1.Ether());
            SetAccountNonce(account, 5);

            Transaction tx = Build.A.Transaction.WithNonce(500).SignedAndResolved().TestObject;
            AcceptTxResult result = pool.SubmitTx(tx, TxHandlingOptions.None);

            result.Should().Be(AcceptTxResult.NonceGapTooWide);
        }
    }
}
`;

const MODIFIED_TX_POOL_TESTS_CS = `using NUnit.Framework;
using FluentAssertions;
using Nethermind.Core.Test.Builders;

namespace Nethermind.TxPool.Test
{
    [TestFixture]
    public class TxPoolTests
    {
        [Test]
        public void Insert_rejects_stale_nonce_and_logs_reason()
        {
            TxPool pool = CreatePool();
            Address account = TestItem.AddressA;
            EnsureSenderBalance(account, 1.Ether());
            SetAccountNonce(account, 5);

            Transaction tx = Build.A.Transaction.WithNonce(3).SignedAndResolved().TestObject;
            AcceptTxResult result = pool.SubmitTx(tx, TxHandlingOptions.None);

            result.Should().Be(AcceptTxResult.OldNonce);
            _logger.TraceEntries.Should().ContainSingle(e => e.Contains("nonce 3"));
        }

        [Test]
        public void Insert_accepts_valid_nonce()
        {
            TxPool pool = CreatePool();
            Address account = TestItem.AddressA;
            EnsureSenderBalance(account, 1.Ether());
            SetAccountNonce(account, 5);

            Transaction tx = Build.A.Transaction.WithNonce(5).SignedAndResolved().TestObject;
            AcceptTxResult result = pool.SubmitTx(tx, TxHandlingOptions.None);

            result.Should().Be(AcceptTxResult.Accepted);
        }

        [Test]
        public void Insert_rejects_nonce_gap_too_wide()
        {
            TxPool pool = CreatePool();
            Address account = TestItem.AddressA;
            EnsureSenderBalance(account, 1.Ether());
            SetAccountNonce(account, 5);

            Transaction tx = Build.A.Transaction.WithNonce(500).SignedAndResolved().TestObject;
            AcceptTxResult result = pool.SubmitTx(tx, TxHandlingOptions.None);

            result.Should().Be(AcceptTxResult.NonceGapTooWide);
        }
    }
}
`;

const ORIGINAL_TX_POOL_CONFIG_CS = `namespace Nethermind.TxPool
{
    public class TxPoolConfig : ITxPoolConfig
    {
        public int GasLimit { get; set; } = 10_000_000;

        public int Size { get; set; } = 2048;

        public bool BlobsSupport { get; set; } = true;
    }
}
`;

const MODIFIED_TX_POOL_CONFIG_CS = `namespace Nethermind.TxPool
{
    public class TxPoolConfig : ITxPoolConfig
    {
        public int GasLimit { get; set; } = 10_000_000;

        public int Size { get; set; } = 4096;

        public bool BlobsSupport { get; set; } = true;
    }
}
`;

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
        additions: 6,
        deletions: 0,
        binary: false,
        original_content: ORIGINAL_TX_POOL_CS,
        modified_content: MODIFIED_TX_POOL_CS,
        content_omitted: "",
        hunks: [],
      },
      {
        path: "src/Nethermind/Nethermind.TxPool.Test/TxPoolTests.cs",
        status: "modified",
        additions: 14,
        deletions: 0,
        binary: false,
        original_content: ORIGINAL_TX_POOL_TESTS_CS,
        modified_content: MODIFIED_TX_POOL_TESTS_CS,
        content_omitted: "",
        hunks: [],
      },
      {
        path: "src/Nethermind/Nethermind.TxPool/TxPoolConfig.cs",
        status: "modified",
        additions: 1,
        deletions: 1,
        binary: false,
        original_content: ORIGINAL_TX_POOL_CONFIG_CS,
        modified_content: MODIFIED_TX_POOL_CONFIG_CS,
        content_omitted: "",
        hunks: [],
      },
    ],
    threads: [
      {
        id: "PRRT_hero_1",
        path: "src/Nethermind/Nethermind.TxPool/TxPool.cs",
        // Anchored to `return AcceptTxResult.OldNonce;` in MODIFIED_TX_POOL_CS.
        line: 27,
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
        // Anchored to `throw new ArgumentNullException(nameof(hash));` in MODIFIED_TX_POOL_CS.
        line: 43,
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
