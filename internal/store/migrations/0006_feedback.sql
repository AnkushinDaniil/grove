-- 0006_feedback.sql: user-recorded quality signals that turn into fixable work
-- items (docs/API.md "Feedback loop").
--
-- A feedback row is one signal ("this skill misfired", "wrong approach") attached
-- to a node, and optionally the session and event that triggered it, classified by
-- kind (skill|tool|model|agent|other) with an optional subject (the skill/tool/
-- model name). It is "open" until resolved_at is set, at which point fix_node_id
-- may link the task node created to address it — closing the loop inside the tree.
--
-- Timestamps are unix milliseconds, matching every other timestamp column (see
-- internal/store/scan.go). node_id is deliberately not a foreign key (matching the
-- events table): feedback outlives node archival and is validated at the API
-- boundary, not by the schema.

CREATE TABLE feedback (
    id           TEXT PRIMARY KEY,
    node_id      TEXT NOT NULL,
    session_id   TEXT NOT NULL DEFAULT '',
    event_id     TEXT NOT NULL DEFAULT '',
    kind         TEXT NOT NULL,
    subject      TEXT NOT NULL DEFAULT '',
    comment      TEXT NOT NULL,
    created_at   INTEGER NOT NULL,
    resolved_at  INTEGER,
    fix_node_id  TEXT NOT NULL DEFAULT ''
);

-- Feedback is listed per node (and, for the stats breakdown, across a subtree).
CREATE INDEX idx_feedback_node ON feedback(node_id);
-- The open/resolved filter and the stats "open" count both key off resolved_at.
CREATE INDEX idx_feedback_resolved ON feedback(resolved_at);
