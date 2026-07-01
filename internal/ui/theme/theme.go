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

	Background lipgloss.Color
	Border     lipgloss.Color
	BorderFocus lipgloss.Color
	Text       lipgloss.Color
	Muted      lipgloss.Color

	Good   lipgloss.Color // low load / healthy / running
	Warn   lipgloss.Color // medium load
	Bad    lipgloss.Color // high load / error / stale
	Accent lipgloss.Color // headline highlights, selection
}

// BtopClassic mirrors real btop's default palette (green/yellow/red gauge
// gradient on black) so aitop reads as "system monitor" on first glance,
// not as a cost dashboard borrowing btop's colors.
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
