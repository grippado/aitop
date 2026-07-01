package cursor

import (
	"io/fs"
	"os"
	"testing"
	"time"
)

// fakeFileInfo/fakeDirEntry give us just enough of os.FileInfo/os.DirEntry
// to drive latestLogFile()/poll() without touching a real filesystem.
type fakeFileInfo struct {
	name string
	size int64
}

func (f fakeFileInfo) Name() string       { return f.name }
func (f fakeFileInfo) Size() int64        { return f.size }
func (f fakeFileInfo) Mode() fs.FileMode  { return 0 }
func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeFileInfo) IsDir() bool        { return false }
func (f fakeFileInfo) Sys() interface{}   { return nil }

type fakeDirEntry struct{ info fakeFileInfo }

func (e fakeDirEntry) Name() string               { return e.info.name }
func (e fakeDirEntry) IsDir() bool                 { return false }
func (e fakeDirEntry) Type() fs.FileMode           { return 0 }
func (e fakeDirEntry) Info() (fs.FileInfo, error)  { return e.info, nil }

// mockFSReader simulates exactly one active log file whose content and
// name can be swapped mid-test to simulate append, shrink/truncate, and
// rotation to a new filename.
type mockFSReader struct {
	fileName string
	content  []byte
}

func (m *mockFSReader) ReadDir(dir string) ([]os.DirEntry, error) {
	return []os.DirEntry{fakeDirEntry{fakeFileInfo{name: m.fileName, size: int64(len(m.content))}}}, nil
}

func (m *mockFSReader) Stat(path string) (os.FileInfo, error) {
	return fakeFileInfo{name: m.fileName, size: int64(len(m.content))}, nil
}

func (m *mockFSReader) ReadFrom(path string, offset int64) ([]byte, int64, error) {
	size := int64(len(m.content))
	if offset > size {
		offset = 0
	}
	return m.content[offset:], size, nil
}

func withMockReader(t *testing.T, m *mockFSReader) {
	t.Helper()
	orig := reader
	reader = m
	t.Cleanup(func() { reader = orig })
}

const sampleLine = `{"sessionId":"s1","rows":[{"pid":1,"ppid":0,"processName":"Cursor Helper: mcp-process","sampleAvgMemMb":100,"cpuDuringSamplePeakPct":5}]}` + "\n"

func TestPoll_AppendAdvancesOffset(t *testing.T) {
	m := &mockFSReader{fileName: "1000.log", content: []byte(sampleLine)}
	withMockReader(t, m)

	a := New()
	data, err := a.poll()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != sampleLine {
		t.Fatalf("expected first poll to return the full line, got %q", data)
	}
	if a.offset != int64(len(sampleLine)) {
		t.Fatalf("offset = %d, want %d", a.offset, len(sampleLine))
	}

	// Append a second line — poll should return only the NEW bytes.
	m.content = append(m.content, []byte(sampleLine)...)
	data, err = a.poll()
	if err != nil {
		t.Fatalf("unexpected error on append: %v", err)
	}
	if string(data) != sampleLine {
		t.Fatalf("expected only the appended line, got %q", data)
	}
}

// TestPoll_ShrinkResetsOffset is the fixture the design doc explicitly
// requires: if the file shrinks below the stored offset (truncated or
// rewritten in place), poll must reset the offset to 0 and re-read from
// the top instead of seeking into garbage or erroring forever.
func TestPoll_ShrinkResetsOffset(t *testing.T) {
	m := &mockFSReader{fileName: "1000.log", content: []byte(sampleLine + sampleLine)}
	withMockReader(t, m)

	a := New()
	if _, err := a.poll(); err != nil {
		t.Fatalf("unexpected error priming offset: %v", err)
	}
	if a.offset == 0 {
		t.Fatalf("expected non-zero offset after priming")
	}

	// Simulate truncation: same filename, much shorter content.
	m.content = []byte(sampleLine)
	data, err := a.poll()
	if err != nil {
		t.Fatalf("unexpected error after shrink: %v", err)
	}
	if a.offset != int64(len(sampleLine)) {
		t.Fatalf("offset after shrink+reread = %d, want %d (should have reset to 0 then re-read the whole shrunk file)", a.offset, len(sampleLine))
	}
	if string(data) != sampleLine {
		t.Fatalf("expected full shrunk content re-read from byte 0, got %q", data)
	}
}

// TestPoll_RotationToNewFilenameResetsOffset covers rotation where Cursor
// starts an entirely new <epoch-ms>.log rather than truncating the old one.
func TestPoll_RotationToNewFilenameResetsOffset(t *testing.T) {
	m := &mockFSReader{fileName: "1000.log", content: []byte(sampleLine)}
	withMockReader(t, m)

	a := New()
	if _, err := a.poll(); err != nil {
		t.Fatalf("unexpected error priming: %v", err)
	}

	m.fileName = "2000.log" // numerically greater -> becomes "latest"
	m.content = []byte(sampleLine)
	data, err := a.poll()
	if err != nil {
		t.Fatalf("unexpected error after rotation: %v", err)
	}
	if a.curFile == "" || a.offset != int64(len(sampleLine)) {
		t.Fatalf("expected fresh read of the new file, offset=%d", a.offset)
	}
	if string(data) != sampleLine {
		t.Fatalf("expected the new file's content, got %q", data)
	}
}

func TestIngest_ParsesRowsIntoLastRows(t *testing.T) {
	a := New()
	if !a.ingest([]byte(sampleLine)) {
		t.Fatal("expected ingest to report at least one parsed line")
	}
	if len(a.lastRows) != 1 || a.lastRows[1].Label != "Cursor Helper: mcp-process" {
		t.Fatalf("lastRows not populated as expected: %+v", a.lastRows)
	}
}

func TestIngest_UnparseableDataReportsFalse(t *testing.T) {
	a := New()
	if a.ingest([]byte("not json at all\nneither is this\n")) {
		t.Fatal("expected ingest to report no lines parsed for garbage input")
	}
}
