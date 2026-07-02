// Package opencode adapts opencode CLI state into aitop's Source
// interface. Unlike every other adapter here, opencode's own state is a
// real SQLite database (~/.local/share/opencode/opencode.db) rather than
// plain JSON/JSONL files — confirmed on this machine's real install: a
// `session` table already carries directory/title/model/token totals
// directly, no transcript reconstruction needed. modernc.org/sqlite (pure
// Go, no cgo) is used to read it, keeping the CGO_ENABLED=0 release build
// intact.
package opencode

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	gproc "github.com/shirou/gopsutil/v3/process"
	_ "modernc.org/sqlite"

	"github.com/grippado/aitop/internal/domain"
	"github.com/grippado/aitop/internal/procstat"
)

// Name identifies this Source.
const Name = "opencode"

type Adapter struct {
	home  string
	procs *procstat.Cache

	mu sync.Mutex
	db *sql.DB

	models *modelsCache
}

func New() *Adapter {
	home, _ := os.UserHomeDir()
	return &Adapter{home: home, procs: procstat.NewCache(), models: newModelsCache(home)}
}

func (a *Adapter) Name() string { return Name }

func (a *Adapter) dbPath() string {
	return filepath.Join(a.home, ".local", "share", "opencode", "opencode.db")
}

func (a *Adapter) Detect(ctx context.Context) bool {
	_, err := reader.Stat(a.dbPath())
	return err == nil
}

// open lazily opens one read-only connection to opencode's live database
// and keeps it for the adapter's lifetime — a single connection (not a
// pool) is deliberate: this is a read-only observer polling every few
// seconds, not a concurrent workload, and it avoids any surprise around
// multiple readers against a database opencode itself is actively writing
// to in WAL mode. mode=ro never modifies the file; SQLite's WAL design
// already guarantees a reader sees a consistent snapshot even while the
// real opencode process keeps writing.
func (a *Adapter) open() (*sql.DB, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.db != nil {
		return a.db, nil
	}
	db, err := sql.Open("sqlite", "file:"+a.dbPath()+"?mode=ro")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	a.db = db
	return db, nil
}

// isOpencodeProcess matches the opencode CLI's own binary — an exact
// process name, or argv[0] exactly "opencode" / ending in "/opencode".
// Mirrors Codex's isCodexProcess for the same reason: a substring match
// against the whole cmdline would false-positive on anything merely
// mentioning "opencode" (a shell command, an edited file path, this very
// package).
func isOpencodeProcess(name, cmdline string) bool {
	if name == "opencode" {
		return true
	}
	argv0 := cmdline
	if i := strings.IndexByte(cmdline, ' '); i >= 0 {
		argv0 = cmdline[:i]
	}
	return argv0 == "opencode" || strings.HasSuffix(argv0, "/opencode")
}

func (a *Adapter) findLivePID(ctx context.Context) (int, bool) {
	procs, err := gproc.ProcessesWithContext(ctx)
	if err != nil {
		return 0, false
	}
	for _, p := range procs {
		name, _ := p.NameWithContext(ctx)
		cmdline, _ := p.CmdlineWithContext(ctx)
		if isOpencodeProcess(name, cmdline) {
			return int(p.Pid), true
		}
	}
	return 0, false
}

// sessionModel mirrors the `model` column's JSON shape — confirmed on this
// machine's real session data: {"id":"...","providerID":"...","variant":"..."}.
type sessionModel struct {
	ID         string `json:"id"`
	ProviderID string `json:"providerID"`
}

