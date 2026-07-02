package cursoragent

import (
	"path/filepath"
	"strings"
	"sync"
)

// cwdResolver reconstructs a session's real working directory from its
// ~/.cursor/projects/<slug> directory name — cursor-agent's own encoding
// (leading "/" dropped, every remaining "/" replaced with "-"; confirmed
// on this machine: "/Users/grippado/www/personal/guia-cumuru" ->
// "Users-grippado-www-personal-guia-cumuru"). Reversing that split is
// ambiguous on its own — a real folder name can itself contain "-" (e.g.
// "guia-cumuru"), and this machine has both a "guia-cumuru" project and a
// "guia-cumuru-client" one, the latter actually being the "client"
// subfolder INSIDE guia-cumuru, not a sibling — so resolve() backtracks
// over every way to group tokens into path segments and keeps the first
// grouping that matches real directories all the way down. Resolved once
// per session and cached, since a session's cwd never changes.
type cwdResolver struct {
	mu    sync.Mutex
	cache map[string]string
}

func newCWDResolver() *cwdResolver {
	return &cwdResolver{cache: map[string]string{}}
}

// resolve takes a transcript path
// (.../projects/<slug>/agent-transcripts/<id>/<id>.jsonl) and returns the
// real cwd, or "" if no token grouping fully resolves to real directories
// (e.g. the session's folder was since renamed or deleted) — an honest
// "unavailable," never a truncated guess.
func (r *cwdResolver) resolve(transcriptPath string) string {
	slug := filepath.Base(filepath.Dir(filepath.Dir(filepath.Dir(transcriptPath))))

	r.mu.Lock()
	if v, ok := r.cache[slug]; ok {
		r.mu.Unlock()
		return v
	}
	r.mu.Unlock()

	cwd := reconstructCWD(slug)

	r.mu.Lock()
	r.cache[slug] = cwd
	r.mu.Unlock()
	return cwd
}

func reconstructCWD(slug string) string {
	if slug == "" {
		return ""
	}
	path, ok := reconstructFrom("", strings.Split(slug, "-"))
	if !ok {
		return ""
	}
	return path
}

// reconstructFrom tries to consume every remaining token into a chain of
// real directories under parent, backtracking over how many leading tokens
// make up each path segment. A dot-prefixed variant is tried at each step
// too — cursor-agent's slugifier drops the leading "." of a dotdir (e.g.
// "~/.notes" slugs to ".../notes", not ".../.notes"), confirmed on this
// machine's own ~/.notes, ~/.ssh, ~/.claude project entries.
func reconstructFrom(parent string, tokens []string) (string, bool) {
	if len(tokens) == 0 {
		return parent, true
	}
	for end := 1; end <= len(tokens); end++ {
		seg := strings.Join(tokens[:end], "-")
		for _, candidate := range [2]string{parent + "/" + seg, parent + "/." + seg} {
			info, err := reader.Stat(candidate)
			if err != nil || !info.IsDir() {
				continue
			}
			if path, ok := reconstructFrom(candidate, tokens[end:]); ok {
				return path, true
			}
		}
	}
	return "", false
}
