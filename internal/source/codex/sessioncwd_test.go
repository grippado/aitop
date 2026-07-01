package codex

import (
	"io/fs"
	"os"
	"testing"
	"time"
)

type fakeFileInfo struct {
	name  string
	isDir bool
}

func (f fakeFileInfo) Name() string       { return f.name }
func (f fakeFileInfo) Size() int64        { return 0 }
func (f fakeFileInfo) Mode() fs.FileMode  { return 0 }
func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeFileInfo) IsDir() bool        { return f.isDir }
func (f fakeFileInfo) Sys() interface{}   { return nil }

type fakeDirEntry struct{ info fakeFileInfo }

func (e fakeDirEntry) Name() string              { return e.info.name }
func (e fakeDirEntry) IsDir() bool                { return e.info.isDir }
func (e fakeDirEntry) Type() fs.FileMode          { return 0 }
func (e fakeDirEntry) Info() (fs.FileInfo, error) { return e.info, nil }

type fakeCWDReader struct {
	dirs  map[string][]fakeDirEntry
	files map[string][]byte
}

func (f *fakeCWDReader) ReadFile(path string) ([]byte, error) {
	if b, ok := f.files[path]; ok {
		return b, nil
	}
	return nil, os.ErrNotExist
}
func (f *fakeCWDReader) ReadDir(path string) ([]os.DirEntry, error) {
	entries, ok := f.dirs[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	out := make([]os.DirEntry, len(entries))
	for i, e := range entries {
		out[i] = e
	}
	return out, nil
}
func (f *fakeCWDReader) Stat(path string) (os.FileInfo, error) { return nil, os.ErrNotExist }

func TestFindSessionCWD_ScansYearMonthDayAndReadsOnlyMatch(t *testing.T) {
	orig := reader
	defer func() { reader = orig }()

	sessionID := "019ef56c-6cb8-70e0-917c-5eac94961d6f"
	rolloutName := "rollout-2026-06-23T13-59-44-" + sessionID + ".jsonl"

	fr := &fakeCWDReader{
		dirs: map[string][]fakeDirEntry{
			"/cfg/sessions":          {{fakeFileInfo{name: "2026", isDir: true}}},
			"/cfg/sessions/2026":     {{fakeFileInfo{name: "06", isDir: true}}},
			"/cfg/sessions/2026/06":  {{fakeFileInfo{name: "23", isDir: true}}},
			"/cfg/sessions/2026/06/23": {{fakeFileInfo{name: rolloutName}}},
		},
		files: map[string][]byte{
			"/cfg/sessions/2026/06/23/" + rolloutName: []byte(
				`{"timestamp":"x","type":"session_meta","payload":{"id":"` + sessionID + `","cwd":"/Users/grippado/www/isaac/backoffice"}}` + "\n" +
					`{"type":"huge_line_that_should_never_be_read_if_first_line_parses"}`),
		},
	}
	reader = fr

	cwd := findSessionCWD("/cfg", sessionID)
	if cwd != "/Users/grippado/www/isaac/backoffice" {
		t.Fatalf("cwd = %q, want the session_meta payload.cwd", cwd)
	}
}

func TestFindSessionCWD_NotFoundReturnsEmpty(t *testing.T) {
	orig := reader
	defer func() { reader = orig }()
	reader = &fakeCWDReader{dirs: map[string][]fakeDirEntry{"/cfg/sessions": {}}}

	if cwd := findSessionCWD("/cfg", "missing-id"); cwd != "" {
		t.Fatalf("expected empty string for an unknown session, got %q", cwd)
	}
}

func TestCWDResolver_CachesAfterFirstLookup(t *testing.T) {
	orig := reader
	defer func() { reader = orig }()

	calls := 0
	reader = &countingReader{fakeCWDReader{dirs: map[string][]fakeDirEntry{"/cfg/sessions": {}}}, &calls}

	r := newCWDResolver()
	r.resolve("/cfg", "s1")
	r.resolve("/cfg", "s1")
	if calls != 1 {
		t.Fatalf("expected the sessions/ tree to be scanned exactly once per session ID (cached after), got %d scans", calls)
	}
}

type countingReader struct {
	fakeCWDReader
	calls *int
}

func (c *countingReader) ReadDir(path string) ([]os.DirEntry, error) {
	*c.calls++
	return c.fakeCWDReader.ReadDir(path)
}