// Sessions surfaces the live opencode session, if any. Only one card:
// opencode's `session` table has no PID column (no process ever recorded
// as owning a session row), so a live process is paired with whichever
// session was most recently updated — the single global opencode.db is
// actively written by exactly the one running process this adapter found,
// making that pairing unambiguous in the common single-instance case.
// Running two concurrent opencode processes collapses them onto this one
// card, attributed to whichever session is more recently active — a known
// v1 limitation, the same one cursor-agent's adapter documents for the
// same structural reason.
func (a *Adapter) Sessions(ctx context.Context) ([]domain.SessionInfo, error) {
	pid, live := a.findLivePID(ctx)
	if !live {
		return nil, nil
	}

	db, err := a.open()
	if err != nil {
		return []domain.SessionInfo{{Tool: Name, PID: pid, Alive: true, Status: "busy", UpdatedAt: time.Now()}}, nil
	}

	row, ok := loadLatestSession(ctx, db)
	if !ok {
		// A live process with no session row yet (cold start) — still
		// surface liveness, honestly with no ID/cwd/detail.
		return []domain.SessionInfo{{Tool: Name, PID: pid, Alive: true, Status: "busy", UpdatedAt: time.Now()}}, nil
	}

	si := domain.SessionInfo{
		Tool:      Name,
		ID:        row.ID,
		PID:       pid,
		Alive:     true,
		CWD:       row.Directory,
		Title:     row.Title,
		Status:    "busy",
		UpdatedAt: msToTime(row.UpdatedMs),
		// TokensIn mirrors the Claude adapter's contextTokens() convention:
		// input + everything cached in/out — the full set of tokens making
		// up the model's context this turn, not just the newest input.
		TokensIn:  row.TokensIn + row.CacheRead + row.CacheWrite,
		TokensOut: row.TokensOut,
	}

	var model sessionModel
	if json.Unmarshal([]byte(row.ModelJSON), &model) == nil && model.ID != "" {
		if window, ok := a.models.contextWindow(model.ProviderID, model.ID); ok && window > 0 {
			if pct := float64(si.TokensIn) / float64(window) * 100; pct <= 100 {
				si.ContextUsedPct = pct
			}
		}
		if name, ok := a.models.friendlyName(model.ProviderID, model.ID); ok {
			si.Model = name
		} else {
			// The models.json cache doesn't have this model (a fresh
			// install, or a provider/model combo it hasn't fetched yet) —
			// fall back to the raw id, at least dash-cleaned, rather than
			// leaving the pill blank.
			si.Model = strings.ReplaceAll(model.ID, "-", " ")
		}
	}

	si.LastAction = a.lastAction(ctx, db, row.ID)

	return []domain.SessionInfo{si}, nil
}

// sessionRow is the raw `session` table columns this adapter reads,
// factored out of Sessions() so the SQL + scan logic is testable against a
// real temp SQLite file without needing a live opencode process running.
type sessionRow struct {
	ID                                         string
	Directory, Title, ModelJSON                string
	TokensIn, TokensOut, CacheRead, CacheWrite int64
	UpdatedMs                                  int64
}

func loadLatestSession(ctx context.Context, db *sql.DB) (sessionRow, bool) {
	var r sessionRow
	row := db.QueryRowContext(ctx,
		`SELECT id, directory, title, model, tokens_input, tokens_output, tokens_cache_read, tokens_cache_write, time_updated FROM session ORDER BY time_updated DESC LIMIT 1`)
	if err := row.Scan(&r.ID, &r.Directory, &r.Title, &r.ModelJSON, &r.TokensIn, &r.TokensOut, &r.CacheRead, &r.CacheWrite, &r.UpdatedMs); err != nil {
		return sessionRow{}, false
	}
	return r, true
}

// lastAction finds the most recently created message in sessionID and
// summarizes its last meaningful part (a tool call or the agent's own
// commentary) — best-effort, "" on any lookup failure.
func (a *Adapter) lastAction(ctx context.Context, db *sql.DB, sessionID string) string {
	var msgID string
	row := db.QueryRowContext(ctx, `SELECT id FROM message WHERE session_id = ? ORDER BY time_created DESC LIMIT 1`, sessionID)
	if err := row.Scan(&msgID); err != nil {
		return ""
	}

	rows, err := db.QueryContext(ctx, `SELECT data FROM part WHERE message_id = ? ORDER BY time_created DESC`, msgID)
	if err != nil {
		return ""
	}
	defer rows.Close()

	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			continue
		}
		if action := summarizePart(raw); action != "" {
			return action
		}
	}
	return ""
}

// Processes returns real CPU/mem for the live opencode process, if any.
func (a *Adapter) Processes(ctx context.Context) ([]domain.ProcessInfo, error) {
	pid, live := a.findLivePID(ctx)
	if !live {
		return nil, nil
	}
	cpuPct, memMB, startedAt, ok := a.procs.Stat(int32(pid))
	if !ok {
		return nil, nil
	}
	return []domain.ProcessInfo{{PID: pid, Tool: Name, Label: "opencode", CPUPct: cpuPct, MemMB: memMB, StartedAt: startedAt}}, nil
}

func msToTime(ms int64) time.Time {
	if ms == 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms)
}
