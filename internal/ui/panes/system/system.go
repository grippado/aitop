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
	barWidth := width - 40
	if barWidth < 10 {
		barWidth = 10
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

	warm := ""
	if snap.Warming {
		warm = "  (warming)"
	}

	sep := lipgloss.NewStyle().Foreground(th.Border).Render(strings.Repeat("─", max(1, width)))

	line1 := fmt.Sprintf(" SYSTEM   CPU %s %s  (agents: %.0f%% of system)%s",
		widgets.Bar(cpuAvg, barWidth, th.GaugeColor), widgets.PctLabel(cpuAvg), agentCPUShare, warm)
	line2 := fmt.Sprintf("          MEM %s %s  %s/%s  (agents: %s)",
		widgets.Bar(memPct, barWidth, th.GaugeColor), widgets.PctLabel(memPct), formatMB(snap.System.MemUsedMB), formatMB(snap.System.MemTotalMB), formatMB(agentMem))
	line3 := fmt.Sprintf("          NET ↑ %s/s  ↓ %s/s", formatBps(snap.System.NetUpBps), formatBps(snap.System.NetDownBps))

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
