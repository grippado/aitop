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

// Wrap greedily word-wraps text into at most maxLines lines, each at most
// width runes wide. If the text doesn't fit in maxLines, the last line is
// truncated with "…". Returns fewer than maxLines lines (never padded)
// when the text is short — callers pad to their own layout's line count.
func Wrap(text string, width, maxLines int) []string {
	if width < 1 {
		width = 1
	}
	if maxLines < 1 {
		maxLines = 1
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}

	var lines []string
	cur := ""
	for i := 0; i < len(words); i++ {
		w := words[i]
		candidate := w
		if cur != "" {
			candidate = cur + " " + w
		}
		if lipgloss.Width(candidate) <= width {
			cur = candidate
			continue
		}
		if cur != "" {
			lines = append(lines, cur)
		}
		cur = w
		if len(lines) == maxLines {
			break
		}
	}
	if cur != "" && len(lines) < maxLines {
		lines = append(lines, cur)
	}

	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}

	// Did everything fit? Rebuild the consumed word count to check for
	// leftover text needing an ellipsis on the last line.
	consumed := 0
	for _, l := range lines {
		consumed += len(strings.Fields(l))
	}
	if consumed < len(words) && len(lines) > 0 {
		last := lines[len(lines)-1]
		r := []rune(last)
		if len(r) > width-1 {
			r = r[:width-1]
		}
		lines[len(lines)-1] = string(r) + "…"
	}

	return lines
}
