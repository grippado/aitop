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

// CardHeight is the exact line count of every RenderCard's output — fixed
// regardless of content (title 1 + rule 1 + mid 6 (actionLines) + rule 1 +
// pills 1 + context 1 + expanded 1 = 12 content lines, + 2 for the rounded
// border; Padding(0, 1) is horizontal-only, so it adds no vertical lines).
// Confirmed empirically, not just derived: a dedicated test renders a real
// card and counts newlines. The UI layer (model.go) relies on this to
// compute which card row is on screen for vertical scrolling — it must be
// kept in sync if RenderCard's structure changes.
const CardHeight = 14

// RenderList stacks cards full-width, one per row — the guaranteed layout
// (v1's default). selected is an index into cs. Every card renders its
// usage detail expanded — there is no collapsed mode.
func RenderList(th theme.Theme, cs []Card, selected int, width int) string {
	var blocks []string
	for i, c := range cs {
		pad := c.Depth * indentPerDepth
		block := RenderCard(th, c, width-pad, i == selected)
		if pad > 0 {
			block = indentBlock(th, block, pad)
		}
		blocks = append(blocks, block)
	}
	return lipgloss.JoinVertical(lipgloss.Left, blocks...)
}

// indentPerDepth is the left gutter, in columns, a spawned child is inset
// per nesting level in LIST layout — enough to read as "under" its parent
// without stealing card width.
const indentPerDepth = 3

