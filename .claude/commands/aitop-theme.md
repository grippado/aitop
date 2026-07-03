---
description: Add a new color theme — the lowest-barrier contribution to aitop. One self-contained change in internal/ui/theme/theme.go.
argument-hint: <theme-name> [palette notes / inspiration]
---

# /aitop-theme

You are adding a new `Theme` value. This is deliberately the easiest way to
contribute to aitop: all of its colors live in one struct, so a theme is a
small, self-contained PR that touches no adapter logic.

**Theme:** `$ARGUMENTS`

## What you're touching (and what you're NOT)

- **Touch:** `internal/ui/theme/theme.go` — add one `Theme` value next to
  `BtopClassic`.
- **Do NOT touch:** adapters, the collector, the domain contract, or the render
  logic. If your theme needs a new color, that's a `Theme` *field* addition
  (which every theme then sets) — flag it, don't hardcode a color in a pane.

## The `Theme` struct (read it first)

`Theme` groups every semantic color the UI draws with. Two groups matter most:

- **Gauge/threshold** (`Good`/`Warn`/`Bad`) drive `GaugeColor(pct)` — btop's
  convention is `<50` good, `<80` warn, else bad. Keep that legibility: green→
  yellow→red readability is the point, whatever your palette.
- **Per-tool identity** (`ToolClaude`, `ToolCodex`, `ToolCursor`,
  `ToolCursorAgent`, `ToolOpencode`, `ToolUnknown`) color each card's border so
  a board of many cards is glanceable. Keep the five tool colors visually
  distinct from each other **and** from `Good`/`Warn`/`Bad`, or the board
  turns to mush. (Note Cursor's light-purple vs cursor-agent's pink are
  intentionally different — don't collapse them.)

## Steps

### 1. Add the `Theme` value
Add `var <Name> = Theme{ ... }` in `theme.go`, setting **every** field (a
zero `lipgloss.Color` renders as default terminal color and will look broken).
Give it a `Name`. Use 256-color codes or hex as the existing theme does.

### 2. Sanity-check contrast
- Tool colors mutually distinct and distinct from gauge colors.
- `Good`/`Warn`/`Bad` still read as a green→yellow→red progression.
- `Muted` and `Text` are legible on `Background`.

### 3. Note the v1/v2 selector caveat
v1 ships exactly one theme and has **no** `--theme` flag or cycle key — wiring a
selector is a tracked v2 item. Your PR only needs to contribute the palette; it
does **not** have to build the selector. Say so in the PR description so a
reviewer doesn't ask "how do I switch to it?"

### 4. Verify
```sh
go build ./... && go vet ./... && CGO_ENABLED=0 go test ./...
```
Optionally preview against synthetic cards:
```sh
go build -o /tmp/aitop ./cmd/aitop && /tmp/aitop --demo
```
(You can't switch themes at runtime in v1; to eyeball a new one, temporarily
point `Default()` at it locally — do not commit that change.)

### 5. Summarize
Report the added theme and the contrast check. Offer a PR; do not open one
unprompted.

## Definition of done
- One new fully-populated `Theme` value in `theme.go`; nothing else changed.
- Contrast sanity holds (tool colors distinct; gauge progression legible).
- Build/vet/test green; PR notes the v2-selector caveat.
