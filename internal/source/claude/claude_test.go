package claude

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

// fakeReader never touches a real ~/.claude directory — every path this
// adapter can request is served from an in-memory map instead.
type fakeReader struct {
	files map[string][]byte
	dirs  map[string][]string // dir path -> file names present
}

func (f *fakeReader) ReadFile(path string) ([]byte, error) {
	if b, ok := f.files[path]; ok {
		return b, nil
	}
	return nil, os.ErrNotExist
}

func (f *fakeReader) ReadDir(path string) ([]os.DirEntry, error) {
	names, ok := f.dirs[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	var out []os.DirEntry
	for _, n := range names {
		out = append(out, fakeDirEntry{fakeFileInfo{name: n}})
	}
	return out, nil
}

func (f *fakeReader) Stat(path string) (os.FileInfo, error) {
	if _, ok := f.dirs[path]; ok {
		return fakeFileInfo{name: path, isDir: true}, nil
	}
	if _, ok := f.files[path]; ok {
		return fakeFileInfo{name: path}, nil
	}
	return nil, os.ErrNotExist
}

func withFakeReader(t *testing.T, f *fakeReader) {
	t.Helper()
	orig := reader
	reader = f
	t.Cleanup(func() { reader = orig })
}

func TestSessions_ParsesSessionFiles(t *testing.T) {
	home := "/home/test"
	f := &fakeReader{
		dirs: map[string][]string{
			home + "/.claude/sessions": {"111.json", "not-json.txt"},
		},
		files: map[string][]byte{
			home + "/.claude/sessions/111.json": []byte(`{"pid":111,"sessionId":"s1","cwd":"/x","status":"busy","updatedAt":1000}`),
		},
	}
	withFakeReader(t, f)

	a := &Adapter{home: home}
	sessions, err := a.Sessions(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session (non-.json file skipped), got %d", len(sessions))
	}
	s := sessions[0]
	if s.ID != "s1" || s.CWD != "/x" || s.Status != "busy" {
		t.Fatalf("unexpected session parsed: %+v", s)
	}
}

func TestUsage_SumsCostFileAndSkipsMissing(t *testing.T) {
	home := "/home/test"
	now := time.Now()
	dayPath := costDayPath(home, now)

	f := &fakeReader{
		files: map[string][]byte{
			dayPath: []byte(`{"uuid-a":{"base":1.0,"current":2.5},"uuid-b":{"base":0,"current":0.75}}`),
		},
	}
	withFakeReader(t, f)

	a := &Adapter{home: home}
	u, err := a.Usage(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := (2.5 - 1.0) + (0.75 - 0)
	if u.CostTodayUSD != want {
		t.Fatalf("CostTodayUSD = %v, want %v", u.CostTodayUSD, want)
	}
	if u.CostMonthUSD != 0 {
		t.Fatalf("expected 0 for missing month file (real zero, not fabricated), got %v", u.CostMonthUSD)
	}
}

func TestUsage_DiscardsExpiredRateLimitWindow(t *testing.T) {
	home := "/home/test"
	past := time.Now().Add(-time.Hour).Unix()

	f := &fakeReader{
		files: map[string][]byte{
			home + "/.claude/.statusline-cache.json": []byte(`{"rl5_pct":90,"rl5_reset":` + itoa(past) + `,"rl7_pct":50,"rl7_reset":` + itoa(past) + `}`),
		},
	}
	withFakeReader(t, f)

	a := &Adapter{home: home}
	u, err := a.Usage(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u.LimitFiveHour != nil || u.LimitWeekly != nil {
		t.Fatalf("expected nil limits for an expired reset window, got 5h=%v 7d=%v", u.LimitFiveHour, u.LimitWeekly)
	}
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
