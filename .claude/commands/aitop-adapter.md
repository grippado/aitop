---
description: Scaffold a whole new Source adapter for a tool aitop doesn't cover yet — end-to-end, verified, wired, and documented.
argument-hint: <tool-name> [state-dir-or-notes]
---

# /aitop-adapter

You are orchestrating the creation of a new `internal/source/<tool>/` adapter.
The unit of contribution in aitop is the **Source adapter**, and this is the
flagship path for adding one. Follow aitop's real conventions
(`.claude/CLAUDE.md`, `CONTRIBUTING.md`) — do not re-derive them.

**Target tool:** `$ARGUMENTS`

## Prime directives (repeat them to yourself before each step)

- **Never fabricate a value the tool can't observe.** Missing → `Available:
  false` / zero field → UI renders `—`. A fake `0`/`$0.00` is the one bug this
  whole project is defined against.
- **All filesystem access goes through the package `Reader`** (`runner.go`).
  Tests swap a fake; never touch a real tool dir or spawn a subprocess in a test.
- **Credential dirs get an allowlist, never a `Walk`/`Glob`.**

## Steps

### 1. Scout (dispatch `aitop-pattern-scout`)
Dispatch the `aitop-pattern-scout` agent for the target tool. It returns the
invariant checklist: where the tool keeps session/usage state on disk, whether
that's plain files (→ `Reader`) or SQLite (→ `database/sql` + `modernc.org/sqlite`),
the credential surface to exclude, and — critically — **which `domain` fields
this tool can genuinely populate vs which must stay `—`**. Pick the closest
existing adapter as the structural template (Claude/Codex for file-based; opencode/
cursor for SQLite).

**Cite the scout report from here on.** If you find yourself deciding
availability by guessing, you skipped this step.

### 2. Implement + test in parallel
Dispatch, in parallel:
- `aitop-adapter-engineer` — implements `source.Source` under
  `internal/source/<tool>/`, with `runner.go` holding the sole `Reader` and
  `<tool>.go` holding `Name/Detect/Processes/Sessions/Usage`. Only populate
  `domain` fields the scout confirmed are real.
- `aitop-test-engineer` — faked-`Reader` table tests for the parser, plus the
  credential-allowlist test if the tool has a credential surface.

### 3. Security gate (dispatch `aitop-security-auditor`) — BLOCKING
Dispatch `aitop-security-auditor` over the new package's diff. It classifies
findings 🔴/🟡/🟢:
- 🔴 (credential dir walked/globbed, unscoped `SELECT *` over a shared store,
  a fabricated value, a credential path not excluded) — **must be fixed before
  proceeding.**
- 🟡 — needs an explicit human sign-off.
- 🟢 — informational.

Do not continue past an open 🔴.

### 4. Wire the UI (dispatch `aitop-ui-integrator`)
Dispatch `aitop-ui-integrator` to add the adapter to the
`all := []source.Source{...}` slice in `cmd/aitop/main.go`, give it a per-tool
identity color in `internal/ui/theme/theme.go` + `ToolColor` if it deserves one,
confirm the collector fans it in and the cards pane renders it, and keep the
`--demo` roster (`internal/demo/demo.go`) and `--once` text/json output honest.

### 5. Document (dispatch `aitop-docs-scribe`)
Dispatch `aitop-docs-scribe` to add a row to the README "What's supported today"
table (with honest `—`/✅ per field, matching the scout report) and write
`internal/source/<tool>/ADAPTER.md` from `.claude/templates/ADAPTER.md`.

### 6. Verify
Run and report:
```sh
go build ./...
go vet ./...
CGO_ENABLED=0 go test -race ./...
```
All three green, or stop and report the failure verbatim.

### 7. Summarize (do NOT open a PR unless asked)
Report: files added, which `domain` fields the adapter fills vs leaves `—` and
why, the security-auditor verdict, and the verify output. Then **offer** to open
a PR — do not open one unprompted.

## Definition of done
- `source.Source` implemented, `Detect` cheap, `Reader` split intact.
- Every populated field is real; every unavailable field is `—`, matching the
  scout report and the README table.
- `aitop-security-auditor` returns zero open 🔴.
- Build, vet, and race tests green.
- `ADAPTER.md` written; README table updated.
