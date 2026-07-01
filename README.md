# aitop

**This `aitop`:** real CPU/MEM/NET usage of your AI coding agent processes (Claude Code, Codex CLI, Cursor), in one terminal pane — cost is a footnote, not the headline. If you were looking for a token/cost tracker, that's a different `aitop` ([bugkill3r/aitop](https://github.com/bugkill3r/aitop) or [samusgray/aitop](https://github.com/samusgray/aitop)) — this one is a real system-resource monitor, `btop`-style, scoped to AI tools.

<!-- TODO before launch: record `aitop --demo` and drop the GIF here. -->

## What it does

- **Real system resources, not just session logs.** Per-core CPU bars, memory, network — read via [gopsutil](https://github.com/shirou/gopsutil), the same category of numbers real `btop`/`htop` show, filtered to processes that belong to an AI coding tool.
- **Multi-tool from day one.** Claude Code, Codex CLI, and Cursor — Cursor in particular isn't covered by any other "aitop"-named project as of this writing.
- **Read-only.** No approve/reply/merge actions (that's [agent-dashboard](https://github.com/bjornjee/agent-dashboard)'s job, and it requires tmux — aitop doesn't). aitop only ever observes.
- **Honest about gaps.** Missing data renders as `—`, never a fabricated `0`. Cursor has no local cost data and aitop says so instead of guessing.

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
aitop --demo             # synthetic data, no real tool required
```

### Keybindings

| Key | Action |
|-----|--------|
| `q` / `Ctrl+C` | quit |
| `Tab` / `Shift+Tab` | cycle pane focus |
| `1`-`5` | jump to a numbered box |
| `j`/`k`, arrows | move selection |
| `f` | filter the process table by tool |
| `o` | cycle sort column |
| `space` | pause/resume refresh |
| `u` | expand/collapse the usage panel |
| `r` | force refresh |
| `?` | help |

## Why it exists

Every existing "aitop"-named project (and the excellent but tmux-bound `agent-dashboard`) either tracks cost/tokens, or requires tmux, or covers Claude Code alone. None of them show real per-core CPU/memory/network the way `btop` does, and none of them support Cursor. aitop's whole reason to exist is that gap: a genuine system monitor for the AI-tool era, not another cost dashboard borrowing `btop`'s color palette.

## What's supported today (v1)

| Tool | Processes/CPU/MEM | Sessions | Cost/tokens |
|------|---|---|---|
| Claude Code | ✅ | ✅ | Cost ✅, tokens/context% — no passive on-disk source yet |
| Codex CLI | ✅ | ✅ | Not available locally (see adapter source for why) |
| Cursor | ✅ (Cursor's own telemetry) | ✅ | Not available (proprietary/cloud-side) |
| Other AI CLIs (aider, windsurf, opencode, ...) | Best-effort via process-name match | — | — |

v2 roadmap: dedicated adapters for aider/windsurf/opencode, an external plugin mechanism, a local trend history, and community-contributed themes (see [CONTRIBUTING.md](./CONTRIBUTING.md)).

## License

MIT
