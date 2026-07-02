package cursor

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"

	_ "modernc.org/sqlite"
)

// composerDBPath is Cursor IDE's own VSCode-style global state store —
// confirmed on this machine to hold composer.composerHeaders (an index of
// every chat/agent session, real title + real workspace path included) in
// ItemTable, and per-message "bubbles" (bubbleId:<composerId>:<bubbleId>
// keys, real text/tool-call/token data) in cursorDiskKV. This is a big
// file — 300+MB on this machine, the IDE's entire chat history across
// every workspace ever opened — so every query here is either a single
// indexed key lookup or an indexed GLOB prefix scan (confirmed via EXPLAIN
// QUERY PLAN: GLOB hits the key index; the equivalent LIKE pattern does
// not and falls back to a full table scan — GLOB is used everywhere here
// for exactly that reason), never a full scan.
func composerDBPath(home string) string {
	return filepath.Join(home, "Library", "Application Support", "Cursor", "User", "globalStorage", "state.vscdb")
}

// composerHeader mirrors one entry of composer.composerHeaders' JSON
// allComposers array — confirmed shape on this machine's real data.
// ContextUsagePercent is Cursor's OWN computed reading (confirmed present
// on real composers, e.g. 27.96) — authoritative, not derived from a
// guessed context-window size the way Claude Code's adapter has to.
type composerHeader struct {
	ComposerID          string  `json:"composerId"`
	Name                string  `json:"name"`
	LastUpdatedAt       int64   `json:"lastUpdatedAt"`
	IsArchived          bool    `json:"isArchived"`
	IsDraft             bool    `json:"isDraft"`
	ContextUsagePercent float64 `json:"contextUsagePercent"`
	WorkspaceIdentifier struct {
		URI *struct {
			FsPath string `json:"fsPath"`
		} `json:"uri"`
	} `json:"workspaceIdentifier"`
}

type composerHeadersFile struct {
	AllComposers []composerHeader `json:"allComposers"`
}

// bubble mirrors one message in a composer's conversation — confirmed
// shape on this machine's real data (a much larger struct in practice;
// only the fields this adapter reads are declared). type 1 is the user's
// own message, type 2 is the assistant's (either commentary text or a
// tool call, distinguished by ToolFormerData being present).
type bubble struct {
	Type           int    `json:"type"`
	Text           string `json:"text"`
	ToolFormerData *struct {
		Name string `json:"name"`
	} `json:"toolFormerData"`
	TokenCount *struct {
		InputTokens  int64 `json:"inputTokens"`
		OutputTokens int64 `json:"outputTokens"`
	} `json:"tokenCount"`
}

// composerUsage is the latest reading found in a composer's own bubbles.
type composerUsage struct {
	LastAction string
	TokensIn   int64
	TokensOut  int64
}

// composerStore is a lazily-opened, read-only connection to Cursor's own
// state.vscdb — kept open for the adapter's lifetime, mirroring the
// opencode adapter's single-connection convention (a read-only observer
// polling every few seconds, not a concurrent workload; mode=ro never
// modifies a file the real Cursor IDE may be actively writing to).
type composerStore struct {
	home string

	mu sync.Mutex
	db *sql.DB
}

func newComposerStore(home string) *composerStore {
	return &composerStore{home: home}
}

func (s *composerStore) open() (*sql.DB, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		return s.db, nil
	}
	db, err := sql.Open("sqlite", "file:"+composerDBPath(s.home)+"?mode=ro")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s.db = db
	return db, nil
}

