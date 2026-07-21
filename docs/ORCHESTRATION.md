# grove — Orchestration & Integrations Specification (M2)

Companion to [DESIGN.md](DESIGN.md) (M1 core) and the frozen [API.md](API.md).
This is the implementation spec for the orchestration layer, multi-account
profiles, GitHub round-trip, attention system, and security model. Where this
doc and shipped M1 code disagree on mechanics (e.g. realtime = WebSocket per
API.md, daemon TCP port 7433), the shipped contract wins; new tables and node
columns land as migration `0002_orchestration.sql`.

## Decisions

| # | Decision |
|---|---|
| D1 | MCP transport: a stdio shim (`grove mcp`) inside every spawned CLI, bridging over a Unix socket `~/.grove/daemon.sock` (0600). Node identity = per-node bearer token; the only TCP listener is the web UI on `127.0.0.1:7433`. |
| D2 | Spawn is async-only. **No blocking `grove_wait` tool.** Orchestrators end their turn after delegating; the daemon wakes them by injecting event batches. |
| D3 | Headless orchestrator processes are ephemeral (reaped after 15 min idle, resumed on demand). Durable state = transcript + grove DB. Idle costs zero processes. |
| D4 | Status sources are prioritized: hooks/JSON streams (3) > transcript tail (2) > PTY heuristics (1). Lower-priority sources only override readings older than 10 s. |
| D5 | Hook wiring via generated per-node settings passed with `--settings`; user settings files are never modified (settings layers are additive — spike S3). |
| D6 | Progress = free-text summary + checklist (auto-fed from TodoWrite); parent percent = done leaves / total leaves. No invented percentages. |
| D7 | Notifications: `terminal-notifier` with `-open` deep link `http://127.0.0.1:7433/n/<id>`; `osascript` fallback. No custom URL scheme in v1. |
| D8 | Profile `default` adopts the user's existing `~/.claude` untouched. Shared context is an **opt-in** migration to `~/.grove/shared/claude/*` + symlinks. Credentials/state always stay per-profile. |
| D9 | Mid-task account handoff: graceful stop → `claude --resume <sid>` under the other profile, **no** `--fork-session` (one transcript, sequential use). Rate-limit failover default = ask (one click). |
| D10 | GitHub exclusively via `gh` CLI exec (reuses the user's auth incl. enterprise/SSO; grove stores zero GitHub credentials). `go-gitdiff` parses local diffs. |
| D11 | Code review = read-only child node submitting structured findings via MCP; user cherry-picks; posted as ONE batch PR review. Comments polled via a single GraphQL query per PR (60 s active / 300 s background). |
| D12 | Headless permission gating: grove's MCP tool is wired as `--permission-prompt-tool`, routing permission requests to the grove inbox — the one sanctioned blocking tool (Claude blocks on it by design). |

## 1. MCP protocol

**Identity.** At spawn the daemon mints a 32-byte node token (stored SHA-256-hashed,
capability- and subtree-scoped, revoked at terminal state). The CLI gets env
`GROVE_NODE_ID` / `GROVE_NODE_TOKEN` / `GROVE_SOCKET`; `grove mcp` forwards JSON-RPC
over the UDS with the token attached. **Tools never take a "self" parameter — identity
is implicit** (kills sibling spoofing). Tokens travel via env and 0600 per-node
`mcp.json` only — never argv, logs, or plaintext DB.

**Mounting per driver:** Claude `--mcp-config ~/.grove/nodes/<id>/mcp.json`; Codex
`-c 'mcp_servers.grove.command="grove"' -c 'mcp_servers.grove.args=["mcp"]'`; Gemini
`.gemini/settings.json` and OpenCode `opencode.json` written into the node's worktree
(kept out of commits via `.git/info/exclude`).

**External orchestrators** (any CLI the user runs by hand): `claude mcp add grove --
grove mcp` — with no node token the shim uses the user token; the first call
auto-registers an `external` node under the root. External nodes can't be woken, so
their briefing sanctions explicit status polling.

**Tool catalog** (capability sets: worker / orchestrator / review / reply):

- All: `grove_report_progress{summary, checklist[], percent}`,
  `grove_raise_attention{kind: question|decision|blocked|review|error, message, options[]}`,
  `grove_complete{result: done|failed, summary, artifacts[], force}`,
  `grove_get_context{}` (re-orientation after compaction),
  `grove_send_message{node_id, text, priority}` (parent only for workers).
- Orchestrator adds: `grove_spawn_child{title, prompt, role, mode, driver, profile,
  repos[], permission_mode, limits, deliver_result}` → returns immediately
  `{node_id, status:"spawning"}`; `grove_list_children`, `grove_node_status`
  (rollups, progress, PR state), `grove_cancel_child`, messaging own children.
- Review adds `grove_submit_findings{summary, findings[{path,line,end_line,side,severity,title,body,suggestion}]}`;
  reply adds `grove_submit_replies{replies[{thread_id,body,resolve}]}`.
- Hidden: `grove_permission_request` — wired as `--permission-prompt-tool` for
  headless Claude; blocks until the UI answers (60-min timeout → deny); daemon
  auto-answers from per-subtree permission rules before bothering the user.

**Anti-poll guard:** >5 status calls in 60 s appends a hint: *"Stop polling. End your
turn; grove will wake you when children report."*

## 2. Event wake

Per-node queue, 2 s debounce, delivered as one batch:

| Target | Mechanism |
|---|---|
| Claude headless, alive | write a user message into the live `--input-format stream-json` stdin |
| Claude headless, reaped | respawn `claude -p --resume <sid> …` (session id refreshed from init event), then write the batch |
| Codex | `codex exec resume <sid> "<batch>"` |
| Gemini | `gemini --resume <sid> -p "<batch>" --output-format json` |
| OpenCode | `opencode run --session <sid> "<batch>"` (S6) |
| Interactive PTY | bracketed-paste + Enter, only when idle >2 s and no user keystrokes for 3 s; else queue + UI badge |

Wake format: a `<grove-events count=N>[…json…]</grove-events>` block + one
instruction line. Event types: `child_completed/child_failed/child_attention/message/
user_note/review_comments/budget_warning/child_killed`.

**Briefing layers** (most durable first): (1) MCP server `instructions` + tool
descriptions carry the protocol rules — survives compaction on every driver;
(2) node-context template via `--append-system-prompt` (Claude) or first-message
header (others): identity, tree path, worktrees, limits, ORCHESTRATOR vs WORKER
rules ("spawn async → end turn → you'll be woken; never poll; prefer few well-briefed
children; do trivial work yourself"); (3) first user message = the task prompt.

**Limits** (inherited-and-clamped, cgroups-style): max_depth 5, max_children 12,
max_descendants 40, concurrent active leaves per workspace 6 (excess queue FIFO —
backpressure, not failure), spawn rate 30/subtree/hour (breach → `runaway` attention),
optional token_budget per subtree (breach → spawns denied + `budget_warning`).
Pause subtree = stop wakes/spawns, running turns finish; kill = cascade
SIGTERM→SIGKILL bottom-up, tokens revoked.

## 3. Status pipeline

Normalized: `spawning → working ⇄ awaiting_permission|awaiting_input; → idle;
→ done|failed|killed; paused`. `done` requires explicit `grove_complete` — never
inferred from idleness. Store per node `(status, source, confidence, since)`.

Detection matrix: Claude headless = stream-json + hooks + `--permission-prompt-tool`
(D12 closes the invisible-permission gap); Claude PTY = the same generated
`--settings` hooks (Notification→awaiting_*, Stop→idle, SessionStart/End) — the user
answering in the terminal auto-resolves the inbox item; Codex = `exec --json` stream +
`notify` hook (turn-end); Gemini = process lifecycle + chats-dir session-id recovery
(issue #14435); OpenCode = run lifecycle + PTY heuristics (`opencode serve` SSE as a
v1.1 upgrade). PTY heuristics (300 ms quiescence, ANSI-stripped tail): permission
regex → awaiting_permission; trailing `?` → awaiting_input; driver prompt glyphs →
idle; spinner/`esc to interrupt` → working; >45 s silence → idle. All conf 1.

`grove hook` contract: ≤1 MiB stdin JSON → envelope {token, event, payload, ts} →
UDS POST, 3 s timeout → **always exit 0, empty stdout** (a broken grove must never
break the user's CLI). Daemon down → append `~/.grove/spool/<node>.jsonl`, replayed
at startup. Generated settings register SessionStart/UserPromptSubmit/PreToolUse/
PostToolUse/Notification/Stop/SubagentStop/SessionEnd (PostToolUse[TodoWrite] feeds
the checklist). Codex: `notify = ["grove","hook","--event","codex-notify"]` in the
grove-managed per-profile config.toml.

**Rollup** (O(depth) per event, cached, pushed as WS deltas): severity failed=6 >
awaiting_permission=5 > awaiting_input=4 > working=3 > spawning=2 > idle=1 > done=0;
counts summed; attention = own open items + Σ children; percent = leaves_done/leaves_total.

## 4. Attention / inbox

Kinds: permission, question, review_ready, done (only for user-spawned nodes),
failed, rate_limited, review_comments, checks_failed, budget/runaway. Lifecycle
`raised → notified → seen → resolved(auto|user_input|dismissed)`; dedup one open item
per (node, kind); re-notify ≤1/30 s per node. Auto-resolve is the default path
(answering in the terminal clears the item). Dispatch: coalescer (global cap 6/min →
digest) → terminal-notifier (Homebrew dependency) with `-open` deep link, osascript
fallback; doctor warns when click-through is unavailable.

## 5. Multi-account profiles

Profile = {name, family, dir, color, failover_priority, extra_env, cooldown_until}.
Claude → `CLAUDE_CONFIG_DIR`; Codex → `CODEX_HOME`; `default` adopts `~/.claude`
untouched (zero-migration first run).

**Shared context (opt-in `grove profile share enable`):** symlink `skills/ agents/
commands/ plugins/ projects/ todos/ CLAUDE.md` → `~/.grove/shared/claude/…`.
`projects/` is load-bearing: shared transcripts ⇒ `--resume` across accounts (spike
S1). Never shared: `.credentials.json`/Keychain, `.claude.json`, `settings.json`,
`statsig/`, caches. Migration: refuse while nodes run → rsync-seed shared → merge
(conflicts preserved) → `mv *.pre-grove.bak` → symlink → doctor verify; rollback =
restore backups. Profile doctor: symlink integrity, keychain presence (service keyed
by config-dir hash — S2 pins the exact name), `ANTHROPIC_API_KEY` in settings env
flagged as an error, oauthAccount email shown (catches same-account profiles),
version check, shared-projects size, clean daemon env.

**Handoff (b) kill→resume under B:** wait for Stop (≤60 s) or interrupt → SIGTERM →
transcript mtime quiet ≥1 s → spawn under B with `--resume <sid>` + regenerated
settings/mcp (same node token) → record new session id. S1 NO-GO fallback: cold
handoff (new session + grove-composed context pack: briefing, progress, last N turns,
git status). **(c) rate-limit failover:** detection via result-error/text regex +
transcript reset-time lines → `cooldown_until`; policy `ask (default)|auto|off`;
auto has a 1-per-node-per-hour flap guard.

**Usage meters:** transcript tailer extracts per-message usage/model → `usage` table
(sessions grove didn't spawn → `untracked`); ccusage-style rolling 5 h + weekly bars
per profile; pure local JSONL aggregation with byte-offset checkpoints.

## 6. GitHub

Per-repo config auto-detected (`gh repo view --json nameWithOwner,defaultBranchRef`).
(a) **Diff**: throttled fetch → merge-base → committed + uncommitted + untracked
sections → `go-gitdiff` → JSON hunks (files >100 KB lazy-loaded). (b) **PR**: ensure
branch → commit (message required; agent-drafted default) → push → `gh pr create`
with a body template (summary, task-tree context, checklist as checkboxes, test
plan); no AI attribution by default. (c) **Trigger review**: child node, read-only
enforcement = `--allowedTools "Read,Grep,Glob,Bash(git diff:*),Bash(git log:*),
Bash(git show:*),Bash(gh pr view:*),Bash(gh pr diff:*)"` + review-only MCP caps →
`grove_submit_findings` → UI cherry-pick → ONE batch `gh api …/pulls/{n}/reviews`
with anchored comments (anchors pre-validated against `gh pr diff`; unanchorable →
body bullets). (d) **Round-trip**: every PR attached to a node is watched — a node may carry
SEVERAL PRs (multi-repo tasks open one per repo; all listed in `node.meta.prs`,
each polled). Per-PR GraphQL poll (reviewThreads, reviews, statusCheckRollup,
mergeable; 60 s focused / 300 s background) diffs state and raises attention on the
owning node for anything that needs the user: new non-self review **comments**
(grouped `review_comments`), a submitted **review** (`CHANGES_REQUESTED` →
attention with the review body; `APPROVED` → done-flavored info item),
**re-review requested** on the user, mergeability turning blocked, and PR
**merged/closed** (info, auto-clears other PR items). "Draft replies" spawns a
reply node → user approves each → GraphQL `addPullRequestReviewThreadReply`
(+ resolve). Per-thread "Fix it" spawns a worker on the same worktree — for
CHANGES_REQUESTED the one-click action is "Address review" (worker briefed with
the review + threads on the task's existing worktree). (e) **Checks**: rollup badge; fail →
`checks_failed` attention.

**(f) Review radar — a standing per-repo review queue (dedicated UI tab).** For any
repo the user enables it on, a daemon watcher polls open PRs (one `gh api graphql`
search per repo per interval: PRs + my review state + headRefOid + latest review
commit + unresolved threads + updatedAt + checks) and classifies into buckets:

1. **Needs first review** — open PRs with no review from me (configurable filters:
   skip drafts, skip my own PRs, author allow/deny list, base branch).
2. **Re-review** — I reviewed, but commits landed after my last review
   (headRefOid ≠ my last reviewed commit).
3. **Awaiting my reply** — unresolved review threads whose last comment is not mine
   or that mention me.
4. *(optional toggle)* **My PRs with feedback** — my open PRs with change requests
   or unresolved threads to address.

State lives in a `pr_radar` table (repo, pr, bucket, head_oid, my_last_review_oid,
last_seen, snoozed_until); poll diffs produce deduplicated attention items (kind
`review_pending`) and a badge count on the Reviews tab. Each row offers: **Review in
grove** (spawns the existing read-only review child node → findings → cherry-pick →
one batch post — flow (c)), **Draft replies** (flow (d)), **Open on GitHub**,
**Snooze**. Review sessions are ordinary tree nodes under the project, so their
progress and attention ride the normal machinery. Poll interval: 120 s default per
repo, backing off to 600 s when the tab is unfocused; all requests via `gh` (zero
stored credentials). API draft: `GET /api/v1/reviews` → buckets per repo;
`POST /api/v1/reviews/{repo}/{pr}/snooze {until}`; `POST /api/v1/reviews/{repo}/{pr}/review`
(spawn review node).

## 7. Security

UDS (0600) for hooks/MCP/CLI; TCP `127.0.0.1:7433` for the UI only. Web auth: token
→ HttpOnly SameSite=Strict cookie + `X-Grove-CSRF` on mutations. DNS-rebinding
defense: Host ∈ {127.0.0.1, localhost}, Origin validated, **no CORS headers**.
Spawn env allowlist (default-deny): PATH HOME USER SHELL TERM LANG LC_* TMPDIR +
grove/profile vars; always scrubbed: `ANTHROPIC_API_KEY` (silently hijacks
subscription auth — correctness, not just hygiene), `ANTHROPIC_AUTH_TOKEN`,
`OPENAI_API_KEY`, `GEMINI_API_KEY` (unless profile-declared), `GH_TOKEN`/
`GITHUB_TOKEN`, `AWS_*`. Log/DB redaction regexes for common secret shapes;
transcripts never copied into the DB. Threat model: defends against other local
users, browser-origin attacks, accidental secret leakage; NOT against same-user
malware (same posture as the CLIs).

## 8. Memory: MemPalace (M3)

MemPalace is **the** memory backend (no parallel grove-native store). Tree-scoped,
cross-driver memory: one knowledge base shared by claude, codex and gemini workers.
Mapping: workspace ↔ palace, project node ↔ Wing, task subtree ↔ Rooms; room/entry
metadata carries grove node IDs so tree scopes (self | subtree | ancestors | project)
translate into palace filters. Scope enforcement rides the existing subtree-scoped
node tokens.

**Zero-touch lifecycle (hard requirements):**
- *Bootstrap from scratch:* daemon startup + `grove doctor` detect the MemPalace
  installation; if absent, grove installs it automatically (detect install channel —
  npm/uvx/brew — pin a known-good version, log visibly) and initializes the palace:
  Wing created on project creation, Rooms lazily per task subtree. A missing palace
  is never an error the user has to fix by hand.
- *Auto-update:* daily version check against the pinned channel; update applied on
  daemon restart with a changelog line in the daemon log; palace data migrations are
  MemPalace's own, grove just gates on a post-update health probe.
- *Health & degradation:* doctor probes the MCP server (spawn, list_tools, roundtrip
  write/read in a scratch room). If MemPalace is down mid-flight, writes spool to
  `~/.grove/spool/memory.jsonl` and replay on recovery; reads degrade to
  last-injected briefing digests. One attention item, not silent loss.

**Active use without explicit requests — the daemon is a MemPalace MCP client:**
1. *Recall injection at spawn/wake:* before composing a briefing, the daemon queries
   MemPalace (node-scoped: own room + subtree + ancestor wings) and injects a
   bounded "## Memory" section (top-K relevant entries) — the agent benefits even if
   it never calls a tool.
2. *Auto-capture:* on `turn_done`, `grove_complete` and resolved feedback items, the
   daemon writes distilled entries (progress summaries, decisions, fix outcomes) into
   the node's room automatically, attributed `source=auto`. On task archive, key room
   content is distilled one level up (knowledge compacts toward the Wing).
3. *Curated agent tools:* sessions get a curated proxy subset (memory_write,
   memory_search, memory_digest → mapped onto MemPalace's native tools) instead of
   all 34 — no tool-list bloat; briefings mandate "search memory before starting,
   record learnings at milestones".

**Status — phase 1 (`grove memory`, implemented):** The zero-touch lifecycle
(detect → install → init → health-probe) lands in `internal/memory` behind
`grove memory <install|doctor|status>`. Confirmed ground truth (2026-07-21):
MemPalace ships on **PyPI** as `mempalace` (console entry point `mempalace`,
pinned `3.6.0`); the MCP server launches as `mempalace mcp` speaking
newline-delimited JSON-RPC; the health probe is `initialize` → `tools/list`
(the `mempalace_add_drawer`/`mempalace_search` write/read roundtrip is
intentionally skipped so grove never writes into a live palace); data lives in
`~/.mempalace` (marker `config.json`); `mempalace init` seeds it. Install channel
preference is **`uv` → `pipx` → `pip`** — not npm; the "npm/uvx/brew" sketch above
predated confirming the PyPI distribution. The daemon-as-MCP-client behaviors
(recall injection, auto-capture, curated proxy tools) remain **phase 2**.

## 9. Spikes (gating M2 work)

- **S1** cross-profile `--resume` with shared `projects/` (blocks the multi-account headline; fallback: cold handoff).
- **S2** keychain behavior with two config dirs under 24 h concurrent soak (fallback: per-profile spawn mutex).
- **S3** `--settings` hook layering additivity + Notification/SessionEnd reliability incl. SIGTERM (fallback: grove merges user hooks into its generated file; daemon waitpid stays authoritative).
- **S4** stream-json wake loop: post-`result` user injection, resume id continuity, `--permission-prompt-tool` contract (fallback: wake-by-resume always; permissions via allowedTools+plan).
- **S5** PTY bracketed-paste reliability at idle (fallback: interactive orchestrators get UI-banner delivery).
- **S6** Codex/Gemini/OpenCode capability audit (fallback: per-driver feature flags; no-resume drivers become one-shot workers).
- **S7** batch review comment anchoring on tricky diffs (fallback: body bullets).
- **S8** terminal-notifier click-through UX (fallback: osascript, inbox-only links).

S1–S4 block the spine; S5–S8 have designed fallbacks.
