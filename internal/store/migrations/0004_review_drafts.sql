-- 0004_review_drafts.sql: pending PR review comments held in grove until submit.
--
-- A draft is one inline review comment the user is preparing for a pull request,
-- keyed by (dir, pr) — the interactive review workspace (docs/API.md
-- "Interactive review workspace"). Drafts accumulate as the user reviews and are
-- cleared once posted as a batch review via gh. created_at is unix milliseconds,
-- matching every other timestamp column (see internal/store/scan.go).

CREATE TABLE review_drafts (
    id          TEXT PRIMARY KEY,
    dir         TEXT NOT NULL,
    pr          INTEGER NOT NULL,
    path        TEXT NOT NULL,
    line        INTEGER NOT NULL,
    side        TEXT NOT NULL DEFAULT 'RIGHT',
    body        TEXT NOT NULL,
    created_at  INTEGER NOT NULL
);

-- Drafts are always listed for one workspace (dir, pr).
CREATE INDEX idx_review_drafts_dir_pr ON review_drafts(dir, pr);
