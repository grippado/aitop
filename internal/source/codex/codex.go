// Package codex adapts Codex CLI state (~/.codex/) into aitop's Source
// interface. Every file this package reads is named explicitly in
// allowlist.go — there is no directory walk over ~/.codex.
package codex

import (
	"bufio"
	"bytes"
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
const Name = "codex"

// namePattern matches the Codex CLI process command line.
const namePattern = "codex"

type Adapter struct {
	home  string
	procs *procstat.Cache
	cwd   *cwdResolver
}

func New() *Adapter {
	home, _ := os.UserHomeDir()
	return &Adapter{home: home, procs: procstat.NewCache(), cwd: newCWDResolver()}
}

func (a *Adapter) Name() string { return Name }

func (a *Adapter) Detect(ctx context.Context) bool {
	_, err := reader.Stat(allowedFile(a.home, "config"))
	return err == nil
}

// historyEntry mirrors one line of ~/.codex/history.jsonl.
type historyEntry struct {
	SessionID string `json:"session_id"`
	TS        int64  `json:"ts"` // unix seconds
}

// chatProcess is a best-effort, defensively-parsed shape for
// process_manager/chat_processes.json. This file was NOT observed on the
// dev machine during design (Codex wasn't running) — its exact schema is
// unverified. Unknown/missing fields are left zero rather than guessed;
// if the real shape differs, parsing degrades to "file present but no
// usable rows," never a crash.
type chatProcess struct {
	PID       int    `json:"pid"`
	SessionID string `json:"session_id"`
	CWD       string `json:"cwd"`
	Model     string `json:"model"`
}

// Sessions returns recent Codex sessions. Primary source is history.jsonl
// (confirmed real, flat, simple); process_manager/chat_processes.json is
// consulted opportunistically for PID correlation when present and
// parseable, but Sessions never depends on it existing.
func (a *Adapter) Sessions(ctx context.Context) ([]domain.SessionInfo, error) {
	byPID := a.readChatProcesses()

	entries := a.readHistory()
	latestBySession := map[string]historyEntry{}
	for _, e := range entries {
		if prev, ok := latestBySession[e.SessionID]; !ok || e.TS > prev.TS {
			latestBySession[e.SessionID] = e
		}
	}

	livePID, live := a.findLiveCodexPID(ctx)

	var out []domain.SessionInfo
	usedLivePID := false
	for id, e := range latestBySession {
		si := domain.SessionInfo{
			Tool:      Name,
			ID:        id,
			Status:    "idle",
			UpdatedAt: unixToTime(e.TS),
		}
		if cp, ok := byPID[id]; ok {
			si.PID = cp.PID
			si.CWD = cp.CWD
			si.Model = cp.Model
		}
		if si.CWD == "" {
			// chat_processes.json is opportunistic and often absent (see
			// its doc comment) — fall back to the session's own rollout
			// file, whose first line ("session_meta") records the real
			// cwd Codex was launched from. Cached per session ID so this
			// only ever scans the sessions/ tree once per session, not
			// every tick.
			si.CWD = a.cwd.resolve(filepath.Join(a.home, ".codex"), id)
		}
		if live && si.PID == livePID {
			si.Alive = true
			si.Status = "busy"
			usedLivePID = true
		}
		out = append(out, si)
	}

	// A live Codex process we couldn't correlate to any known session
	// still gets surfaced, honestly labeled, rather than hidden.
	if live && !usedLivePID {
		out = append(out, domain.SessionInfo{Tool: Name, ID: "", PID: livePID, Alive: true, Status: "busy"})
	}

	return out, nil
}

func (a *Adapter) readHistory() []historyEntry {
	raw, err := reader.ReadFile(allowedFile(a.home, "history"))
	if err != nil {
		return nil
	}
	var out []historyEntry
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var e historyEntry
		if err := json.Unmarshal(sc.Bytes(), &e); err == nil && e.SessionID != "" {
			out = append(out, e)
		}
	}
	return out
}

// readChatProcesses is best-effort: absence or parse failure is not an
// error, it's the expected common case in v1.
func (a *Adapter) readChatProcesses() map[string]chatProcess {
	raw, err := reader.ReadFile(allowedFile(a.home, "chatProcesses"))
	if err != nil {
		return nil
	}
	var list []chatProcess
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil
	}
	out := map[string]chatProcess{}
	for _, cp := range list {
		if cp.SessionID != "" {
			out[cp.SessionID] = cp
		}
	}
	return out
}

func (a *Adapter) findLiveCodexPID(ctx context.Context) (int, bool) {
	procs, err := gproc.ProcessesWithContext(ctx)
	if err != nil {
		return 0, false
	}
	for _, p := range procs {
		name, _ := p.NameWithContext(ctx)
		cmdline, _ := p.CmdlineWithContext(ctx)
		if strings.Contains(name, namePattern) || strings.Contains(cmdline, namePattern) {
			return int(p.Pid), true
		}
	}
	return 0, false
}

// Processes returns real CPU/mem for the live Codex process, if any.
func (a *Adapter) Processes(ctx context.Context) ([]domain.ProcessInfo, error) {
	pid, ok := a.findLiveCodexPID(ctx)
	if !ok {
		return nil, nil
	}
	cpuPct, memMB, statOK := a.procs.Stat(int32(pid))
	if !statOK {
		return nil, nil
	}
	return []domain.ProcessInfo{{PID: pid, Tool: Name, Label: "codex", CPUPct: cpuPct, MemMB: memMB}}, nil
}

func unixToTime(sec int64) time.Time {
	if sec == 0 {
		return time.Time{}
	}
	return time.Unix(sec, 0)
}
