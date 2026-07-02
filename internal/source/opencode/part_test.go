package opencode

import "testing"

func TestSummarizePart(t *testing.T) {
	cases := []struct {
		name, data, want string
	}{
		{"tool with command", `{"type":"tool","tool":"bash","state":{"input":{"command":"go test ./..."}}}`, "🔧 bash: go test ./..."},
		{"tool with query", `{"type":"tool","tool":"websearch","state":{"input":{"query":"opencode vs claude code"}}}`, "🔧 websearch: opencode vs claude code"},
		{"tool with no known key", `{"type":"tool","tool":"custom","state":{"input":{"unknown_field":"x"}}}`, "🔧 custom"},
		{"reasoning text", `{"type":"reasoning","text":"thinking it through"}`, "💭 thinking it through"},
		{"assistant text", `{"type":"text","text":"here is the answer"}`, "💭 here is the answer"},
		{"empty text", `{"type":"text","text":""}`, ""},
		{"step-start ignored", `{"type":"step-start"}`, ""},
		{"step-finish ignored", `{"type":"step-finish","tokens":{"total":100}}`, ""},
		{"invalid json", `not json`, ""},
	}
	for _, c := range cases {
		if got := summarizePart(c.data); got != c.want {
			t.Errorf("%s: summarizePart(%q) = %q, want %q", c.name, c.data, got, c.want)
		}
	}
}
