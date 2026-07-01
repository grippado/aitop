# aitop

**This `aitop`:** a live, read-only monitor of your AI coding *agents and their contexts* — Claude Code, Codex CLI, Cursor — one card per session, showing what it's doing, how full its context window is, and its token burn, all in one terminal pane. System resources (CPU/MEM/NET) are a condensed footnote at the bottom, not the headline. If you were looking for a pure cost/token tracker, that's a different `aitop` ([bugkill3r/aitop](https://github.com/bugkill3r/aitop) or [samusgray/aitop](https://github.com/samusgray/aitop)) — this one is a context board for your agents, `btop`-styled, not a spend dashboard.

<!-- TODO before launch: record `aitop --demo` and drop the GIF here. -->

## What it does

- **A card per agent session.** Tool identity (border color), state (running/idle), context-window fill, and session token burn, at a glance — across every Claude Code, Codex, and Cursor session running on your machine right now.
- **Multi-tool from day one.** Claude Code, Codex CLI, and Cursor — Cursor in particular isn't covered by any other "aitop"-named project as of this writing.
- **Resources, condensed.** A 4-line footer shows real aggregate CPU/MEM/NET (via [gopsutil](https://github.com/shirou/gopsutil)) plus how much of that your AI-tool processes account for — present, but deliberately secondary.
- **Read-only.** No approve/reply/merge actions (that's [agent-dashboard](https://github.com/bjornjee/agent-dashboard)'s job, and it requires tmux — aitop doesn't). aitop only ever observes.
- **Honest about gaps.** Missing data renders as `—`, never a fabricated `0`. Cursor has no local cost data and aitop says so instead of guessing; the same goes for any field a given adapter hasn't populated yet.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/grippado/aitop/main/install.sh | sh
```

```sh
brew install grippado/aitop/aitop
```

```sh
go install github.com/grippado/aitop/cmd/aitop@latest
```

## Usage

```sh
aitop                    # live TUI, 2s refresh
aitop --once             # one text snapshot, then exit
aitop --once --json      # machine-readable snapshot
aitop --refresh 5s       # override the tick interval
aitop --demo             # synthetic agent cards, no real tool required
```

### Keybindings

| Key | Action |
|-----|--------|
| `q` / `Ctrl+C` | quit |
| `j`/`k`, arrows | move selection |
| `f` | filter cards by tool |
| `o` | cycle sort column (context / tokens / age / tool) |
| `v` | toggle list/grid card layout |
| `u` | expand/collapse the focused card's usage detail |
| `space` | pause/resume refresh |
| `r` | force refresh |
| `?` | help |

## Why it exists

Every existing "aitop"-named project (and the excellent but tmux-bound `agent-dashboard`) either tracks cost/tokens as the headline, requires tmux, or covers Claude Code alone. None of them present agent sessions as a live, glanceable board the way a mutirão of open threads deserves, and none of them support Cursor. aitop's whole reason to exist is that gap: a real-time context board for the agents actually running on your machine, with resource usage as a footnote instead of the main event.

## What's supported today (v1)

| Tool | Process/CPU/MEM | Sessions | Context% / tokens | Cost |
|------|---|---|---|---|
| Claude Code | ✅ | ✅ | Not available locally yet (no passive on-disk source — see adapter source) | ✅ |
| Codex CLI | ✅ | ✅ | Not available locally (see adapter source for why) | Not available (bills via your own API key) |
| Cursor | ✅ (Cursor's own telemetry) | ✅ | Not available | Not available (proprietary/cloud-side) |
| Other AI CLIs (aider, windsurf, opencode, ...) | Best-effort via process-name match | — | — | — |

Card fields that depend on backend data no adapter populates yet (last session action, git branch/dirty state, per-session — as opposed to per-tool — token/context tracking) render as `—` on real data today; `--demo` shows what the card looks like once those land.

v2 roadmap: dedicated adapters for aider/windsurf/opencode, an external plugin mechanism, per-session (not just per-tool) usage tracking, branch/dirty detection, a local trend history, and community-contributed themes (see [CONTRIBUTING.md](./CONTRIBUTING.md)).

## License

MIT
