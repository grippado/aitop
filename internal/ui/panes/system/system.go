// Package system renders the two headline boxes (1: per-core CPU, 2: MEM/NET)
// — real, whole-system resource numbers, exactly like real btop. This is the
// pane that has to read as "genuine system monitor" at a glance, which is
// the single biggest differentiator vs. every existing "aitop"-named project.
package system

import (
	"fmt"

	"github.com/grippado/aitop/internal/domain"
	"github.com/grippado/aitop/internal/ui/theme"
	"github.com/grippado/aitop/internal/ui/widgets"
)

// RenderCPU draws box 1: one bar per core, plus an "AI-total" annotation
// showing how much of total system capacity the tracked AI-tool processes
// are using right now.
func RenderCPU(th theme.Theme, snap domain.Snapshot, width, height int, focused bool) string {
	box := th.Box("1:CPU (AI procs)", focused, width, height)
	barWidth := width - 14
	if barWidth < 4 {
		barWidth = 4
	}

	var body string
	for i, pct := range snap.System.PerCoreCPUPct {
		body += fmt.Sprintf("Core%-2d %s %s\n", i, widgets.Bar(pct, barWidth, th.GaugeColor), widgets.PctLabel(pct))
	}

	var aiTotal float64
	for _, p := range snap.Processes {
		aiTotal += p.CPUPct
	}
	cores := len(snap.System.PerCoreCPUPct)
	var aiShare float64
	if cores > 0 {
		aiShare = aiTotal / float64(cores)
	}
	warmNote := ""
	if snap.Warming {
		warmNote = " (warming)"
	}
	body += fmt.Sprintf("\nAI-total: %.0f%% of %d cores%s", aiShare, cores, warmNote)

	return box.Render(body)
}

// RenderMemNet draws box 2: system memory and network throughput, plus what
// share of memory the tracked AI-tool processes account for.
func RenderMemNet(th theme.Theme, snap domain.Snapshot, width, height int, focused bool) string {
	box := th.Box("2:MEM/NET", focused, width, height)
	barWidth := width - 14
	if barWidth < 4 {
		barWidth = 4
	}

	memPct := 0.0
	if snap.System.MemTotalMB > 0 {
		memPct = snap.System.MemUsedMB / snap.System.MemTotalMB * 100
	}

	var aiMem float64
	for _, p := range snap.Processes {
		aiMem += p.MemMB
	}
	var aiMemShare float64
	if snap.System.MemUsedMB > 0 {
		aiMemShare = aiMem / snap.System.MemUsedMB * 100
	}

	body := fmt.Sprintf(
		"RSS   %s %s\nNet↑ %s/s  ↓ %s/s\n\nAgent share: %.0f%% of system MEM",
		widgets.Bar(memPct, barWidth, th.GaugeColor), formatMB(snap.System.MemUsedMB),
		formatBps(snap.System.NetUpBps), formatBps(snap.System.NetDownBps),
		aiMemShare,
	)

	return box.Render(body)
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
