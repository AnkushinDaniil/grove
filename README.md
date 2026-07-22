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

## Run it like an app

```sh
grove                    # one command: starts the daemon in the background
                         # (if needed) and opens the UI

grove service install    # start the daemon at login (launchd on macOS,
                         # systemd --user on Linux); pairs with the dock icon
```

**Dock icon**: grove ships as an installable PWA — open the UI once, then
*Safari: Share → Add to Dock* or *Chrome: Install grove* from the address bar.
With the login service installed, clicking that icon is the whole story: the
daemon is already running and the app opens instantly.

## Remote access from your phone

grove's daemon binds to `127.0.0.1` only, so by default it's reachable just
from the machine it runs on. [Tailscale](https://tailscale.com) turns that
into a private, authenticated tunnel to your phone with zero grove code:

```sh
tailscale serve 7433
```

This proxies the daemon over your tailnet at `https://<machine>.<tailnet>.ts.net`
with a real, browser-trusted certificate — no grove configuration needed. The
daemon token still gates every request; you're reaching the same
authenticated daemon, just over a different network. The HTTPS origin also
unlocks two things loopback HTTP can't:

- **Push notifications** for attention alerts (permission prompts, questions,
  errors, done) even with the tab closed — `GET/POST /api/v1/push/*`, see
  [docs/API.md](docs/API.md#web-push-apiv1push). Open the UI once over the
  tailnet URL and allow notifications when the browser prompts.
- **Installable PWA** — *Safari: Share → Add to Home Screen* or *Chrome:
  Install app* — so grove opens like a native app.

grove's Host allowlist (a DNS-rebinding guard, `internal/server/middleware.go`)
accepts your tailnet's `*.ts.net` hostname in addition to
`127.0.0.1`/`localhost`, since `tailscale serve` proxies the original Host
header through unchanged. This doesn't reopen the hole the check exists to
close: Tailscale MagicDNS names under `.ts.net` are only ever issued to
devices already admitted to your own tailnet, so nobody outside it can make
an arbitrary `*.ts.net` name resolve to your loopback interface. Keep any
allowlist change this narrow and documented — never a general CORS
relaxation.

### Troubleshooting checklist

- [ ] `tailscale status` shows this machine and your phone on the same
      tailnet, both `active`.
- [ ] `tailscale serve status` shows `7433` proxied over `https`.
- [ ] You're opening the `https://*.ts.net` URL Tailscale printed — not
      `http://127.0.0.1:7433`, which stays loopback-only on purpose.
- [ ] REST calls work (the UI loads, `GET /api/v1/version` responds) but the
      tree/terminal views don't live-update over the tailnet → the WebSocket
      upgrade has its own, separate origin allowlist
      (`internal/ws/ws.go`'s `OriginPatterns`). It accepts `*.ts.net` for the
      same reason the Host allowlist does, so live updates should work over
      `tailscale serve`; if they don't, confirm the browser is actually
      opening `wss://<machine>.<tailnet>.ts.net` (not a mixed-content
      `ws://`) and that the tab's Origin is the tailnet URL.
- [ ] Push subscribing succeeds (`POST /push/subscribe` → 204) but no
      notification ever arrives → confirm the browser actually granted the
      notification permission (check the site's settings), and that the
      daemon (and the machine it runs on) is awake — grove only dispatches
      pushes while `grove serve` is running.

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
