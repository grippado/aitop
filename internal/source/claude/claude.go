// Package claude adapts Claude Code CLI state (~/.claude/) into aitop's
// Source interface: active/recent sessions, real process CPU/mem, and
// cost/rate-limit usage.
package claude

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	gproc "github.com/shirou/gopsutil/v3/process"

	"github.com/grippado/aitop/internal/domain"
	"github.com/grippado/aitop/internal/procstat"
)

// Name identifies this Source.
const Name = "claude-code"

// namePatterns match Claude Code process command lines not covered by a
// session file (e.g. the daemon), used as a fallback in Processes.
var namePatterns = []string{"claude --dangerously-skip-permissions", "claude daemon run", "/claude "}

type Adapter struct {
	// configDir is the resolved Claude Code data directory (contains
	// sessions/, .cost-day-*.json, etc) — see resolveConfigDir.
	configDir  string
	procs      *procstat.Cache
	transcript *transcriptTracker
}

func New() *Adapter {
	home, _ := os.UserHomeDir()
	return &Adapter{configDir: resolveConfigDir(home), procs: procstat.NewCache(), transcript: newTranscriptTracker()}
}

// resolveConfigDir finds Claude Code's actual runtime data directory.
// CLAUDE_CONFIG_DIR wins if set (explicit override, no guessing). Otherwise
// this machine's dotfiles route Claude *config* (commands/skills/settings)
// through ~/cangaco/.ai/claude/ via symlinks, but *runtime* data
// (sessions/, cost files) is written directly by Claude Code — so both
// candidates are probed and whichever actually has a sessions/ dir wins,
// defaulting to the ~/.claude convention if neither does (Detect() then
// honestly reports false).
func resolveConfigDir(home string) string {
	if v := os.Getenv("CLAUDE_CONFIG_DIR"); v != "" {
		return v
	}
	candidates := []string{
		filepath.Join(home, ".claude"),
		filepath.Join(home, "cangaco", ".ai", "claude"),
	}
	for _, c := range candidates {
		if _, err := reader.Stat(filepath.Join(c, "sessions")); err == nil {
			return c
		}
	}
	return candidates[0]
}

func (a *Adapter) Name() string { return Name }

func (a *Adapter) sessionsDir() string { return filepath.Join(a.configDir, "sessions") }

func (a *Adapter) Detect(ctx context.Context) bool {
	_, err := reader.Stat(a.sessionsDir())
	return err == nil
}

// sessionFile mirrors the fields aitop actually uses from
// ~/.claude/sessions/<pid>.json — confirmed on-disk shape, not a guess.
type sessionFile struct {
	PID       int    `json:"pid"`
	SessionID string `json:"sessionId"`
	CWD       string `json:"cwd"`
	Status    string `json:"status"`
	UpdatedAt int64  `json:"updatedAt"` // unix ms
}

func (a *Adapter) Sessions(ctx context.Context) ([]domain.SessionInfo, error) {
	entries, err := reader.ReadDir(a.sessionsDir())
	if err != nil {
		return nil, nil // Detect() already gated existence; a race here is not an error
	}

	var out []domain.SessionInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := reader.ReadFile(filepath.Join(a.sessionsDir(), e.Name()))
		if err != nil {
			continue
		}
		var sf sessionFile
		if err := json.Unmarshal(raw, &sf); err != nil {
			continue
		}
		si := domain.SessionInfo{
			Tool:      Name,
			ID:        sf.SessionID,
			PID:       sf.PID,
			Alive:     sf.PID != 0 && processAlive(sf.PID),
			CWD:       sf.CWD,
			Status:    normalizeStatus(sf.Status),
			UpdatedAt: msToTime(sf.UpdatedAt),
		}
		if usage, ok := a.transcript.usageFor(a.configDir, sf.CWD, sf.SessionID); ok {
			si.LastAction = usage.LastAction
			si.Title = usage.Title
		}
		out = append(out, si)
	}
	return out, nil
}

// Processes returns real CPU/mem for every session PID, plus a name-pattern
// fallback scan for Claude processes not tied to any session file (e.g. the
// supervisor daemon). The fallback scan enumerates all processes — accepted
// here since it runs at this adapter's own ~5s cadence, not the collector's
// 2s system tick.
func (a *Adapter) Processes(ctx context.Context) ([]domain.ProcessInfo, error) {
	sessions, _ := a.Sessions(ctx)
	seen := map[int]bool{}
	var out []domain.ProcessInfo

	for _, s := range sessions {
		if s.PID == 0 || !s.Alive {
			continue
		}
		if cpuPct, memMB, startedAt, ok := a.procs.Stat(int32(s.PID)); ok {
			out = append(out, domain.ProcessInfo{PID: s.PID, Tool: Name, Label: "claude", CPUPct: cpuPct, MemMB: memMB, StartedAt: startedAt})
			seen[s.PID] = true
		}
	}

	procs, err := gproc.ProcessesWithContext(ctx)
	if err != nil {
		return out, nil
	}
	for _, p := range procs {
		pid := int(p.Pid)
		if seen[pid] {
			continue
		}
		cmdline, err := p.CmdlineWithContext(ctx)
		if err != nil || !matchesAny(cmdline, namePatterns) {
			continue
		}
		if cpuPct, memMB, startedAt, ok := a.procs.Stat(p.Pid); ok {
			ppid, _ := p.PpidWithContext(ctx)
			out = append(out, domain.ProcessInfo{PID: pid, PPID: int(ppid), Tool: Name, Label: "claude daemon", CPUPct: cpuPct, MemMB: memMB, StartedAt: startedAt})
		}
	}
	return out, nil
}

func matchesAny(s string, patterns []string) bool {
	for _, p := range patterns {
		if strings.Contains(s, p) {
			return true
		}
	}
	return false
}

func processAlive(pid int) bool {
	ok, err := gproc.PidExists(int32(pid))
	return err == nil && ok
}

func normalizeStatus(s string) string {
	switch s {
	case "busy", "idle":
		return s
	default:
		return "unknown"
	}
}

func msToTime(ms int64) time.Time {
	if ms == 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms)
}
