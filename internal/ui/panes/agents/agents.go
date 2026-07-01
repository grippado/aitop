// Package agents renders box 4: the sortable/filterable process+session
// table — aitop's equivalent of real btop's process pane, scoped to AI
// tools. Missing data always renders as "—", never a fabricated 0.
package agents

import (
	"fmt"
	"sort"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/grippado/aitop/internal/domain"
	"github.com/grippado/aitop/internal/ui/theme"
	"github.com/grippado/aitop/internal/ui/widgets"
)

// SortColumn identifies which column drives table ordering, cycled via 'o'.
type SortColumn int

const (
	SortCPU SortColumn = iota
	SortMem
	SortDuration
	SortTool
)

func (c SortColumn) String() string {
	switch c {
	case SortMem:
		return "mem"
	case SortDuration:
		return "dur"
	case SortTool:
		return "tool"
	default:
		return "cpu"
	}
}

// Row is a denormalized process+session row for display.
type Row struct {
	PID     int
	Tool    string
	Project string
	State   string
	DurSec  float64
	CPUPct  float64
	MemMB   float64
	Tokens  int64
	CostUSD float64
	HasCost bool
	HasTok  bool
}

// BuildRows joins Sessions and Processes by PID into display rows, applying
// an optional tool filter ("" = all).
func BuildRows(snap domain.Snapshot, toolFilter string) []Row {
	procByPID := map[int]domain.ProcessInfo{}
	for _, p := range snap.Processes {
		procByPID[p.PID] = p
	}

	var rows []Row
	seen := map[int]bool{}
	for _, s := range snap.Sessions {
		if toolFilter != "" && s.Tool != toolFilter {
			continue
		}
		r := Row{PID: s.PID, Tool: s.Tool, Project: s.CWD, State: s.Status}
		if !s.UpdatedAt.IsZero() {
			r.DurSec = time.Since(s.UpdatedAt).Seconds()
		}
		if p, ok := procByPID[s.PID]; ok {
			r.CPUPct, r.MemMB = p.CPUPct, p.MemMB
			seen[s.PID] = true
		}
		rows = append(rows, r)
	}
	// Processes with no matching session still show up (e.g. Cursor helpers).
	for _, p := range snap.Processes {
		if seen[p.PID] {
			continue
		}
		if toolFilter != "" && p.Tool != toolFilter {
			continue
		}
		rows = append(rows, Row{PID: p.PID, Tool: p.Tool, Project: p.Label, State: "running", CPUPct: p.CPUPct, MemMB: p.MemMB})
	}
	return rows
}

// Sort orders rows in place by the given column, descending for
// load-like columns.
func Sort(rows []Row, col SortColumn) {
	sort.SliceStable(rows, func(i, j int) bool {
		switch col {
		case SortMem:
			return rows[i].MemMB > rows[j].MemMB
		case SortDuration:
			return rows[i].DurSec > rows[j].DurSec
		case SortTool:
			return rows[i].Tool < rows[j].Tool
		default:
			return rows[i].CPUPct > rows[j].CPUPct
		}
	})
}

func Render(th theme.Theme, rows []Row, selected int, sortCol SortColumn, width, height int, focused bool) string {
	box := th.Box(fmt.Sprintf("4:PROCESSES (sort:%s)", sortCol), focused, width, height)

	header := fmt.Sprintf("%-6s %-8s %-16s %-6s %6s %6s %5s %8s %7s\n",
		"PID", "TOOL", "PROJECT", "STATE", "DUR", "CPU%", "MEM%", "TOK", "$")

	var body string
	for i, r := range rows {
		dur := widgets.Dash
		if r.DurSec > 0 {
			dur = formatDur(r.DurSec)
		}
		tok := widgets.Dash
		if r.HasTok {
			tok = fmt.Sprintf("%dk", r.Tokens/1000)
		}
		cost := widgets.Dash
		if r.HasCost {
			cost = fmt.Sprintf("%.2f", r.CostUSD)
		}
		line := fmt.Sprintf("%-6d %-8s %-16.16s %-6s %6s %6.1f %4.1f%% %8s %7s",
			r.PID, r.Tool, r.Project, r.State, dur, r.CPUPct, r.MemMB, tok, cost)
		if i == selected && focused {
			line = lipgloss.NewStyle().Foreground(th.Accent).Render(line)
		}
		body += line + "\n"
	}

	return box.Render(header + body)
}

func formatDur(sec float64) string {
	d := time.Duration(sec) * time.Second
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh%02dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}
