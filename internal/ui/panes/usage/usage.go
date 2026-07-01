// Package usage renders box 5: a single, deliberately understated line of
// cost/token/rate-limit data. This is the inverse of bugkill3r/aitop's
// cost-first hierarchy — here it's a footnote, not the headline.
package usage

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/grippado/aitop/internal/domain"
	"github.com/grippado/aitop/internal/ui/theme"
	"github.com/grippado/aitop/internal/ui/widgets"
)

// Render draws the collapsed (default) or expanded ('u' key) usage line.
func Render(th theme.Theme, snap domain.Snapshot, expanded bool, width, height int, focused bool) string {
	box := th.Box("5:USAGE & COST", focused, width, height)
	dim := th.Muted

	var parts []string
	for _, u := range snap.Usage {
		if !u.Available {
			parts = append(parts, fmt.Sprintf("%s: %s", u.Tool, widgets.Dash))
			continue
		}
		seg := fmt.Sprintf("%s: $%.2f today", u.Tool, u.CostTodayUSD)
		if u.ContextUsedPct > 0 {
			seg += fmt.Sprintf(" · ctx %.0f%%", u.ContextUsedPct)
		}
		if u.LimitWeekly != nil {
			seg += fmt.Sprintf(" · wk lim %.0f%%", *u.LimitWeekly)
		}
		if expanded {
			seg += fmt.Sprintf(" · %dk in / %dk out tok", u.TokensIn/1000, u.TokensOut/1000)
			if u.LimitFiveHour != nil {
				seg += fmt.Sprintf(" · 5h lim %.0f%%", *u.LimitFiveHour)
			}
			seg += fmt.Sprintf(" · month $%.2f", u.CostMonthUSD)
		}
		parts = append(parts, seg)
	}

	line := strings.Join(parts, "  |  ")
	styled := lipgloss.NewStyle().Foreground(dim).Render(line)
	return box.Render(styled)
}
