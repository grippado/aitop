// Command aitop is a real-time terminal dashboard for AI coding agent
// resource usage (Claude Code, Codex CLI, Cursor) — CPU/MEM/NET first,
// cost/tokens as a footnote.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/grippado/aitop/internal/collector"
	"github.com/grippado/aitop/internal/demo"
	"github.com/grippado/aitop/internal/domain"
	"github.com/grippado/aitop/internal/source"
	"github.com/grippado/aitop/internal/source/claude"
	"github.com/grippado/aitop/internal/source/codex"
	"github.com/grippado/aitop/internal/source/cursor"
	"github.com/grippado/aitop/internal/source/fallback"
	"github.com/grippado/aitop/internal/ui"
	"github.com/grippado/aitop/internal/version"
)

func main() {
	var (
		once     = flag.Bool("once", false, "print a single snapshot and exit, instead of the live TUI")
		asJSON   = flag.Bool("json", false, "with --once, print the snapshot as JSON instead of a text summary")
		demoMode = flag.Bool("demo", false, "use synthetic data instead of reading real tool state (for screenshots/GIFs/dev)")
		refresh  = flag.Duration("refresh", 2*time.Second, "process/CPU sample interval")
		showVer  = flag.Bool("version", false, "print version and exit")
	)
	flag.Parse()

	if *showVer {
		fmt.Println(version.Version)
		return
	}

	var pull ui.PullFunc
	if *demoMode {
		gen := demo.New()
		pull = gen.Snapshot
	} else {
		ctx := context.Background()
		all := []source.Source{claude.New(), codex.New(), cursor.New(), fallback.New()}
		coll := collector.New(source.Resolve(ctx, all), *refresh)
		pull = coll.Snapshot
	}

	if *once {
		runOnce(pull, *demoMode, *asJSON)
		return
	}

	m := ui.New(pull, *refresh)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "aitop:", err)
		os.Exit(1)
	}
}

// runOnce prints a single snapshot, scriptable via --json. CPU% has no
// baseline on a single read, so in non-demo mode we take two samples ~200ms
// apart before emitting — see internal/collector. The snapshot is still
// flagged Warming so downstream consumers know it's approximate.
func runOnce(pull ui.PullFunc, isDemo bool, asJSON bool) {
	snap := pull()
	if !isDemo {
		snap.Warming = true
	}

	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(snap)
		return
	}
	printText(snap)
}

func printText(s domain.Snapshot) {
	warm := ""
	if s.Warming {
		warm = " (approximate — CPU% has no baseline in a single snapshot)"
	}
	fmt.Printf("aitop snapshot @ %s%s\n\n", s.TakenAt.Format(time.RFC3339), warm)
	fmt.Printf("system: cores=%d mem=%.0f/%.0fMB net_up=%.0fB/s net_down=%.0fB/s\n", len(s.System.PerCoreCPUPct), s.System.MemUsedMB, s.System.MemTotalMB, s.System.NetUpBps, s.System.NetDownBps)
	fmt.Println("tools:")
	for _, t := range s.Tools {
		fmt.Printf("  %-14s installed=%-5v running=%-5v sessions=%d note=%q\n", t.Tool, t.Installed, t.Running, t.SessionCount, t.Note)
	}
	fmt.Println("processes:")
	for _, p := range s.Processes {
		fmt.Printf("  pid=%-8d tool=%-12s cpu=%.1f%% mem=%.0fMB %s\n", p.PID, p.Tool, p.CPUPct, p.MemMB, p.Label)
	}
	fmt.Println("sessions:")
	for _, sess := range s.Sessions {
		fmt.Printf("  tool=%-12s id=%-10s alive=%-5v status=%-6s cwd=%s\n", sess.Tool, sess.ID, sess.Alive, sess.Status, sess.CWD)
	}
	fmt.Println("usage:")
	for _, u := range s.Usage {
		if !u.Available {
			fmt.Printf("  tool=%-12s available=false\n", u.Tool)
			continue
		}
		limit5, limit7 := "—", "—"
		if u.LimitFiveHour != nil {
			limit5 = fmt.Sprintf("%.0f%%", *u.LimitFiveHour)
		}
		if u.LimitWeekly != nil {
			limit7 = fmt.Sprintf("%.0f%%", *u.LimitWeekly)
		}
		fmt.Printf("  tool=%-12s today=$%.2f month=$%.2f tokens_in=%d tokens_out=%d ctx=%.0f%% 5h=%s 7d=%s\n",
			u.Tool, u.CostTodayUSD, u.CostMonthUSD, u.TokensIn, u.TokensOut, u.ContextUsedPct, limit5, limit7)
	}
}
