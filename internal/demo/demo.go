// Package demo produces synthetic snapshots for `aitop --demo` — used to
// build/screenshot/record the UI without depending on any real tool being
// installed or running. Also the fixture used by `aitop --once --demo`.
package demo

import (
	"math"
	"time"

	"github.com/grippado/aitop/internal/domain"
)

// Generator emits a Snapshot that oscillates over time so a live --demo
// session looks alive (moving CPU bars) rather than static.
type Generator struct {
	start time.Time
}

func New() *Generator {
	return &Generator{start: time.Now()}
}

// Snapshot returns the current synthetic state.
func (g *Generator) Snapshot() domain.Snapshot {
	now := time.Now()
	t := now.Sub(g.start).Seconds()

	const cores = 8
	perCore := make([]float64, cores)
	for i := range perCore {
		perCore[i] = 10 + 45*math.Abs(math.Sin(t/3+float64(i)))
	}

	fiveHour := 34.0
	weekly := 12.0

	return domain.Snapshot{
		TakenAt: now,
		Warming: false,
		System: domain.SystemStats{
			PerCoreCPUPct: perCore,
			MemUsedMB:     11000 + 500*math.Sin(t/5),
			MemTotalMB:    32768,
			NetUpBps:      12000 + 4000*math.Abs(math.Sin(t/2)),
			NetDownBps:    340,
		},
		Tools: []domain.ToolStatus{
			{Tool: "claude-code", Installed: true, Running: true, SessionCount: 2, OldestSessionSec: 2*3600 + 14*60},
			{Tool: "codex", Installed: true, Running: false, SessionCount: 0},
			{Tool: "cursor", Installed: true, Running: true, SessionCount: 1, OldestSessionSec: 41 * 60},
		},
		Processes: []domain.ProcessInfo{
			{PID: 41221, Tool: "claude-code", Label: "claude", CPUPct: 8 + 4*math.Abs(math.Sin(t)), MemMB: 350, StartedAt: g.start},
			{PID: 41090, Tool: "claude-code", Label: "claude daemon", CPUPct: 0.1, MemMB: 190, StartedAt: g.start},
			{PID: 52110, Tool: "cursor", Label: "Cursor Helper: mcp-process", CPUPct: 2.0, MemMB: 130, StartedAt: g.start},
		},
		Sessions: []domain.SessionInfo{
			{Tool: "claude-code", ID: "demo-1", PID: 41221, Alive: true, CWD: "guia-cumuru", Status: "busy", UpdatedAt: now.Add(-14 * time.Minute)},
			{Tool: "claude-code", ID: "demo-2", PID: 41090, Alive: true, CWD: "cangaco", Status: "idle", UpdatedAt: now.Add(-2*time.Hour - 14*time.Minute)},
			{Tool: "codex", ID: "demo-3", PID: 0, Alive: false, CWD: "", Status: "idle"},
			{Tool: "cursor", ID: "demo-4", PID: 52110, Alive: true, CWD: "aitop", Status: "busy", UpdatedAt: now.Add(-41 * time.Minute)},
		},
		Usage: []domain.UsageInfo{
			{Tool: "claude-code", Available: true, CostTodayUSD: 0.43, CostMonthUSD: 12.10, TokensIn: 70000, TokensOut: 21000, LimitFiveHour: &fiveHour, LimitWeekly: &weekly},
			{Tool: "codex", Available: true, ContextUsedPct: 34, TokensIn: 5000, TokensOut: 1200},
			{Tool: "cursor", Available: false},
		},
	}
}
