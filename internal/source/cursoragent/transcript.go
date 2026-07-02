package cursoragent

import (
	"bufio"
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
	"sync"
)

// contentBlock mirrors one entry of message.content[] — confirmed shape on
// this machine's real transcripts: {"type":"tool_use","name":"Shell",
// "input":{"command":"...","description":"..."}} or {"type":"text",
// "text":"..."}. cursor-agent's own tool set (Read/Write/Grep/Glob/Shell/
// WebSearch/WebFetch/StrReplace/Task, confirmed on real session data) uses
// different field names per tool, so Input stays raw and detail extraction
// tries a priority list of the keys actually observed rather than one
// fixed struct shape.
type contentBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

type transcriptLine struct {
	Role    string `json:"role"` // "user" | "assistant"
	Message struct {
		Content []contentBlock `json:"content"`
	} `json:"message"`
}

// userQueryPattern extracts the real query text cursor-agent wraps every
// user message in — confirmed shape: "<timestamp>...</timestamp>\n
// <user_query>\n{text}\n</user_query>" (the leading <timestamp> tag is
// only present sometimes, e.g. after a gap in activity; <user_query> is
// always there).
var userQueryPattern = regexp.MustCompile(`(?s)<user_query>\s*(.*?)\s*</user_query>`)

// detailKeys is the priority list of input keys this adapter knows how to
// summarize, drawn from every tool_use.input shape observed on this
// machine's real cursor-agent transcripts (Shell, Read, Write, StrReplace,
// Grep, Glob, WebSearch, WebFetch, Task).
var detailKeys = []string{"command", "path", "search_term", "url", "pattern", "glob_pattern", "target_directory", "description"}

// transcriptUsage is the latest reading found in a session's own
// transcript. No token/cost data exists in this format (unlike Claude
// Code's transcript, which carries a real "usage" block per turn) — see
// usage.go's Usage() for the honest gap this leaves.
type transcriptUsage struct {
	// Title is the first genuine user message, unwrapped from its
	// <user_query> tags — a real quote from the session, not a fabricated
	// summary, the same approach Codex's adapter uses for its own
	// title-less transcript format.
	Title string
	// LastAction is a short summary of the most recent tool call or
	// thinking snippet — same "🔧 name: detail" / "💭 text" convention as
	// the Claude/Codex adapters.
	LastAction string
}

// transcriptTracker tail-follows a session's transcript file for its most
// recent title/last-action reading, mirroring the byte-offset +
// rotation-safe pattern used by every other adapter's transcript reader.
type transcriptTracker struct {
	mu      sync.Mutex
	sizes   map[string]int64
	offsets map[string]int64
	latest  map[string]transcriptUsage
}

func newTranscriptTracker() *transcriptTracker {
	return &transcriptTracker{
		sizes:   map[string]int64{},
		offsets: map[string]int64{},
		latest:  map[string]transcriptUsage{},
	}
}

func (t *transcriptTracker) usageFor(sessionID, path string) (transcriptUsage, bool) {
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

		if l.Role == "user" && found.Title == "" {
			if title := firstUserQueryTitle(l.Message.Content); title != "" {
				found.Title = title
				have = true
			}
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

func firstUserQueryTitle(blocks []contentBlock) string {
	for _, b := range blocks {
		if b.Type != "text" {
			continue
		}
		m := userQueryPattern.FindStringSubmatch(b.Text)
		if m == nil {
			continue
		}
		return synthesizeTitle(m[1])
	}
	return ""
}

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
		detail := extractDetail(b.Input)
		if detail == "" {
			return "🔧 " + name
		}
		return "🔧 " + name + ": " + clampText(detail, 200)
	case "text":
		txt := strings.TrimSpace(b.Text)
		if txt == "" || userQueryPattern.MatchString(txt) {
			// A raw <user_query>-wrapped block is the human's own message,
			// re-surfaced on the same transcript line shape as assistant
			// text — not the agent's own commentary, so not a "last
			// action" worth showing.
			return ""
		}
		return "💭 " + clampText(txt, 200)
	default:
		return ""
	}
}

// extractDetail pulls a short human-readable summary out of a tool_use's
// input, trying detailKeys in priority order — genuinely per-tool-shape
// since cursor-agent's tools each use their own argument names (Shell:
// "command", Read/Write: "path", WebSearch: "search_term", ...).
func extractDetail(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	for _, key := range detailKeys {
		v, ok := m[key]
		if !ok {
			continue
		}
		var s string
		if err := json.Unmarshal(v, &s); err == nil && s != "" {
			return s
		}
	}
	return ""
}

// synthesizeTitle turns the first genuine user message into a title-like
// string: first line only, whitespace-collapsed, clamped — mirrors
// Codex's synthesizeCodexTitle for the same reason (no native auto-title
// equivalent found in this transcript format, unlike Claude Code's
// "ai-title" event).
func synthesizeTitle(text string) string {
	text = strings.TrimSpace(text)
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		text = text[:i]
	}
	return clampText(text, 70)
}

func clampText(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