// indentBlock left-pads every line of a rendered card by pad columns, with
// a dim vertical guide in the first column so a nested (spawned) child reads
// as belonging to the card above it. It adds no LINES — height stays
// CardHeight — so model.go's scroll math (row * CardHeight) is untouched;
// the card itself was already rendered pad columns narrower to keep the
// total width constant.
func indentBlock(th theme.Theme, block string, pad int) string {
	if pad < 1 {
		return block
	}
	guide := lipgloss.NewStyle().Foreground(th.Border).Render("│")
	prefix := guide + strings.Repeat(" ", pad-1)
	lines := strings.Split(block, "\n")
	for i, ln := range lines {
		lines[i] = prefix + ln
	}
	return strings.Join(lines, "\n")
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
// cleaner): a header (always a title — the real one, or a computed
// fallback, see renderTitle), a body (a narrow gutter — tokens, then the
// state badge below them, as its own small chip rather than sharing a
// line with the action text — a vertical divider, and last-action text
// using the entire remaining height), and a footer (the tool/model/cwd
// pill, the context-window reading, and the 5h/7d usage detail) — there
// is no collapsed mode, every card always shows all three.
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

	// Last action gets the full 6-line body height to itself now that
	// neither the context reading (moved to the footer) nor the state
	// badge (moved into the gutter, below) shares this block — room
	// enough to actually read a wrapped tool call or thinking snippet,
	// not just its first couple of words.
	const actionLines = 6
	totalLines := actionLines

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
	lines = append(lines, renderTitle(th, c, textWidth))
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

// renderGutter draws the narrow left column: the token counts, then a
// blank line, then the state badge as its own small chip — moved down
// here (out of the body, where it used to share a line with the last-
// action text) so the badge stays a glanceable, fixed-position "is this
// thing alive" signal regardless of how much action text a card has.
func renderGutter(th theme.Theme, c Card) []string {
	label := lipgloss.NewStyle().Foreground(th.Muted).Render("Tokens:")
	badge, badgeColor := stateBadge(c)
	badgeStyled := lipgloss.NewStyle().Foreground(badgeColor).Render(badge)

	var lines []string
	if !c.HasTokens {
		lines = []string{label, " " + widgets.Dash}
	} else {
		inStyle := lipgloss.NewStyle().Foreground(th.TokenIn)
		outStyle := lipgloss.NewStyle().Foreground(th.TokenOut)
		lines = []string{
			label,
			" " + inStyle.Render("IN ↑"+formatTokens(c.TokensIn)),
			" " + outStyle.Render("OUT↓"+formatTokens(c.TokensOut)),
		}
	}
	return append(lines, "", badgeStyled)
}

// renderBody draws the last session action, read from the session's own
// transcript when the adapter supports it — word-wrapped across the
// body's full actionLines height (no longer sharing a line with the state
// badge, which moved into the gutter — see renderGutter), never a
// fabricated activity string. "—" when the adapter has no such source
// (Codex, Cursor without a live composer match).
func renderBody(c Card, width int, actionLines int) []string {
	var lines []string
	if c.LastAction != "" {
		lines = widgets.Wrap(c.LastAction, width, actionLines)
	}
	if len(lines) == 0 {
		lines = []string{widgets.Dash}
	}
	for len(lines) < actionLines {
		lines = append(lines, "")
	}
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

// renderContextLine draws "Context: [bar] USED/TOTAL (PCT%)" when the
// card has a real token count backing that ratio, or just "Context: [bar]
// (PCT%)" when it doesn't. TOTAL isn't stored on Card directly — when
// shown, it's derived from TokensIn/ContextPct (TokensIn = TOTAL *
// ContextPct/100), which keeps this UI-layer function ignorant of any
// specific model's window size instead of hardcoding one. That derivation
// only holds when ContextPct was itself computed from TokensIn in the
// first place (true for Claude/Codex/opencode) — Cursor's ContextPct
// comes from its own independent reading (Cursor's own
// contextUsagePercent field) with no guaranteed TokensIn to match, so
// showing "0/0" there would be a fabricated ratio, not a real one; c.
// HasTokens is what distinguishes the two cases.
func renderContextLine(th theme.Theme, c Card, width int) string {
	label := "Context: "
	var suffix string
	if c.HasTokens {
		total := int64(float64(c.TokensIn) * 100 / c.ContextPct)
		suffix = fmt.Sprintf(" %s/%s (%s)", formatTokens(c.TokensIn), formatTokens(total), widgets.PctLabel(c.ContextPct))
	} else {
		suffix = fmt.Sprintf(" (%s)", widgets.PctLabel(c.ContextPct))
	}

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
// title. Every card gets a header line, always: a blank gap up top read
// as a rendering bug in practice (confirmed against real feedback: a live
// Claude Code session with no ai-title yet just looked broken, not
// "honestly missing data" the way a "—" elsewhere in the card does).
// Real titles render bold/accented; a computed fallbackTitle renders
// plain/muted instead, so it's visually obvious which one a card has —
// still true information (derived from the session's own CWD), never a
// fabricated summary standing in for the real thing.
func renderTitle(th theme.Theme, c Card, width int) string {
	// The RFC 0003 lineage segment (kind badge + provenance) rides on the
	// right of the header line rather than a line of its own — every card is
	// a fixed CardHeight, and a new line would break the scroll math in
	// model.go. The title keeps the left, truncated to whatever the lineage
	// doesn't claim (at least minTitle columns).
	const minTitle = 10
	lineage := renderLineage(th, c, width-minTitle-1)

	titleBudget := width
	if lineage != "" {
		titleBudget = width - lipgloss.Width(lineage) - 1 // 1-col gap
	}
	var title string
	if c.Title != "" {
		title = lipgloss.NewStyle().Bold(true).Foreground(th.Accent).Render(widgets.TruncateRight(c.Title, titleBudget))
	} else {
		title = lipgloss.NewStyle().Foreground(th.Muted).Render(widgets.TruncateRight(fallbackTitle(c), titleBudget))
	}
	if lineage == "" {
		return title
	}
	gap := width - lipgloss.Width(title) - lipgloss.Width(lineage)
	if gap < 1 {
		gap = 1
	}
	return title + strings.Repeat(" ", gap) + lineage
}

// renderLineage builds the dim header suffix that surfaces RFC 0003 session
// lineage: a kind badge ("[bg]" / "[interactive]") when the tool reports a
// kind, and a provenance note for a spawned child. Honest by construction —
// the badge shows only when Kind is non-empty, and "▸ spawned by <parent>"
// only when the parent actually resolved to a card on the board; otherwise
// "▸ spawned (parent not on board)", never a fabricated parent name. Returns
// "" when there's nothing to say (no kind, no parent) so the title keeps the
// whole width and no blank badge appears. Head-truncates (keeping the badge)
// when it can't fit max columns, since the LIST indent already carries the
// nesting signal.
func renderLineage(th theme.Theme, c Card, max int) string {
	if max < 3 {
		return ""
	}
	var parts []string
	if c.Kind != "" {
		parts = append(parts, "["+c.Kind+"]")
	}
	if c.ParentPID != 0 {
		if c.ParentLabel != "" {
			parts = append(parts, "▸ spawned by "+c.ParentLabel)
		} else {
			parts = append(parts, "▸ spawned (parent not on board)")
		}
	}
	if len(parts) == 0 {
		return ""
	}
	s := strings.Join(parts, " ")
	if r := []rune(s); lipgloss.Width(s) > max {
		s = string(r[:max-1]) + "…"
	}
	return lipgloss.NewStyle().Foreground(th.Muted).Render(s)
}

// fallbackTitle computes a placeholder header for a session whose adapter
// has no real title (Codex before its first genuine user message, Cursor
// IDE with no transcript source at all today). Prefers the project
// directory name — the most useful "what is this session actually on"
// signal short of a real title — falling back further to a bare
// "<tool> session" when even CWD is unavailable, so this never returns "".
func fallbackTitle(c Card) string {
	if proj := lastPathSegment(c.CWD); proj != "" {
		return proj
	}
	return friendlyTool(c.Tool) + " session"
}

// lastPathSegment returns the final component of a filesystem path, e.g.
// "/Users/demo/www/isaac/backoffice" -> "backoffice". "" for an empty
// path or one that's only slashes.
func lastPathSegment(path string) string {
	path = strings.TrimRight(path, "/")
	if path == "" {
		return ""
	}
	i := strings.LastIndexByte(path, '/')
	if i < 0 {
		return path
	}
	return path[i+1:]
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
