package opencode

import "os"

// Reader is the only interface through which this package touches the
// plain filesystem — swappable in tests so Detect()/the models.json cache
// never touch a real ~/.local/share/opencode or ~/.cache/opencode
// directory. This deliberately does NOT cover opencode.db itself: that's
// read through database/sql + modernc.org/sqlite (see opencode.go's
// open()), which has its own, more faithful way to fake state in tests —
// a real temporary SQLite file (see opencode_test.go's openTestDB) rather
// than an in-memory path->bytes map, since the point being tested is SQL
// query behavior, not byte-level file parsing.
type Reader interface {
	ReadFile(path string) ([]byte, error)
	Stat(path string) (os.FileInfo, error)
}

type osReader struct{}

func (osReader) ReadFile(path string) ([]byte, error)  { return os.ReadFile(path) }
func (osReader) Stat(path string) (os.FileInfo, error) { return os.Stat(path) }

var reader Reader = osReader{}
