<!--
  👋 Hey, thanks for opening a PR against aitop!
  Fill what applies, delete what doesn't. This template is a friend, not a form.
  (Optional: drop a celebratory/reaction gif right here 🎬 — go on, we love it.)
-->

## 🎯 What's this?

<!-- One or two sentences. What does this PR do, and why? -->

## 🏷️ Type

<!-- Tick all that fit -->

- [ ] 🔌 New Source adapter (a tool aitop didn't cover yet)
- [ ] ✨ Adapter enhancement (turned a `—` into a real value)
- [ ] 🎨 New theme
- [ ] 🐛 Bug fix
- [ ] 📝 Docs / RFC / learning material
- [ ] 🧹 Chore / refactor / CI
- [ ] 🌀 Something else:

## 🕵️ The aitop honesty check (the one that matters)

aitop's whole soul is: **never fabricate a value a tool can't observe.** Missing
data renders `—`, never a fake `0` / `$0.00` / `0/0`.

- [ ] Every field I populate traces to a **real read** — no guessed values
- [ ] Every field I *can't* get renders `—` (`Available:false` / zero+omitted), on purpose
- [ ] If I touched a credential-ish dir: **named-path allowlist**, no `Walk`/`Glob`, + a guard test
- [ ] If I touched SQLite: opened `mode=ro`, queries are exact-key / id-scoped `GLOB` — no `SELECT *`
- [ ] Not applicable (docs/theme/chore) 🙂

## ✅ Ran the gauntlet

```sh
go build ./...
go vet ./...
CGO_ENABLED=0 go test -race ./...   # -race needs cgo locally: CGO_ENABLED=1
```

- [ ] All three green 🟢
- [ ] Docs-only PR — nothing to build (CI skips `*.md` / `docs/**` anyway)

## 📸 Show it off (optional but 💛)

<!--
  Card layout changed? A new theme? A new adapter's card?
  Drop a screenshot or a VHS gif here. See CONTRIBUTING.md for how demo.gif is made.
-->

## 🙋 First time here?

- [ ] I added myself to the **Contributors** list in [`CONTRIBUTING.md`](../CONTRIBUTING.md) 🎉
- [ ] Built a product on top of aitop? Add it to [`docs/BUILT-WITH-AITOP.md`](../docs/BUILT-WITH-AITOP.md) (*from aitop, …*)

## 🔗 Related

<!-- Closes #... / relates to RFC ... / Slack-thread-in-your-head #42 -->

---

<sub>🤖 New to the repo? It's built to be contributed to *by* agents too — try `/aitop-adapter`, `/aitop-theme`, or read [`docs/agentic-orchestration.md`](../docs/agentic-orchestration.md).</sub>
