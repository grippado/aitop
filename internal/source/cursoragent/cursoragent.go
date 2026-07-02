// Package cursoragent adapts cursor-agent CLI state (~/.cursor/) into
// aitop's Source interface. cursor-agent is a standalone, headless CLI
// agent tool Cursor ships separately from its IDE — a distinct PROCESS
// that can run independently of Cursor.app — but NOT a fully separate
// storage system: its on-disk transcripts
// (~/.cursor/projects/*/agent-transcripts/) are, confirmed in practice,
// the same tree Cursor IDE's own "Agent" panel writes into (a real
// cursor-agent transcript filename and a Cursor IDE composerId can be the
// literal same UUID). Sessions() accounts for that overlap explicitly —
// see its own doc comment. The existing cursor package intentionally does
// not track cursor-agent's own process (see its isCursorIDEProcess doc
// comment) — this package is that gap filled in.
package cursoragent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	gproc "github.com/shirou/gopsutil/v3/process"

	"github.com/grippado/aitop/internal/domain"
	"github.com/grippado/aitop/internal/procstat"
)

// Name identifies this Source.
const Name = "cursor-agent"

type Adapter struct {
	home       string
	procs      *procstat.Cache
	transcript *transcriptTracker
	cwd        *cwdResolver
}

func New() *Adapter {
	home, _ := os.UserHomeDir()
	return &Adapter{
		home:       home,
		procs:      procstat.NewCache(),
		transcript: newTranscriptTracker(),
		cwd:        newCWDResolver(),
	}
}

func (a *Adapter) Name() string { return Name }

// Detect checks for cursor-agent's own CLI config file — distinct from
// every file the cursor (IDE) package reads, so a machine with only the
// IDE installed (no cursor-agent CLI ever run) correctly reports false.
func (a *Adapter) Detect(ctx context.Context) bool {
	_, err := reader.Stat(filepath.Join(a.home, ".cursor", "cli-config.json"))
	return err == nil
}

// isCursorAgentProcess matches the cursor-agent CLI's own invocation only:
// argv[0]'s basename exactly "cursor-agent" — the wrapper shell script's
// own name, preserved in argv0 through its exec into the real underlying
// node binary (confirmed on this machine: `ps` shows the original
// "/Users/.../bin/cursor-agent ..." invocation, not "node ..."). This
// deliberately excludes the "node .../index.js worker-server" background
// helper cursor-agent also spawns (argv0 "node", not "cursor-agent") and
// cursor-agent-svc, a separate always-on service binary unrelated to any
// specific live session.
func isCursorAgentProcess(cmdline string) bool {
	argv0 := cmdline
	if i := strings.IndexByte(cmdline, ' '); i >= 0 {
		argv0 = cmdline[:i]
	}
	return filepath.Base(argv0) == "cursor-agent"
}

func (a *Adapter) findLivePID(ctx context.Context) (int, bool) {
	procs, err := gproc.ProcessesWithContext(ctx)
	if err != nil {
		return 0, false
	}
	for _, p := range procs {
		cmdline, err := p.CmdlineWithContext(ctx)
		if err != nil || !isCursorAgentProcess(cmdline) {
			continue
		}
		return int(p.Pid), true
	}
	return 0, false
}

// Sessions surfaces the live cursor-agent CLI session, if any. Only ever
// one card: cursor-agent's on-disk state has no PID->session mapping this
// adapter could find (no chat_processes.json equivalent the way Codex
// has).
//
// The live PID's own cwd (via processCwd) is used to scope the transcript
// search to that ONE workspace's own agent-transcripts/ directory —
// confirmed necessary in practice, not just defensive: Cursor IDE's own
// "Agent" panel writes into the exact same ~/.cursor/projects/*/agent-
// transcripts/ tree cursor-agent CLI uses (same composerId, even — a
// cursor-agent transcript filename and a Cursor IDE composerId can be
// literally the same UUID), so an earlier version of this adapter that
// picked whichever transcript anywhere was most recently written could
// attribute a completely different, unrelated task (running in the IDE,
// nothing to do with this CLI process) to this card. If processCwd fails
// (lsof unavailable, permission denied), this falls back to the old
// global "most recently written wins" heuristic — less precise (can still
// misattribute across two genuinely concurrent cursor-agent CLI
// processes, or once more against the IDE's Agent panel), but still
// better than showing nothing, and never silently hidden.
func (a *Adapter) Sessions(ctx context.Context) ([]domain.SessionInfo, error) {
	pid, live := a.findLivePID(ctx)
	if !live {
		return nil, nil
	}

	si := domain.SessionInfo{
		Tool:      Name,
		PID:       pid,
		Alive:     true,
		Status:    "busy",
		UpdatedAt: time.Now(),
		Model:     friendlyModelName(currentModel(a.home)),
	}

	liveCWD, haveCWD := processCwd(ctx, pid)

	var path, sessionID string
	if haveCWD {
		path, sessionID = findLatestTranscriptInWorkspace(a.home, liveCWD)
	}
	if path == "" {
		path, sessionID = findLatestTranscript(a.home)
	}
	if path == "" {
		// A live process with no transcript written yet (cold start) —
		// still surface liveness, honestly with no ID/cwd/detail.
		return []domain.SessionInfo{si}, nil
	}
	si.ID = sessionID
	if haveCWD {
		si.CWD = liveCWD
	} else {
		si.CWD = a.cwd.resolve(path)
	}

	if usage, ok := a.transcript.usageFor(sessionID, path); ok {
		si.Title = usage.Title
		si.LastAction = usage.LastAction
	}
	return []domain.SessionInfo{si}, nil
}

