# `cursoragent` adapter

`Source.Name()` → `"cursor-agent"`. Reads the cursor-agent **CLI**'s transcripts
under `~/.cursor` — distinct from every file the Cursor IDE adapter reads. The
adapter with the most `—`: no token or usage data exists in this format at all.

## State on disk

- **Detect target / model:** `~/.cursor/cli-config.json` (`model.modelId`,
  re-read every call, not cached).
- **Transcripts:** `~/.cursor/projects/<slug(cwd)>/agent-transcripts/<id>/<id>.jsonl`,
  where slug drops the leading `/` and turns remaining `/` into `-`.

## Mechanism

Plain files through the package **`Reader`** (`runner.go`). Also shells out to
`lsof` for the live process cwd (`proccwd.go`) because gopsutil's `Cwd` is
unimplemented on darwin without cgo (`CGO_ENABLED=0` build). **No SQLite.**

## Field availability

| domain field | avail | source / reason |
|---|---|---|
| `SessionInfo.Model` | ✅ | `cli-config.json` `model.modelId` — a **global** CLI setting, accurate for the one live session but not authoritative for past/other sessions |
| `SessionInfo.Title` | ✅ | **synthesized** — first genuine user message unwrapped from `<user_query>` tags, first line, clamped to 70 (no native auto-title in this format) |
| `SessionInfo.LastAction` | ✅ | from the transcript |
| `SessionInfo.CWD` | ✅ | `lsof` live cwd, or reconstructed from the slug (`cwd.go`) |
| `SessionInfo.TokensIn` / `TokensOut` | — | **never set** — no token/cost data exists in this transcript format (role/message/content only, confirmed on real data) |
| `SessionInfo.ContextUsedPct` | — | same — no token data to derive from |
| `UsageInfo` (cost / limits) | — | **`Available:false`** — no per-turn usage block and no local cost ledger. (The per-run debug log under `$TMPDIR` carries a rough `estimated_tokens`, but its location/lifetime aren't stable enough for v1.) |

## Credential surface

**None** read or excluded — reads only `cli-config.json` and transcripts.

## Known quirks

- **Only ever one card:** no PID→session mapping exists (no
  `chat_processes.json` equivalent). Multiple concurrent runs collapse to one
  card — a known v1 limitation (opencode's adapter documents the same).
- **Shared-tree overlap with Cursor IDE:** `~/.cursor/projects/*/agent-transcripts/`
  is the *same* tree the IDE's Agent panel writes into — a cursor-agent
  transcript filename and an IDE composerId can be the same UUID. Search is
  scoped to the live PID's own cwd (`findLatestTranscriptInWorkspace`) to avoid
  misattribution; falls back to global "most recently written wins" only if
  `processCwd` fails.
- **Process match** (`isCursorAgentProcess`): argv[0] basename exactly
  `cursor-agent`; excludes the `node ... worker-server` helper and
  `cursor-agent-svc`.
- **cwd reconstruction** (`cwd.go`) backtracks over `-` groupings against real
  dirs (handles names containing `-`, e.g. `guia-cumuru` vs `guia-cumuru-client`)
  and dot-prefixed variants; returns `""` if nothing resolves — an honest
  "unavailable," never a truncated guess.
- **`processCwd`** (`proccwd.go`) shells to `lsof -a -p <pid> -d cwd -Fn`; fails
  closed.
