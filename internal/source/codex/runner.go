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
	// ReadFrom reads path from byte offset to EOF, returning the new data
	// and the file's current total size — used to tail-follow a session's
	// rollout file for token/action updates without re-reading the whole
	// (potentially large) file every tick.
	ReadFrom(path string, offset int64) (data []byte, size int64, err error)
}

type osReader struct{}

func (osReader) ReadFile(path string) ([]byte, error)       { return os.ReadFile(path) }
func (osReader) ReadDir(path string) ([]os.DirEntry, error) { return os.ReadDir(path) }
func (osReader) Stat(path string) (os.FileInfo, error)      { return os.Stat(path) }

func (osReader) ReadFrom(path string, offset int64) ([]byte, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, 0, err
	}
	size := info.Size()
	if offset > size {
		offset = 0
	}
	if _, err := f.Seek(offset, 0); err != nil {
		return nil, size, err
	}
	buf := make([]byte, size-offset)
	n, err := f.Read(buf)
	if err != nil && n == 0 {
		return nil, size, err
	}
	return buf[:n], size, nil
}

var reader Reader = osReader{}
