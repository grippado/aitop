// Package fallback catches AI-ish CLI processes that don't have a
// dedicated Source adapter yet (aider, windsurf, opencode, ...). No
// session/usage parsing — just visibility via a configurable name-pattern
// scan, so nothing "AI-shaped" is ever completely invisible in aitop even
// before a real adapter exists for it.
package fallback

import (
	"context"
	"strings"

	gproc "github.com/shirou/gopsutil/v3/process"

	"github.com/grippado/aitop/internal/domain"
	"github.com/grippado/aitop/internal/procstat"
)

// Name identifies this Source. It always reports Installed since it isn't
// tied to one specific tool.
const Name = "fallback"

// DefaultPatterns are matched case-sensitively against each process's
// command line. Extend via WithPatterns — a future ~/.config/aitop/
// config.toml `[[fallback.patterns]]` array feeds this same constructor.
var DefaultPatterns = []string{"aider", "windsurf", "opencode"}

type Adapter struct {
	patterns []string
	procs    *procstat.Cache
}

func New(patterns ...string) *Adapter {
	if len(patterns) == 0 {
		patterns = DefaultPatterns
	}
	return &Adapter{patterns: patterns, procs: procstat.NewCache()}
}

func (a *Adapter) Name() string { return Name }

// Detect is always true: this adapter's job is exactly to catch whatever a
// dedicated adapter would otherwise miss.
func (a *Adapter) Detect(ctx context.Context) bool { return true }

func (a *Adapter) Sessions(ctx context.Context) ([]domain.SessionInfo, error) { return nil, nil }

func (a *Adapter) Usage(ctx context.Context) (domain.UsageInfo, error) {
	return domain.UsageInfo{Tool: Name, Available: false}, nil
}

func (a *Adapter) Processes(ctx context.Context) ([]domain.ProcessInfo, error) {
	procs, err := gproc.ProcessesWithContext(ctx)
	if err != nil {
		return nil, nil
	}
	var out []domain.ProcessInfo
	for _, p := range procs {
		cmdline, err := p.CmdlineWithContext(ctx)
		if err != nil {
			continue
		}
		match := matchPattern(cmdline, a.patterns)
		if match == "" {
			continue
		}
		if cpuPct, memMB, ok := a.procs.Stat(p.Pid); ok {
			ppid, _ := p.PpidWithContext(ctx)
			out = append(out, domain.ProcessInfo{
				PID: int(p.Pid), PPID: int(ppid),
				Tool: "unknown:" + match, Label: cmdline,
				CPUPct: cpuPct, MemMB: memMB,
			})
		}
	}
	return out, nil
}

func matchPattern(cmdline string, patterns []string) string {
	for _, p := range patterns {
		if strings.Contains(cmdline, p) {
			return p
		}
	}
	return ""
}
