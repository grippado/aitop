// Package tools renders box 3: the per-tool status strip (installed?,
// running?, session count, oldest session uptime) — read-only, no actions,
// unlike agent-dashboard's state groups which let you approve/reply/merge.
package tools

import (
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/grippado/aitop/internal/domain"
	"github.com/grippado/aitop/internal/ui/theme"
)

func Render(th theme.Theme, snap domain.Snapshot, width, height int, focused bool) string {
	box := th.Box("3:TOOLS", focused, width, height)

	var body string
	for i, t := range snap.Tools {
		dot := "○"
		dotColor := th.Muted
		switch {
		case !t.Installed:
			dot, dotColor = "·", th.Muted
		case t.Running:
			dot, dotColor = "●", th.Good
		case t.Note != "":
			dot, dotColor = "●", th.Warn
		}

		status := fmt.Sprintf("%d sess", t.SessionCount)
		if t.OldestSessionSec > 0 {
			status += "  oldest " + formatDuration(t.OldestSessionSec)
		}
		installed := "installed✓"
		if !t.Installed {
			installed = "not found"
		}
		note := ""
		if t.Note != "" {
			note = "  " + t.Note
		}

		dotStyled := lipgloss.NewStyle().Foreground(dotColor).Render(dot)
		body += fmt.Sprintf("[%d] %-12s %s %-24s %s%s\n", i+1, t.Tool, dotStyled, status, installed, note)
	}

	return box.Render(body)
}

func formatDuration(sec float64) string {
	d := time.Duration(sec) * time.Second
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh%02dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}
