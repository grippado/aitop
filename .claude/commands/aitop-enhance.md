---
description: Audit an existing Source adapter, then fill a "—" field or add detection (model, context%, a rate limit) — audit-then-apply.
argument-hint: <tool-name> [gap, e.g. "model detection" | "context%"] [apply:true]
---

# /aitop-enhance

You are evolving an **existing** adapter under `internal/source/<tool>/`. Unlike
`/aitop-adapter` (which creates), this improves one that already ships — most
often by turning an honest `—` into a real value the tool actually exposes now.
It always audits first; it only mutates under `apply:true`.

**Target:** `$ARGUMENTS`

## Personality: audit, then apply

This command is a hybrid guardian. Auditing is unconditional and always
produces a report; mutation is gated. That keeps the flow transparent and
idempotent — you can run it read-only to see what's fillable before touching
anything.

## Steps

### 1. Read the ground truth
Read `internal/source/<tool>/ADAPTER.md` and the adapter's `usage.go` +
`<tool>.go`. The `ADAPTER.md` already records which `domain` fields are filled
vs `—` and **why**. A `—` is one of two things:
- **Structurally impossible** (e.g. Cursor's cost is cloud-side and proprietary;
  cursor-agent's transcript carries no per-turn usage). Leave it. Do not
  "enhance" a field the tool genuinely cannot observe — that would be
  fabrication, the one thing aitop is defined against.
- **Not-yet-implemented but observable** (the tool writes it somewhere the
  adapter doesn't read yet). This is the enhance target.

### 2. Audit (always) — dispatch `aitop-security-auditor` in report mode
Produce a findings report over the adapter:
- Each `domain` field: filled (✅) or `—`, and for each `—` whether it's
  structurally impossible or a fillable gap.
- 🔴 any existing fabrication or credential/SQL-scope violation (fix regardless
  of the requested gap).
- 🟡 fillable gaps ranked by confidence that the tool truly exposes the value.
- 🟢 fields correctly left `—` for a structural reason.

Print the report. If not `apply:true`, stop here.

### 3. Apply (only under `apply:true`)
For the targeted gap:
- Dispatch `aitop-pattern-scout` to confirm the on-disk source of the value and
  that it's genuinely available (not a guess). If the scout can't find a real
  source, **abort the fill** and report why — an unfillable field stays `—`.
- Dispatch `aitop-adapter-engineer` to read the new source through the package
  `Reader` (or the disciplined SQLite path) and populate the field. Suppress
  impossible readings (e.g. a computed context% > 100% is omitted, not shown).
- Dispatch `aitop-test-engineer` to extend the faked-`Reader` tests for the new
  parse path, including the "source absent → field stays `—`" case.

### 4. Re-audit (BLOCKING) + wire + document
- Re-run `aitop-security-auditor` over the diff. Zero open 🔴 to proceed.
- Dispatch `aitop-docs-scribe` to update the README support-table cell (`—` →
  ✅) and the field's row in `internal/source/<tool>/ADAPTER.md`.

### 5. Verify + summarize
```sh
go build ./... && go vet ./... && CGO_ENABLED=0 go test -race ./...
```
Report the audit verdict, what changed, and the verify output. Offer a PR; do
not open one unprompted.

## Definition of done
- The targeted `—` is now a real value from a confirmed on-disk source, or the
  report explains why it must stay `—`.
- No previously-honest field became a guess.
- Tests cover both the populated path and the source-absent path.
- Security-auditor clean, build/vet/race green, README + `ADAPTER.md` updated.
