# Learning agentic orchestration, in practice

aitop is a small program with a deliberately oversized agent structure. That's
on purpose. The domain here — read some files, render some cards — is simple
enough that it never gets in the way, which makes this repo a clean, *readable*
worked example of **how to orchestrate AI agents to contribute to a real
codebase without breaking its invariants.**

If you've read blog posts about "multi-agent workflows" and wanted to see one
you can actually run, clone, and modify — this is that. Everything below is
implemented in [`.claude/`](../.claude/); this doc is the guided tour and the
transferable lessons.

> Read [`.claude/README.md`](../.claude/README.md) for the index and
> [`.claude/CLAUDE.md`](../.claude/CLAUDE.md) for the invariants. This guide
> explains the *why*, so you can rebuild the pattern in your own project.

## The core idea: encode conventions as a structure, not a wiki page

Most repos put their contribution rules in a `CONTRIBUTING.md` and hope people
read it. The problem: an agent (or a rushed human) re-derives those rules from
scratch on every task, and the subtle ones — here, "never fabricate a value a
tool can't observe" — are exactly the ones that get missed under pressure.

The move is to turn the rules into **things that run**:

- **Knowledge files next to the code** (`internal/source/<tool>/ADAPTER.md`) so
  the invariants for a unit of work live where the work happens, already written
  down. An agent reads one file instead of reverse-engineering five adapters.
- **Specialized agents** (`.claude/agents/`) each owning one responsibility, so
  no single prompt has to hold the whole rulebook.
- **Orchestrator commands** (`.claude/commands/`) that sequence those agents in
  the right order with the right gates.

The payoff is measurable: less context re-loading per run, and a much lower
chance of the one mistake the project can't tolerate.

## Pattern 1 — The lifecycle triad (propose → create → evolve)

Real contributions come in three shapes, and each wants a different amount of
ceremony:

| Command | Stage | When |
|---|---|---|
| `/aitop-rfc` | **propose** | The shape isn't obvious yet — decide it as prose first |
| `/aitop-adapter` | **create** | Build the whole new thing |
| `/aitop-enhance` | **evolve** | Improve something that already ships |

Why split them? Because collapsing "propose" and "create" into one command
forces every change through the same heavyweight path, and collapsing "create"
and "evolve" makes small improvements as scary as new features. Three commands,
three altitudes. (A fourth, `/aitop-theme`, is a deliberate on-ramp — the
lowest-barrier contribution gets its own guided path so a first-timer isn't
dropped into the adapter machinery.)

**Transferable lesson:** name your contribution *shapes* first, then build one
orchestrator per shape. Don't build one mega-command with a `mode:` flag.

## Pattern 2 — Fan-out, then gate

Inside `/aitop-adapter`, the work isn't a straight line:

```
pattern-scout                 (1 agent, runs first — maps the ground truth)
  → { adapter-engineer ‖ test-engineer }   (2 agents in parallel — independent work)
  → security-auditor          (1 agent — BLOCKING gate)
  → ui-integrator → docs-scribe
  → verify (build / vet / race)
```

Two ideas are doing the heavy lifting:

- **Scout first.** The most consequential decision — *which data fields are real
  vs which must render `—`* — is made once, up front, by a read-only agent, and
  then **cited** by everyone downstream. If a later agent finds itself guessing
  availability, it skipped a step. Front-loading the irreversible judgment is
  cheaper than discovering it in review.
- **Parallel where independent, barrier where not.** Implementation and tests
  don't depend on each other, so they run concurrently. But the security audit
  *must* see the finished diff, so it's a barrier — nothing proceeds past an open
  🔴.

**Transferable lesson:** separate "discovery" (do once, share) from "production"
(fan out) from "verification" (gate). Parallelize the middle; never parallelize
across a gate.

## Pattern 3 — The guardian gate

`aitop-security-auditor` is adversarial by design and it can **block**. It exists
because aitop has exactly two unacceptable failure modes — leaking a credential
(a broad directory walk or an unscoped SQL query over a shared store) and
fabricating data (a fake `0`/`$0.00`). A blocking, single-purpose auditor that
classifies findings 🔴/🟡/🟢 catches both far more reliably than hoping the
engineer agent polices itself.

**Transferable lesson:** identify the 1–2 mistakes your project genuinely cannot
ship, and give them a dedicated adversarial reviewer with veto power — separate
from the agent that wrote the code. Self-review is weaker than a fresh skeptic.

## Pattern 4 — Cache the intent next to the code

Every `ADAPTER.md` records, for one adapter: where its data lives, how it reads
it (plain files vs the disciplined SQLite exception), which fields are real vs
`—` **and why**, its credential surface, and its known quirks. This is the same
reason good codebases keep design docs next to modules — except here the primary
reader is the *next agent*, so it's terse, factual, and load-bearing.

**Transferable lesson:** a knowledge file earns its keep when it stops the next
contributor from re-deriving a non-obvious decision. Write them for the decisions
that are expensive to rediscover, not for what the code already says plainly.

## Try it

1. Read one adapter's `ADAPTER.md`, then its code. Notice how the doc front-loads
   the "real vs `—`" map.
2. Run `/aitop-theme <name>` — the gentlest orchestrator — and watch how little
   the structure asks of you for a small change.
3. Read `/aitop-adapter` and trace the fan-out → gate flow against the diagram
   above.
4. Add a tool aitop doesn't cover yet with `/aitop-adapter <tool>`, and watch the
   scout → engineer/test → auditor → integrator → scribe hand-offs.

## Where this goes next

This structure is meant to *evolve* as the codebase does — that evolution is
itself the study. See [RFC 0002 — Evolving agentic
structures](./rfcs/0002-evolving-agentic-structures.md) for how the same fleet
grows to cover non-read-only actions, external plugins, and harness control, and
how each step is recorded as a case study rather than a silent refactor.

The first such record is [Case study 0001 — Session
lineage](./case-studies/0001-session-lineage.md): a full feature run through
`/aitop-enhance`, including the moment the honesty gate caught a fabricated
statistic the author had written into the RFC prose itself.

And if you build something on top of aitop, or evolve this structure in your own
repo, we want to hear about it — see [Built with
aitop](./BUILT-WITH-AITOP.md).
