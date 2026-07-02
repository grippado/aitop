package claude

import (
	"bufio"
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
)

// transcriptUsage is the latest real token reading found in a session's
// own transcript — this is what actually fills the gap ccstatusline's
// usage.json leaves: it has rate limits (5h/7d) but no context-window
// size or token counts. Every Claude Code assistant turn's transcript
// line carries a real "usage" block (confirmed on this machine's actual
// session file) with these exact fields.
type transcriptUsage struct {
	Model                    string
	InputTokens              int64
	CacheCreationInputTokens int64
	CacheReadInputTokens     int64
	OutputTokens             int64
	// LastAction is a short summary of the most recent tool call or
	// thinking snippet, e.g. "🔧 Bash: go test ./..." — the same content
	// mutirao's stream-fmt.jq extracts from message.content[], just
	// condensed to one line instead of a live multi-line stream.
	LastAction string
	// Title is Claude Code's own auto-generated session title (a
	// top-level {"type":"ai-title","aiTitle":"..."} line it writes to the
	// transcript itself, re-titling as the conversation's topic shifts) —
	// the direct analog of mutirao's per-mão task title.
	Title string
}

// contextTokens is "how many tokens make up the model's context this
// turn" — input + everything cached in/out — the same quantity Claude
// Code's own statusLine hook divides by context_window_size to get a
// used-percentage. This is a real, computed approximation, not the exact
// number Claude Code's live hook would report (this adapter has no access
// to that live stream), but it's grounded in the same transcript data,
// not fabricated.
func (u transcriptUsage) contextTokens() int64 {
	return u.InputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens
}

// contextWindowForModel is a best-effort lookup: every current Claude
// model this adapter has seen uses a 200k-token context window. If a
// model reports something unrecognized, 200000 is still used as the
// default rather than leaving ContextUsedPct entirely unavailable —
// noted here in case a future model changes this.
func contextWindowForModel(model string) int64 {
	return 200000
}

// contentBlock mirrors one entry of message.content[] — confirmed shape
// on this machine's real transcripts: {"type":"tool_use","name":"Bash",
// "input":{"command":"..."}} or {"type":"text","text":"..."}. Same
// fields mutirao's stream-fmt.jq reads (input.file_path // .command //
// .pattern // .description // .path).
type contentBlock struct {
	Type  string `json:"type"`
	Text  string `json:"text"`
	Name  string `json:"name"`
	Input struct {
		FilePath    string `json:"file_path"`
		Command     string `json:"command"`
		Pattern     string `json:"pattern"`
		Description string `json:"description"`
		Path        string `json:"path"`
	} `json:"input"`
}

