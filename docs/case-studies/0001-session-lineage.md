# Case study 0001 — Session lineage, shipped through the agent structure

- Feature: [RFC 0003 — session lineage (spawned-by)](../rfcs/0003-session-lineage-spawned-by.md)
- Driver: the `/aitop-enhance claude` lifecycle command
- Date: 2026-07-09

This is the first worked record of the agent structure in
[`agentic-orchestration.md`](../agentic-orchestration.md) doing a real feature
end to end — the "recorded as a case study rather than a silent refactor" this
repo keeps promising. It exists because the single most valuable thing that
happened during the run was **the honesty gate catching a fabricated number the
author had written into a doc.** That's the whole thesis, caught on tape.

## What shipped

Two honest `SessionInfo` fields and the UI to read them:

- `Kind` (`"bg"` | `"interactive"`) — verbatim from the `kind` key already sitting
  in `~/.claude/sessions/<pid>.json`, previously read-and-discarded.
- `ParentPID` — the on-board session that spawned this one, resolved by a bounded
  gopsutil PPID walk (`0` = a root / no visible parent). No new file I/O, no new
  dependency, no new credential surface.
- UI: a `[bg]`/`[interactive]` badge, a `▸ spawned by <parent>` line, and children
  indented under their parent in list view — at unchanged card height.

The motivating observation: a mutirão of `kind:bg` background sessions was landing
as N unrelated cards, when it was really one interactive root and its spawned
children. Both framing facts were verified on-disk *before* writing the RFC.

## The hand-off chain (as it actually ran)

`/aitop-enhance` is "audit, then apply." Here is the real sequence, each step a
specialized agent enacting its `.claude/agents/*.md` definition:

| # | Agent | Produced |
|---|---|---|
| 1 | `aitop-security-auditor` (audit) | Field-by-field honesty map; classified `Kind` + `ParentPID` as genuinely observable (fillable), not fabrication. Gate: **PASS to apply.** |
| 2 | `aitop-pattern-scout` | Confirmed both sources against the **live** machine: `kind` present in every session file (values only `bg`/`interactive`); PPID walk resolving two `bg` children to the interactive root `86904`; roots → `0`. Flagged the "don't stop at the first non-session ancestor" hazard. |
| 3 | `aitop-adapter-engineer` | `domain` fields + adapter: `sessionFile.Kind` passthrough, `resolveParentPIDs` bounded walk (`maxParentWalkHops = 40`, self-reference guarded), and an **injectable `ppidOf` hook** so the walk is testable without live gopsutil. `go build`/`go vet` green. |
| 4 | `aitop-test-engineer` | Faked-`Reader` + faked-`ppidOf` tests: kind parse, kind-absent→`""`, walk-through-untracked-ancestor, orphan→`0`, self-reference, and bounded/cycle (run in a goroutine so a non-terminating walk would hang, not pass). Also caught that the new walk made two *pre-existing* tests hit live gopsutil, and guarded them. |
| 5 | `aitop-ui-integrator` | Badge + provenance + list-view nesting at fixed card height; `ParentLabel` resolved **only against PIDs that became cards** (else blank, or honest `▸ spawned (parent not on board)`); `--demo` roster with a real root + two children. |
| 6 | `aitop-security-auditor` (re-audit) | Blocking pass over the full diff — invariants 2/3/5/6 each checked with `file:line` citations, `-race` green. **CLEAN — zero 🔴.** |
| 7 | `aitop-docs-scribe` | README "supported today" note (lineage is Claude-only, honest gap elsewhere) + `claude/ADAPTER.md` field rows and the structural known-gap paragraph. |

## The moment the structure earned its keep

The audit gate (step 1) is meant to stop fabrication *before* code. But the sharper
catch came at the re-audit's sibling check on the **RFC prose itself**: the author
(the orchestrating model) had written a confident statistic into the proposal —

> "A scan of **204 transcripts** on this machine found **0** lines with
> `isSidechain:true`."

The auditor re-ran the scan and found **634 of 879** transcripts contain
`isSidechain:true`. The "0/204" was an artifact of a shallow glob
(`projects/*/*.jsonl` saw one nesting level; the real tree needs a recursive
walk). Writing that number into `ADAPTER.md` — the *honesty* doc — would have
published exactly the fabricated figure the whole project is defined against.

The fix wasn't to correct the count. It was to notice the count was never needed:
the real guarantee is **structural** — aitop's cards come exclusively from
`sessions/<pid>.json`, and a sidechain never gets one, so an in-process Task
subagent can never be a card, no matter how many sidechains exist. The RFC was
amended to state it structurally, with no fragile number.

**Transferable lesson:** a "never fabricate" invariant is only real if something
re-derives the author's claims against ground truth. Here the author was an LLM
and the claim was in prose, not code — and the gate still caught it. Put the
adversarial check *outside* the thing being checked, including outside the human
(or model) writing the docs.

## Honest note on how these agents were dispatched

This run was orchestrated from a session whose working directory was **outside**
the repo, so the native `subagent_type: aitop-*` registry didn't resolve. Each
step was instead a `general-purpose` agent instructed to **read and adopt its
`.claude/agents/<name>.md` role definition** before working. The specialization
came from the role file being read and followed, not from a pre-registered agent
type. That the structure survives being invoked this way — the role definitions
are portable, self-contained instructions — is itself a small evolution finding,
in the spirit of [RFC 0002](../rfcs/0002-evolving-agentic-structures.md).

## Verification of record

`go build ./...` · `go vet ./...` · `CGO_ENABLED=1 go test -race ./...` — all green
at the re-audit and again before commit. The feature is honest by construction:
every field is a real reading or a rendered `—`/blank, never a guess.
