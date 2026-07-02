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

// isCodexProcess matches the actual Codex CLI binary — an exact process
// name, or argv[0] exactly "codex" / ending in "/codex". A substring
// match against the whole cmdline (the previous approach) is a real false-
// positive trap: any unrelated process whose arguments merely MENTION
// "codex" (a shell command referencing this very package's path, someone
// editing a file named codex.go, ...) would match. Confirmed in practice:
// a live debugging shell command got misidentified as a running Codex
// process this way.
func isCodexProcess(name, cmdline string) bool {
	if name == "codex" {
		return true
	}
	argv0 := cmdline
	if i := strings.IndexByte(cmdline, ' '); i >= 0 {
		argv0 = cmdline[:i]
	}
	return argv0 == "codex" || strings.HasSuffix(argv0, "/codex")
}

type Adapter struct {
	home       string
	procs      *procstat.Cache
	cwd        *cwdResolver
	transcript *codexTranscriptTracker
}

func New() *Adapter {
	home, _ := os.UserHomeDir()
	return &Adapter{home: home, procs: procstat.NewCache(), cwd: newCWDResolver(), transcript: newCodexTranscriptTracker()}
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

	// Correlating a live process to a session by PID never actually works
	// on this machine: chat_processes.json (the only source of PID->
	// session mapping) is absent, so byPID is always empty and si.PID
	// stays 0 for every history-derived session. Resolving the live
	// process's own session ID directly (from the most recently written
	// rollout file's name) and matching on ID instead is what actually
	// works — and avoids a real duplication bug this replaced: matching
	// on PID alone meant a live session already present in history.jsonl
	// got a SECOND card from the orphan-fallback path below, both with
	// the same session ID.
	liveSessionID := ""
	if live {
		liveSessionID, _ = findLatestRolloutSessionID(filepath.Join(a.home, ".codex"))
	}

	var out []domain.SessionInfo
	usedLiveID := false
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
		if live && id == liveSessionID {
			si.PID = livePID
			si.Alive = true
			si.Status = "busy"
			usedLiveID = true
		}
		a.enrichWithTranscript(&si)
		out = append(out, si)
	}

	// A live process whose session isn't in history.jsonl yet (started
	// moments ago) still gets surfaced as its own card, honestly labeled,
	// rather than hidden or duplicated.
	if live && !usedLiveID {
		si := domain.SessionInfo{Tool: Name, ID: liveSessionID, PID: livePID, Alive: true, Status: "busy"}
		if si.CWD == "" && liveSessionID != "" {
			si.CWD = a.cwd.resolve(filepath.Join(a.home, ".codex"), liveSessionID)
		}
		a.enrichWithTranscript(&si)
		out = append(out, si)
	}

	return out, nil
}

// enrichWithTranscript fills TokensIn/TokensOut/ContextUsedPct/LastAction
// from the session's own rollout file, when it has an ID to resolve one
// by. Mirrors the Claude adapter's per-session transcript enrichment —
// same architectural fix for the same class of bug (tool-wide numbers
// applied identically to every card of a tool).
func (a *Adapter) enrichWithTranscript(si *domain.SessionInfo) {
	if si.ID == "" {
		return
	}
	usage, ok := a.transcript.usageFor(filepath.Join(a.home, ".codex"), si.ID)
	if !ok {
		return
	}
	si.TokensIn = usage.TokensIn
	si.TokensOut = usage.TokensOut
	if usage.HasContext {
		si.ContextUsedPct = usage.ContextUsedPct
	}
	if usage.LastAction != "" {
		si.LastAction = usage.LastAction
	}
	if usage.Title != "" {
		si.Title = usage.Title
	}
	if usage.Model != "" {
		si.Model = usage.Model
	}
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
		if isCodexProcess(name, cmdline) {
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
	cpuPct, memMB, startedAt, statOK := a.procs.Stat(int32(pid))
	if !statOK {
		return nil, nil
	}
	return []domain.ProcessInfo{{PID: pid, Tool: Name, Label: "codex", CPUPct: cpuPct, MemMB: memMB, StartedAt: startedAt}}, nil
}

func unixToTime(sec int64) time.Time {
	if sec == 0 {
		return time.Time{}
	}
	return time.Unix(sec, 0)
}
