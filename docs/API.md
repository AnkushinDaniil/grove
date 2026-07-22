# grove API contract (v1)

Frozen contract between the daemon (`internal/api`, `internal/ws`) and the web UI.
JSON everywhere, `snake_case` fields, timestamps as RFC 3339 strings (omitted when
zero). Auth: `POST /auth/session {token}` sets an HttpOnly SameSite=Strict cookie;
every non-GET request must also send header `X-Grove-CSRF: 1`. The daemon binds
`127.0.0.1` only.

## Entities

```jsonc
// Node
{
  "id": "…", "parent_id": "…", "kind": "workspace|project|task",
  "title": "…", "brief": "…",
  "status": "idle|starting|running|awaiting_input|done|failed|interrupted",
  "attention": "none|permission|question|done|error|review",
  "attention_reason": "…", "attention_since": "2026-07-20T12:00:00Z",
  "driver": "", "profile_id": "",            // empty = inherited
  "current_session_id": "", "workspace_dir": "",
  "work_dir": "",                            // user-set cwd, empty = inherited
  "meta": {}, "position": 0,
  "created_at": "…", "updated_at": "…", "archived_at": "…"
}
// Session
{
  "id": "…", "node_id": "…", "driver": "claude", "profile_id": "",
  "mode": "pty|headless", "driver_session_id": "",
  "status": "starting|running|awaiting_input|exited|failed|interrupted",
  "exit_code": 0, "cwd": "…", "started_at": "…", "ended_at": "…"
}
// Event
{
  "id": "…", "node_id": "…", "session_id": "…",
  "type": "session_started|text|tool_call|tool_result|awaiting_input|turn_done|session_ended|error|usage",
  "payload": { /* type-specific object, see internal/core/payload.go */ },
  "requires_attention": false, "acked_at": "…", "created_at": "…"
}
```

## REST (`/api/v1`)

| Method & path | Body | Returns |
|---|---|---|
| `GET  /tree` | — | `{rev, nodes: Node[], sessions: Session[]}` |
| `POST /nodes` | `{parent_id, kind, title, brief?, driver?, profile_id?, work_dir?}` | `Node` (201) |
| `PATCH /nodes/{id}` | `{title?, brief?, driver?, profile_id?, work_dir?, meta?}` | `Node` |
| `POST /nodes/{id}/archive` | — | `{archived: [id…]}` |
| `POST /nodes/{id}/ack` | — | `Node` |
| `POST /nodes/{id}/sessions` | `{mode, prompt?, resume_id?}` | `Session` (201) |
| `POST /nodes/{id}/prompt` | `{text}` | 204 |
| `POST /sessions/{id}/stop` | — | 204 |
| `GET  /nodes/{id}/events?after=<event_id>&limit=<n>` | — | `Event[]` (ascending by id) |
| `GET  /inbox` | — | `Event[]` (unacked attention, newest first) |
| `GET  /fs/dirs?prefix=<text>` | — | `{dirs: string[], home}` |
| `GET  /version` | — | `{version, commit}` |
| `GET  /usage?window=5h\|week` | — | `{profiles: [UsageWindow]}` |
| `POST /auth/session` | `{token}` | 204 + cookie |
| `GET  /auth/me` | — | 204 or 401 |
| `POST /internal/hook?node=&driver=&event=` | raw hook JSON | 204 (auth: `X-Grove-Hook-Token`) |

Clarifications (rulings on ambiguities):
- **Every** path in the table above lives under `/api/v1` — including
  `/api/v1/auth/session`, `/api/v1/auth/me`, `/api/v1/internal/hook`. Separately,
  the daemon serves a bare `GET /auth` HTML page (browser entry point used by
  `grove open`) that exchanges the `#t=<token>` fragment via `/api/v1/auth/session`.
- `POST /nodes/{id}/prompt` **echoes** the injected prompt as a `text` event with
  `payload.role = "user"` before it reaches the agent, so conversation views read
  naturally in headless mode (`TextPayload.Role`: empty/absent = assistant).
