// Package claude adapts Claude Code CLI state (~/.claude/) into aitop's
// Source interface: active/recent sessions, real process CPU/mem, and
// cost/rate-limit usage.
//
// Known gap: Claude Desktop's "local agent mode" / Cowork feature runs its
// own embedded Claude Code sessions inside an isolated VM (confirmed on
// this machine: Apple's Virtualization.framework hosts it — see
// ~/Library/Application Support/Claude/vm_bundles/claudevm.bundle/, which
// has real VM disk images (rootfs.img, sessiondata.img, efivars.fd), plus
// ~/Library/Application Support/Claude/claude-code-vm/<version>/claude,
// the guest-side binary). That session's process and its own
// ~/.claude/sessions/ equivalent live inside the VM's guest filesystem —
// invisible to this adapter, which only ever reads the HOST's
// ~/.claude/sessions/ and scans HOST processes. There is no host-visible
// PID or session file for a Desktop-embedded session to pick up, by the
// sandbox's own design. Not fixed here — would require querying the VM
// itself (it exposes an internal IP via vm_bundles/claudevm.bundle/vmIP),
// which is out of scope: no documented/stable API to rely on.
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
	Kind      string `json:"kind"`      // Claude Code's own tag: "bg" | "interactive"
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
			Kind:      sf.Kind, // verbatim passthrough; empty if the file omits it
			Status:    normalizeStatus(sf.Status),
			UpdatedAt: msToTime(sf.UpdatedAt),
		}
		if usage, ok := a.transcript.usageFor(a.configDir, sf.CWD, sf.SessionID); ok {
			si.LastAction = usage.LastAction
			si.Title = usage.Title
			si.Model = friendlyModelName(usage.Model)
			si.TokensIn, si.TokensOut, si.ContextUsedPct, _ = deriveTokenFields(usage)
		}
		out = append(out, si)
	}
	resolveParentPIDs(ctx, out)
	return out, nil
}

// maxParentWalkHops bounds the PPID walk so a pathological / cyclic process
// tree can never spin the adapter. 40 matches the probe used to verify RFC
// 0003 on a real machine (a bg child resolved to its interactive parent in
// far fewer hops, through non-session hosts).
const maxParentWalkHops = 40

// ppidOf returns the parent PID of pid via gopsutil. It is a package-level var
// so tests can inject a synthetic process tree and never touch the live
// machine (golden invariant 3: the walk uses the gopsutil process API, the
// same category as the daemon-fallback scan — never the file Reader). ok=false
// means the process left the tree / can't be inspected, which ends the walk.
var ppidOf = func(ctx context.Context, pid int) (ppid int, ok bool) {
	p, err := gproc.NewProcessWithContext(ctx, int32(pid))
	if err != nil {
		return 0, false
	}
	pp, err := p.PpidWithContext(ctx)
	if err != nil {
		return 0, false
	}
	return int(pp), true
}

// resolveParentPIDs sets ParentPID on each ALIVE session by walking its PPID
// chain until it reaches the FIRST ancestor that is itself a tracked session
// PID. The chain legitimately passes through untracked hosts ("claude",
// "bg-pty-host") before reaching the real parent session, so the walk does NOT
// stop at the first non-session ancestor. A root / orphan (walk ends or hits
// the cap without finding a tracked ancestor) keeps ParentPID 0. Never sets a
// session's ParentPID to its own PID.
func resolveParentPIDs(ctx context.Context, sessions []domain.SessionInfo) {
	sessionPIDs := make(map[int]bool, len(sessions))
	for _, s := range sessions {
		if s.PID != 0 {
			sessionPIDs[s.PID] = true
		}
	}
	for i := range sessions {
		s := &sessions[i]
		if s.PID == 0 || !s.Alive {
			continue
		}
		pid := s.PID
		for hop := 0; hop < maxParentWalkHops; hop++ {
			ppid, ok := ppidOf(ctx, pid)
			if !ok || ppid == 0 || ppid == 1 {
				break // root / left the process tree
			}
			if ppid != s.PID && sessionPIDs[ppid] {
				s.ParentPID = ppid // first tracked ancestor wins
				break
			}
			pid = ppid // keep walking through untracked hosts
		}
	}
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
