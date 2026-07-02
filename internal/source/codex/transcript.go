package codex

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"sync"
)

// rolloutEnvelope mirrors one line of a rollout-*.jsonl file's top-level
// shape — confirmed on this machine's real Codex session data (type is
// one of "session_meta", "turn_context", "response_item", "event_msg").
type rolloutEnvelope struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// eventMsgPayload covers the two event_msg subtypes this adapter reads:
// "token_count" (real cumulative usage + the AUTHORITATIVE context window
// size — Codex tells us this directly, no guessing needed the way Claude
// required) and "agent_message" (the agent's own commentary text).
type eventMsgPayload struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Info    *struct {
		TotalTokenUsage struct {
			InputTokens  int64 `json:"input_tokens"`
			OutputTokens int64 `json:"output_tokens"`
			TotalTokens  int64 `json:"total_tokens"`
		} `json:"total_token_usage"`
		ModelContextWindow int64 `json:"model_context_window"`
	} `json:"info"`
}

// responseItemPayload covers "function_call" and "message" response_items
// — Codex's equivalent of Claude's tool_use/text content blocks.
type responseItemPayload struct {
	Type      string `json:"type"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON-encoded string, e.g. {"command":["bash","-lc","..."]}
	Role      string `json:"role"`      // "user" | "assistant", for message items
	Content   []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

// codexUsage is the latest real reading found in a session's own rollout
// file.
type codexUsage struct {
	TokensIn       int64
	TokensOut      int64
	ContextUsedPct float64
	HasContext     bool
	LastAction     string
	// Title is synthesized, not native — Codex has no equivalent of
	// Claude Code's auto-generated "ai-title" event. It's the first
	// genuine user message (Codex always sends an <environment_context>
	// boilerplate message first; that one is skipped), first line only,
	// clamped — a real quote from the session, not a fabricated summary,
	// kept for the same visual slot Claude's title occupies.
	Title string
}

// codexTranscriptTracker tail-follows each session's rollout file for its
// most recent token/action reading — the same byte-offset + rotation-safe
// pattern as Claude's transcript tracker and Cursor's log tail.
type codexTranscriptTracker struct {
	mu      sync.Mutex
	paths   map[string]string
	sizes   map[string]int64
	offsets map[string]int64
	latest  map[string]codexUsage
}

func newCodexTranscriptTracker() *codexTranscriptTracker {
	return &codexTranscriptTracker{
		paths:   map[string]string{},
		sizes:   map[string]int64{},
		offsets: map[string]int64{},
		latest:  map[string]codexUsage{},
	}
}

func (t *codexTranscriptTracker) usageFor(configDir, sessionID string) (codexUsage, bool) {
	t.mu.Lock()
	path, cached := t.paths[sessionID]
	t.mu.Unlock()
	if !cached {
		path = findSessionRolloutPath(configDir, sessionID)
		t.mu.Lock()
		t.paths[sessionID] = path
		t.mu.Unlock()
	}
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

func (t *codexTranscriptTracker) get(sessionID string) (codexUsage, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	u, ok := t.latest[sessionID]
	return u, ok
}

func (t *codexTranscriptTracker) ingest(sessionID string, data []byte) {
	t.mu.Lock()
	found := t.latest[sessionID] // preserve whatever was already known
	t.mu.Unlock()
	have := false

	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		var env rolloutEnvelope
		if err := json.Unmarshal(sc.Bytes(), &env); err != nil {
			continue
		}
		switch env.Type {
		case "event_msg":
			var em eventMsgPayload
			if err := json.Unmarshal(env.Payload, &em); err != nil {
				continue
			}
			switch em.Type {
			case "token_count":
				if em.Info != nil && em.Info.ModelContextWindow > 0 {
					found.TokensIn = em.Info.TotalTokenUsage.InputTokens
					found.TokensOut = em.Info.TotalTokenUsage.OutputTokens
					// model_context_window is authoritative (Codex's own
					// figure, not a guess) — the >100% suppression is
					// just a defensive backstop, not the load-bearing
					// honesty mechanism it is for Claude.
					if pct := float64(em.Info.TotalTokenUsage.TotalTokens) / float64(em.Info.ModelContextWindow) * 100; pct <= 100 {
						found.ContextUsedPct = pct
						found.HasContext = true
					}
					have = true
				}
			case "agent_message":
				if strings.TrimSpace(em.Message) != "" {
					found.LastAction = "💭 " + clampCodexText(em.Message, 200)
					have = true
				}
			}
		case "response_item":
			var ri responseItemPayload
			if err := json.Unmarshal(env.Payload, &ri); err != nil {
				continue
			}
			switch {
			case ri.Type == "function_call":
				name := ri.Name
				if name == "" {
					name = "tool"
				}
				found.LastAction = "🔧 " + name + ": " + clampCodexText(summarizeCodexArgs(ri.Arguments), 200)
				have = true
			case ri.Type == "message" && ri.Role == "user" && found.Title == "":
				// First GENUINE user message only. Codex wraps several
				// kinds of injected, non-conversational context in
				// XML-like tags — <environment_context> always comes
				// first, <user_shell_command> shows up when the user ran
				// something in their own shell — confirmed both on this
				// machine's real sessions. Rather than enumerate every
				// wrapper tag by name, skip anything starting with "<":
				// a genuine natural-language request never does. Set
				// once, never overwritten by later turns — same as
				// Claude's ai-title once it settles on a topic.
				for _, c := range ri.Content {
					text := strings.TrimSpace(c.Text)
					if text == "" || strings.HasPrefix(text, "<") {
						continue
					}
					found.Title = synthesizeCodexTitle(text)
					have = true
					break
				}
			}
		}
	}
	if !have {
		return
	}
	t.mu.Lock()
	t.latest[sessionID] = found
	t.mu.Unlock()
}

// summarizeCodexArgs extracts a short human-readable summary from a
// function_call's JSON-encoded arguments string — confirmed shape on this
// machine for shell calls: {"command":["bash","-lc","..."]}. Falls back
// to the raw string if it isn't that shape (a different tool's args).
func summarizeCodexArgs(raw string) string {
	var args struct {
		Command []string `json:"command"`
	}
	if err := json.Unmarshal([]byte(raw), &args); err == nil && len(args.Command) > 0 {
		return strings.Join(args.Command, " ")
	}
	return raw
}

func clampCodexText(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

// synthesizeCodexTitle turns the first genuine user message into a
// title-like string: first line only (a multi-line request's later lines
// are usually detail, not topic), whitespace-collapsed, clamped to a
// length in the same ballpark as Claude Code's own auto-titles. This is a
// real quote, not an LLM-generated summary — honestly shorter/rougher
// than Claude's title, not pretending otherwise.
func synthesizeCodexTitle(text string) string {
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		text = text[:i]
	}
	return clampCodexText(text, 70)
}
