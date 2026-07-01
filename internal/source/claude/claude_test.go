package claude

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type fakeFileInfo struct {
	name  string
	isDir bool
	size  int64
}

func (f fakeFileInfo) Name() string       { return f.name }
func (f fakeFileInfo) Size() int64        { return f.size }
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
		_, isDir := f.dirs[filepath.Join(path, n)]
		out = append(out, fakeDirEntry{fakeFileInfo{name: n, isDir: isDir}})
	}
	return out, nil
}

func (f *fakeReader) Stat(path string) (os.FileInfo, error) {
	if _, ok := f.dirs[path]; ok {
		return fakeFileInfo{name: path, isDir: true}, nil
	}
	if b, ok := f.files[path]; ok {
		return fakeFileInfo{name: path, size: int64(len(b))}, nil
	}
	return nil, os.ErrNotExist
}

func (f *fakeReader) ReadFrom(path string, offset int64) ([]byte, int64, error) {
	b, ok := f.files[path]
	if !ok {
		return nil, 0, os.ErrNotExist
	}
	size := int64(len(b))
	if offset > size {
		offset = 0
	}
	return b[offset:], size, nil
}

func withFakeReader(t *testing.T, f *fakeReader) {
	t.Helper()
	orig := reader
	reader = f
	t.Cleanup(func() { reader = orig })
}

func TestSessions_ParsesSessionFiles(t *testing.T) {
	configDir := "/home/test/.claude"
	f := &fakeReader{
		dirs: map[string][]string{
			configDir + "/sessions": {"111.json", "not-json.txt"},
		},
		files: map[string][]byte{
			configDir + "/sessions/111.json": []byte(`{"pid":111,"sessionId":"s1","cwd":"/x","status":"busy","updatedAt":1000}`),
		},
	}
	withFakeReader(t, f)

	a := &Adapter{configDir: configDir}
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
	configDir := "/home/test/.claude"
	now := time.Now()
	dayPath := costDayPath(configDir, now)

	f := &fakeReader{
		files: map[string][]byte{
			dayPath: []byte(`{"uuid-a":{"base":1.0,"current":2.5},"uuid-b":{"base":0,"current":0.75}}`),
		},
	}
	withFakeReader(t, f)

	a := &Adapter{configDir: configDir}
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
	if !u.Available {
		t.Fatalf("expected Available=true: at least the cost-day file was real data")
	}
}

func TestUsage_DiscardsExpiredRateLimitWindow(t *testing.T) {
	setFakeCacheDir(t, "/home/test/.cache")
	past := time.Now().Add(-time.Hour).Format(time.RFC3339Nano)

	f := &fakeReader{
		files: map[string][]byte{
			"/home/test/.cache/ccstatusline/usage.json": []byte(
				`{"sessionUsage":90,"sessionResetAt":"` + past + `","weeklyUsage":50,"weeklyResetAt":"` + past + `"}`),
		},
	}
	withFakeReader(t, f)

	a := &Adapter{configDir: "/home/test/.claude"}
	u, err := a.Usage(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u.LimitFiveHour != nil || u.LimitWeekly != nil {
		t.Fatalf("expected nil limits for an expired reset window, got 5h=%v 7d=%v", u.LimitFiveHour, u.LimitWeekly)
	}
	if u.Available {
		t.Fatalf("expected Available=false: no cost file and only an expired rate-limit reading, nothing real")
	}
}

func TestUsage_LiveCcstatuslineCacheIsUsed(t *testing.T) {
	setFakeCacheDir(t, "/home/test/.cache")
	future := time.Now().Add(2 * time.Hour).Format(time.RFC3339Nano)

	f := &fakeReader{
		files: map[string][]byte{
			"/home/test/.cache/ccstatusline/usage.json": []byte(
				`{"sessionUsage":0,"sessionResetAt":"` + future + `","weeklyUsage":22,"weeklyResetAt":"` + future + `"}`),
		},
	}
	withFakeReader(t, f)

	a := &Adapter{configDir: "/home/test/.claude"}
	u, err := a.Usage(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u.LimitFiveHour == nil || *u.LimitFiveHour != 0 {
		t.Fatalf("expected LimitFiveHour=0 (a real reading, not absent), got %v", u.LimitFiveHour)
	}
	if u.LimitWeekly == nil || *u.LimitWeekly != 22 {
		t.Fatalf("expected LimitWeekly=22, got %v", u.LimitWeekly)
	}
	if !u.Available {
		t.Fatalf("expected Available=true: live rate-limit reading counts as real data")
	}
}

func TestUsage_NothingFoundIsUnavailable(t *testing.T) {
	setFakeCacheDir(t, "/home/test/.cache")
	withFakeReader(t, &fakeReader{})

	a := &Adapter{configDir: "/home/test/.claude"}
	u, err := a.Usage(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u.Available {
		t.Fatalf("expected Available=false when no cost file and no rate-limit cache exist at all")
	}
	if u.CostTodayUSD != 0 || u.LimitFiveHour != nil || u.LimitWeekly != nil {
		t.Fatalf("expected all fields at zero-value/nil, got %+v", u)
	}
}

// setFakeCacheDir points ccstatuslineCacheDir() at a fake location for the
// duration of the test via XDG_CACHE_HOME, so no real ~/.cache is touched.
func setFakeCacheDir(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("XDG_CACHE_HOME", dir)
}
