---
description: Draft an RFC proposing a new adapter or feature before building it — captures data provenance, honest gaps, and the credential surface.
argument-hint: <tool-or-feature-name> [notes / links]
---

# /aitop-rfc

You are drafting a proposal in `docs/rfcs/NNNN-<slug>.md` — the "propose" stage,
run *before* `/aitop-adapter` when a change is worth agreeing on first (a new
tool with an unusual state layout, a schema-touching feature, anything where the
honest availability of fields isn't obvious). Small, obvious adapters can skip
straight to `/aitop-adapter`.

**Subject:** `$ARGUMENTS`

## Why RFC-first for aitop specifically

aitop's contract is honesty: every field is either real or `—`. The hardest part
of a new adapter is *deciding which is which* — and that decision is much cheaper
to review as prose than as code. An RFC front-loads it.

## Steps

### 1. Number the RFC
List `docs/rfcs/`. The next RFC is `max(existing) + 1`, zero-padded to 4 digits.
`0001` (agentic-architecture) and `0002` (evolving structures) are already in use.

### 2. Scout the tool (for adapter RFCs) — dispatch `aitop-pattern-scout`
If proposing a new adapter, dispatch `aitop-pattern-scout` to map the tool's
on-disk state so the RFC's provenance table is real, not imagined: where
sessions/usage live, plain files vs SQLite, the credential surface, and which
`domain` fields are genuinely observable.

### 3. Write `docs/rfcs/NNNN-<slug>.md`
Copy `docs/rfcs/TEMPLATE.md` and fill it in — that file is the single source of
truth for RFC structure (status emojis included).

### 4. Summarize
Report the RFC path and the key availability decisions (which fields will be
real, which stay `—`, and why). Offer a PR; do not open one unprompted.

## Definition of done
- `docs/rfcs/NNNN-<slug>.md` exists, correctly numbered.
- Every `domain` field is classified available/`—` with a concrete on-disk
  reason — no aspirational ✅.
- Credential surface and storage mechanism are named.