- `Node.meta` is a JSON **object** on the wire (`json.RawMessage` over the
  internally stored string); `PATCH /nodes/{id}` accepts an object, invalid JSON → 400.
- `Node.work_dir` is the user-set working directory, **inherited like `driver`/`profile_id`**
  (nearest non-empty ancestor wins). Sessions start in `workspace_dir` (the machine-managed
  worktree) if set, else the inherited `work_dir`, else the user's home. On `POST`/`PATCH` a
  non-empty value is normalized first — `~`, `~/x` and bare relative paths resolve against
  the daemon user's home (matching `/fs/dirs` completion semantics) — and must then be an
  existing directory → else 400. The stored value is always absolute; `PATCH` with an explicit
  empty string clears the override (falls back to inheritance).
- `attention: "review"` intentionally has no M1 event type — it is
  forward-declared for the M2 GitHub review round-trip and until then appears only
  on nodes, never as an inbox event.
- `GET /fs/dirs?prefix=<text>` powers terminal-style `work_dir` tab-completion.
  It lists the **directories** (real dirs, and symlinks that resolve to dirs —
  never files) inside the prefix's parent whose final segment case-insensitively
  `HasPrefix`-matches; a trailing `/` (or an empty prefix, treated as the home
  dir) lists the whole directory, and a leading `~`/`~/` expands to the daemon
  user's home (also returned as `home`). Hidden entries (leading `.`) appear only
  when the typed segment itself starts with `.`. Results are absolute,
  case-insensitively sorted, and capped at 50; it never recurses. An
  unreadable/nonexistent parent returns `{"dirs": [], "home"}` with 200 (nothing
  to complete is not an error), and a `prefix` containing a NUL byte → 400. Trust
  model: grove is a single-user loopback daemon exposing the authenticated user's
  own filesystem back to them, so there is no path-traversal boundary to enforce.

Errors: `{"error": "human readable message"}` with 4xx/5xx. Validation failures → 400,
unknown ids → 404, auth → 401.

```jsonc
// UsageWindow — one profile's consumption in the requested window.
// utilization is a 0..1 estimate against the plan's limit (null = unknown);
// resets_at appears when a rate-limit reset time was detected.
{
  "profile_id": "…", "name": "personal", "driver": "claude",
  "window": "5h", "window_start": "…", "window_end": "…",
  "input_tokens": 0, "output_tokens": 0, "cache_read_tokens": 0,
  "cost_usd": 0.0, "utilization": 0.42, "resets_at": "…",
  "cooldown_until": "…"   // set while the profile is rate-limited
}
```

The daemon aggregates usage locally (no network calls) from grove's own
normalized `usage` events: the aggregator backfills historical events once at
startup and then folds live usage off the tree's delta feed into 5-minute
rollup buckets. `window=5h` sums a rolling last 5 hours, `window=week` a rolling
7 days (default `5h`; any other value → 400). It returns one `UsageWindow` per
`(profile_id, driver)` with usage in the window — the empty (inherited) profile
reports `name: "default"`, and the driver comes from the event's session. In
this iteration `utilization` is always `null` (no plan-limit model yet, so the
UI renders raw token counts), `cache_read_tokens` is `0`, and
`resets_at`/`cooldown_until` are omitted (rate-limit detection not yet wired).
No usage in the window → `{"profiles": []}`.

### `GET /stats?scope=<node_id>&range=24h|7d|30d` (draft — additive evolution allowed)

Aggregates over the scope subtree (default: whole workspace), all computed from the
local DB (events / sessions / usage):