// Processes returns real CPU/mem for the live cursor-agent CLI process, if
// any.
func (a *Adapter) Processes(ctx context.Context) ([]domain.ProcessInfo, error) {
	pid, live := a.findLivePID(ctx)
	if !live {
		return nil, nil
	}
	cpuPct, memMB, startedAt, ok := a.procs.Stat(int32(pid))
	if !ok {
		return nil, nil
	}
	return []domain.ProcessInfo{{PID: pid, Tool: Name, Label: "cursor-agent", CPUPct: cpuPct, MemMB: memMB, StartedAt: startedAt}}, nil
}

// slugifyCWD turns a real absolute path into cursor-agent's own project-
// dir encoding (leading "/" dropped, every remaining "/" replaced with
// "-") — the forward direction of what cwd.go's reconstructCWD undoes.
// Always unambiguous, unlike the reverse (a real folder name can itself
// contain "-"; going forward from a known-real path never has that
// problem).
func slugifyCWD(cwd string) string {
	return strings.ReplaceAll(strings.TrimPrefix(cwd, "/"), "/", "-")
}

// findLatestTranscriptInWorkspace scopes the transcript search to a
// single project directory — home/.cursor/projects/<slug(cwd)>/agent-
// transcripts/ — instead of the whole tree, so a card built from a known
// live PID's own cwd never picks up a different, unrelated workspace's
// activity (see Sessions' doc comment for why this matters in practice).
func findLatestTranscriptInWorkspace(home, cwd string) (path, sessionID string) {
	transcriptsDir := filepath.Join(home, ".cursor", "projects", slugifyCWD(cwd), "agent-transcripts")
	sessions, err := reader.ReadDir(transcriptsDir)
	if err != nil {
		return "", ""
	}

	var bestPath string
	var bestMod time.Time
	for _, s := range sessions {
		if !s.IsDir() {
			continue
		}
		file := filepath.Join(transcriptsDir, s.Name(), s.Name()+".jsonl")
		info, err := reader.Stat(file)
		if err != nil {
			continue
		}
		if info.ModTime().After(bestMod) {
			bestMod = info.ModTime()
			bestPath = file
		}
	}
	if bestPath == "" {
		return "", ""
	}
	return bestPath, strings.TrimSuffix(filepath.Base(bestPath), ".jsonl")
}

// findLatestTranscript scans every ~/.cursor/projects/*/agent-transcripts/*
// directory for the most recently modified *.jsonl file, returning its
// path and the session ID encoded in its filename (a UUID, same as its
// parent directory name). Bounded: two directory levels, no recursion
// beyond that. The global fallback Sessions() uses when the live PID's
// own cwd couldn't be resolved — see findLatestTranscriptInWorkspace for
// the precise, preferred path.
func findLatestTranscript(home string) (path, sessionID string) {
	projectsDir := filepath.Join(home, ".cursor", "projects")
	projects, err := reader.ReadDir(projectsDir)
	if err != nil {
		return "", ""
	}

	var bestPath string
	var bestMod time.Time
	for _, p := range projects {
		if !p.IsDir() {
			continue
		}
		transcriptsDir := filepath.Join(projectsDir, p.Name(), "agent-transcripts")
		sessions, err := reader.ReadDir(transcriptsDir)
		if err != nil {
			continue
		}
		for _, s := range sessions {
			if !s.IsDir() {
				continue
			}
			file := filepath.Join(transcriptsDir, s.Name(), s.Name()+".jsonl")
			info, err := reader.Stat(file)
			if err != nil {
				continue
			}
			if info.ModTime().After(bestMod) {
				bestMod = info.ModTime()
				bestPath = file
			}
		}
	}
	if bestPath == "" {
		return "", ""
	}
	return bestPath, strings.TrimSuffix(filepath.Base(bestPath), ".jsonl")
}
