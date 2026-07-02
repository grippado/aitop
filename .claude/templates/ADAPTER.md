<!--
Template for internal/source/<tool>/ADAPTER.md — the knowledge file that lives
next to an adapter's code. Fill EVERY section from the real code. The one rule:
what renders "—" at runtime is "—" here. No aspirational ✅.
Delete this comment when you fill it in.
-->

# `<tool>` adapter

`Source.Name()` → `"<name-string>"`. One-line description of the tool.

## State on disk

- **Detect target:** `<path>` (what `Detect()` checks — cheap, side-effect-free)
- **Sessions:** `<path pattern>` (+ env overrides like `XDG_*` / `<TOOL>_CONFIG_DIR`)
- **Usage / cost:** `<path>` or "none on disk"
- **Model / context source:** `<path/key>`

## Mechanism

- **Reader (plain files):** which files go through the package `Reader`
  (`runner.go`) — the only place `os.ReadFile`/`ReadDir`/`Stat` is called.
- **SQLite (if any):** which DB, opened `mode=ro`, and the exact query shapes
  (exact-key lookup / id-scoped `GLOB` / scoped `WHERE ... ORDER BY` — never
  `SELECT *`). Note that SQLite access is the documented exception and does NOT
  go through the `Reader`.

## Field availability

`—` = the UI renders a dash because the value is genuinely unavailable. Never
upgrade a `—` to ✅ without a real on-disk source (use `/aitop-enhance`).

| domain field | avail | source / reason |
|---|---|---|
| `SessionInfo.Model` | ✅ / — | ... |
| `SessionInfo.Title` | ✅ / — | native field / synthesized from first user msg |
| `SessionInfo.LastAction` | ✅ / — | ... |
| `SessionInfo.TokensIn` / `TokensOut` | ✅ / — | ... |
| `SessionInfo.ContextUsedPct` | ✅ / — | window source, or "no window exposed" |
| `SessionInfo.CWD` | ✅ / — | ... |
| `UsageInfo` (cost / limits) | ✅ / — | reason if `Available:false` |

## Credential surface

- Credential/API-key file(s) under the tool dir: `<name>` (or "none").
- Excluded via named-path allowlist (`allowlist.go`), guarded by
  `allowlist_test.go`. Never `Walk`/`Glob` over the dir.

## Known quirks

- PID ↔ session mapping: yes/no → (drives whether N concurrent sessions = N
  cards or one collapsed card).
- Title: native vs synthesized. Model source. Context-window source.
- Shared-tree / attribution hazards with other tools' files.
- Dedup logic, stale-PID handling, degraded-but-alive notes.
