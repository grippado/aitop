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
| `SessionInfo.Kind` | ✅ | verbatim passthrough of the session file's `kind` key (`"bg"` \| `"interactive"`); left empty if the file omits it — never synthesized |
| `SessionInfo.ParentPID` | ✅ | bounded gopsutil PPID walk (`resolveParentPIDs`, cap `maxParentWalkHops = 40`) from each **alive** session up to the FIRST ancestor that is itself a tracked session PID; the walk passes *through* untracked hosts (`claude`, `bg-pty-host`) and stops at that first tracked ancestor. A root / orphan (walk ends or hits the cap without a tracked ancestor) keeps `0`. Never set to the session's own PID |
| `UsageInfo.CostTodayUSD` / `CostMonthUSD` | ✅ | summed `current - base` across UUIDs in the cost files |
| `UsageInfo.LimitFiveHour` / `LimitWeekly` | ✅ | ccstatusline cache, **only** if the reset time is in the future |
| `UsageInfo` tokens/context | — | deliberately **not** set tool-wide — lives per-`SessionInfo` so two sessions never show identical numbers |

`UsageInfo.Available = costFound || limitsFound` — never true with all fields at
zero (which would read as "confirmed $0" when nothing was found). `sumCostFile`
returning `ok=false` means "no reading," not "zero spend."

## Credential surface

**`~/.claude/.credentials.json`** (Claude Code's OAuth token / API credential)
lives directly under `~/.claude`, a sibling of `sessions/`. It is
**structurally unreachable** by this adapter: the package only ever `ReadDir`s
the `sessions/` and `projects/` subtrees and reads named cost/cache files —
never the config root, never `Walk`/`Glob`. Unlike `codex` (`allowlist.go` +
`allowlist_test.go`), there is no dedicated guard test asserting this yet; the
protection is structural-by-convention. Adding a `claude/allowlist_test.go`
would make it CI-enforced.

## Known quirks

- **Claude Desktop VM gap:** Desktop "local agent mode" / Cowork runs Claude Code
  inside an isolated VM; those sessions live in the guest FS and are invisible
  here — this adapter only reads the **host's** `~/.claude/sessions/`.
- **In-process Task subagents (sidechains) can never be cards:** aitop's cards
  come exclusively from `sessions/<pid>.json`, one per on-board session, and a
  Task subagent has no such file — it runs inside its parent's process and
  exists only as `isSidechain:true` lines inside the parent's transcript, with
  no PID or session file of its own. So a sidechain never surfaces as a card,
  and structurally no card is ever a sidechain; `ParentPID` therefore links only
  real, file-backed sessions to each other, never an in-process subagent.
- **Context window is a guess:** originally 200k (gave a nonsensical 297%); now
  hardcoded `1_000_000`, with >100% suppression as the load-bearing honesty
  mechanism ("better to omit it than show a confidently wrong number"). Codex and
  opencode get an authoritative window instead.
- **Model:** `friendlyModelName` returns `""` for non-matching ids including the
  `<synthetic>` internal marker.
- **Transcript tail** is rotation-safe: offset resets on shrink; `resolvePath`
  caches the transcript location and falls back to a bounded `projects/` scan.
