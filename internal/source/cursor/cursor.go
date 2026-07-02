// Package cursor adapts Cursor's own built-in process-monitor telemetry
// (~/Library/Application Support/Cursor/process-monitor/*.log) into
// aitop's Source interface. Cursor ships real per-process CPU/mem samples
// itself — no need to shell out to gopsutil for this one. No cost data
// exists locally for Cursor, so Usage always reports Available:false.
package cursor

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	gproc "github.com/shirou/gopsutil/v3/process"

	"github.com/grippado/aitop/internal/domain"
	"github.com/grippado/aitop/internal/procstat"
)

// Name identifies this Source.
const Name = "cursor"

func monitorDir(home string) string {
	return filepath.Join(home, "Library", "Application Support", "Cursor", "process-monitor")
}

// sample mirrors one line of a process-monitor/<epoch-ms>.log file.
type sample struct {
	SessionID string      `json:"sessionId"`
	Rows      []sampleRow `json:"rows"`
}

type sampleRow struct {
	PID                 int     `json:"pid"`
	PPID                int     `json:"ppid"`
	ProcessName         string  `json:"processName"`
	SampleAvgMemMb      float64 `json:"sampleAvgMemMb"`
	CPUDuringSamplePeak float64 `json:"cpuDuringSamplePeakPct"`
}

type Adapter struct {
	home string

	// starts is used only for CreateTime lookups (Cursor's own log has no
	// process-start-time field) — CPU%/mem still come from the log itself,
	// which is more accurate than gopsutil sampling for this tool.
	starts *procstat.Cache

	// composer reads Cursor's own state.vscdb for real title/last-action/
	// token data — see composer.go. The process-monitor log this file
	// reads gives liveness/CPU/mem/a bare workspace folder name; composer
	// fills in everything a real chat transcript would, the same role
	// every other adapter's transcript tracker plays.
	composer *composerStore

	mu          sync.Mutex
	curFile     string
	offset      int64
	lastRows    map[int]domain.ProcessInfo // last-seen sample per PID, survives across ticks
	lastSess    map[int]string             // pid -> sessionId, from the log
	lastCWD     map[string]string          // sessionId -> best workspace label seen
	lastCWDRank map[string]int             // sessionId -> the [n-RANK] that produced lastCWD, higher wins
}

// isCursorOwnProcess restricts capture to Cursor's own executables — the
// main app binary and its Helper variants. process-monitor's rows[] also
// includes descendants of Cursor's integrated terminal (a user's shell,
// and whatever they run in it: git, docker, kubectl, ssh, ...), which are
// NOT Cursor's own resource consumption and must not be attributed to it.
func isCursorOwnProcess(processName string) bool {
	return strings.HasPrefix(processName, "/Applications/Cursor.app/") ||
		strings.HasPrefix(processName, "Cursor Helper")
}

// extensionHostWorkspace extracts a workspace label from an extension-host
// row's processName, e.g. "Cursor Helper (Plugin): extension-host  backoffice [1-5]"
// -> ("backoffice", 5). Cursor's own naming convention, not a real
// filesystem path — the best available cwd-ish signal since the log
// carries no explicit cwd field. "empty" (Cursor's placeholder for a
// window with no folder open) is treated as no signal. The trailing
// [n-RANK] second number is used as a recency proxy: a session can have
// several open workspaces, and the highest-ranked one wins as the card's
// representative cwd.
var extensionHostPattern = regexp.MustCompile(`extension-host\s+(\S+)\s+\[\d+-(\d+)\]`)

func extensionHostWorkspace(processName string) (label string, rank int, ok bool) {
	m := extensionHostPattern.FindStringSubmatch(processName)
	if m == nil || m[1] == "empty" {
		return "", 0, false
	}
	rank, err := strconv.Atoi(m[2])
	if err != nil {
		return "", 0, false
	}
	return m[1], rank, true
}

func New() *Adapter {
	home, _ := os.UserHomeDir()
	return &Adapter{
		home:        home,
		starts:      procstat.NewCache(),
		composer:    newComposerStore(home),
		lastRows:    map[int]domain.ProcessInfo{},
		lastSess:    map[int]string{},
		lastCWD:     map[string]string{},
		lastCWDRank: map[string]int{},
	}
}

func (a *Adapter) Name() string { return Name }

