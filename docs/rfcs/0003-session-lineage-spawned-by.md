# RFC 0003 — Session lineage: showing where a spawned session came from

- Status: 🌱 Draft   <!-- 🌱 Draft · 🔎 In review · ✅ Accepted · 🚚 Shipped · ⚰️ Superseded -->
- Type: feature
- Author: [@grippado](https://github.com/grippado)

## 🪄 Summary

A Claude Code session can spawn *other* Claude Code sessions as real
background processes (`kind:"bg"` — worktree mutirões, detached `claude -p`
runs). Today each one lands as a free-floating card with no hint that it was
launched *by* another card on the same screen, so the board reads as N unrelated
sessions when it is really one interactive root and its spawned children. This
RFC adds two honest, already-observable fields — `SessionInfo.Kind` and
`SessionInfo.ParentPID` — and a UI that badges each card with its `kind` and,
for a spawned child, prints `▸ spawned by <parent title>` and indents it under
its parent. No new data source, no fabrication.

## 🤔 Motivation

Observed on a real machine (the maintainer's), five live sessions:

```
86904  kind=interactive  rf-monorepo-platina-readiness  (~/www/isaac)   ← root
 ├─ 34595  kind=bg  rf-monorepo-platina-readiness  (www/isaac)
 ├─ 35622  kind=bg  dc064b33                       (worktree/cpu-4342)
 └─ 81405  kind=bg  "Evoluir API de detalhes..."   (~/)
66126  kind=interactive  grippado-64              (~/)                  ← root (separate)
```

The three `bg` cards all descend, by PPID, from the interactive `86904` — that
parent→child edge is real and observable, but the board shows none of it. The
result is a mild but persistent "wait, which of these is the one I'm driving,
and which did it launch?" every time a mutirão is running.

Two facts frame the whole design, both verified on-disk before writing this:

1. **These are NOT Task-tool subagents.** A Task/subagent runs *in-process*,
   sharing its parent's PID and context window; it is written **inline** into
   its parent's transcript as `isSidechain:true` lines (these are common — on
   this machine 634 of 879 transcripts contain them) and **never** gets its own
   `sessions/<pid>.json`. That last fact is the structural guarantee, provable
   without a fragile count: aitop's cards come exclusively from
   `sessions/<pid>.json`, one per real *process* session, and none is ever a
   sidechain — so an in-process Task subagent can never surface as a card. The
   things that do surface as extra cards are exclusively separate *processes*.
   So the UI must **not** call these "sub-agents" — that would overclaim a
   relationship that isn't a sidechain. They are **spawned background
   sessions**. Naming matters here precisely because honesty is the project's
   whole thesis.

2. **The signals already exist.** The `kind` field is written by Claude Code
   into every `sessions/<pid>.json` (`"kind":"bg"` | `"interactive"`) and is
   currently **parsed away** — `sessionFile` in `internal/source/claude/claude.go`
   doesn't read it. PPID is already read via gopsutil in `Processes()` for the
   daemon fallback. This feature is almost entirely *stop discarding data we
   already touch*.

## 🕵️ Data provenance

New/changed `domain` fields. Every `—` is a promise the implementation keeps.

| domain field | available? | source on disk | notes |
|---|---|---|---|
| `SessionInfo.Kind` | ✅ (Claude) | `sessions/<pid>.json` → `"kind"` | Verbatim Claude Code value (`"bg"` / `"interactive"`). Empty (`—`) for any adapter that has no equivalent — never guessed. |
| `SessionInfo.ParentPID` | ✅ (Claude) | PPID walk over live processes | Set **only** when the walk reaches another *tracked session's* PID; `0` otherwise. `0` = "no visible parent on this board", not "orphan proven". |
| `ProcessInfo.PPID` | ✅ (already) | gopsutil | Already collected; the walk generalizes it beyond the daemon fallback. |

Cross-tool honesty: `Kind` and `ParentPID` stay zero/empty for Codex, Cursor,
cursor-agent, and opencode until each adapter has a real source. The UI renders a
plain top-level card with no badge in that case — identical to today.

## 🗄️ Storage & mechanism

Plain files — the package `Reader`, no new I/O primitive.

- **`Kind`**: add the field to the `sessionFile` struct and copy it onto
  `SessionInfo` in `Sessions()`. One line of parsing; the JSON is already read.
- **`ParentPID`**: walk each live session PID's PPID chain (gopsutil
  `PpidWithContext`, already imported) until it either reaches a PID that is
  itself a tracked session — record that as `ParentPID` — or exits the process
  tree / hits a bounded guard (cap the walk, e.g. 40 hops, matching the probe
  used to verify this). The chain legitimately passes through non-session hosts
  (`bg-pty-host`, `ClaudeCode.app`) before reaching the real parent session, so
  the walk must not stop at the first non-session ancestor.

