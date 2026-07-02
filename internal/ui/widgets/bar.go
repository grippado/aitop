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

// SegmentedBar renders a fill bar like Bar, but with a highlighted leading
// sub-segment inside the filled portion — e.g. "how much of used memory
// is attributable to tracked AI-agent processes" drawn in its own color,
// followed by the rest of the used portion in the base color, then empty
// space. total and highlight are both on the same 0-100 scale (highlight
// is a subset of total, not additional on top of it).
func SegmentedBar(total, highlight float64, width int, baseColor, highlightColor lipgloss.Color) string {
	if width < 1 {
		width = 1
	}
	total = clampPct(total)
	highlight = clampPct(highlight)
	if highlight > total {
		highlight = total
	}

	totalFilled := int(total / 100 * float64(width))
	if totalFilled > width {
		totalFilled = width
	}
	highlightWidth := int(highlight / 100 * float64(width))
	if highlightWidth > totalFilled {
		highlightWidth = totalFilled
	}
	restWidth := totalFilled - highlightWidth
	emptyWidth := width - totalFilled

	highlighted := lipgloss.NewStyle().Foreground(highlightColor).Render(strings.Repeat("|", highlightWidth))
	rest := lipgloss.NewStyle().Foreground(baseColor).Render(strings.Repeat("|", restWidth))
	return "[" + highlighted + rest + strings.Repeat(" ", emptyWidth) + "]"
}

func clampPct(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

// Dash renders "—" for unavailable/missing data instead of a misleading 0.
const Dash = "—"

// TruncateRight keeps the tail of s, prefixing "…" when it had to cut —
// used wherever the most specific part of a string (a path's deepest
// folder, the end of a sentence) matters more than its start.
func TruncateRight(s string, max int) string {
	if max < 1 {
		return ""
	}
	if lipgloss.Width(s) <= max {
		return s
	}
	if max <= 1 {
		return "…"
	}
	runes := []rune(s)
	keep := max - 1
	if keep > len(runes) {
		keep = len(runes)
	}
	return "…" + string(runes[len(runes)-keep:])
}

// Wrap greedily word-wraps text into at most maxLines lines, each at most
// width runes wide. If the text doesn't fit in maxLines, the last line is
// truncated with "…". Returns fewer than maxLines lines (never padded)
// when the text is short — callers pad to their own layout's line count.
//
// A single "word" wider than width on its own (an unbroken file path with
// no spaces, e.g. "/Users/.../transcript.go") is hard-broken into
// width-sized chunks first — without this, that one word would be
// emitted wider than the target width and overflow past the caller's
// box/column, which is exactly what happened in narrow grid-layout cards.
func Wrap(text string, width, maxLines int) []string {
	if width < 1 {
		width = 1
	}
	if maxLines < 1 {
		maxLines = 1
	}

	chunks := chunkWords(strings.Fields(text), width)
	if len(chunks) == 0 {
		return nil
	}

	var lines []string
	cur := ""
	idx := 0
	for idx < len(chunks) {
		w := chunks[idx]
		candidate := w
		if cur != "" {
			candidate = cur + " " + w
		}
		if lipgloss.Width(candidate) <= width {
			cur = candidate
			idx++
			continue
		}
		if cur == "" {
			// Shouldn't happen post-chunking unless width is absurdly
			// small, but never emit nothing for a non-empty chunk.
			cur = w
			idx++
		}
		lines = append(lines, cur)
		cur = ""
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

	if idx < len(chunks) && len(lines) > 0 {
		last := lines[len(lines)-1]
		r := []rune(last)
		if len(r) > width-1 {
			r = r[:width-1]
		}
		lines[len(lines)-1] = string(r) + "…"
	}

	return lines
}

// chunkWords splits any word wider than width into width-sized pieces (a
// hard break), so the greedy line-packer in Wrap never has to place a
// single chunk that's wider than the target width.
func chunkWords(words []string, width int) []string {
	var out []string
	for _, w := range words {
		if lipgloss.Width(w) <= width {
			out = append(out, w)
			continue
		}
		r := []rune(w)
		for len(r) > 0 {
			take := width
			if take > len(r) {
				take = len(r)
			}
			out = append(out, string(r[:take]))
			r = r[take:]
		}
	}
	return out
}
