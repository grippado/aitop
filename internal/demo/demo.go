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

// sessionPhase filters the fixed pool of demo sessions (passed positionally
// in demo-1..demo-6 order) down to whichever subset is "present" at t
// seconds since the generator started — three phases, timed against
// vhs.tape's own Sleep durations, that build the board up rather than
// dumping all six at once:
//
//   - t <  4s: just claude-code, codex, and opencode (demo-1/3/6) — a
//     clean, information-dense opening, each card fully populated.
//   - t < 11s: cursor-agent joins (demo-5).
//   - t >=11s: a second Claude Code session joins (demo-2, "a new claude
//     code") alongside Cursor (demo-4) — Cursor's card is genuinely the
//     sparsest one (the IDE adapter has no transcript/title source, see
//     cards.go), so it lands late, one card among five full ones, instead
//     of being the first thing a viewer sees.
func sessionPhase(t float64, demo1, demo2, demo3, demo4, demo5, demo6 domain.SessionInfo) []domain.SessionInfo {
	switch {
	case t < 4:
		return []domain.SessionInfo{demo1, demo3, demo6}
	case t < 11:
		return []domain.SessionInfo{demo1, demo3, demo6, demo5}
	default:
		return []domain.SessionInfo{demo1, demo3, demo6, demo5, demo2, demo4}
	}
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
	codexFiveHour := 8.0

	// Context% oscillates gently so a live --demo session visibly "fills
	// up" over time, selling the context-monitor thesis rather than
	// sitting static.
	claudeCtx := 55 + 10*math.Sin(t/9)
	codexCtx := 22 + 6*math.Sin(t/11+1)
	opencodeCtx := 59 + 8*math.Sin(t/7+2)

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
			{Tool: "codex", Installed: true, Running: true, SessionCount: 1, OldestSessionSec: 6 * 60},
			{Tool: "cursor", Installed: true, Running: true, SessionCount: 1, OldestSessionSec: 41 * 60},
			{Tool: "cursor-agent", Installed: true, Running: true, SessionCount: 1, OldestSessionSec: 3 * 60},
			{Tool: "opencode", Installed: true, Running: true, SessionCount: 1, OldestSessionSec: 19 * 60},
		},
		Processes: []domain.ProcessInfo{
			{PID: 41221, Tool: "claude-code", Label: "claude", CPUPct: 8 + 4*math.Abs(math.Sin(t)), MemMB: 350, StartedAt: g.start},
			{PID: 41090, Tool: "claude-code", Label: "claude daemon", CPUPct: 0.1, MemMB: 190, StartedAt: g.start},
			{PID: 60123, Tool: "codex", Label: "codex", CPUPct: 3 + 2*math.Abs(math.Sin(t/2)), MemMB: 210, StartedAt: g.start},
			{PID: 52110, Tool: "cursor", Label: "Cursor Helper: mcp-process", CPUPct: 2.0, MemMB: 130, StartedAt: g.start},
			{PID: 52111, Tool: "cursor", Label: "Cursor Helper (Renderer)", CPUPct: 5.5, MemMB: 620, StartedAt: g.start},
			{PID: 71340, Tool: "cursor-agent", Label: "cursor-agent", CPUPct: 4 + 2*math.Abs(math.Sin(t/2+1)), MemMB: 275, StartedAt: g.start},
			{PID: 81552, Tool: "opencode", Label: "opencode", CPUPct: 6 + 3*math.Abs(math.Sin(t/3+2)), MemMB: 410, StartedAt: g.start},
		},
		// Sessions carry their OWN tokens/context/title/last-action — kept
		// deliberately distinct per session (unlike Usage below, which is
		// tool-wide cost/limits only) to preview the exact thing a real
		// bug report was about: two sessions of the same tool must never
		// show identical numbers.
		//
		// The set itself isn't static either: it builds up over time (three
		// sessions -> four -> six) as if agents were actually starting one
		// after another — a live board actually changing shape, not just
		// its numbers wiggling. See sessionPhase above for the exact
		// timing this is tuned against (vhs.tape's Sleep durations).
		Sessions: sessionPhase(t,
			domain.SessionInfo{
				Tool: "claude-code", ID: "demo-1", PID: 41221, Alive: true, CWD: "/Users/demo/www/guia-cumuru", Branch: "main", Dirty: true, Model: "opus 4.8", Status: "busy", UpdatedAt: now.Add(-14 * time.Minute),
				Title: "Corrigir tábua de marés", LastAction: "🔧 Bash: go test ./modules/mares/... -run TestTabua",
				TokensIn: 70000, TokensOut: 21000, ContextUsedPct: claudeCtx,
			},
			domain.SessionInfo{
				Tool: "claude-code", ID: "demo-2", PID: 41090, Alive: true, CWD: "/Users/demo/cangaco", Branch: "main", Model: "sonnet 5", Status: "idle", UpdatedAt: now.Add(-2*time.Hour - 14*time.Minute),
				Title: "Sincronizar dotfiles", LastAction: "💭 Aguardando confirmação do usuário para o merge",
				TokensIn: 12400, TokensOut: 3100,
			},
			domain.SessionInfo{
				Tool: "codex", ID: "demo-3", PID: 60123, Alive: true, CWD: "/Users/demo/www/isaac/backoffice", Model: "gpt-5.4-mini", Status: "busy", UpdatedAt: now.Add(-6 * time.Minute),
				LastAction: "🔧 shell: pnpm --filter backoffice build",
				TokensIn:   5000, TokensOut: 1200, ContextUsedPct: codexCtx,
			},
			domain.SessionInfo{Tool: "cursor", ID: "demo-4", PID: 52110, Alive: true, CWD: "/Users/demo/www/aitop", Status: "busy", UpdatedAt: now.Add(-41 * time.Minute)},
			domain.SessionInfo{
				Tool: "cursor-agent", ID: "demo-5", PID: 71340, Alive: true, CWD: "/Users/demo", Model: "sonnet 5", Status: "busy", UpdatedAt: now.Add(-3 * time.Minute),
				Title: "Revisar contrato da API de pagamentos", LastAction: "🔧 Grep: pattern=\"func.*Charge\" path=internal/billing",
			},
			domain.SessionInfo{
				Tool: "opencode", ID: "demo-6", PID: 81552, Alive: true, CWD: "/Users/demo/www/personal/gripp-link", Model: "deepseek v4 flash free", Status: "busy", UpdatedAt: now.Add(-19 * time.Minute),
				Title: "Ajustar SEO da landing page", LastAction: "🔧 bash: pnpm build && pnpm lighthouse",
				TokensIn: 118298, TokensOut: 4006, ContextUsedPct: opencodeCtx,
			},
		),
		// Usage stays tool-wide: cost and rate limits genuinely have no
		// per-session source, unlike tokens/context% above.
		Usage: []domain.UsageInfo{
			{Tool: "claude-code", Available: true, CostTodayUSD: 0.43, CostMonthUSD: 12.10, LimitFiveHour: &fiveHour, LimitWeekly: &weekly},
			{Tool: "codex", Available: true, LimitFiveHour: &codexFiveHour},
			{Tool: "cursor", Available: false},
			{Tool: "cursor-agent", Available: false},
			{Tool: "opencode", Available: false},
		},
	}
}
