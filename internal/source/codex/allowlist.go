package codex

import "path/filepath"

// allowedFile is the complete, closed set of ~/.codex paths this adapter
// may ever read. Nothing outside this list is opened — no directory
// listing of ~/.codex, no glob, no walk. auth.json is intentionally never
// in this list: it holds a plaintext OpenAI API key.
func allowedFile(home, name string) string {
	switch name {
	case "config", "chatProcesses", "history":
		return filepath.Join(home, ".codex", allowedRelPaths[name])
	default:
		panic("codex adapter: unknown allowlisted file key " + name)
	}
}

var allowedRelPaths = map[string]string{
	"config":        "config.toml",
	"chatProcesses": filepath.Join("process_manager", "chat_processes.json"),
	"history":       "history.jsonl",
}
