package claude

import "os"

// Reader is the only interface through which this package touches the
// filesystem. It's the sole file allowed to call os.ReadFile/os.ReadDir/
// os.Stat directly — tests swap it for a mock so nothing here ever touches
// a real ~/.claude directory.
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
