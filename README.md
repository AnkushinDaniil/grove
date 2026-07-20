# grove

**A tree-of-agents manager for AI coding CLIs.**

grove orchestrates Claude Code, Codex CLI, Gemini CLI, OpenCode and other terminal coding
agents as a **recursive tree**: workspace → projects → tasks → subtasks, to any depth.
One agent session per node. Parent nodes run *orchestrator agents* that spawn children,
track their progress, and escalate to you only when needed.

Think of it as a very advanced tmux for agent fleets:

- **Tree orchestration** — parents aggregate child status; orchestrators are event-driven
  headless turns (idle nodes cost zero processes).
- **Worktree per task** — every task gets its own git worktree (and branch) per repo;
  projects may span multiple repositories; subtask worktrees stack on their parent's branch.
- **Attention inbox** — hook-first detection of "needs you" moments (permission prompts,
  questions, completions, errors) with native notifications and deep links.
- **Multi-account** — run several provider accounts side by side with shared skills,
  agents, and resumable sessions; fail over when one account hits its rate limit.
- **GitHub round-trip** — diffs, PR creation, one-click review runs, review-comment replies.
- **Local-first** — a single Go daemon with an embedded web UI. Your data stays on your machine.

## Status

**Beta.** The single-player core works: tree + sessions + terminals + worktrees +
web UI. Orchestrator agents, multi-account failover, GitHub round-trip, MemPalace
memory and the mobile PWA are landing next — see the
[design](docs/DESIGN.md) and [orchestration spec](docs/ORCHESTRATION.md).

## Install

**Recommended — release archive** (includes the embedded web UI):

```sh
# macOS (Apple Silicon); see Releases for darwin/linux × arm64/amd64
# (tag-pinned URL: GitHub's "latest" alias skips prereleases)
curl -L https://github.com/AnkushinDaniil/grove/releases/download/v0.1.0-beta.1/grove_0.1.0-beta.1_darwin_arm64.tar.gz | tar xz
./grove serve   # daemon on http://127.0.0.1:7433
./grove open    # opens the web UI with your auth token
```

`go install github.com/AnkushinDaniil/grove/cmd/grove@latest` also works but
produces a **UI-less** daemon (API only — web assets can't ship in the Go
module); build with the UI from source via `make build-release`.

Upgrade: download the newer archive (Homebrew tap coming).

## Architecture at a glance

```
┌─────────────────────────── grove daemon (Go) ────────────────────────────┐
│  REST + WS state hub + per-terminal WS      MCP server (orchestrator     │
│  ┌────────┐  ┌─────────┐  ┌──────────┐      tools, per-node tokens)      │
│  │  tree  │←→│ session │←→│  driver  │→ claude / codex / gemini / ...    │
│  │ actor  │  │ manager │  │ registry │      (PTY or headless stream)     │
│  └────────┘  └─────────┘  └──────────┘                                   │
│   SQLite      PTY + ring    worktree engine (per-task, multi-repo)       │
└──────────────────────────────────────────────────────────────────────────┘
                     ↑ embedded React + xterm.js web UI
```

## Development

```sh
make test    # go test -race ./...
make build   # build the daemon (without embedded UI)
make lint    # golangci-lint
```

## License

Apache-2.0
