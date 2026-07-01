// Package widgets holds small rendering helpers shared across panes.
package widgets

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Bar renders a fixed-width ASCII gauge bar filled to pct (0-100), colored
// via colorFn(pct), e.g. theme.Theme.GaugeColor.
func Bar(pct float64, width int, colorFn func(float64) lipgloss.Color) string {
	if width < 1 {
		width = 1
	}
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	filled := int(pct / 100 * float64(width))
	if filled > width {
		filled = width
	}
	bar := strings.Repeat("|", filled) + strings.Repeat(" ", width-filled)
	style := lipgloss.NewStyle().Foreground(colorFn(pct))
	return style.Render("[" + bar + "]")
}

// PctLabel formats a percentage consistently, e.g. "42%".
func PctLabel(pct float64) string {
	return fmt.Sprintf("%3.0f%%", pct)
}

// Dash renders "—" for unavailable/missing data instead of a misleading 0.
const Dash = "—"
