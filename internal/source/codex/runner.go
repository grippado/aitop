package codex

import "os"

// Reader is the only interface through which this package touches the
// filesystem. Every call site passes one of the exact paths named in
// allowedFile — there is no recursive directory traversal or wildcard
// expansion over ~/.codex anywhere in this package, and there must never
// be. auth.json (plaintext OpenAI API key) is deliberately absent from
// that list and must never be opened here. See allowlist_test.go for the
// CI-enforced guard.
type Reader interface {
	ReadFile(path string) ([]byte, error)
	Stat(path string) (os.FileInfo, error)
}

type osReader struct{}

func (osReader) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }
func (osReader) Stat(path string) (os.FileInfo, error) { return os.Stat(path) }

var reader Reader = osReader{}
