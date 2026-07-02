package cursor

import "os"

// Reader is the only interface through which this package touches the
// filesystem — swappable in tests so the rotation/shrink fixture never
// touches a real Cursor installation.
type Reader interface {
	ReadDir(dir string) ([]os.DirEntry, error)
	Stat(path string) (os.FileInfo, error)
	// ReadFrom reads path from byte offset to EOF, returning the new data
	// and the file's current total size (so callers can detect shrink on
	// the *next* call without a separate Stat race).
	ReadFrom(path string, offset int64) (data []byte, size int64, err error)
}

type osReader struct{}

func (osReader) ReadDir(dir string) ([]os.DirEntry, error) { return os.ReadDir(dir) }
func (osReader) Stat(path string) (os.FileInfo, error)     { return os.Stat(path) }

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
		// Shrink happened between the caller's last known offset and now;
		// let the caller's own size check catch this too, but don't try
		// to seek past EOF here.
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