func (a *Adapter) Detect(ctx context.Context) bool {
	_, err := reader.Stat(monitorDir(a.home))
	return err == nil
}

// latestLogFile picks the highest <epoch-ms>.log by NUMERIC filename value
// (the canonical ordering key per Cursor's own naming), not mtime.
func (a *Adapter) latestLogFile() (string, error) {
	entries, err := reader.ReadDir(monitorDir(a.home))
	if err != nil {
		return "", err
	}
	var best string
	var bestTS int64 = -1
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
			continue
		}
		tsStr := strings.TrimSuffix(e.Name(), ".log")
		ts, err := strconv.ParseInt(tsStr, 10, 64)
		if err != nil {
			continue
		}
		if ts > bestTS {
			bestTS = ts
			best = e.Name()
		}
	}
	if best == "" {
		return "", errors.New("no process-monitor log files found")
	}
	return filepath.Join(monitorDir(a.home), best), nil
}

// poll advances the tail-follow position, handling rotation/shrink, and
// returns any newly-appended bytes. Called by both Processes and Sessions
// so they always see a consistent view within one tick.
func (a *Adapter) poll() ([]byte, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	latest, err := a.latestLogFile()
	if err != nil {
		return nil, err
	}

	if latest != a.curFile {
		// The active log rotated to a new filename — start fresh on it.
		a.curFile = latest
		a.offset = 0
	}

	info, err := reader.Stat(a.curFile)
	if err != nil {
		return nil, err
	}
	if info.Size() < a.offset {
		// Same filename, but shrunk (truncated/rotated in place) — the old
		// offset is now meaningless. Reset and re-read from the top rather
		// than seeking into garbage.
		a.offset = 0
	}

	data, newSize, err := reader.ReadFrom(a.curFile, a.offset)
	if err != nil {
		return nil, err
	}
	a.offset = newSize
	return data, nil
}

func (a *Adapter) ingest(data []byte) (parsedAny bool) {
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		var s sample
		if err := json.Unmarshal(sc.Bytes(), &s); err != nil {
			continue
		}
		parsedAny = true
		for _, r := range s.Rows {
			if !isCursorOwnProcess(r.ProcessName) {
				// A descendant of Cursor's integrated terminal (a shell,
				// or whatever the user ran in it) — real, but not Cursor's
				// own resource consumption. Don't attribute it to the tool.
				continue
			}
			// CPU%/mem come from Cursor's own log (more accurate than a
			// fresh gopsutil sample); only CreateTime is looked up
			// separately, since the log carries no start-time field.
			_, _, startedAt, _ := a.starts.Stat(int32(r.PID))
			a.lastRows[r.PID] = domain.ProcessInfo{
				PID:       r.PID,
				PPID:      r.PPID,
				Tool:      Name,
				Label:     r.ProcessName,
				CPUPct:    r.CPUDuringSamplePeak,
				MemMB:     r.SampleAvgMemMb,
				StartedAt: startedAt,
			}
			if s.SessionID != "" {
				a.lastSess[r.PID] = s.SessionID
				if label, rank, ok := extensionHostWorkspace(r.ProcessName); ok && rank >= a.lastCWDRank[s.SessionID] {
					a.lastCWD[s.SessionID] = label
					a.lastCWDRank[s.SessionID] = rank
				}
			}
		}
	}
	return parsedAny
}

// Processes returns the last-known-good per-PID samples. If new log data
// arrived this tick but none of it parsed, that's surfaced as an error
// (becomes a visible "detected, log not parseable" note upstream) while
// still returning whatever was cached from earlier ticks — never a silent
// blank-out.
func (a *Adapter) Processes(ctx context.Context) ([]domain.ProcessInfo, error) {
	data, err := a.poll()

	a.mu.Lock()
	defer a.mu.Unlock()

	var polErr error
	if err != nil {
		polErr = fmt.Errorf("cursor detectado, log não parseável (%w)", err)
	} else if len(data) > 0 {
		if !a.ingest(data) {
			polErr = errors.New("cursor detectado, log não parseável (formato inesperado)")
		}
	}

	out := make([]domain.ProcessInfo, 0, len(a.lastRows))
	for _, p := range a.lastRows {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PID < out[j].PID })

	return out, polErr
}

