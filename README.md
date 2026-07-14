# aitop

**This `aitop`:** a live, read-only monitor of your AI coding *agents and their contexts* — Claude Code, Codex CLI, Cursor, cursor-agent CLI, opencode — one card per session, showing what it's doing, which model is driving it, how full its context window is, and its token burn, all in one terminal pane. System resources (CPU/MEM/NET) are a condensed footnote at the bottom, not the headline. If you were looking for a pure cost/token tracker, that's a different `aitop` ([bugkill3r/aitop](https://github.com/bugkill3r/aitop) or [samusgray/aitop](https://github.com/samusgray/aitop)) — this one is a context board for your agents, `btop`-styled, not a spend dashboard.

![aitop demo](demo.gif)

## What it does

- **A card per agent session, always expanded.** Tool identity (border color) and, when the adapter knows it, the model actually driving that session — `claude code (opus 4.8)`, `opencode (deepseek v4 flash free)` — a title synthesized from the session's own first request (or Claude Code's own auto-generated title, when there is one), the state badge, the last action taken (tool call or message, word-wrapped), a `Context: [bar] 234k/1000k (23%)` line — a real reading, never a percentage guessed from nothing — and its 5h/7d rate-limit + process detail underneath, across every Claude Code, Codex, Cursor, cursor-agent, and opencode session running on your machine right now.
- **Per-session, not per-tool.** Two Claude Code sessions running side by side get their own token counts and context bars — they never mix, and a card for a dead session never lingers as if it were still running.
- **Multi-tool from day one.** Claude Code, Codex CLI, Cursor, cursor-agent CLI, and opencode — Cursor and cursor-agent in particular aren't covered by any other "aitop"-named project as of this writing.
- **Resources, condensed.** A short footer shows real aggregate CPU/MEM/NET (via [gopsutil](https://github.com/shirou/gopsutil)); the MEM bar is segmented to show how much of that is attributable to your AI-tool processes specifically — present, but deliberately secondary to the cards above it.
- **Read-only.** No approve/reply/merge actions (that's [agent-dashboard](https://github.com/bjornjee/agent-dashboard)'s job, and it requires tmux — aitop doesn't). aitop only ever observes.
- **Honest about gaps.** Missing data renders as `—`, never a fabricated `0`. Cursor has no local cost data and aitop says so instead of guessing; the same goes for any field a given adapter hasn't populated yet.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/grippado/aitop/main/install.sh | sh
```

```sh
go install github.com/grippado/aitop/cmd/aitop@latest
```

Homebrew tap (`brew install grippado/aitop/aitop`) is planned but not live yet — it needs a separate `grippado/homebrew-aitop` repo this release doesn't publish to.

## Usage

```sh
aitop                    # live TUI, 2s refresh
aitop --once             # one text snapshot, then exit
aitop --once --json      # machine-readable snapshot
aitop --serve            # read-only browser dashboard at http://127.0.0.1:8787
aitop --serve --addr :9000   # bind a different host:port (localhost by default)
aitop --refresh 5s       # override the tick interval
aitop --demo             # synthetic agent cards, no real tool required
```

### Browser view — `aitop serve`

`aitop serve` renders the **same** board in a browser: one card per session,
same fields, same btop palette, same `—`-for-missing honesty as the TUI — it's a
read-only consumer of the very same `Snapshot`, not a second data path. It only
*adds* what a terminal can't do well: a list/grid layout toggle (list gives each
card the full width so the last action reads in full — grid packs columns, like
the TUI's `v`), tool/sort dropdowns, per-session context/token sparklines, a
raw-JSON inspector, and a copy-to-clipboard chip that
hands you the right `/aitop-enhance` command when a card shows a fillable `—`
(you run it in Claude Code — aitop never executes anything; it stays read-only).
It serves over stdlib `net/http` with the SPA embedded in the binary (no CDN, no
build step), binds `127.0.0.1` by default, and the TUI and web view are kept in
lockstep by design — see [RFC 0004](./docs/rfcs/0004-aitop-web-view.md).

### Keybindings

| Key | Action |
|-----|--------|
| `q` / `Ctrl+C` | quit |
| `j`/`k`, arrows | move selection |
| `f` | filter cards by tool |
| `o` | cycle sort column (context / tokens / age / tool) |
| `v` | toggle list/grid card layout |
| `space` | pause/resume refresh |
| `r` | force refresh |
| `?` | help |

## Why it exists

Every existing "aitop"-named project (and the excellent but tmux-bound `agent-dashboard`) either tracks cost/tokens as the headline, requires tmux, or covers Claude Code alone. None of them present agent sessions as a live, glanceable board the way a mutirão of open threads deserves, and none of them support Cursor or cursor-agent. aitop's whole reason to exist is that gap: a real-time context board for the agents actually running on your machine, with resource usage as a footnote instead of the main event.

## What's supported today

| Tool | Process/CPU/MEM | Sessions | Title / last action | Model | Context% / tokens | Rate limits |
|------|---|---|---|---|---|---|
| Claude Code | ✅ | ✅ | ✅ (own auto-title + transcript) | ✅ (own transcript) | ✅ (per-session, own transcript) | ✅ (5h/7d, via `ccstatusline`'s cache) |
| Codex CLI | ✅ | ✅ | ✅ (synthesized from first request + transcript) | ✅ (own rollout's turn_context) | ✅ (per-session; Codex's own rollout gives the true model context window, no guessing) | Not available (no local rate-limit source) |
| Cursor (IDE) | ✅ (Cursor's own telemetry) | ✅ | ✅ (real title + last message/tool call, from Cursor's own `state.vscdb`) | Not available | Tokens: ✅ (per-composer, same source) · Context%: not available (no window-size reading found) | Not available (proprietary/cloud-side) |
| cursor-agent (CLI) | ✅ | ✅ | ✅ (first request + own transcript) | ✅ (currently selected model, from cursor-agent's own config) | Not available (no per-turn usage in its transcript format) | Not available |
| opencode | ✅ | ✅ | ✅ (opencode's own session title) | ✅ (own models catalog cache gives the real display name) | ✅ (per-session, straight from opencode's own SQLite state; real context window from its models catalog) | Not available (its own cost figure is lifetime-cumulative, not day/month-scoped — see the adapter's own doc comment) |
| Other AI CLIs (aider, windsurf, ...) | Best-effort via process-name match | — | — | — | — | — |

Session lineage — the `[bg]`/`[interactive]` kind badge and the `▸ spawned by <parent>` nesting that indents a spawned child under its parent in list view — is **Claude Code only** today: it comes from Claude Code's own `kind` tag plus a bounded process-tree walk to the first ancestor that is itself a tracked session. The other adapters leave both blank until they expose a real source for them — no fake badge, no guessed parent (hence no extra ✅ column above).

Cost-in-dollars was dropped from the expanded card view: on this machine's real data the cost-day/cost-month files aitop originally read haven't been written in weeks, so it never showed anything but a fake `$0.00`. Fields still unpopulated by any adapter (git branch/dirty state) render as `—` on real data today; `--demo` shows what the card looks like once those land.

v2 roadmap: dedicated adapters for aider/windsurf, an external plugin mechanism, branch/dirty detection, a local trend history, and community-contributed themes (see [CONTRIBUTING.md](./CONTRIBUTING.md)).

## Build on it, learn from it

aitop is small on purpose, and it's meant to be a base, not just a tool:

- **Build a product on its data.** `aitop --once --json` emits a structured,
  honest, per-session snapshot. Poll it, chart it, alert on it — or fork it into
  something that *acts* (harness control). See [Built with
  aitop](./docs/BUILT-WITH-AITOP.md), and add what you built (*from aitop, …*).
- **Learn agentic orchestration from it.** The repo carries a deliberately rich
  AI-agent contribution structure (`.claude/`) as a readable, runnable worked
  example — see the [learning guide](./docs/agentic-orchestration.md) and
  [RFC 0001](./docs/rfcs/0001-agentic-contribution-architecture.md) /
  [RFC 0002](./docs/rfcs/0002-evolving-agentic-structures.md).
- **Contribute in minutes.** A new theme, a new tool adapter, or a fix — the
  agent commands (`/aitop-theme`, `/aitop-adapter`, `/aitop-enhance`) walk you
  through it. See [CONTRIBUTING](./CONTRIBUTING.md).

Feedback and evolutions — of aitop *and* of its agent structure — are explicitly
wanted. Open an [issue](https://github.com/grippado/aitop/issues) or a
[discussion](https://github.com/grippado/aitop/discussions).

## License

MIT
