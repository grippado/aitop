package codex

import "os"

// Reader is the only interface through which this package touches the
// filesystem. ReadDir is scoped exclusively to the sessions/ subtree
// (year/month/day cascade in sessioncwd.go, never the ~/.codex root) to
// find a specific rollout-*.jsonl by filename — it is never used to list
// ~/.codex itself, and there is no recursive directory walk or wildcard
// expansion anywhere in this package. auth.json (plaintext OpenAI API key)
// lives directly under ~/.codex, a sibling of sessions/, so it is
// structurally unreachable from this scoped traversal. See
// allowlist_test.go for the CI-enforced guard (still meaningful with
// ReadDir in play).
type Reader interface {
	ReadFile(path string) ([]byte, error)
	ReadDir(path string) ([]os.DirEntry, error)
	Stat(path string) (os.FileInfo, error)
}

type osReader struct{}

func (osReader) ReadFile(path string) ([]byte, error)      { return os.ReadFile(path) }
func (osReader) ReadDir(path string) ([]os.DirEntry, error) { return os.ReadDir(path) }
func (osReader) Stat(path string) (os.FileInfo, error)      { return os.Stat(path) }

var reader Reader = osReader{}
