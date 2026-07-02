# Built with aitop

aitop is intentionally a small, honest, read-only lens on your AI agents. But the
data it surfaces — one structured record per live session, per tool — is a
foundation other tools can stand on. This page is two things: a **how-to** for
building on aitop, and a **gallery** of what people have built.

If aitop is the sensor, this is where the products that use the sensor live.

## Build on the JSON

Everything aitop knows is available as a single machine-readable snapshot:

```sh
aitop --once --json
```

That prints a `domain.Snapshot` — the exact same type the live TUI renders — with
`system`, `tools`, `processes`, `sessions`, and `usage` arrays. The shape is
defined in [`internal/domain/types.go`](../internal/domain/types.go) and it is
**JSON-pure by design**: no runtime handles, everything round-trips through
`encoding/json`. That wasn't an accident — it's what keeps aitop usable as a data
source, and what keeps the v2 out-of-process plugin path open.

Poll it, pipe it, ship it somewhere:

```sh
# a crude alerting loop: shout when any session's context passes 90%
while :; do
  aitop --once --json \
    | jq -r '.sessions[] | select(.context_used_pct > 90) | "⚠️  \(.tool) \(.id) at \(.context_used_pct)%"'
  sleep 30
done
```

Because the snapshot is per-session and honest (missing data is `—`/omitted,
never a fabricated `0`), it's safe to build decisions on top of it.

### Ideas worth stealing

- **Dashboards & history.** Persist snapshots over time; chart context growth,
  token burn, or how long sessions stay open. aitop shows *now*; you could show
  *the last week*.
- **Alerting & routing.** Notify when a session goes idle, when context nears the
  window, when a tool's rate limit is close. Push it to Slack, a phone, a status
  light.
- **Non-read-only harness control.** aitop core only ever observes — but nothing
  stops a downstream tool from *acting* on what it observes: pause a runaway
  agent, nudge a stalled one, rebalance work across sessions, gate spend. If you
  cross that line, cross it carefully — the design thinking for a guarded,
  consent-gated action layer is sketched in
  [RFC 0002](./rfcs/0002-evolving-agentic-structures.md#v3--beyond-read-only-the-invariant-itself-changes).
- **Your own adapter, upstreamed.** If you taught aitop to read a tool it didn't
  cover, consider contributing it back with `/aitop-adapter` (see
  [CONTRIBUTING](../CONTRIBUTING.md)).

## The gallery — *from aitop, …*

Built something on top of aitop's data? A dashboard, an alerter, a harness
controller, a fork that adds actions? **Add it here** via a PR — one entry, this
format:

```md
### <Project name>
*from aitop, <one line on what it does>.*
— by [<your name>](<link>) · <repo or site link>
```

<!-- Add your project above this line. Keep it one entry, alphabetical-ish, honest. -->

> _This gallery is new and deliberately empty — be the first `from aitop, …`._

## We want your feedback (on aitop itself, too)

This is a young project and the whole point is to be built on and improved. Two
kinds of feedback are equally welcome:

- **On what you built:** tell us in a [Discussion] (or add it to the gallery
  above) — especially if aitop's JSON was missing something you needed, or if you
  pushed it somewhere read-only never anticipated.
- **On aitop itself:** an adapter reading a field wrong, a tool it should cover, a
  data point the snapshot should expose, a rough edge in the agent structure —
  open an [Issue] or a [Discussion]. Evolutions of the agent structure are
  especially interesting to us; see
  [RFC 0002](./rfcs/0002-evolving-agentic-structures.md).

[Issue]: https://github.com/grippado/aitop/issues
[Discussion]: https://github.com/grippado/aitop/discussions
