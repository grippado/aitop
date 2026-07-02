// Package cursoragent adapts cursor-agent CLI state (~/.cursor/) into
// aitop's Source interface. cursor-agent is a standalone, headless CLI
// agent tool Cursor ships separately from its IDE — a distinct process
// with its own on-disk state (~/.cursor/projects/*/agent-transcripts/),
// confirmed on this machine to be running independently of Cursor.app.
// The existing cursor package intentionally does not track it (see its
// isCursorIDEProcess doc comment) — this package is that gap filled in.
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
// has), so a live process is paired with whichever transcript under
// ~/.cursor/projects/*/agent-transcripts/ was most recently written to —
// the same "most recently written wins" heuristic Codex's adapter uses to
// resolve a just-started session before its own history file catches up.
// Running two concurrent cursor-agent CLI sessions on the same machine
// collapses them onto this one card, attributed to whichever is more
// recently active — a known v1 limitation, not silently hidden.
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

	path, sessionID := findLatestTranscript(a.home)
	if path == "" {
		// A live process with no transcript written yet (cold start) —
		// still surface liveness, honestly with no ID/cwd/detail.
		return []domain.SessionInfo{si}, nil
	}
	si.ID = sessionID
	si.CWD = a.cwd.resolve(path)

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

// findLatestTranscript scans every ~/.cursor/projects/*/agent-transcripts/*
// directory for the most recently modified *.jsonl file, returning its
// path and the session ID encoded in its filename (a UUID, same as its
// parent directory name). Bounded: two directory levels, no recursion
// beyond that.
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
