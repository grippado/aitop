package cards

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/grippado/aitop/internal/ui/theme"
	"github.com/grippado/aitop/internal/ui/widgets"
)

const gutterWidth = 11

// RenderList stacks cards full-width, one per row — the guaranteed layout
// (v1's default). selected is an index into cs. Every card renders its
// usage detail expanded — there is no collapsed mode.
func RenderList(th theme.Theme, cs []Card, selected int, width int) string {
	var blocks []string
	for i, c := range cs {
		blocks = append(blocks, RenderCard(th, c, width, i == selected))
	}
	return lipgloss.JoinVertical(lipgloss.Left, blocks...)
}

// RenderGrid packs cards two-per-row, half width each — best-effort layout
// toggled via 'v'. Falls back gracefully to a ragged last row when the
// count is odd.
func RenderGrid(th theme.Theme, cs []Card, selected int, width int) string {
	colWidth := width/2 - 1
	if colWidth < 24 {
		// Not enough room for two columns — list mode is the honest
		// answer here rather than a mangled grid.
		return RenderList(th, cs, selected, width)
	}

	var rows []string
	for i := 0; i < len(cs); i += 2 {
		left := RenderCard(th, cs[i], colWidth, i == selected)
		if i+1 < len(cs) {
			right := RenderCard(th, cs[i+1], colWidth, i+1 == selected)
			rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right))
		} else {
			rows = append(rows, left)
		}
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// RenderCard draws one card in three vertically stacked zones, each
// directly abutting the next full-width rule with no blank-line padding
// (tried once with breathing room around the rules, but tighter reads
// cleaner): a header (the title, if any), a body (a narrow token gutter, a
// vertical divider, and the dominant state badge + last-action text), and
// a footer (the tool/model/cwd pill, the context-window reading, and the
// 5h/7d usage detail) — there is no collapsed mode, every card always
// shows all three.
//
// lipgloss width arithmetic (verified empirically, not assumed): the final
// rendered block's total width = Style.Width(n) + 2 (the rounded border,
// one column each side). Padding is spent OUT OF n, not added on top. So
// to make the final card exactly `width` columns wide: the value handed
// to .Width() is `width - 2` (styleWidth), and the actual text budget
// available to content lines is `styleWidth - 2` for Padding(0, 1), i.e.
// `width - 4` (textWidth).
func RenderCard(th theme.Theme, c Card, width int, selected bool) string {
	borderColor := th.ToolColor(c.Tool)
	if selected {
		borderColor = th.Accent
	}

	styleWidth := width - 2
	if styleWidth < 4 {
		styleWidth = 4
	}
	textWidth := styleWidth - 2
	if textWidth < 20 {
		textWidth = 20
	}
	bodyWidth := textWidth - gutterWidth - 2 // divider "│" + 1 space
	if bodyWidth < 10 {
		bodyWidth = 10
	}

	// Last action gets 5 lines now that the context reading no longer
	// shares this block (it moved to the footer, below) — room enough to
	// actually read a wrapped tool call or thinking snippet, not just its
	// first couple of words. Dominant (the state badge) stays 1 line.
	const actionLines = 5
	totalLines := 1 + actionLines

	gutter := renderGutter(th, c)
	body := renderBody(c, bodyWidth, actionLines)

	gutterLines := padLines(gutter, totalLines)
	bodyLines := padLines(body, totalLines)

	var mid []string
	divider := lipgloss.NewStyle().Foreground(th.Border).Render("│")
	for i := range gutterLines {
		mid = append(mid, lipgloss.NewStyle().Width(gutterWidth).Render(gutterLines[i])+divider+" "+bodyLines[i])
	}

	rule := lipgloss.NewStyle().Foreground(th.Border).Render(strings.Repeat("─", textWidth))

	var lines []string
	if title := renderTitle(th, c, textWidth); title != "" {
		lines = append(lines, title)
	}
	lines = append(lines, rule)
	lines = append(lines, mid...)
	lines = append(lines, rule)
	lines = append(lines, renderPills(c, textWidth))
	lines = append(lines, renderContextOrFallback(th, c, textWidth))

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	content = lipgloss.JoinVertical(lipgloss.Left, content, renderExpanded(th, c))

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(0, 1).
		Width(styleWidth).
		Render(content)
}

