// Package theme centralizes every color aitop's UI uses in a single struct.
// v1 ships exactly one theme (BtopClassic) — no cycling, no persistence,
// no --theme flag. That's a deliberate v1/v2 cut, not an oversight: keeping
// every color behind this one struct is what makes it a single-file change
// to add a new theme later. Community theme PRs are welcome — see
// CONTRIBUTING.md.
package theme

import "github.com/charmbracelet/lipgloss"

// Theme groups every semantic color aitop's panes draw with.
type Theme struct {
	Name string

	Background  lipgloss.Color
	Border      lipgloss.Color
	BorderFocus lipgloss.Color
	Text        lipgloss.Color
	Muted       lipgloss.Color

	Good   lipgloss.Color // low load / healthy / running
	Warn   lipgloss.Color // medium load
	Bad    lipgloss.Color // high load / error / stale
	Accent lipgloss.Color // headline highlights, selection

	// TokenIn/TokenOut color the session token arrows on each agent card.
	TokenIn  lipgloss.Color // "IN ↑" arrow
	TokenOut lipgloss.Color // "OUT ↓" arrow

	// Per-tool identity colors — this is what lets a card's border say
	// "this one's Codex" at a glance across a board of many cards. Keys
	// are the Source.Name() strings used throughout the backend
	// (claude-code, codex, cursor); anything else (fallback/unknown
	// process-name matches) gets ToolUnknown.
	ToolClaude  lipgloss.Color
	ToolCodex   lipgloss.Color
	ToolCursor  lipgloss.Color
	ToolUnknown lipgloss.Color
}

// BtopClassic mirrors real btop's default palette (green/yellow/red gauge
// gradient on black) for gauges/thresholds, with distinct per-tool identity
// colors for the card borders that are now aitop's primary visual language.
var BtopClassic = Theme{
	Name:        "btop-classic",
	Background:  lipgloss.Color("0"),
	Border:      lipgloss.Color("8"),
	BorderFocus: lipgloss.Color("2"),
	Text:        lipgloss.Color("15"),
	Muted:       lipgloss.Color("245"),
	Good:        lipgloss.Color("2"),
	Warn:        lipgloss.Color("3"),
	Bad:         lipgloss.Color("1"),
	Accent:      lipgloss.Color("6"),

	TokenIn:  lipgloss.Color("2"), // green
	TokenOut: lipgloss.Color("1"), // red

	ToolClaude:  lipgloss.Color("209"), // coral
	ToolCodex:   lipgloss.Color("36"),  // teal
	ToolCursor:  lipgloss.Color("141"), // light purple
	ToolUnknown: lipgloss.Color("245"), // same as Muted
}

// Default returns aitop's default (and, in v1, only) theme.
func Default() Theme { return BtopClassic }

// GaugeColor picks Good/Warn/Bad for a 0-100 percentage, matching btop's
// threshold convention (<50 good, <80 warn, else bad).
func (t Theme) GaugeColor(pct float64) lipgloss.Color {
	switch {
	case pct >= 80:
		return t.Bad
	case pct >= 50:
		return t.Warn
	default:
		return t.Good
	}
}

// ToolColor maps a Source.Name() (or a "unknown:<name>" fallback tag) to
// its identity color. Prefix-matches "unknown:" so every fallback-adapter
// process (aider, windsurf, opencode, ...) shares one neutral color until
// each gets a dedicated adapter and its own slot here.
func (t Theme) ToolColor(tool string) lipgloss.Color {
	switch {
	case tool == "claude-code":
		return t.ToolClaude
	case tool == "codex":
		return t.ToolCodex
	case tool == "cursor":
		return t.ToolCursor
	default:
		return t.ToolUnknown
	}
}

// Box returns a lipgloss style for a bordered pane box, highlighted when
// focused.
func (t Theme) Box(title string, focused bool, width, height int) lipgloss.Style {
	border := t.Border
	if focused {
		border = t.BorderFocus
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Foreground(t.Text).
		Width(width).
		Height(height).
		Padding(0, 1)
}
