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
  non-empty value must be an existing absolute directory → else 400; `PATCH` with an explicit
  empty string clears the override (falls back to inheritance).
- `attention: "review"` intentionally has no M1 event type — it is
  forward-declared for the M2 GitHub review round-trip and until then appears only
  on nodes, never as an inbox event.

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

The daemon aggregates usage locally from session transcripts (no network calls);
until the aggregator lands the endpoint returns `{"profiles": []}`.

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
