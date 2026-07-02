package cursoragent

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
)

// cliConfig mirrors the fields this adapter reads from
// ~/.cursor/cli-config.json's own "model" object — confirmed shape on
// this machine's real config: {"model":{"modelId":"claude-sonnet-5",...}}.
// This is cursor-agent's CURRENTLY SELECTED model, a global CLI setting
// rather than something recorded per-session — accurate for the one live
// session this adapter ever surfaces (see Sessions()'s doc comment), but
// would be wrong to treat as authoritative for a past/different session.
type cliConfig struct {
	Model struct {
		ModelID string `json:"modelId"`
	} `json:"model"`
}

// currentModel reads cursor-agent's globally selected model id, re-read
// on every call (not cached) since the user can switch models mid-session
// and this should track that, not go stale.
func currentModel(home string) string {
	raw, err := reader.ReadFile(filepath.Join(home, ".cursor", "cli-config.json"))
	if err != nil {
		return ""
	}
	var cfg cliConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return ""
	}
	return cfg.Model.ModelID
}

// claudeModelPattern matches Anthropic's own API model id shape (cursor-
// agent can run Claude models alongside its own, e.g. Composer) —
// confirmed on this machine's real config: "claude-sonnet-5".
var claudeModelPattern = regexp.MustCompile(`^claude-([a-z]+)-(\d+)(?:-(\d+))?$`)

// friendlyModelName turns a raw model id into the short display label
// cards show in their tool pill: "claude-sonnet-5" -> "sonnet 5" (the same
// transform the Claude Code adapter applies to its own model strings, for
// the same underlying id format), or a dash-to-space fallback for any
// other provider's id, e.g. "composer-2.5" -> "composer 2.5". Never
// returns a raw, unreadable id verbatim without at least this much
// cleanup.
func friendlyModelName(model string) string {
	if model == "" {
		return ""
	}
	if m := claudeModelPattern.FindStringSubmatch(model); m != nil {
		if m[3] == "" {
			return m[1] + " " + m[2]
		}
		return m[1] + " " + m[2] + "." + m[3]
	}
	return strings.ReplaceAll(model, "-", " ")
}
