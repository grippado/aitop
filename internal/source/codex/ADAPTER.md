# `codex` adapter

`Source.Name()` → `"codex"`. Reads Codex CLI's rollout transcripts and process
state from a **closed allowlist** of `~/.codex` paths.

## State on disk

Only these paths are ever opened (`allowlist.go`):
- **Detect target / config:** `~/.codex/config.toml`.
- **Process map:** `~/.codex/process_manager/chat_processes.json`.
- **History:** `~/.codex/history.jsonl`.
- **Rollout transcripts:** `~/.codex/sessions/YYYY/MM/DD/rollout-<ts>-<sessionID>.jsonl`.

## Mechanism

Plain files through the package **`Reader`** (`runner.go`). `ReadDir` is scoped
**exclusively** to the `sessions/` subtree — there is no recursive walk or
wildcard expansion anywhere in the package. **No SQLite** in v1 (the
`state_5.sqlite`/`logs_2.sqlite` files are noted only as v2 investigation).

## Field availability

| domain field | avail | source / reason |
|---|---|---|
| `SessionInfo.Model` | ✅ | `turn_context` (e.g. `gpt-5.4-mini`), used as-is (already human-readable) |
| `SessionInfo.Title` | ✅ | **synthesized** — first genuine user message (skips `<wrapper>` lines), first line, clamped to 70; a real quote, not a summary |
| `SessionInfo.LastAction` | ✅ | from the rollout transcript |
| `SessionInfo.TokensIn` / `TokensOut` | ✅ | per-session, from the transcript token-count events |
| `SessionInfo.ContextUsedPct` | ✅ | **authoritative** — `model_context_window` from the `token_count` event (>100% suppression is just a defensive backstop here, not load-bearing) |
| `SessionInfo.CWD` | ✅ | `chat_processes.json`, else rollout `session_meta` first line |
| `SessionInfo.PID` | ✅ (best-effort) | `chat_processes.json` when present; often absent (see quirks) |
| `UsageInfo` (cost / limits) | — | **`Available:false`** — Codex bills through the user's own OpenAI API key (`auth.json`, never opened); no local per-session USD ledger. `config.toml`'s `[tui].status_line` is a list of field *names*, not live numbers. |

## Credential surface

**`~/.codex/auth.json`** (plaintext OpenAI API key) is intentionally **never** in
the allowlist. It lives directly under `~/.codex`, a sibling of `sessions/`, so
it's structurally unreachable from the scoped traversal. Guarded by a
CI-enforced test in `allowlist_test.go`.

## Known quirks

- **PID correlation often fails:** on many machines `chat_processes.json` is
  absent, so `byPID` is empty; the live process is matched instead by resolving
  its session ID from the most recently written rollout filename
  (`findLatestRolloutSessionID`). This replaced a duplication bug where matching
  on PID alone gave a live session a second orphan card.
- **Process match** (`isCodexProcess`): exact name `codex` or argv[0]
  `codex`/`.../codex` — substring match caused false positives (a debugging shell
  once got misidentified).
- **Title synthesized**, not native; skips `<environment_context>` /
  `<user_shell_command>` wrapper lines.
- `chat_processes.json` schema is unverified and defensively parsed.