// Sessions groups cached rows by sessionId, and marks liveness from an
// independent ps check — never gated on the log having parsed correctly,
// so a corrupted log still shows Cursor as running.
func (a *Adapter) Sessions(ctx context.Context) ([]domain.SessionInfo, error) {
	_, _ = a.poll() // best-effort refresh; errors surface via Processes

	alive := processAlive(ctx)

	a.mu.Lock()
	defer a.mu.Unlock()

	seen := map[string]bool{}
	var out []domain.SessionInfo
	for pid, sid := range a.lastSess {
		if seen[sid] {
			continue
		}
		seen[sid] = true
		status := "idle"
		if alive {
			status = "busy"
		}
		si := domain.SessionInfo{
			Tool:      Name,
			ID:        sid,
			PID:       pid,
			Alive:     alive,
			CWD:       a.lastCWD[sid], // "" when no extension-host workspace label was ever seen
			Status:    status,
			UpdatedAt: time.Now(),
		}
		a.enrichWithComposer(&si, a.lastCWD[sid])
		out = append(out, si)
	}
	if len(out) == 0 && alive {
		// Process is running but we have no session-tagged rows yet (cold
		// start, or a log we can't parse) — still surface liveness.
		si := domain.SessionInfo{Tool: Name, Alive: true, Status: "busy", UpdatedAt: time.Now()}
		a.enrichWithComposer(&si, "")
		out = append(out, si)
	}
	return out, nil
}

// enrichWithComposer fills ID/Title/CWD/LastAction/tokens/context% from
// Cursor's own state.vscdb when a matching composer is found for
// workspaceLabel — the same per-session transcript enrichment every other
// adapter's Sessions() does, just against a real SQLite store instead of
// a JSONL transcript. CWD gets upgraded from the process-monitor log's
// bare folder name to the composer's real full filesystem path when a
// match is found — a strictly more precise reading of the same fact, not
// a different one.
//
// si.ID becomes the composer's own ComposerID — this matters beyond
// display: confirmed on this machine, a cursor-agent CLI run shares its
// composerId with Cursor IDE's own composer store (the CLI's transcript
// file is literally named after it) when the same real task shows up in
// both places. Setting ID here is what lets cards.BuildCards recognize
// "this IDE session and that cursor-agent session are the same real task"
// and dedup the redundant card instead of showing two.
func (a *Adapter) enrichWithComposer(si *domain.SessionInfo, workspaceLabel string) {
	c, ok := a.composer.bestComposerForWorkspace(workspaceLabel)
	if !ok {
		return
	}
	si.ID = c.ComposerID
	si.Title = c.Name
	if c.WorkspaceIdentifier.URI != nil && c.WorkspaceIdentifier.URI.FsPath != "" {
		si.CWD = c.WorkspaceIdentifier.URI.FsPath
	}
	if c.ContextUsagePercent > 0 && c.ContextUsagePercent <= 100 {
		si.ContextUsedPct = c.ContextUsagePercent
	}
	if usage, ok := a.composer.usageForComposer(c.ComposerID); ok {
		si.LastAction = usage.LastAction
		si.TokensIn = usage.TokensIn
		si.TokensOut = usage.TokensOut
	}
}

// isCursorIDEProcess matches the real Cursor.app IDE process only. A
// loose strings.Contains(name, "Cursor") false-positives on macOS's own
// always-running CursorUIViewService (a text-input system helper,
// nothing to do with the Cursor IDE) and on "cursor-agent" (Cursor's
// separate CLI/headless agent tool — a real, distinct process this
// adapter doesn't track today, not the IDE this package is built
// around). Confirmed in practice: with Cursor.app fully closed, the old
// check still reported "alive" because of CursorUIViewService.
func isCursorIDEProcess(name, exe string) bool {
	if name == "Cursor" || strings.HasPrefix(name, "Cursor Helper") {
		return true
	}
	return strings.HasPrefix(exe, "/Applications/Cursor.app/")
}

func processAlive(ctx context.Context) bool {
	procs, err := gproc.ProcessesWithContext(ctx)
	if err != nil {
		return false
	}
	for _, p := range procs {
		name, _ := p.NameWithContext(ctx)
		exe, _ := p.ExeWithContext(ctx)
		if isCursorIDEProcess(name, exe) {
			return true
		}
	}
	return false
}
