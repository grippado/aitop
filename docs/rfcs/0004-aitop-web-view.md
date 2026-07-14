<!--
  📜 aitop RFC 0004 — the browser view (`aitop serve`)
-->

# RFC 0004 — `aitop serve`: a browser view over the same honest Snapshot

- Status: 🚚 Shipped
- Type: feature
- Author: [@grippado](https://github.com/grippado)
- Builds on: [RFC 0002](./0002-evolving-agentic-structures.md) · [Built with aitop](../BUILT-WITH-AITOP.md)

## 🪄 Summary

Add a read-only browser dashboard, `aitop serve`, that renders the exact same
`domain.Snapshot` the TUI renders — one card per live agent session — but with
the room a browser gives: live sparklines of context/token growth, richer
filtering and sorting ("control over the data"), and a raw-JSON inspector. It
reuses the TUI's `PullFunc` (`collector.Snapshot`) verbatim, adds **zero new
dependencies** (stdlib `net/http` + `embed`), stays **read-only** (only `GET`,
binds `127.0.0.1` by default), and keeps the **never-fabricate** rule (missing
fields render `—`, never `0`). It is the first officially-in-repo instance of
["Build a product on aitop's data"](../BUILT-WITH-AITOP.md#build-on-the-json).

## 🤔 Motivation

`aitop --once --json` already declares the `Snapshot` a public substrate, and
`docs/BUILT-WITH-AITOP.md` explicitly invites dashboards on top of it. A browser
is the natural second surface: a terminal card is fixed-height and monochrome;
a browser can show history-over-time (sparklines), let you pivot/filter/inspect
the data freely, and stay open on a second monitor. This RFC brings that surface
**inside the repo** as an official, honest, read-only view — not a fork — so the
JSON contract gets a first-party consumer that keeps it honest.

## 🗄️ Storage & mechanism

No new on-disk reads. The web server is a pure consumer of the `Snapshot` the
`collector` already produces:

```
--serve ─► webserver.Run(pull, refresh, addr)
             ├─ hub: ONE poll loop (ticker @ refresh) → caches last Snapshot → fan-out to SSE subs
             ├─ GET /api/snapshot → last Snapshot as JSON (same shape as --once --json)
             ├─ GET /api/stream   → text/event-stream, one Snapshot per tick
             └─ GET / + assets    → SPA embedded via go:embed (no CDN, no build step)
```

The **single poll loop** matters: multiple SSE clients would otherwise each call
`collector.Snapshot` concurrently (it samples CPU for 200ms and its doc states
"concurrency not required"). The hub polls once and broadcasts, so N browser tabs
cost the same as one. Assets are embedded with `//go:embed web/*` and served from
memory — the `Reader`-rule (invariant #3) is untouched: the web server reads no
tool state, only serves what the adapters already surfaced.

## 🕵️ Data provenance

The web view invents nothing. Every field is the same `domain` field the TUI
shows, with the same honesty:

| domain field | in the browser | notes |
|---|---|---|
| Model / Title / LastAction | shown when present, `—` when empty | identical to the TUI card |
| Context % / tokens | real bar + client-side sparkline | sparkline history lives **only in the browser tab's memory**; the server persists nothing. Close the tab, history is gone — honest by construction |
| Cost / rate limits | shown when `UsageInfo.Available`, else `—` | never a fabricated `$0.00` |
| System CPU/MEM/NET | footer, MEM segmented AI-vs-system | mirrors `internal/ui/panes/system` |

## 🔐 Credential surface

None. The server never opens a tool directory or a credential file — it only
serializes an in-memory `Snapshot`. The security surface is **network exposure**,
not credentials: it binds `127.0.0.1:8787` by default. Binding to a routable
address (`--addr 0.0.0.0:...`) is an explicit, documented user choice, not a
default, and there is no auth layer in this MVP (out of scope, below).

## 🧭 Alternatives considered

- **A separate `web/` app (React/Svelte + build).** More visual ceiling, but
  adds an npm toolchain and a build step, and can't ship inside the single Go
  binary. Rejected for the MVP against the repo's zero-dep / `CGO_ENABLED=0`
  discipline; a fork is free to do this on the same API.
- **Polling `/api/snapshot` from JS instead of SSE.** Simpler, but every tab
  re-triggers a CPU sample and there's no single-poll fan-out. SSE + hub is
  strictly better for the multi-tab case and reconnects for free.
- **Executing the agentic commands from the browser.** This inverts the
  read-only invariant — it is RFC 0002 §V3 "beyond read-only" territory and
  needs an action-auditor + consent-gate + its own RFC. Explicitly **out of
  scope**; see below.

## 🤝 Parity principle: the TUI and the web view evolve together

Non-negotiable, and the reason the frontend is a deliberate port of
`internal/ui/panes/cards` rather than its own thing: **the browser view must not
diverge in *content* from the terminal.** Same cards, same fields, same btop
palette, same `—`-for-missing honesty. The two surfaces are one product read two
ways.

The web view may only differ where the richer medium *adds an affordance the
terminal can't offer well* — a clickable button, a link, a hover, a scrollback
the TUI lacks. Concretely, everything the browser does beyond the TUI is one of:

| Web-only affordance | Terminal analog it enriches |
|---|---|
| tool/sort **dropdowns** | the TUI's `f` (filter) / `o` (sort) keybinds |
| **list/grid toggle** | the TUI's `v` — list gives each card the full width (action in full); grid packs columns |
| **copy-command chip** on a fillable `—` | (none — a link/button is the point) |
| per-session **sparklines** | (none — scrollback the TUI can't hold) |
| **raw-JSON** inspector | `aitop --once --json` in a second pane |

The rule for every future change: a new field or signal lands in **both**
surfaces (or a follow-up issue tracks the lagging one). When the TUI grows a
column, the web card grows the same one; when the web adds an affordance, it must
be *additive* (a button/link), never a different reading of the same data. That's
what keeps "the browser version" honest — it's the same board, with buttons.

## 🌱 Relation to RFC 0002

This is a **product on the JSON**, not a change to the read-only invariant: the
core still only observes. Where it touches the agentic structure is the
*suggestion* layer — when a card shows a `—` an enhance pass could fill, the UI
offers a **copy-to-clipboard** of the ready command (`/aitop-enhance cursor`).
That "induces evolution" (the maintainer's framing) without executing anything:
the board points at the gap and hands you the command; a human runs it in Claude
Code. Actually *running* commands from the browser is the V3 inversion RFC 0002
reserves for a much stricter guardian — deliberately deferred.

## ❓ Open questions

- Should history ever be server-side (a real trend store), or is client-only the
  honest default forever? (MVP: client-only.)
- If exposed on a LAN, what's the minimum honest auth story — a token flag, or
  "don't, use an SSH tunnel"? (MVP: localhost + docs.)

## 🚚 Rollout

1. `internal/webserver/` — `server.go` (hub + 3 handlers), `embed.go`, `web/` assets, `server_test.go` (httptest over a fake `pull`, no real dirs).
2. `cmd/aitop/main.go` — `--serve` + `--addr` flags; reuse the same `collector`/`demo` `PullFunc`.
3. README — a `aitop serve` line in Usage + a "Built with aitop → now first-party" note.
4. Verify: `go build ./...`, `CGO_ENABLED=1 go test -race ./...`, `go vet ./...`, and a manual `--serve` / `--serve --demo` check that numbers match `--once --json`.

---

<sub>📚 New to aitop RFCs? Read [0001](./0001-agentic-contribution-architecture.md) and [0002](./0002-evolving-agentic-structures.md). 💛</sub>
