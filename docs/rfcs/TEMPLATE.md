<!--
  📜 aitop RFC template
  Copy me to docs/rfcs/NNNN-<kebab-slug>.md (NNNN = next free number, zero-padded).
  Delete these comments and any section that genuinely doesn't apply.
  Tip: run `/aitop-rfc <subject>` and an agent fills most of this from the real code. 🤖
-->

# RFC NNNN — <Snappy Title Here>

- Status: 🌱 Draft   <!-- 🌱 Draft · 🔎 In review · ✅ Accepted · 🚚 Shipped · ⚰️ Superseded -->
- Type: adapter · feature · process
- Author: [@you](https://github.com/you)

## 🪄 Summary

<!-- One tight paragraph. If you can't summarize it in a tweet, the RFC isn't ready. -->

## 🤔 Motivation

<!-- What's missing / broken / annoying today? Why is it worth the maintenance surface? -->

## 🕵️ Data provenance   <!-- adapter RFCs only — delete for feature/process RFCs -->

The most important table in any adapter RFC. Every `—` is a **promise** the
implementation must keep: never upgrade a `—` to ✅ without a real on-disk source.

| domain field | available? | source on disk | notes |
|---|---|---|---|
| Model | ✅ | `<path/key>` | … |
| Title | ✅ / — | native field / synthesized | … |
| Context % / tokens | ✅ / — | `<source>` | e.g. "tool exposes no window size" |
| Cost / rate limits | — | — | e.g. "billing is cloud-side / API-key based" |
| Last action | ✅ / — | `<source>` | … |

## 🗄️ Storage & mechanism

<!-- Plain files (→ the package Reader) or SQLite (→ database/sql + modernc.org/sqlite, pure Go)?
     If SQLite: the exact query shape — exact-key lookup / id-scoped GLOB. Never SELECT *. -->

## 🔐 Credential surface

<!-- Any credential/API-key file in the tool's dir? Name it, exclude it via a named-path
     allowlist (never Walk/Glob), and note the guard test that will enforce it. Or "none". -->

## 🧭 Alternatives considered

<!-- What else did you weigh, and why not? Honesty here saves a reviewer's time. -->

## ❓ Open questions

<!-- The stuff you're genuinely unsure about. It's fine — that's what the RFC is for. -->

## 🚚 Rollout

<!-- Which agents/commands build it (usually /aitop-adapter)? Test plan? README row?
     Anything that has to happen in a specific order? -->

---

<sub>📚 New to aitop RFCs? Read [0001](./0001-agentic-contribution-architecture.md) (why this structure exists) and [0002](./0002-evolving-agentic-structures.md) (how it evolves). RFCs are how the agent structure records its own evolution — welcome aboard. 💛</sub>
