# grove — Design

A local-first **tree-of-agents manager** for AI coding CLIs (Claude Code, Codex CLI,
Gemini CLI, OpenCode, extensible). One Go daemon, an embedded React web UI, and a
recursive tree of orchestrated agent sessions.

## Why

Existing session managers (cmux, Vibe Kanban, claude-squad, Crystal, …) are **flat**:
a list of sessions or a kanban of tasks. Real work is hierarchical: a workspace has
projects, projects have tasks, tasks have subtasks — and a *project* can span several
repositories that change together. grove models exactly that:

- **Tree of nodes** — workspace → project → task → subtask, any depth. One agent
  session per node. Parents run *orchestrator agents* that spawn/track children and
  escalate to the human only when needed.
- **Worktree per task** — every task gets its own git branch + worktree per involved
  repo. Subtask worktrees stack on the parent's branch, so review/merge flows fall out
  of the tree shape naturally.
- **Attention inbox** — hook-first detection of "needs you" moments (permission
  prompts, questions, completion, errors), with native notifications and deep links.
- **Multi-account** — several provider accounts side by side (isolated config dirs)
  with *shared* skills/agents/sessions, and failover when an account hits rate limits.
- **GitHub round-trip** — diffs, PR creation, one-click review runs, review replies.

## Two load-bearing decisions

1. **Orchestrators are event-driven headless turns, not parked processes.** Child
   events accumulate into a digest; the daemon then runs *one* `claude -p --resume
   <orch-session> "<digest>"` turn (grove MCP tools attached) and the process exits.
   Idle nodes cost **zero processes** — a live CLI process idles at 150–400 MB, so this
   is what makes 100-node trees feasible.
2. **Attention detection is hook-first.** grove generates per-session hook wiring
   (Claude `--settings`, Codex `notify`) that calls `grove hook …`, a tiny subcommand
   POSTing the hook JSON to the daemon. Works identically for PTY and headless runs; no
   TUI escape-code scraping. PTY quiescence heuristics exist only as a fallback for
   CLIs without hooks.

## Architecture

```
┌─────────────────────────── grove daemon (Go) ────────────────────────────┐
│  api (REST) ── ws (state hub + per-terminal sockets) ── mcpserv (/mcp)   │
│        │                    │                                │           │
│      tree  ◄──────────  session  ──────────►  driver registry            │
│   (state actor)      (spawn/pump/budget)   claude│codex│gemini│opencode  │
│        │                    │                                            │
│      store            term (PTY, ring,        worktree + gitcli          │
│    (SQLite)            scrollback)          (per-task, multi-repo)       │
│                                                                          │
│  orch (digest → wake turns)   profile (accounts)   github   notify       │
└──────────────────────────────────────────────────────────────────────────┘
                   ▲ embedded React + xterm.js web UI (go:embed)
```

- **`internal/tree`** — the single serialized writer for tree state. Persist-then-apply
  mutations, monotonic `rev`, delta broadcast to bounded subscribers (slow consumers
  are dropped and re-snapshot). Parent rollups and driver/profile inheritance are
  derived on demand, never stored.
- **`internal/driver`** — pure per-CLI adapters: argv/env/file construction, native
  JSONL → normalized events (`session_started / text / tool_call / tool_result /
  awaiting_input / turn_done / session_ended / error / usage`), capability flags.
  Table-tested against recorded fixtures; no process handling.
- **`internal/session`** — owns processes: spawn/resume/stop with process groups,
  headless stdout pump → parser → tree, hook ingestion, `max_running_sessions` budget
  (user-initiated beats orchestrator-spawned; idle nodes have no process at all).
- **`internal/term`** — PTY lifecycle (creack/pty behind a small interface), 512 KiB
  ring buffer, atomic replay-then-live attach, scrollback persisted to disk so a daemon
  restart can replay history and offer `--resume`.
- **`internal/worktree` + `internal/gitcli`** — task workspaces under
  `~/.grove/worktrees/<short8>-<slug>/` with one worktree per repo on a shared branch
  `grove/<short8>-<slug>`; stacked bases from the parent task; dirty/unmerged
  detection marks worktrees *orphaned* (never silently deleted); squash merge-back.
- **`internal/store`** — SQLite (pure-Go driver) with embedded migrations; WAL,
  single-writer via tree; append-only events double as audit log and inbox.
- **`internal/mcpserv` + `internal/orch`** — the daemon is an MCP server; orchestrator
  turns get per-node bearer tokens scoping them to their subtree. Tools:
  `grove_create_task`, `grove_list_children`, `grove_get_node`, `grove_send_prompt`,
  `grove_merge_child`, `grove_report(summary, needs_user)`.
- **`internal/profile`** — one config dir per account (`CLAUDE_CONFIG_DIR`,
  `CODEX_HOME`); shared context (`skills/`, `agents/`, `commands/`, `plugins/`,
  `projects/`) symlinked from `~/.grove/shared`; spawned env is scrubbed
  (`ANTHROPIC_API_KEY` would silently bypass account isolation). Credentials are never
  read or copied — on macOS the keychain entry is keyed by the config dir path, and
  refresh tokens rotate, so snapshot/swap schemes are explicitly rejected.

## Realtime protocol

- REST `/api/v1` for all mutations; one JSON WebSocket `/ws/state` for server-pushed
  tree/session/event deltas (`rev`-gapped clients refetch the snapshot); one **binary**
  WebSocket per *attached* terminal (`/ws/term/{session}`) — backpressure isolation so
  a busy terminal can never starve state updates.
- Auth: random token in `~/.grove/token` → HttpOnly SameSite=Strict cookie, Origin
  allowlist, CSRF header on mutations, daemon binds `127.0.0.1` (remote access =
  Tailscale, same token).

## Performance economy

The daemon is cheap (idle node = a row + a struct; target <40 MiB RSS at 100 idle
nodes). The expensive resource is child CLI processes — bounded by the session budget.
The expensive UI resource is live terminals — the UI mounts xterm.js only for visible
terminals (LRU pool) and enables the WebGL renderer only on the focused one.

## Testing

`internal/testutil/fakeagent` is a scripted fake CLI (emit JSONL, wait for stdin,
misbehave on demand) registered as a driver — the whole session/term/tree/API stack is
integration-tested with zero real CLI spend. Drivers are golden-tested against
recorded fixtures. Worktree/gitcli run against throwaway git repos. Everything runs
with `-race`; core packages target ≥80% coverage.

## Roadmap

- **M1 (usable single-player)**: tree + Claude sessions in PTY/headless + terminals +
  worktrees + web UI.
- **M2**: Codex/Gemini/OpenCode drivers, orchestrator loop + MCP, profiles +
  multi-account failover, GitHub PR flow, macOS notifications.
- **M3**: review round-trips, inbox polish, packaging (Homebrew tap), Tailscale docs,
  Windows/ConPTY.
