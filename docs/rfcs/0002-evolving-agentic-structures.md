# RFC 0002 — Evolving agentic structures in complex systems

- Status: Draft (living document — this RFC is meant to grow)
- Type: process / roadmap / case study
- Builds on: [RFC 0001](./0001-agentic-contribution-architecture.md)

## Summary

RFC 0001 established aitop's agent structure. This RFC is about its **second
derivative**: how that structure should *evolve* as the system grows, and — just
as importantly — how each evolution is recorded so the repo doubles as a
longitudinal **case study in evolving agentic orchestration**. The thesis: the
interesting engineering isn't the first version of an agent fleet, it's how it
adapts when the domain gets harder (new tools, non-read-only actions, external
plugins). aitop is small enough to make that adaptation legible.

## Motivation

Agent structures are usually presented as finished artifacts — a diagram in a
blog post. In practice they *drift and grow*: a new failure mode appears, a new
contribution shape emerges, a gate that was overkill becomes essential (or vice
versa). Almost nobody shows that trajectory. Because aitop's domain is simple and
its structure is versioned in-repo (RFCs + `.claude/`), it can show the whole
arc: what changed, why, and what it cost. That's a teaching asset the ecosystem
is short on.

## Principle: every structural change is an RFC, not a silent refactor

The rule that makes this a case study rather than just a codebase:

> When the agent fleet changes shape — a new agent, a new gate, a retired
> command, a re-drawn hand-off — it lands with an RFC in `docs/rfcs/` explaining
> the pressure that forced it and the trade-off taken.

So the RFC series *is* the changelog of the structure's evolution. A reader in a
year can trace not just what the fleet looks like, but the sequence of pressures
that shaped it.

## Evolution vectors (roadmap)

These are the directions the structure is expected to grow. Each becomes its own
RFC when it's built.

### V1 → V1.x — deepen coverage (low risk)
- **More adapters** (aider, windsurf, ...). Each new tool tests whether
  `aitop-pattern-scout`'s field-availability checklist generalizes. If a tool
  breaks the mold (e.g. no local state at all), that's an RFC.
- **Enhance passes** that turn `—` into real values as tools expose more. Watches
  whether the "structurally impossible vs not-yet-implemented" distinction in
  `/aitop-enhance` holds up.

### V2 — external plugins (the domain gets distributed)
The `domain` types are already JSON-pure specifically to keep this open: an
out-of-process adapter could speak the same `Snapshot` shape over a subprocess +
JSON-RPC boundary. When that lands, the agent fleet has to grow a **plugin-author
path** — a new orchestrator (`/aitop-plugin`?) and likely a **compatibility
auditor** agent that checks a plugin's JSON against the schema instead of
checking Go against the `Reader` rule. This is the first evolution where the
*guardian* changes shape, and it's the most interesting one to document.

### V3 — beyond read-only (the invariant itself changes)
aitop is read-only today, on principle. But the JSON it emits is a natural
substrate for tools that *do* act — pause a runaway agent, nudge a stalled one,
rebalance work across sessions (harness control). aitop core stays observational,
but a downstream product (or an opt-in module) could add write actions. **That
inverts the project's defining constraint**, so it demands the most careful
evolution of the structure:
- A new, much stricter guardian: an **action auditor** that treats every
  state-mutating call the way `aitop-security-auditor` treats a credential read
  today — allowlisted, consent-gated, reversible, logged.
- A new contribution shape (propose → create → evolve → **operate**?), because a
  write action has a runtime blast radius a render never does.
- This is exactly the scenario the case study exists to illuminate: *what happens
  to an agent fleet when the one rule everything was built around stops holding?*

## Success criteria for the case study

1. Every shape-change to `.claude/` since RFC 0001 has a corresponding RFC
   stating the pressure and the trade-off.
2. A newcomer can read the RFC series in order and reconstruct *why* the fleet
   looks the way it does — not just what it is.
3. At least one evolution (plugins or non-read-only) documents a case where a
   gate had to change, showing the structure adapting rather than ossifying.
4. The transferable lessons in [the learning guide](../agentic-orchestration.md)
   stay in sync with what the structure actually does.

## Open questions

- When does an agent earn retirement? A fleet that only grows becomes its own
  kind of debt. What's the signal to *merge* two agents back together?
- How much of the guardian's judgment can be encoded as a test (CI-enforced) vs.
  must stay a review-time agent call?
- For non-read-only actions: is the right boundary "aitop core never writes, forks
  do," or "aitop ships an opt-in, heavily-gated action module"? (This is a values
  question as much as a technical one — hence an RFC, in the open.)

## Note

If you're evolving this structure — here or in a fork — that trajectory is the
contribution we most want to see written down. Open a discussion or add an RFC,
and tell us about the product you built on top (see [Built with
aitop](../BUILT-WITH-AITOP.md)).
