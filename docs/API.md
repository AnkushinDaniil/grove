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
| `POST /nodes` | `{parent_id, kind, title, brief?, driver?, profile_id?}` | `Node` (201) |
| `PATCH /nodes/{id}` | `{title?, brief?, driver?, profile_id?, meta?}` | `Node` |
| `POST /nodes/{id}/archive` | — | `{archived: [id…]}` |
| `POST /nodes/{id}/ack` | — | `Node` |
| `POST /nodes/{id}/sessions` | `{mode, prompt?, resume_id?}` | `Session` (201) |
| `POST /nodes/{id}/prompt` | `{text}` | 204 |
| `POST /sessions/{id}/stop` | — | 204 |
| `GET  /nodes/{id}/events?after=<event_id>&limit=<n>` | — | `Event[]` (ascending by id) |
| `GET  /inbox` | — | `Event[]` (unacked attention, newest first) |
| `GET  /version` | — | `{version, commit}` |
| `POST /auth/session` | `{token}` | 204 + cookie |
| `GET  /auth/me` | — | 204 or 401 |
| `POST /internal/hook?node=&driver=&event=` | raw hook JSON | 204 (auth: `X-Grove-Hook-Token`) |

Errors: `{"error": "human readable message"}` with 4xx/5xx. Validation failures → 400,
unknown ids → 404, auth → 401.

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
