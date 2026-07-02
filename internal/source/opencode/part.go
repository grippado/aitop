package opencode

import (
	"encoding/json"
	"strings"
)

// partData mirrors one row's `data` JSON in the `part` table — confirmed
// shapes on this machine's real session data: {"type":"tool","tool":
// "bash","state":{"input":{"command":"..."}}}, {"type":"text","text":
// "..."}, {"type":"reasoning","text":"..."}, plus "step-start"/
// "step-finish" bookkeeping entries this adapter has nothing to show for.
type partData struct {
	Type  string `json:"type"`
	Text  string `json:"text"`
	Tool  string `json:"tool"`
	State struct {
		Input json.RawMessage `json:"input"`
	} `json:"state"`
}

// detailKeys is the priority list of tool input keys this adapter knows
// how to summarize, drawn from every tool part observed on this machine's
// real opencode sessions (bash, websearch, read/write-style tools).
var detailKeys = []string{"command", "query", "path", "pattern", "url", "description"}

// summarizePart condenses one part into a short "🔧 name: detail" /
// "💭 text" description, mirroring the Claude/Codex/cursor-agent adapters'
// convention — "" for part types with nothing worth summarizing
// (step-start, step-finish, empty text).
func summarizePart(raw string) string {
	var p partData
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return ""
	}
	switch p.Type {
	case "tool":
		name := p.Tool
		if name == "" {
			name = "tool"
		}
		detail := extractDetail(p.State.Input)
		if detail == "" {
			return "🔧 " + name
		}
		return "🔧 " + name + ": " + clampText(detail, 200)
	case "text", "reasoning":
		txt := strings.TrimSpace(p.Text)
		if txt == "" {
			return ""
		}
		return "💭 " + clampText(txt, 200)
	default:
		return ""
	}
}

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

func clampText(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
