package codex

import "testing"

func TestIsCodexProcess(t *testing.T) {
	cases := []struct {
		name, cmdline string
		want          bool
	}{
		{"codex", "codex", true},
		{"codex", "codex --resume", true},
		{"", "/opt/homebrew/bin/codex --resume", true},
		{"", "codex", true},
		// The real false positive observed in practice: an unrelated shell
		// command whose ARGUMENTS merely mention "codex" (e.g. a path to
		// this very adapter's package) must never match.
		{"zsh", "/bin/zsh -c cd ~/www/personal/aitop/internal/source/codex && go test", false},
		{"vim", "vim internal/source/codex/codex.go", false},
		{"", "echo building codex adapter", false},
		{"bash", "bash -c grep codex file.txt", false},
	}
	for _, c := range cases {
		if got := isCodexProcess(c.name, c.cmdline); got != c.want {
			t.Errorf("isCodexProcess(%q, %q) = %v, want %v", c.name, c.cmdline, got, c.want)
		}
	}
}
