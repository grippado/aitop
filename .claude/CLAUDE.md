# aitop — Agent Brief

> This file is the always-loaded context for any Claude Code (or compatible)
> agent working in this repo. It exists so agents stop re-deriving the same
> invariants on every run. Read it first; the per-adapter `ADAPTER.md` files
> and the `.claude/README.md` index are the next layer down.

`aitop` is a read-only terminal dashboard (Go, Bubble Tea) that shows one card
per live AI coding-agent session — Claude Code, Codex CLI, Cursor, cursor-agent,
opencode — with the model driving it, its context-window fill, and its token
burn. System CPU/MEM/NET is a footnote, not the headline.

The project's whole reason to exist is **breadth of honest coverage**. That
makes the single highest-value contribution *adding another Source adapter*,
and it makes the single worst regression *fabricating a number a tool can't
actually observe*. Everything below serves those two facts.

## Architecture map

```
cmd/aitop/main.go            entrypoint; builds the []source.Source slice, wires collector→ui
internal/domain/types.go     JSON-pure data contract every adapter produces & every pane consumes
internal/source/source.go    the Source interface every adapter implements
internal/source/registry.go  Resolve(): keeps only adapters whose Detect() is true
internal/source/<tool>/      one package per tool (claude, codex, cursor, cursoragent, opencode)
  runner.go                  the ONLY file allowed to touch os.ReadFile/ReadDir/Stat (the Reader)
  <tool>.go                  Source implementation (Name/Detect/Processes/Sessions/Usage)
  usage.go / transcript.go / model.go / ...   parsing helpers
  ADAPTER.md                 that adapter's provenance + invariants (read before editing it)
internal/source/fallback/    best-effort process-name match for tools without a dedicated adapter
internal/collector/          fans every adapter in concurrently (500ms timeout each) → one Snapshot/tick
internal/ui/                 Bubble Tea model + panes (cards, system) + theme + widgets
internal/demo/demo.go        synthetic sessions for --demo (screenshots/GIFs; never real state)
```

## The seven golden invariants

Every command, agent, and doc in this structure assumes these. Do not violate
them; do not re-derive them.

1. **The Source contract.** Implement `source.Source`
   (`internal/source/source.go`): `Name`, `Detect`, `Processes`, `Sessions`,
   `Usage`. `Detect` must be cheap and side-effect-free.

2. **Never fabricate.** If a tool can't provide a value, return
   `UsageInfo.Available = false` or leave the `domain` field zero/empty — the
   UI renders `—`. A fabricated `0`, `$0.00`, or `0/0` is a bug, not a
   placeholder. This is the rule the whole project is judged on.

3. **All filesystem access goes through the package `Reader`.** Only
   `<tool>/runner.go` may call `os.ReadFile`/`os.ReadDir`/`os.Stat`/`ReadFrom`.
   Everything else takes the `Reader` interface so tests swap a fake. Tests
   never touch a real `~/.claude`/`~/.codex`/Cursor dir and never spawn a
   subprocess.

4. **SQLite is the one exception, and it's disciplined.** `opencode`
   (`opencode.db`) and `cursor/composer.go` (Cursor's shared `state.vscdb`) use
   `database/sql` + `modernc.org/sqlite` (pure Go — keeps `CGO_ENABLED=0`
   release builds intact), tested against a real temp DB. Every query is an
   exact-key lookup or an id-scoped `GLOB` the adapter already resolved — never
   `SELECT *` over a store that also holds other extensions' data.

5. **Credential safety is an allowlist, not a habit.** Never
   `filepath.Walk`/`Glob` over a directory that can hold credentials. Use an
   explicit named-path allowlist and a test asserting the credential file is
   never in it (`internal/source/codex/allowlist.go` + `allowlist_test.go` is
   the reference).

6. **The `domain` types stay JSON-pure.** No `context.Context`, no live OS
   handles — everything round-trips through `encoding/json`. That's what keeps
   the v2 external-plugin path (subprocess + JSON-RPC) open. Per-session tokens
   live on `SessionInfo`; tool-wide usage lives on `UsageInfo`; they never mix.

7. **Green means all three.** `go build ./...`,
   `CGO_ENABLED=1 go test -race ./...` (`-race` requires cgo; the release build
   stays `CGO_ENABLED=0`), and `go vet ./...`. A new adapter
   isn't done until the README support table and the `--demo` roster are honest
   about what it does and doesn't provide.

## The agentic contribution structure

This repo ships a small fleet of orchestrator commands and specialized agents
so a contribution follows the project's real conventions by construction — a
pattern this maintainer has already applied in a production work environment
and is bringing here as a reference for agentic open-source contribution.

**Lifecycle commands** (`.claude/commands/`):

| Command | Stage | What it does |
|---|---|---|
| `/aitop-rfc` | propose | Draft `docs/rfcs/NNNN-*.md` for a new adapter/feature before building |
| `/aitop-adapter` | create | Scaffold a whole new Source adapter end-to-end (the flagship) |
| `/aitop-enhance` | evolve | Audit an existing adapter, then fill a `—` field / add detection |
| `/aitop-theme` | contribute | The lowest-barrier path: add a `Theme` value |

**Specialized agents** (`.claude/agents/`): `aitop-pattern-scout`,
`aitop-adapter-engineer`, `aitop-test-engineer`, `aitop-security-auditor`
(the credential/honesty gate), `aitop-ui-integrator`, `aitop-docs-scribe`.

See `.claude/README.md` for the full index and the orchestration diagram.

## Working rules for agents

- **Read before you write.** Before editing an adapter, read its `ADAPTER.md`
  and the closest sibling adapter — the invariants are already written down.
- **`aitop-pattern-scout` runs first** in the create flow, and its output
  should be visibly cited ("per the scout report, this tool has no per-turn
  usage → leave `ContextUsedPct` zero"). If you re-derived it, you skipped a
  step.
- **`aitop-security-auditor` is a gate, not a suggestion.** A 🔴 finding
  (credential dir walked, unscoped SQL, fabricated value) blocks the PR.
- **Honesty over completeness.** A card that says `—` is correct; a card that
  guesses is a regression the whole project is defined against.
- **Docs are markdown-only and CI-free.** `.github/workflows/ci.yml` uses
  `paths-ignore: ["*.md", docs/**]`, so `.md`-only changes don't trigger CI —
  but they still must be true.
