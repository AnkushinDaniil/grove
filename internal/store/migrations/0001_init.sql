-- 0001_init.sql: initial grove schema.
--
-- Timestamps are stored as INTEGER unix milliseconds. A zero time.Time in Go
-- maps to SQL NULL for nullable columns (see internal/store/scan.go).

CREATE TABLE nodes (
    id                  TEXT PRIMARY KEY,
    parent_id           TEXT REFERENCES nodes(id),
    kind                TEXT NOT NULL,
    title               TEXT NOT NULL,
    brief               TEXT NOT NULL DEFAULT '',
    status              TEXT NOT NULL,
    attention           TEXT NOT NULL,
    attention_reason    TEXT NOT NULL DEFAULT '',
    attention_since     INTEGER,
    driver              TEXT NOT NULL DEFAULT '',
    profile_id          TEXT NOT NULL DEFAULT '',
    current_session_id  TEXT NOT NULL DEFAULT '',
    workspace_dir       TEXT NOT NULL DEFAULT '',
    meta                TEXT NOT NULL DEFAULT '{}',
    position            INTEGER NOT NULL DEFAULT 0,
    created_at          INTEGER NOT NULL,
    updated_at          INTEGER NOT NULL,
    archived_at         INTEGER
);

CREATE INDEX idx_nodes_parent_live ON nodes(parent_id) WHERE archived_at IS NULL;
CREATE INDEX idx_nodes_attention_live ON nodes(attention) WHERE attention <> 'none' AND archived_at IS NULL;

CREATE TABLE sessions (
    id                          TEXT PRIMARY KEY,
    node_id                     TEXT NOT NULL REFERENCES nodes(id),
    driver                      TEXT NOT NULL,
    profile_id                  TEXT NOT NULL DEFAULT '',
    mode                        TEXT NOT NULL,
    driver_session_id           TEXT NOT NULL DEFAULT '',
    parent_driver_session_id    TEXT NOT NULL DEFAULT '',
    status                      TEXT NOT NULL,
    exit_code                   INTEGER,
    transcript_path             TEXT NOT NULL DEFAULT '',
    cwd                         TEXT NOT NULL,
    started_at                  INTEGER NOT NULL,
    ended_at                    INTEGER
);

CREATE INDEX idx_sessions_node_started ON sessions(node_id, started_at DESC);

CREATE TABLE repos (
    id            TEXT PRIMARY KEY,
    project_id    TEXT NOT NULL REFERENCES nodes(id),
    name          TEXT NOT NULL,
    source_path   TEXT NOT NULL,
    default_base  TEXT NOT NULL DEFAULT '',
    created_at    INTEGER NOT NULL,
    UNIQUE(project_id, name)
);

CREATE TABLE worktrees (
    id          TEXT PRIMARY KEY,
    node_id     TEXT NOT NULL REFERENCES nodes(id),
    repo_id     TEXT NOT NULL REFERENCES repos(id),
    path        TEXT NOT NULL,
    branch      TEXT NOT NULL,
    base_ref    TEXT NOT NULL,
    status      TEXT NOT NULL,
    created_at  INTEGER NOT NULL,
    removed_at  INTEGER,
    UNIQUE(node_id, repo_id)
);

CREATE TABLE events (
    id                  TEXT PRIMARY KEY,
    node_id             TEXT NOT NULL,
    session_id          TEXT NOT NULL DEFAULT '',
    type                TEXT NOT NULL,
    payload             TEXT NOT NULL DEFAULT '{}',
    requires_attention  INTEGER NOT NULL DEFAULT 0,
    acked_at            INTEGER,
    created_at          INTEGER NOT NULL
);

CREATE INDEX idx_events_node_id ON events(node_id, id);
CREATE INDEX idx_events_inbox ON events(requires_attention, acked_at) WHERE requires_attention = 1 AND acked_at IS NULL;

CREATE TABLE profiles (
    id          TEXT PRIMARY KEY,
    driver      TEXT NOT NULL,
    name        TEXT NOT NULL,
    config_dir  TEXT NOT NULL,
    is_default  INTEGER NOT NULL DEFAULT 0,
    created_at  INTEGER NOT NULL,
    UNIQUE(driver, name)
);

CREATE TABLE settings (
    key    TEXT PRIMARY KEY,
    value  TEXT NOT NULL
);