func renderGutter(th theme.Theme, c Card) []string {
	label := lipgloss.NewStyle().Foreground(th.Muted).Render("Tokens:")
	if !c.HasTokens {
		return []string{label, " " + widgets.Dash, ""}
	}
	inStyle := lipgloss.NewStyle().Foreground(th.TokenIn)
	outStyle := lipgloss.NewStyle().Foreground(th.TokenOut)
	return []string{
		label,
		" " + inStyle.Render("IN ↑"+formatTokens(c.TokensIn)),
		" " + outStyle.Render("OUT↓"+formatTokens(c.TokensOut)),
	}
}

func renderBody(c Card, width int, actionLines int) []string {
	badge, badgeColor := stateBadge(c)
	badgeStyled := lipgloss.NewStyle().Foreground(badgeColor).Render(badge)

	// Dominant: the state badge, alone. Context/tokens used to be crammed
	// onto this line too (a bar sometimes, raw "234k ctx" text other
	// times depending on whether the pct was reliable) — redundant with
	// the token gutter's IN figure (the same contextTokens() sum) and
	// inconsistent card-to-card. That richer context reading lives in the
	// footer instead now — see renderContextOrFallback.
	dominant := badgeStyled

	// Secondary: last session action, read from the session's own
	// transcript when the adapter supports it (Claude Code today) —
	// word-wrapped across actionLines lines, never a fabricated activity
	// string. "—" when the adapter has no such source (Codex, Cursor).
	var secondary []string
	if c.LastAction != "" {
		secondary = widgets.Wrap(c.LastAction, width, actionLines)
	}
	if len(secondary) == 0 {
		secondary = []string{widgets.Dash}
	}
	for len(secondary) < actionLines {
		secondary = append(secondary, "")
	}

	lines := []string{dominant}
	lines = append(lines, secondary...)
	return lines
}

// renderContextOrFallback draws the card's footer context-window reading
// — "Context: [bar] USED/TOTAL (PCT%)" — below the tool/cwd pill. Falls
// back to branch/dirty (whatever the ~{PWD} pill doesn't already cover)
// when context isn't available, then a bare dash. Branch/dirty is
// unpopulated by any adapter today, so in practice this line is context
// or nothing right now — but the fallback chain is what keeps that a
// "today" fact, not a hardcoded assumption.
func renderContextOrFallback(th theme.Theme, c Card, width int) string {
	switch {
	case c.HasContext:
		return renderContextLine(th, c, width)
	case c.Branch != "":
		dirty := ""
		if c.Dirty {
			dirty = "*"
		}
		return c.Branch + dirty
	default:
		return widgets.Dash
	}
}

// renderContextLine draws "Context: [bar] USED/TOTAL (PCT%)". TOTAL isn't
// stored on Card directly — it's derived from TokensIn/ContextPct
// (TokensIn = TOTAL * ContextPct/100), which keeps this UI-layer function
// ignorant of any specific model's window size instead of hardcoding one.
func renderContextLine(th theme.Theme, c Card, width int) string {
	label := "Context: "
	total := int64(float64(c.TokensIn) * 100 / c.ContextPct)
	suffix := fmt.Sprintf(" %s/%s (%s)", formatTokens(c.TokensIn), formatTokens(total), widgets.PctLabel(c.ContextPct))

	barWidth := width - lipgloss.Width(label) - lipgloss.Width(suffix) - 2 // Bar()'s own "[" "]"
	if barWidth < 6 {
		barWidth = 6
	}
	return label + widgets.Bar(c.ContextPct, barWidth, th.GaugeColor) + suffix
}

func stateBadge(c Card) (string, lipgloss.Color) {
	switch c.Status {
	case "busy":
		return "● running", lipgloss.Color("2")
	case "idle":
		return "◌ idle", lipgloss.Color("245")
	default:
		// "◍ thinking" is reserved for a future, more granular session
		// state no current adapter distinguishes from plain "busy" —
		// not fabricated here.
		return "◌ unknown", lipgloss.Color("245")
	}
}

