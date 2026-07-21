-- 0003_usage_rollup.sql: pre-aggregated token/cost usage in 5-minute buckets.
--
-- Rows are the rolled-up sum of `usage` events, keyed by 5-minute bucket plus
-- the (profile, driver, model) the usage was attributed to. bucket_start is
-- unix milliseconds truncated to a 5-minute boundary. GET /usage sums the
-- buckets overlapping the requested rolling window; the aggregator upserts into
-- this table, accumulating on the primary key (see internal/usage).

CREATE TABLE usage_rollup (
    bucket_start   INTEGER NOT NULL,
    profile_id     TEXT NOT NULL DEFAULT '',
    driver         TEXT NOT NULL,
    model          TEXT NOT NULL DEFAULT '',
    input_tokens   INTEGER NOT NULL DEFAULT 0,
    output_tokens  INTEGER NOT NULL DEFAULT 0,
    cost_usd       REAL NOT NULL DEFAULT 0,
    PRIMARY KEY (bucket_start, profile_id, driver, model)
);

-- The window query filters by bucket_start range, then groups by profile/driver.
CREATE INDEX idx_usage_rollup_bucket ON usage_rollup(bucket_start);
