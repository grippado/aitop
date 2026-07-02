package cursor

import (
	"regexp"
	"strings"
)

// claudeModelPattern matches Anthropic's own API model id shape — Cursor
// IDE can run Claude models alongside its own (Composer), same as
// cursor-agent — confirmed on real data: "claude-sonnet-5".
var claudeModelPattern = regexp.MustCompile(`^claude-([a-z]+)-(\d+)(?:-(\d+))?$`)

// friendlyModelName turns a raw model id into the short display label
// cards show in their tool pill: "claude-sonnet-5" -> "sonnet 5" (the
// same transform the Claude Code and cursor-agent adapters apply to their
// own model strings, for the same underlying id format), or a
// dash-to-space fallback for any other model id, e.g. "composer-2.5" ->
// "composer 2.5" — confirmed on this machine's real composer bubble data.
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
