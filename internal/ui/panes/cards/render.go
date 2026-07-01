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
// (v1's default). selected is an index into cs; expanded applies only to
// that selected card.
func RenderList(th theme.Theme, cs []Card, selected int, width int, expanded bool) string {
	var blocks []string
	for i, c := range cs {
		blocks = append(blocks, RenderCard(th, c, width, i == selected, i == selected && expanded))
	}
	return lipgloss.JoinVertical(lipgloss.Left, blocks...)
}

// RenderGrid packs cards two-per-row, half width each — best-effort layout
// toggled via 'v'. Falls back gracefully to a ragged last row when the
// count is odd.
func RenderGrid(th theme.Theme, cs []Card, selected int, width int, expanded bool) string {
	colWidth := width/2 - 1
	if colWidth < 24 {
		// Not enough room for two columns — list mode is the honest
		// answer here rather than a mangled grid.
		return RenderList(th, cs, selected, width, expanded)
	}

	var rows []string
	for i := 0; i < len(cs); i += 2 {
		left := RenderCard(th, cs[i], colWidth, i == selected, i == selected && expanded)
		if i+1 < len(cs) {
			right := RenderCard(th, cs[i+1], colWidth, i+1 == selected, i+1 == selected && expanded)
			rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right))
		} else {
			rows = append(rows, left)
		}
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// RenderCard draws one 3-zone card: a narrow token gutter, a vertical
// divider, a dominant/secondary/tertiary body, and a footer pill row.
//
// lipgloss width arithmetic (verified empirically, not assumed): the final
// rendered block's total width = Style.Width(n) + 2 (the rounded border,
// one column each side). Padding is spent OUT OF n, not added on top. So
// to make the final card exactly `width` columns wide: the value handed
// to .Width() is `width - 2` (styleWidth), and the actual text budget
// available to content lines is `styleWidth - 2` for Padding(0, 1), i.e.
// `width - 4` (textWidth).
func RenderCard(th theme.Theme, c Card, width int, selected bool, expanded bool) string {
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

	gutter := renderGutter(th, c)
	body := renderBody(th, c, bodyWidth)

	gutterLines := padLines(gutter, 3)
	bodyLines := padLines(body, 3)

	var mid []string
	divider := lipgloss.NewStyle().Foreground(th.Border).Render("│")
	for i := range gutterLines {
		mid = append(mid, lipgloss.NewStyle().Width(gutterWidth).Render(gutterLines[i])+divider+" "+bodyLines[i])
	}

	pillLine := renderPills(c, textWidth)

	content := lipgloss.JoinVertical(lipgloss.Left, append(mid, "", pillLine)...)
	if expanded {
		content = lipgloss.JoinVertical(lipgloss.Left, content, renderExpanded(th, c))
	}

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

func renderBody(th theme.Theme, c Card, width int) []string {
	badge, badgeColor := stateBadge(c)
	badgeStyled := lipgloss.NewStyle().Foreground(badgeColor).Render(badge)

	// Dominant: state badge + context bar. Exactly one of these two
	// pieces of information dominates the card — this is it.
	var dominant string
	if c.HasContext {
		// Reserve exact overhead: badge, 2 spaces, Bar()'s own "[" "]"
		// brackets, 1 space, and a 4-char pct label ("100%") — len(badge)
		// would undercount by 2 (● and ◌ are 3-byte UTF-8 runes), so this
		// uses display width, not byte length.
		overhead := lipgloss.Width(badge) + 2 + 2 + 1 + 4
		barWidth := width - overhead
		if barWidth < 6 {
			barWidth = 6
		}
		dominant = fmt.Sprintf("%s  %s %s", badgeStyled, widgets.Bar(c.ContextPct, barWidth, th.GaugeColor), widgets.PctLabel(c.ContextPct))
	} else {
		dominant = fmt.Sprintf("%s  ctx %s", badgeStyled, widgets.Dash)
	}

	// Secondary: last session action. No adapter surfaces this yet (it'd
	// need per-turn transcript parsing), so it's honestly always "—" for
	// now rather than a fabricated activity string.
	secondary := widgets.Dash

	// Tertiary: whatever the ~{PWD} footer pill doesn't already cover —
	// branch/dirty state. Also unpopulated by any adapter today.
	tertiary := widgets.Dash
	if c.Branch != "" {
		dirty := ""
		if c.Dirty {
			dirty = "*"
		}
		tertiary = c.Branch + dirty
	}

	return []string{dominant, secondary, tertiary}
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

// renderPills builds the card's footer line — left pill for tool identity,
// right pill for ~{PWD} — truncated so the combined line never exceeds
// width and wraps onto a second line inside the card.
func renderPills(c Card, width int) string {
	left := friendlyTool(c.Tool)
	if c.Model != "" {
		left += " | " + c.Model
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
		return truncateRight(leftPill, width)
	}
	rightPill := " " + truncateRight(right, budget-2) + " " // -2 for the pill's own spaces

	gap := width - lipgloss.Width(leftPill) - lipgloss.Width(rightPill)
	if gap < minGap {
		gap = minGap
	}
	return leftPill + strings.Repeat(" ", gap) + rightPill
}

// truncateRight keeps the tail of s (the most specific part of a path
// tends to be its deepest folder), prefixing "…" when it had to cut.
func truncateRight(s string, max int) string {
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
	return muted.Render(fmt.Sprintf("today $%.2f · month $%.2f · 5h %s · 7d %s · %s", c.CostTodayUSD, c.CostMonthUSD, limit5, limit7, procs))
}

func friendlyTool(tool string) string {
	switch tool {
	case "claude-code":
		return "claude code"
	case "codex":
		return "codex"
	case "cursor":
		return "cursor"
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