// bestComposerForWorkspace returns the most recently updated, non-
// archived, non-draft composer whose workspace folder matches
// workspaceLabel — a case-insensitive basename match against the
// composer's real fsPath, since the process-monitor log this package's
// main file reads only ever gives a bare folder name (e.g. "backoffice"),
// never a full path. workspaceLabel == "" picks the single most recently
// updated composer across every workspace instead — the same
// "most-recently-active-wins" fallback the cursor-agent and opencode
// adapters use when a precise PID->session mapping isn't available.
//
// A composer with no Name yet (a real session, just not auto-titled by
// Cursor yet — the same gap Claude Code's own transcript has before its
// first ai-title event) is NOT filtered out here: leaving si.Title empty
// in that case is correct, and the UI's own fallbackTitle already handles
// it — filtering it out here would instead surface a STALE older,
// already-named composer as if it were the current one, which is worse.
func (s *composerStore) bestComposerForWorkspace(workspaceLabel string) (composerHeader, bool) {
	db, err := s.open()
	if err != nil {
		return composerHeader{}, false
	}

	var raw string
	row := db.QueryRow(`SELECT value FROM ItemTable WHERE key = 'composer.composerHeaders'`)
	if err := row.Scan(&raw); err != nil {
		return composerHeader{}, false
	}

	var hf composerHeadersFile
	if err := json.Unmarshal([]byte(raw), &hf); err != nil {
		return composerHeader{}, false
	}

	var best composerHeader
	found := false
	for _, c := range hf.AllComposers {
		if c.IsArchived || c.IsDraft {
			continue
		}
		if workspaceLabel != "" {
			if c.WorkspaceIdentifier.URI == nil || !strings.EqualFold(filepath.Base(c.WorkspaceIdentifier.URI.FsPath), workspaceLabel) {
				continue
			}
		}
		if !found || c.LastUpdatedAt > best.LastUpdatedAt {
			best = c
			found = true
		}
	}
	return best, found
}

// usageForComposer reads composerID's own message bubbles for the most
// recent last-action reading and the most recent non-zero token count —
// the same "keep whatever was last found, overwrite as newer readings
// appear" convention every other adapter's transcript reader uses, since
// not every bubble carries a token count (most read 0/0; only the
// composer's own periodic accounting bubbles have real numbers).
// ORDER BY rowid recovers chronological order after the GLOB search
// (whose result order otherwise follows the key's — effectively random,
// since a bubbleId's UUID suffix carries no chronological meaning).
func (s *composerStore) usageForComposer(composerID string) (composerUsage, bool) {
	db, err := s.open()
	if err != nil {
		return composerUsage{}, false
	}

	rows, err := db.Query(`SELECT value FROM cursorDiskKV WHERE key GLOB ? ORDER BY rowid`, "bubbleId:"+composerID+":*")
	if err != nil {
		return composerUsage{}, false
	}
	defer rows.Close()

	var u composerUsage
	have := false
	for rows.Next() {
		var raw sql.NullString
		if err := rows.Scan(&raw); err != nil || !raw.Valid {
			continue
		}
		var b bubble
		if err := json.Unmarshal([]byte(raw.String), &b); err != nil {
			continue
		}
		if action := summarizeBubble(b); action != "" {
			u.LastAction = action
			have = true
		}
		if b.TokenCount != nil && (b.TokenCount.InputTokens > 0 || b.TokenCount.OutputTokens > 0) {
			u.TokensIn = b.TokenCount.InputTokens
			u.TokensOut = b.TokenCount.OutputTokens
			have = true
		}
	}
	return u, have
}

// summarizeBubble condenses one bubble into a short "🔧 name" / "💭 text"
// description, mirroring the convention every other adapter's transcript
// reader uses. A type-1 (user) bubble ending up as the LAST bubble only
// happens when the assistant hasn't replied yet — still worth showing,
// tagged distinctly ("👤") rather than mislabeled as the agent's own
// commentary.
func summarizeBubble(b bubble) string {
	if b.ToolFormerData != nil && b.ToolFormerData.Name != "" {
		return "🔧 " + b.ToolFormerData.Name
	}
	text := strings.TrimSpace(b.Text)
	if text == "" {
		return ""
	}
	prefix := "💭 "
	if b.Type == 1 {
		prefix = "👤 "
	}
	return prefix + clampComposerText(text, 200)
}

func clampComposerText(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