type transcriptLine struct {
	Type    string `json:"type"`    // top-level: "assistant", "user", "ai-title", "bridge-session", ...
	AiTitle string `json:"aiTitle"` // populated only when Type == "ai-title"
	Message struct {
		Model   string         `json:"model"`
		Content []contentBlock `json:"content"`
		Usage   struct {
			InputTokens              int64 `json:"input_tokens"`
			CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
			OutputTokens             int64 `json:"output_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// summarizeLastAction condenses the LAST content block of a turn into one
// display line — mirrors mutirao's tool_use/text handling, minus the
// live multi-line stream (aitop polls a snapshot, it doesn't tail a
// pane). Returns "" for block types with nothing worth summarizing
// (e.g. empty text, or a type this adapter doesn't recognize).
func summarizeLastAction(blocks []contentBlock) string {
	if len(blocks) == 0 {
		return ""
	}
	b := blocks[len(blocks)-1]
	switch b.Type {
	case "tool_use":
		name := b.Name
		if name == "" {
			name = "tool"
		}
		detail := firstNonEmpty(b.Input.FilePath, b.Input.Command, b.Input.Pattern, b.Input.Description, b.Input.Path)
		if detail == "" {
			return "🔧 " + name
		}
		return "🔧 " + name + ": " + clampText(detail, 50)
	case "text":
		txt := strings.TrimSpace(b.Text)
		if txt == "" {
			return ""
		}
		return "💭 " + clampText(txt, 60)
	default:
		return ""
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// clampText collapses whitespace/newlines (mirrors mutirao's
// gsub("\\s+";" ")) and truncates to n runes with an ellipsis.
func clampText(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

// transcriptTracker tail-follows each session's transcript file for its
// most recent token usage, mirroring the byte-offset + rotation-safe
// approach used for Cursor's process-monitor log: only new bytes are
// read each call, and a shrink/rotation resets the offset instead of
// seeking into garbage.
type transcriptTracker struct {
	mu      sync.Mutex
	paths   map[string]string // sessionID -> resolved transcript path, cached (doesn't move)
	sizes   map[string]int64  // sessionID -> file size as of last read (detects shrink)
	offsets map[string]int64
	latest  map[string]transcriptUsage
}

func newTranscriptTracker() *transcriptTracker {
	return &transcriptTracker{
		paths:   map[string]string{},
		sizes:   map[string]int64{},
		offsets: map[string]int64{},
		latest:  map[string]transcriptUsage{},
	}
}

// usageFor returns the latest known usage for sessionID, tailing its
// transcript for any new data first. ok=false means no usage line has
// ever been found for this session (not "zero usage" — genuinely no
// reading yet).
func (t *transcriptTracker) usageFor(configDir, cwd, sessionID string) (transcriptUsage, bool) {
	path := t.resolvePath(configDir, cwd, sessionID)
	if path == "" {
		return t.get(sessionID)
	}

	info, err := reader.Stat(path)
	if err != nil {
		return t.get(sessionID)
	}
	size := info.Size()

	t.mu.Lock()
	offset := t.offsets[sessionID]
	if t.sizes[sessionID] > size {
		offset = 0 // rotated/truncated since we last looked
	}
	t.mu.Unlock()

	if size > offset {
		data, newSize, err := reader.ReadFrom(path, offset)
		if err == nil {
			t.ingest(sessionID, data)
			t.mu.Lock()
			t.offsets[sessionID] = newSize
			t.sizes[sessionID] = newSize
			t.mu.Unlock()
		}
	}

	return t.get(sessionID)
}

func (t *transcriptTracker) get(sessionID string) (transcriptUsage, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	u, ok := t.latest[sessionID]
	return u, ok
}

func (t *transcriptTracker) ingest(sessionID string, data []byte) {
	t.mu.Lock()
	found := t.latest[sessionID] // preserve whatever was already known
	t.mu.Unlock()
	have := false

	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		var l transcriptLine
		if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
			continue
		}

		if l.Type == "ai-title" && l.AiTitle != "" {
			found.Title = l.AiTitle
			have = true
		}

		u := l.Message.Usage
		if u.InputTokens != 0 || u.OutputTokens != 0 || u.CacheReadInputTokens != 0 || u.CacheCreationInputTokens != 0 {
			found.Model = l.Message.Model
			found.InputTokens = u.InputTokens
			found.CacheCreationInputTokens = u.CacheCreationInputTokens
			found.CacheReadInputTokens = u.CacheReadInputTokens
			found.OutputTokens = u.OutputTokens
			have = true
		}

		if action := summarizeLastAction(l.Message.Content); action != "" {
			found.LastAction = action
			have = true
		}
	}
	if !have {
		return
	}
	t.mu.Lock()
	t.latest[sessionID] = found
	t.mu.Unlock()
}

// resolvePath finds a session's transcript under
// <configDir>/projects/<encoded-cwd>/<sessionID>.jsonl. The encoded-cwd
// guess (cwd with every "/" replaced by "-") is Claude Code's usual
// convention and avoids a directory scan in the common case; if that
// exact file doesn't exist, a bounded fallback lists projects/ once and
// checks each project dir for <sessionID>.jsonl directly (no deeper
// recursion). Resolved paths are cached — a transcript's location never
// changes for the life of a session.
func (t *transcriptTracker) resolvePath(configDir, cwd, sessionID string) string {
	t.mu.Lock()
	if p, ok := t.paths[sessionID]; ok {
		t.mu.Unlock()
		return p
	}
	t.mu.Unlock()

	projectsDir := filepath.Join(configDir, "projects")

	if cwd != "" {
		guess := filepath.Join(projectsDir, strings.ReplaceAll(cwd, "/", "-"), sessionID+".jsonl")
		if _, err := reader.Stat(guess); err == nil {
			t.mu.Lock()
			t.paths[sessionID] = guess
			t.mu.Unlock()
			return guess
		}
	}

	entries, err := reader.ReadDir(projectsDir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		candidate := filepath.Join(projectsDir, e.Name(), sessionID+".jsonl")
		if _, err := reader.Stat(candidate); err == nil {
			t.mu.Lock()
			t.paths[sessionID] = candidate
			t.mu.Unlock()
			return candidate
		}
	}
	return ""
}