```jsonc
{
  "range": "7d", "scope": "…",
  "tokens": {
    "total": {"input": 0, "output": 0, "cache_read": 0, "cost_usd": 0.0},
    "by_day":     [{"day": "2026-07-20", "input": 0, "output": 0, "cost_usd": 0.0}],
    "by_driver":  [{"driver": "claude", "input": 0, "output": 0, "cost_usd": 0.0}],
    "by_profile": [{"profile_id": "…", "name": "…", "input": 0, "output": 0, "cost_usd": 0.0}],
    "top_nodes":  [{"node_id": "…", "title": "…", "input": 0, "output": 0, "cost_usd": 0.0}]
  },
  "agents": {
    "sessions_active": 0, "sessions_by_day": [{"day": "…", "started": 0, "done": 0, "failed": 0}],
    "avg_session_minutes": 0.0, "by_driver": [{"driver": "…", "count": 0}]
  },
  "flow": {
    "tasks_created": 0, "tasks_done": 0, "tasks_failed": 0,
    "median_task_hours": 0.0,
    "attention_wait_p50_minutes": 0.0, "attention_wait_p95_minutes": 0.0,
    "prs_opened": 0, "prs_merged": 0
  },
  "tools":  [{"name": "Bash", "calls": 0, "errors": 0}],           // from tool_call/tool_result events
  "models": [{"model": "claude-sonnet-5", "input": 0, "output": 0, "cost_usd": 0.0}],
  "skills": [{"skill": "code-review", "invocations": 0}],          // Skill-tool calls parsed from payloads
  "feedback": [{"kind": "skill", "subject": "code-review", "open": 0, "total": 0}]
}
```

## Feedback loop

User-recorded quality signals ("this skill misfired", "wrong approach") that turn
into fixable work items:

| Method & path | Body | Returns |
|---|---|---|
| `POST /feedback` | `{node_id, session_id?, event_id?, kind: skill\|tool\|model\|agent\|other, subject?, comment}` | `Feedback` (201) |
| `GET  /feedback?status=open\|resolved\|all` | — | `Feedback[]` |
| `POST /feedback/{id}/resolve` | `{fix_node_id?}` | `Feedback` |

`Feedback = {id, node_id, session_id, event_id, kind, subject, comment, created_at,
resolved_at, fix_node_id}`. The UI offers "Create fix task" on any open feedback item:
it spawns a task node briefed with the feedback context (and, for kind=skill, the
skill name/path), then links it via `fix_node_id` — closing the loop inside the tree.

## Review Radar (`/api/v1/reviews`)