// renderTitle draws the card's header line — a short descriptive label
// for what the session is working on (e.g. Claude Code's own
// auto-generated session title), the analog of mutirao's per-mão task
// title. Returns "" when the adapter has no such source (Codex, Cursor
// today), in which case RenderCard omits the line entirely rather than
// leaving a blank header.
func renderTitle(th theme.Theme, c Card, width int) string {
	if c.Title == "" {
		return ""
	}
	return lipgloss.NewStyle().Bold(true).Foreground(th.Accent).Render(widgets.TruncateRight(c.Title, width))
}

// renderPills builds the card's footer line — left pill for tool identity,
// right pill for ~{PWD} — truncated so the combined line never exceeds
// width and wraps onto a second line inside the card.
func renderPills(c Card, width int) string {
	left := friendlyTool(c.Tool)
	if c.Model != "" {
		left += " (" + c.Model + ")"
	}
	leftPill := " " + left + " "

	right := shortenHome(c.CWD)
	if right == "" {
		right = widgets.Dash
	}

	minGap := 1
	budget := width - lipgloss.Width(leftPill) - minGap
	if budget < 4 {
		// Not enough room for any meaningful path — drop the right pill
		// entirely rather than corrupt the line.
		return widgets.TruncateRight(leftPill, width)
	}
	rightPill := " " + widgets.TruncateRight(right, budget-2) + " " // -2 for the pill's own spaces

	gap := width - lipgloss.Width(leftPill) - lipgloss.Width(rightPill)
	if gap < minGap {
		gap = minGap
	}
	return leftPill + strings.Repeat(" ", gap) + rightPill
}

// renderExpanded draws the card's always-visible usage detail line (5h/7d
// limits + process summary). Cost (today/month $) was dropped from here: on this
// adapter's actual data source, the cost-day file mechanism is dead (no
// file has been written in weeks — see claude/usage.go), so it always
// rendered "$0.00 · $0.00" — dead weight, not a real reading, not worth
// the line space. c.CostTodayUSD/CostMonthUSD stay on Card in case a
// future source makes cost real again; this function just stops
// displaying them until there's something to show.
func renderExpanded(th theme.Theme, c Card) string {
	muted := lipgloss.NewStyle().Foreground(th.Muted)
	if !c.HasCost {
		return muted.Render("usage: " + widgets.Dash)
	}
	limit5, limit7 := widgets.Dash, widgets.Dash
	if c.LimitFiveHour != nil {
		limit5 = fmt.Sprintf("%.0f%%", *c.LimitFiveHour)
	}
	if c.LimitWeekly != nil {
		limit7 = fmt.Sprintf("%.0f%%", *c.LimitWeekly)
	}
	procs := widgets.Dash
	if c.ProcCount > 0 {
		procs = fmt.Sprintf("%d procs, %.0f%% CPU summed", c.ProcCount, c.ProcCPUSum)
	}
	return muted.Render(fmt.Sprintf("5h %s · 7d %s · %s", limit5, limit7, procs))
}

func friendlyTool(tool string) string {
	switch tool {
	case "claude-code":
		return "claude code"
	case "codex":
		return "codex"
	case "cursor":
		return "cursor"
	case "cursor-agent":
		return "cursor agent"
	case "opencode":
		return "opencode"
	default:
		return strings.TrimPrefix(tool, "unknown:")
	}
}

func formatTokens(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1000:
		return fmt.Sprintf("%.0fk", float64(n)/1000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

func shortenHome(path string) string {
	if path == "" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err == nil && strings.HasPrefix(path, home) {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}

func padLines(lines []string, n int) []string {
	for len(lines) < n {
		lines = append(lines, "")
	}
	return lines
}

// AgeString renders AgeSec as a human duration, matching the old process
// table's format (used by SortAge's UI label / any future detail view).
func AgeString(sec float64) string {
	if sec <= 0 {
		return widgets.Dash
	}
	d := time.Duration(sec) * time.Second
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh%02dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}
