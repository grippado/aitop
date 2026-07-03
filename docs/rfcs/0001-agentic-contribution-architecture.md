# RFC 0001 — Agentic contribution architecture

- Status: Accepted
- Type: process / meta
- Supersedes: —

## Summary

aitop ships a small, self-documenting fleet of AI-agent orchestrator commands
and specialized subagents (under `.claude/`) plus per-adapter knowledge files
(`internal/source/<tool>/ADAPTER.md`), so that a contribution — human- or
agent-driven — follows the project's real conventions by construction. This RFC
records why that structure exists, how it's shaped, and what "done" means for it.
It also establishes `docs/rfcs/` as the home for future proposals.

## Motivation

aitop's value is **breadth of honest coverage** of AI coding tools. Two facts
follow:

1. The highest-leverage contribution is *adding another Source adapter*.
2. The worst regression is *fabricating a value a tool can't observe* — a card
   that guesses is worse than one that says `—`.

Both make adapter work unusually convention-heavy for its size: the `Reader`
interface split, the disciplined SQLite exception, the credential allowlist, the
JSON-pure domain, and above all the per-field "is this real or `—`?" judgment.
Left as prose in `CONTRIBUTING.md`, every contributor (and every agent run)
re-derives it — and the honesty judgment is exactly the part that's easy to get
wrong under time pressure.

This is a solved problem elsewhere: this maintainer has already run a
lifecycle-triad agent structure (propose → create → evolve, plus a guardian
audit gate) in a production work environment, where it turned a repo into a
reference for agentic maturity by caching engineering invariants next to the code
and letting specialized agents consume them. This RFC brings that pattern to
aitop as an **open, copyable** reference for agentic open-source contribution.

## Design

### The unit is the Source adapter

Where the reference structure keyed everything on a UI *component*, aitop keys it
on the **Source adapter** (`internal/source/<tool>/`) — the true unit of
contribution here. Each adapter gets a knowledge file (`ADAPTER.md`) recording
its data provenance and invariants, the analog of caching design + engineering
intent next to each component.

### Lifecycle commands (`.claude/commands/`)

| Command | Stage | Analog |
|---|---|---|
| `/aitop-rfc` | propose | draft `docs/rfcs/NNNN-*.md` before building |
| `/aitop-adapter` | create | scaffold a new adapter end-to-end (flagship) |
| `/aitop-enhance` | evolve | audit an existing adapter, fill a `—` field |
| `/aitop-theme` | contribute | lowest-barrier path: add a `Theme` value |

### Specialized agents (`.claude/agents/`)

`aitop-pattern-scout` (front door — maps state + decides field availability),
`aitop-adapter-engineer` (implements `source.Source`), `aitop-test-engineer`
(faked-`Reader` tests + credential guard), **`aitop-security-auditor`** (the
guardian gate), `aitop-ui-integrator` (wiring + render), `aitop-docs-scribe`
(README table + `ADAPTER.md`).

### Orchestration

```
/aitop-adapter <tool>
  → pattern-scout (invariant checklist + field availability)
  → { adapter-engineer ‖ test-engineer }
  → security-auditor        ← GATE: 🔴 blocks
  → ui-integrator
  → docs-scribe
  → verify (build / vet / race)
  → PR (only when asked)
```

`/aitop-enhance` runs the same spine scoped to one adapter + one gap; `/aitop-rfc`
stops at the proposal; `/aitop-theme` skips the adapter agents.

### The invariants the structure encodes

The seven golden invariants live in `.claude/CLAUDE.md` (Source contract; never
fabricate; `Reader`-only I/O; disciplined SQLite exception; credential allowlist;
JSON-pure domain; build/vet/race green). Every command and agent references them;
none re-derives them.

## Why an audit gate, specifically

The `aitop-security-auditor` is deliberately adversarial and **blocking**,
because the two failure modes it guards are the two the project is defined
against: credential exposure (a broad `Walk`/`Glob` or `SELECT *` over a store
that also holds secrets) and fabrication (a `0`/`$0.00`/`0/0` on a path where no
real value was read). A 🔴 finding stops the flow; 🟡 needs human sign-off; 🟢 is
informational. This mirrors a report-then-gate "guardian" role that worked well
in the reference environment.

## Success criteria

1. A new-adapter PR passes `aitop-security-auditor` with **zero open 🔴**.
2. The `pattern-scout` report is **visibly cited** in the adapter's field-
   availability decisions — no field is marked ✅ without a named on-disk source.
3. Every `—` in a shipped card is traceable to a structural reason in that
   adapter's `ADAPTER.md`.
4. Context-loading cost drops versus re-deriving the invariants each run, because
   agents read the cached `ADAPTER.md` / `CLAUDE.md` instead of re-reading five
   adapters.
5. `go build ./...`, `go vet ./...`, and `CGO_ENABLED=1 go test -race ./...`
   (`-race` requires cgo; release builds stay `CGO_ENABLED=0`) stay green.

## Notes

- These are markdown/`.claude` files only. `.github/workflows/ci.yml` uses
  `paths-ignore: ["*.md", docs/**]`, so this structure does **not** trigger CI —
  its correctness is enforced by review and by the agents themselves, not by a
  pipeline.
- Future proposals live in `docs/rfcs/NNNN-<slug>.md`, numbered sequentially (`0001` and `0002` already exist).
