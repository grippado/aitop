# aitop — Agentic contribution structure

This directory turns aitop into a repo an AI agent can contribute to *correctly*
on the first try. Instead of asking a contributor (human or agent) to internalize
every convention from `CONTRIBUTING.md` and re-read five adapters, it encodes
those conventions as **orchestrator commands** that dispatch **specialized
agents**, backed by **per-adapter knowledge files**.

It mirrors a lifecycle-triad pattern (propose → create → evolve) plus a guardian
audit gate — a structure this maintainer has already run in a production work
environment, brought here as an open, copyable reference.

> New here? Start with [`CLAUDE.md`](./CLAUDE.md) — the seven golden invariants
> every piece below assumes.

## Commands (`commands/`)

Run these from inside the repo with Claude Code (`/aitop-...`).

| Command | Stage | Use it when |
|---|---|---|
| [`/aitop-rfc`](./commands/aitop-rfc.md) | **propose** | You want to add a tool/feature and should agree the shape first. Writes `docs/rfcs/NNNN-*.md`. |
| [`/aitop-adapter`](./commands/aitop-adapter.md) | **create** | You're adding a whole new Source adapter. The flagship — orchestrates all six agents to a verified, wired, documented adapter. |
| [`/aitop-enhance`](./commands/aitop-enhance.md) | **evolve** | An existing adapter shows a `—` you can now fill (model, context%, a rate limit), or you want to audit one. Audit-then-apply. |
| [`/aitop-theme`](./commands/aitop-theme.md) | **contribute** | Your first contribution. Adds a `Theme` value in one file — the lowest-barrier way in. |

## Agents (`agents/`)

Each is a single-responsibility specialist. Commands dispatch them; you can also
invoke one directly.

| Agent | Responsibility |
|---|---|
| [`aitop-pattern-scout`](./agents/aitop-pattern-scout.md) | Reads the target tool's on-disk state + the closest existing adapter, emits the invariant checklist: which files, Reader vs SQLite, credential surface, which `domain` fields are *genuinely* available. The token-saving front door — runs first. |
| [`aitop-adapter-engineer`](./agents/aitop-adapter-engineer.md) | Implements `source.Source` under `internal/source/<tool>/` with the `runner.go` `Reader` split. |
| [`aitop-test-engineer`](./agents/aitop-test-engineer.md) | Writes faked-`Reader` table tests and the credential-allowlist test. Never real dirs, never subprocesses. |
| [`aitop-security-auditor`](./agents/aitop-security-auditor.md) | **The gate.** Audits credential-dir access (allowlist, not Walk/Glob), SQL scoping, and "no fabricated value" across the diff. 🔴 blocks. |
| [`aitop-ui-integrator`](./agents/aitop-ui-integrator.md) | Wires the adapter into `main.go`'s slice, confirms the collector/cards render it, keeps `--demo` and `--once` honest. |
| [`aitop-docs-scribe`](./agents/aitop-docs-scribe.md) | Updates the README "What's supported today" table and authors the adapter's `ADAPTER.md`. |

## Orchestration — `/aitop-adapter`

```
/aitop-adapter <tool>
   │
   ▼
aitop-pattern-scout ──────────► invariant checklist (files, Reader/SQLite,
   │                             credential surface, available domain fields)
   ▼
┌─ aitop-adapter-engineer  (implements source.Source + runner.go)
└─ aitop-test-engineer     (faked-Reader tests + allowlist test)
   │
   ▼
aitop-security-auditor  ◄─── GATE: 🔴 (walked cred dir / unscoped SQL /
   │                          fabricated value) blocks; 🟡 needs sign-off; 🟢 ok
   ▼
aitop-ui-integrator  (main.go slice, collector/cards render, --demo/--once honest)
   │
   ▼
aitop-docs-scribe  (README support table + internal/source/<tool>/ADAPTER.md)
   │
   ▼
verify: go build ./... && go vet ./... && CGO_ENABLED=0 go test -race ./...
   │
   ▼
open PR   (only when the human asks)
```

`/aitop-enhance` runs the same spine but scoped to one existing adapter and one
gap; `/aitop-rfc` stops at the proposal; `/aitop-theme` skips the adapter agents
entirely and goes straight to the single-file theme change.

## Knowledge files

- **`templates/ADAPTER.md`** — the skeleton `/aitop-adapter` fills for a new
  tool.
- **`internal/source/<tool>/ADAPTER.md`** — one per existing adapter, next to
  its code: state location, Reader/SQLite split, which `domain` fields it fills
  vs leaves `—` and *why*, credential surface, and known quirks. Read the one
  next to any adapter before editing it.

## Further reading

- [`docs/agentic-orchestration.md`](../docs/agentic-orchestration.md) — the
  learning guide: what this structure teaches and how to rebuild it elsewhere.
- [`docs/rfcs/0001-...`](../docs/rfcs/0001-agentic-contribution-architecture.md)
  — why this structure exists (design rationale + success criteria).
- [`docs/rfcs/0002-...`](../docs/rfcs/0002-evolving-agentic-structures.md) — how
  it's meant to evolve (non-read-only, plugins, harness control), as a case study.
- [`docs/BUILT-WITH-AITOP.md`](../docs/BUILT-WITH-AITOP.md) — build a product on
  aitop's JSON; add yours to the *from aitop, …* gallery.
