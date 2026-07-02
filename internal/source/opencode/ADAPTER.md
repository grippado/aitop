# `opencode` adapter

`Source.Name()` → `"opencode"`. The only **SQLite-primary** adapter: opencode's
own state is a real database, not JSON/JSONL files.

## State on disk

- **Detect target / DB:** `~/.local/share/opencode/opencode.db`.
- **Models cache:** `~/.cache/opencode/models.json` (honors `XDG_CACHE_HOME`; a
  ~3MB real file) — gives the real context-window size and display names.

## Mechanism

**SQLite-primary** via **`database/sql` + `modernc.org/sqlite`** (pure Go, keeps
`CGO_ENABLED=0`), opened read-only (`file:...?mode=ro`, `SetMaxOpenConns(1)`).
WAL guarantees a consistent reader snapshot while opencode is writing. Query
shapes (`opencode.go`) — scoped `WHERE`/`ORDER BY`, never `SELECT *`:
- `SELECT id, directory, title, model, tokens_input, tokens_output, tokens_cache_read, tokens_cache_write, time_updated FROM session ORDER BY time_updated DESC LIMIT 1`
- `SELECT id FROM message WHERE session_id = ? ORDER BY time_created DESC LIMIT 1`
- `SELECT data FROM part WHERE message_id = ? ORDER BY time_created DESC`

The package **`Reader`** (`runner.go`) covers **only** the plain-file access
(`Detect` + the `models.json` cache); it deliberately does **not** cover
`opencode.db` — that's the documented SQLite exception.

## Field availability

| domain field | avail | source / reason |
|---|---|---|
| `SessionInfo.Model` | ✅ | `models.json` friendly name, else dash-cleaned raw id |
| `SessionInfo.Title` | ✅ | **native** — `session.title` column |
| `SessionInfo.LastAction` | ✅ | `part` table via `summarizePart` (`tool`/`text`/`reasoning`; `step-*` ignored) |
| `SessionInfo.TokensIn` | ✅ | `tokens_input + tokens_cache_read + tokens_cache_write` (mirrors Claude's contextTokens convention) |
| `SessionInfo.TokensOut` | ✅ | `tokens_output` |
| `SessionInfo.ContextUsedPct` | ✅ | `models.json` window, only if present and pct ≤ 100 (omitted otherwise) |
| `SessionInfo.CWD` | ✅ | `session.directory` |
| `UsageInfo` (cost) | — | **`Available:false`** — `session.cost` is real but **lifetime-cumulative** for that session, not day/month-scoped. Summing by `time_created` would misattribute a long-running session's entire cost to its start day — subtly wrong, not honest. Tokens still live per-session on `SessionInfo`. |

## Credential surface

**None** read or excluded — reads only `opencode.db` and `models.json`.

## Known quirks

- **Only one card:** the `session` table has no PID column, so a live process is
  paired with the most-recently-updated session. Two concurrent opencode
  processes collapse onto one card — a known v1 limitation (same as
  cursor-agent).
- **Authoritative window (best-effort):** `models.json` gives the real context
  window + display name — the same authoritative-source-over-guessing preference
  Codex gets for free and Claude has to fake with a hardcoded 1M. On a fresh
  install without the cache, `contextWindow`/`friendlyName` report not-found and
  the caller omits `ContextUsedPct` / uses the dash-cleaned raw id.
- **Process match** (`isOpencodeProcess`): exact name/argv[0] `opencode`, mirrors
  Codex to avoid substring false-positives.
- **WAL safety:** `mode=ro` never modifies the file opencode is actively writing.
