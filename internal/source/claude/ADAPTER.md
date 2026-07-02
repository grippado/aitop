# `claude` adapter

`Source.Name()` → `"claude-code"`. Reads Claude Code's own session state,
transcripts, cost files, and the `ccstatusline` rate-limit cache.

## State on disk

- **Config dir** (`resolveConfigDir`): `CLAUDE_CONFIG_DIR` env wins; otherwise
  probes `~/.claude` then `~/cangaco/.ai/claude`, picking whichever has a
  `sessions/` dir, defaulting to `~/.claude`.
- **Detect target:** the resolved config dir has a `sessions/` dir.
- **Sessions:** `<configDir>/sessions/<pid>.json`.
- **Transcripts:** `<configDir>/projects/<encoded-cwd>/<sessionID>.jsonl`, where
  encoded-cwd is the cwd with every `/` replaced by `-`.
- **Cost:** `<configDir>/.cost-day-YYYY-MM-DD.json` and `.cost-month-YYYY-MM.json`.
- **Rate limits:** `~/.cache/ccstatusline/usage.json` (honors `XDG_CACHE_HOME`).

## Mechanism

All plain files through the package **`Reader`** (`runner.go`) — the only file
calling `os.ReadFile`/`ReadDir`/`Stat`. `ReadFrom(path, offset)` tail-follows a
growing transcript without re-reading the whole file each tick (offset resets on
shrink/rotation). **No SQLite.**

## Field availability

| domain field | avail | source / reason |
|---|---|---|
| `SessionInfo.Model` | ✅ | each assistant message's `message.model`, via `friendlyModelName` (returns `""` for unknown / `<synthetic>` ids) |
| `SessionInfo.Title` | ✅ | **native** — Claude's own `{"type":"ai-title"}` transcript line |
| `SessionInfo.LastAction` | ✅ | latest tool call / thinking snippet from the transcript |
| `SessionInfo.TokensIn` / `TokensOut` | ✅ | per-session, from the transcript `usage` block (`deriveTokenFields`) |
| `SessionInfo.ContextUsedPct` | ✅ | derived against a hardcoded `1_000_000` window; any computed pct > 100% is **omitted** (see quirks) |
| `SessionInfo.CWD` | ✅ | from the session file |
| `UsageInfo.CostTodayUSD` / `CostMonthUSD` | ✅ | summed `current - base` across UUIDs in the cost files |
| `UsageInfo.LimitFiveHour` / `LimitWeekly` | ✅ | ccstatusline cache, **only** if the reset time is in the future |
| `UsageInfo` tokens/context | — | deliberately **not** set tool-wide — lives per-`SessionInfo` so two sessions never show identical numbers |

`UsageInfo.Available = costFound || limitsFound` — never true with all fields at
zero (which would read as "confirmed $0" when nothing was found). `sumCostFile`
returning `ok=false` means "no reading," not "zero spend."

## Credential surface

**None excluded** — Claude Code stores no plaintext API key this adapter reads;
the package only ever touches `sessions/`, `projects/`, the cost files, and the
ccstatusline cache. No `Walk`/`Glob`.

## Known quirks

- **Claude Desktop VM gap:** Desktop "local agent mode" / Cowork runs Claude Code
  inside an isolated VM; those sessions live in the guest FS and are invisible
  here — this adapter only reads the **host's** `~/.claude/sessions/`.
- **Context window is a guess:** originally 200k (gave a nonsensical 297%); now
  hardcoded `1_000_000`, with >100% suppression as the load-bearing honesty
  mechanism ("better to omit it than show a confidently wrong number"). Codex and
  opencode get an authoritative window instead.
- **Model:** `friendlyModelName` returns `""` for non-matching ids including the
  `<synthetic>` internal marker.
- **Transcript tail** is rotation-safe: offset resets on shrink; `resolvePath`
  caches the transcript location and falls back to a bounded `projects/` scan.
