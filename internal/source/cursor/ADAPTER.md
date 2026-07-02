# `cursor` adapter

`Source.Name()` → `"cursor"`. Reads the Cursor **IDE**'s process-monitor logs
(plain files) and its composer state (SQLite). The canonical example of the "no
fabricated data" rule: usage is always `—`.

## State on disk

- **Detect target / process logs:** `~/Library/Application Support/Cursor/process-monitor/<epoch-ms>.log`.
  The latest file is picked by **numeric filename value, not mtime**.
- **Composer DB:** `~/Library/Application Support/Cursor/User/globalStorage/state.vscdb`
  — a single global VSCode-style store shared with every other extension's data
  (hundreds of MB on a real machine).

## Mechanism

**Both.** The package **`Reader`** (`runner.go`) tail-follows the process-monitor
log. Composer data uses **`database/sql` + `modernc.org/sqlite`** (pure Go),
opened read-only (`file:...?mode=ro`, `SetMaxOpenConns(1)`). Query shapes
(`composer.go`):
- Exact-key lookup: `SELECT value FROM ItemTable WHERE key = 'composer.composerHeaders'`.
- Id-scoped GLOB: `SELECT value FROM cursorDiskKV WHERE key GLOB ? ORDER BY rowid`
  with `"bubbleId:"+composerID+":*"`.

Both hit the key index (confirmed via `EXPLAIN QUERY PLAN`; `LIKE` would fall
back to a full scan — `GLOB` is used deliberately). Never a full scan over the
shared store.

## Field availability

| domain field | avail | source / reason |
|---|---|---|
| `SessionInfo.Model` | ✅ | composer, via `friendlyModelName` (dash→space for non-Claude ids, e.g. `composer-2.5` → `composer 2.5`) |
| `SessionInfo.Title` | ✅ | composer `Name` |
| `SessionInfo.LastAction` | ✅ | composer bubbles |
| `SessionInfo.TokensIn` / `TokensOut` | ✅ | per-session, from composer bubbles |
| `SessionInfo.ContextUsedPct` | ✅ | **authoritative** — Cursor's own `contextUsagePercent`, not derived from a guessed window |
| `SessionInfo.CWD` | ✅ | upgraded from the log's workspace label to the composer's real `fsPath` |
| `UsageInfo` (cost / tokens) | — | **`Available:false`, always** — Cursor's cost/token accounting is proprietary and cloud-side, nothing observable locally. Never a fabricated `$0` (the domain contract cites this as the canonical `—` case). |

## Credential surface

**None** read or excluded — the adapter reads only the process-monitor log and
`state.vscdb`; no API key on disk. (The SQL discipline above is the security
control here, applied to a shared store rather than a credential file.)

## Known quirks

- **Stale-PID dedup** (`prune`): ingest keeps every last-seen row; a PID from a
  closed/restarted window lingers with its old sessionId, which resolves to the
  **same** composer as the live window → two identical cards for one task.
  `prune` drops rows whose PID no longer exists.
- **Cursor's own CPU/mem** are used directly from the log (more accurate than
  gopsutil); only `StartedAt` comes from gopsutil.
- **Own-process filter** (`isCursorOwnProcess`): only `/Applications/Cursor.app/`
  and `Cursor Helper` binaries; integrated-terminal descendants excluded.
- **Liveness filter** (`isCursorIDEProcess`): excludes macOS
  `CursorUIViewService` and `cursor-agent` false-positives (with the app fully
  closed, the old check still reported "alive" via `CursorUIViewService`).
- **Cross-tool dedup:** setting `si.ID = ComposerID` lets `cards.BuildCards`
  recognize a `cursor-agent` CLI run sharing a composerId as the same task and
  dedup it.
- **Parse-failure honesty:** a bad log surfaces `"cursor detectado, log não
  parseável"` while still returning cached rows — degraded, not silent.