Cost note: the PPID walk runs at the Claude adapter's own ~5s cadence (same
place the existing full-process scan already runs), not the collector's 2s
system tick — consistent with invariant on `Processes()`.

## 🔐 Credential surface

**None.** No new files are read. `kind` comes from a session file already parsed;
PPID from gopsutil. No directory is walked or globbed, so the allowlist
invariant (golden rule 5) is untouched.

## 🧭 Alternatives considered

- **Call them "sub-agents" / a `SUB-AGENTS` bracket** (the original sketch).
  Rejected: overclaims a Task-sidechain relationship these processes don't have.
  In a project whose README sells "honest about gaps," the label would age badly.
  Kept the *visual nesting* idea, dropped the *word*.
- **Infer parentage from `kind:"bg"` alone.** Insufficient: `kind` says
  "background," not "child of X." A detached `claude -p` from cron is `bg` with
  no visible parent and must stay top-level with just a `bg` badge, not forced
  under a bracket. `kind` is the badge; PPID is the tree. Both are needed.
- **Match by title/cwd instead of PPID.** Fragile and fabricated-adjacent — two
  bg sessions in the same repo would false-link. PPID is the ground truth.
- **Surface in-process Task subagents as cards.** Impossible without fabricating:
  they have no PID of their own and no `sessions/<pid>.json` — a sidechain is
  only `isSidechain:true` lines inside its parent's transcript. Documented as a
  permanent known gap in `ADAPTER.md`, in the spirit of the existing Desktop-VM
  gap note.

## ❓ Open questions

- **Label wording.** Direction agreed: honest, provenance-forward —
  `▸ spawned by <parent title>` on the child card plus a `kind` badge
  (`[bg]` / `[interactive]`). Exact glyph/placement is the UI integrator's call.
- **Ordering & indent in list vs grid layout.** Children should render directly
  under their parent in list view; grid view (the `v` toggle) needs a rule —
  simplest is to keep grid flat but still badge `bg` + show `spawned by`.
- **Orphan `bg` sessions** (parent process already dead): `ParentPID` resolves to
  `0`. Show top-level with `[bg]` badge and no `spawned by` line — correct, since
  the parent is genuinely not on the board.
- **Cross-tool lineage** (a Claude session spawning a Codex run, or vice-versa):
  out of scope here; the PPID walk could generalize later since it's PID-based,
  not Claude-specific.

## 🚚 Rollout

1. `domain`: add `Kind` and `ParentPID` to `SessionInfo` (JSON-pure, golden
   invariant 6) with doc comments stating the honesty contract.
2. Claude adapter (`/aitop-enhance claude`): parse `kind`; add the bounded PPID
   walk; populate both fields. `aitop-security-auditor` gate confirms no new
   credential/walk surface.
3. UI (`aitop-ui-integrator`): `kind` badge + `▸ spawned by <parent>` + indent;
   define the grid-view rule.
4. `--demo` roster (`internal/demo/demo.go`): add one interactive root with two
   spawned `bg` children so screenshots/GIFs show the lineage honestly.
5. Docs: README support-table note that lineage is Claude-only today; a known-gap
   paragraph in the Claude `ADAPTER.md` recording *why* in-process Task subagents
   can never be cards (no `sessions/<pid>.json` of their own; sidechains are
   inline `isSidechain:true` lines in the parent's transcript) — stated
   structurally, with no fragile transcript count.
6. Tests: `Kind` parsed from a fake session file; PPID walk resolves a child to
   its tracked parent and an orphan `bg` to `ParentPID:0`, via the fake `Reader`
   / a synthetic process tree — never a real `~/.claude`.

---

<sub>📚 New to aitop RFCs? Read [0001](./0001-agentic-contribution-architecture.md) (why this structure exists) and [0002](./0002-evolving-agentic-structures.md) (how it evolves). RFCs are how the agent structure records its own evolution — welcome aboard. 💛</sub>
