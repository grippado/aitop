// Package system renders the condensed resource footer — what used to be
// the headline (per-core CPU boxes) is now a secondary, ~6-7 line strip at
// the bottom of the screen. Agents and their contexts are the product now;
// this is just "and here's what it's costing your machine."
package system

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/grippado/aitop/internal/domain"
	"github.com/grippado/aitop/internal/ui/theme"
	"github.com/grippado/aitop/internal/ui/widgets"
)

// RenderFooter draws the whole-system CPU/MEM/NET strip, with an "agents:"
// share annotation on the CPU and MEM lines showing how much of that is
// attributable to tracked AI-tool processes. CPU is a single aggregate
// number here, not per-core bars — the per-core detail that used to be
// aitop's headline is gone on purpose; this pane's job is a one-glance
// footnote, not a system-monitor centerpiece.
func RenderFooter(th theme.Theme, snap domain.Snapshot, width int) string {
	// Bars are deliberately short — this footer's job is a glance, not a
	// centerpiece, and a short bar leaves real room for the trailing
	// figures (pct, MB, agent-share) instead of pushing them past the
	// terminal edge where they'd get clipped.
	barWidth := width / 5
	if barWidth < 10 {
		barWidth = 10
	}
	if barWidth > 24 {
		barWidth = 24
	}

	cpuAvg := average(snap.System.PerCoreCPUPct)
	var agentCPU float64
	for _, p := range snap.Processes {
		agentCPU += p.CPUPct
	}
	cores := len(snap.System.PerCoreCPUPct)
	var agentCPUShare float64
	if cores > 0 {
		agentCPUShare = agentCPU / float64(cores)
	}

	memPct := 0.0
	if snap.System.MemTotalMB > 0 {
		memPct = snap.System.MemUsedMB / snap.System.MemTotalMB * 100
	}
	var agentMem float64
	for _, p := range snap.Processes {
		agentMem += p.MemMB
	}
	var agentMemShare float64
	if snap.System.MemTotalMB > 0 {
		agentMemShare = agentMem / snap.System.MemTotalMB * 100
	}

	warm := ""
	if snap.Warming {
		warm = "  (warming)"
	}

	sep := lipgloss.NewStyle().Foreground(th.Border).Render(strings.Repeat("─", max(1, width)))

	// MEM's bar highlights the agent-attributable slice (harnesses and
	// whatever they invoke) in the accent color, inside the same used-
	// memory fill — not a separate bar, a sub-segment of it.
	line1 := fmt.Sprintf(" SYSTEM   CPU %s %s  (agents: %.0f%% of system)%s",
		widgets.Bar(cpuAvg, barWidth, th.GaugeColor), widgets.PctLabel(cpuAvg), agentCPUShare, warm)
	line2 := fmt.Sprintf("          MEM %s %s  %s/%s  (agents: %s)",
		widgets.SegmentedBar(memPct, agentMemShare, barWidth, th.GaugeColor(memPct), th.Accent), widgets.PctLabel(memPct), formatMB(snap.System.MemUsedMB), formatMB(snap.System.MemTotalMB), formatMB(agentMem))
	line3 := fmt.Sprintf("          NET ↑ %s/s  ↓ %s/s", formatBps(snap.System.NetUpBps), formatBps(snap.System.NetDownBps))

	// These lines already contain ANSI-styled substrings from the bars —
	// lipgloss.Style.MaxWidth truncates ANSI-aware (verified: it doesn't
	// corrupt escape codes mid-sequence the way a raw rune-slice would).
	safe := lipgloss.NewStyle().MaxWidth(width)
	line1 = safe.Render(line1)
	line2 = safe.Render(line2)
	line3 = safe.Render(line3)

	return lipgloss.JoinVertical(lipgloss.Left, sep, line1, line2, line3, sep)
}

func average(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	var sum float64
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func formatMB(mb float64) string {
	if mb >= 1024 {
		return fmt.Sprintf("%.1fG", mb/1024)
	}
	return fmt.Sprintf("%.0fM", mb)
}

func formatBps(bps float64) string {
	if bps >= 1024*1024 {
		return fmt.Sprintf("%.1fMB", bps/1024/1024)
	}
	if bps >= 1024 {
		return fmt.Sprintf("%.1fKB", bps/1024)
	}
	return fmt.Sprintf("%.0fB", bps)
}