A standing queue of open GitHub PRs across watched repositories, classified by
what they need from the user. All GitHub access is via the `gh` CLI (the daemon
stores no tokens; auth is the user's existing `gh` login). Watched repos are
directories stored in the `review_dirs` setting, auto-seeded from distinct node
`work_dir` values on first read.

| Method & path | Body | Returns |
|---|---|---|
| `GET  /reviews` | — | `{login, repos: [ReviewRepo], errors: [string]}` |
| `GET  /reviews/sources` | — | `{dirs: [string]}` |
| `POST /reviews/sources` | `{dirs: [string]}` | `{dirs: [string]}` (absolute existing git dirs; else 400) |
| `POST /reviews/start` | `{dir, pr, title?}` | `Node` (201) — spawns a review task node |

```jsonc
// ReviewRepo — one watched repository's classified PRs.
{
  "dir": "/Users/…/nethermind",
  "name_with_owner": "NethermindEth/nethermind",
  "buckets": {
    // Each is a list of PR objects. A PR appears in exactly one bucket,
    // first match wins in this order:
    "needs_review":   [PR],  // open, not draft, not mine, no review from me (or review requested from me)
    "re_review":      [PR],  // I reviewed, but new commits landed since (head oid changed)
    "reviewed":       [PR],  // I already reviewed; kept for revisiting
    "mine":           [PR]   // authored by me, open
  }
}
// PR
{
  "number": 12540, "title": "…", "author": "kamilchodola", "url": "https://github.com/…/pull/12540",
  "is_draft": false, "updated_at": "2026-07-22T09:01:12Z",
  "review_decision": "REVIEW_REQUIRED|APPROVED|CHANGES_REQUESTED|",
  "checks": "passing|failing|pending|none",
  "additions": 0, "deletions": 0
}
```

`POST /reviews/start` spawns a **task** node under the workspace root (title
`Review <owner/repo>#<pr>`), with the repo dir as its `work_dir` and a brief
instructing a read-only review of that PR — it then runs like any other node
(the user starts a session on it). Re-review detection uses head-oid state
persisted per (repo, pr); until a PR has been seen twice its commits-since
state is unknown and it stays in `needs_review`/`reviewed` by review presence.

## Interactive review workspace (`/api/v1/reviews/pr`)

One PR = one review workspace: the PR diff rendered with inline comment threads,
LLM-assisted drafting, and batch submit — all via `gh` (no stored tokens).

| Method & path | Body | Returns |
|---|---|---|
| `GET  /reviews/pr?dir=&pr=` | — | `PRReview` |
| `GET  /reviews/pr/drafts?dir=&pr=` | — | `{drafts: [DraftComment]}` |
| `POST /reviews/pr/drafts` | `{dir, pr, path, line, side, body}` | `DraftComment` (201) |
| `DELETE /reviews/pr/drafts/{id}` | — | 204 |
| `POST /reviews/pr/ai-draft` | `{dir, pr, kind: comment\|reply, path?, line?, thread_id?, instruction?}` | `{text}` |
| `POST /reviews/pr/submit` | `{dir, pr, event: APPROVE\|REQUEST_CHANGES\|COMMENT, body, draft_ids: []}` | `{url}` |
| `POST /reviews/pr/reply` | `{dir, pr, thread_id, body, resolve}` | 204 |

```jsonc
// PRReview
{
  "number": 12540, "title": "…", "author": "…", "url": "…",
  "state": "OPEN", "head_sha": "…", "base_ref": "master",
  "checks": "passing|failing|pending|none", "review_decision": "…",
  "body": "…",                                   // PR description (markdown)
  "files": [{
    "path": "src/…", "old_path": "…", "status": "modified|added|removed|renamed",
    "additions": 0, "deletions": 0, "binary": false,
    "hunks": [{ "header": "@@ … @@",
      "lines": [{ "op": " |+|-", "old_line": 0, "new_line": 0, "text": "…" }] }]
  }],
  "threads": [{
    "id": "PRRT_…", "path": "src/…", "line": 42, "side": "RIGHT|LEFT",
    "is_resolved": false, "diff_hunk": "…",
    "comments": [{ "id": "…", "author": "…", "body": "…", "created_at": "…", "is_mine": false }]
  }]
}
// DraftComment — a pending review comment held in grove until submit.
{ "id": "…", "dir": "…", "pr": 12540, "path": "src/…", "line": 42, "side": "RIGHT", "body": "…", "created_at": "…" }
```

`ai-draft` runs a headless claude in the repo work dir with the diff (and the
target line's hunk or the thread's context) and returns editable suggested
text — the human always reviews/edits before it becomes a draft or reply.
`submit` posts one batch review via `gh api …/pulls/{n}/reviews` (event + body
+ the referenced drafts as `comments[]`, anchors pre-validated) and clears
those drafts. `reply` posts to an existing thread (GraphQL
`addPullRequestReviewThreadReply`, `resolveReviewThread` when `resolve`).
Drafts persist in a `review_drafts` table keyed by (dir, pr).

## Diff content for rich rendering (Pierre)

The UI renders diffs with `@pierre/diffs`, which needs full before/after file
contents (not patches). Both PR review and worktree review supply per file:

```jsonc
// PRReviewFile / WorktreeFile content fields (added to the existing file shape)
{
  "path": "src/…", "old_path": "…", "status": "modified|added|removed|renamed",
  "additions": 0, "deletions": 0, "binary": false,
  "original_content": "…full base file text… (\"\" for added/binary/omitted)",
  "modified_content": "…full head/working file text… (\"\" for removed/binary/omitted)",
  "content_omitted": "" ,   // "" | "binary" | "too_large"
  "hunks": [ … ]            // kept as a fallback; Pierre computes its own diff
}
```

Files over ~512 KB or binary set `content_omitted` and leave the contents empty
(the UI shows a "view on GitHub / open in editor" placeholder).

## Worktree review (`/api/v1/reviews/worktree`)

Review the changes an agent made in a task node's worktree BEFORE opening a PR
or merging — grove's own loop (worktree per task → review → merge/PR). Same
diff+comment surface as PR review, but the diff is local (git) and comments are
local notes that can drive a fix session or a batch PR review later.

| Method & path | Body | Returns |
|---|---|---|
| `GET  /reviews/worktree?node=&repo=` | — | `WorktreeReview` |
| `GET  /reviews/worktree/comments?node=&repo=` | — | `{comments: [WorktreeComment]}` |
| `POST /reviews/worktree/comments` | `{node, repo, path, line, side, body}` | `WorktreeComment` (201) |
| `DELETE /reviews/worktree/comments/{id}` | — | 204 |
| `POST /reviews/worktree/merge` | `{node, repo}` | `{merged, message}` |
| `POST /reviews/worktree/address` | `{node, repo}` | `Session` (201) — starts a session prompted with the comments |

```jsonc
// WorktreeReview
{
  "node_id": "…", "repo": "nethermind", "worktree_path": "…", "branch": "grove/…",
  "base_ref": "master", "has_uncommitted": true,
  "files": [ WorktreeFile ]     // same content-bearing shape as PRReviewFile above
}
// WorktreeComment — a local review note keyed to (node, repo, path, line).
{ "id": "…", "node_id": "…", "repo": "…", "path": "…", "line": 42, "side": "RIGHT", "body": "…", "created_at": "…" }
```

`GET /reviews/worktree` diffs the worktree's working tree against its
merge-base with `base_ref` (all the agent's changes, committed + uncommitted).
`address` composes the comments into a prompt and starts a PTY session on the
node so the agent fixes them; `merge` squash-merges the worktree into its
parent (a clean tree required). `ai-draft` (the PR endpoint) is reused for
worktree comments by passing the worktree path as `dir` and `pr: 0`.

## Repos (`/api/v1/projects/{id}/repos`)

Git repositories registered on a project node. Once a project has repos, new
task nodes under it auto-provision a worktree per repo (branch
`grove/<short8>-<slug>`), which is what makes worktree review, merge-back and
PR-from-task work.

| Method & path | Body | Returns |
|---|---|---|
| `GET  /projects/{id}/repos` | — | `{repos: [Repo]}` |
| `POST /projects/{id}/repos` | `{name?, source_path, default_base?}` | `Repo` (201) |
| `DELETE /repos/{id}` | — | 204 |

```jsonc
// Repo
{ "id": "…", "project_id": "…", "name": "nethermind", "source_path": "/Users/…/nethermind",
  "default_base": "" /* "" = auto-detect origin/HEAD */, "created_at": "…" }
```

`source_path` must be an absolute path to a git repository (validated: exists,
is a git work tree). `name` defaults to the repo directory's basename; it must
be a plain directory name (used as the worktree subdir). Adding a repo only
affects tasks created afterward. `DELETE /repos/{id}` is idempotent (204) and a
**soft delete**: a repo still referenced by task worktrees is tombstoned rather
than dropped, so those worktrees stay intact and reviewable while the repo
disappears from the project's list (its name frees up for reuse).

## WebSocket `/ws/state` (JSON text frames, server-push)

- On connect the server always sends
  `{"t":"hello","rev":N,"nodes":[…],"sessions":[…],"inbox":[Event…]}`.
- Then `{"t":"delta","rev":N,"nodes":[…]?,"sessions":[…]?,"events":[…]?}` — `rev`
  values are consecutive; a client seeing a gap (or the socket closing) reconnects
  and starts from the fresh `hello`.

## WebSocket `/ws/term/{session_id}`

- **Binary frames**: server→client terminal output bytes; client→server keystrokes.
- **Text frames** (JSON control): client sends `{"t":"resize","cols":C,"rows":R}`
  (first message); server sends `{"t":"live"}` once the scrollback replay is done and
  `{"t":"exit","code":N}` when the process ends.
