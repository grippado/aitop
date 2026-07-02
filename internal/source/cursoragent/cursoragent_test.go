package cursoragent

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

type fakeFileInfo struct {
	name    string
	isDir   bool
	size    int64
	modTime time.Time
}

func (f fakeFileInfo) Name() string       { return f.name }
func (f fakeFileInfo) Size() int64        { return f.size }
func (f fakeFileInfo) Mode() fs.FileMode  { return 0 }
func (f fakeFileInfo) ModTime() time.Time { return f.modTime }
func (f fakeFileInfo) IsDir() bool        { return f.isDir }
func (f fakeFileInfo) Sys() interface{}   { return nil }

type fakeDirEntry struct{ info fakeFileInfo }

func (e fakeDirEntry) Name() string               { return e.info.name }
func (e fakeDirEntry) IsDir() bool                { return e.info.isDir }
func (e fakeDirEntry) Type() fs.FileMode          { return 0 }
func (e fakeDirEntry) Info() (fs.FileInfo, error) { return e.info, nil }

// fakeReader never touches a real ~/.cursor directory — every path this
// adapter can request is served from an in-memory map instead. dirs also
// implicitly marks a path as an existing directory for Stat, so
// reconstructCWD's filesystem checks work without a separate flag.
type fakeReader struct {
	dirs  map[string][]string // dir path -> entry names present
	files map[string][]byte
	mtime map[string]time.Time // optional per-file mtime override
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
		return fakeFileInfo{name: filepath.Base(path), isDir: true}, nil
	}
	if b, ok := f.files[path]; ok {
		return fakeFileInfo{name: filepath.Base(path), size: int64(len(b)), modTime: f.mtime[path]}, nil
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

func TestIsCursorAgentProcess(t *testing.T) {
	cases := []struct {
		cmdline string
		want    bool
	}{
		{"/Users/grippado/.local/bin/cursor-agent --use-system-ca /x/index.js", true},
		{"cursor-agent", true},
		{"node /Users/grippado/.local/share/cursor-agent/versions/x/index.js worker-server", false},
		{"/usr/bin/cursor-agent-svc", false}, // a different binary, not an exact "cursor-agent" match
		{"vim cursor-agent.go", false},
	}
	for _, c := range cases {
		if got := isCursorAgentProcess(c.cmdline); got != c.want {
			t.Errorf("isCursorAgentProcess(%q) = %v, want %v", c.cmdline, got, c.want)
		}
	}
}

func TestReconstructCWD_BacktracksOverDashedFolderNames(t *testing.T) {
	f := &fakeReader{
		dirs: map[string][]string{
			"/Users":                                          {"grippado"},
			"/Users/grippado":                                 {"www"},
			"/Users/grippado/www":                             {"personal"},
			"/Users/grippado/www/personal":                    {"guia-cumuru"},
			"/Users/grippado/www/personal/guia-cumuru":        {"client"},
			"/Users/grippado/www/personal/guia-cumuru/client": {},
		},
	}
	withFakeReader(t, f)

	got := reconstructCWD("Users-grippado-www-personal-guia-cumuru")
	want := "/Users/grippado/www/personal/guia-cumuru"
	if got != want {
		t.Fatalf("reconstructCWD(guia-cumuru) = %q, want %q", got, want)
	}

	got = reconstructCWD("Users-grippado-www-personal-guia-cumuru-client")
	want = "/Users/grippado/www/personal/guia-cumuru/client"
	if got != want {
		t.Fatalf("reconstructCWD(guia-cumuru-client) = %q, want %q (the real client/ subfolder, not a fabricated sibling)", got, want)
	}
}

func TestReconstructCWD_TriesDotPrefixedVariant(t *testing.T) {
	f := &fakeReader{
		dirs: map[string][]string{
			"/Users":                 {"grippado"},
			"/Users/grippado":        {".notes"},
			"/Users/grippado/.notes": {},
		},
	}
	withFakeReader(t, f)

	got := reconstructCWD("Users-grippado-notes")
	want := "/Users/grippado/.notes"
	if got != want {
		t.Fatalf("reconstructCWD(notes) = %q, want %q", got, want)
	}
}

func TestReconstructCWD_NoFullMatchReturnsEmpty(t *testing.T) {
	f := &fakeReader{
		dirs: map[string][]string{
			"/Users": {"grippado"},
			// "/Users/grippado" deliberately absent: the slug's session
			// folder no longer exists on disk.
		},
	}
	withFakeReader(t, f)

	if got := reconstructCWD("Users-grippado-www"); got != "" {
		t.Fatalf("expected empty string when no token grouping fully resolves, got %q", got)
	}
}

func TestFindLatestTranscript_PicksMostRecentAcrossProjects(t *testing.T) {
	home := "/home/test"
	old := filepath.Join(home, ".cursor", "projects", "proj-a", "agent-transcripts", "id-a", "id-a.jsonl")
	newer := filepath.Join(home, ".cursor", "projects", "proj-b", "agent-transcripts", "id-b", "id-b.jsonl")

	f := &fakeReader{
		dirs: map[string][]string{
			filepath.Join(home, ".cursor", "projects"):                                        {"proj-a", "proj-b"},
			filepath.Join(home, ".cursor", "projects", "proj-a"):                              {"agent-transcripts"},
			filepath.Join(home, ".cursor", "projects", "proj-b"):                              {"agent-transcripts"},
			filepath.Join(home, ".cursor", "projects", "proj-a", "agent-transcripts"):         {"id-a"},
			filepath.Join(home, ".cursor", "projects", "proj-b", "agent-transcripts"):         {"id-b"},
			filepath.Join(home, ".cursor", "projects", "proj-a", "agent-transcripts", "id-a"): {"id-a.jsonl"},
			filepath.Join(home, ".cursor", "projects", "proj-b", "agent-transcripts", "id-b"): {"id-b.jsonl"},
		},
		files: map[string][]byte{
			old:   []byte(`{}`),
			newer: []byte(`{}`),
		},
		mtime: map[string]time.Time{
			old:   time.Unix(1000, 0),
			newer: time.Unix(2000, 0),
		},
	}
	withFakeReader(t, f)

	path, sessionID := findLatestTranscript(home)
	if path != newer {
		t.Fatalf("findLatestTranscript path = %q, want the more recently modified %q", path, newer)
	}
	if sessionID != "id-b" {
		t.Fatalf("findLatestTranscript sessionID = %q, want %q", sessionID, "id-b")
	}
}

func TestTranscriptTracker_ExtractsTitleAndLastAction(t *testing.T) {
	sessionID := "s1"
	path := "/home/test/.cursor/projects/proj/agent-transcripts/s1/s1.jsonl"

	data := `{"role":"user","message":{"content":[{"type":"text","text":"<timestamp>x</timestamp>\n<user_query>\nqual a diferença de capabilitie do sonnet e fo opus 4.8?\n</user_query>"}]}}
{"role":"assistant","message":{"content":[{"type":"tool_use","name":"WebSearch","input":{"search_term":"Claude Opus 4.8 vs Sonnet"}}]}}
`
	f := &fakeReader{files: map[string][]byte{path: []byte(data)}}
	withFakeReader(t, f)

	tr := newTranscriptTracker()
	usage, ok := tr.usageFor(sessionID, path)
	if !ok {
		t.Fatalf("expected a usage reading, got none")
	}
	wantTitle := "qual a diferença de capabilitie do sonnet e fo opus 4.8?"
	if usage.Title != wantTitle {
		t.Fatalf("Title = %q, want %q", usage.Title, wantTitle)
	}
	wantAction := "🔧 WebSearch: Claude Opus 4.8 vs Sonnet"
	if usage.LastAction != wantAction {
		t.Fatalf("LastAction = %q, want %q", usage.LastAction, wantAction)
	}
}

func TestTranscriptTracker_SkipsUserQueryTextAsLastAction(t *testing.T) {
	sessionID := "s1"
	path := "/home/test/.cursor/projects/proj/agent-transcripts/s1/s1.jsonl"

	data := `{"role":"user","message":{"content":[{"type":"text","text":"<user_query>\nquestion\n</user_query>"}]}}
`
	f := &fakeReader{files: map[string][]byte{path: []byte(data)}}
	withFakeReader(t, f)

	tr := newTranscriptTracker()
	usage, ok := tr.usageFor(sessionID, path)
	if !ok {
		t.Fatalf("expected a usage reading (title, at least), got none")
	}
	if usage.LastAction != "" {
		t.Fatalf("LastAction should stay empty for a raw user_query text block, got %q", usage.LastAction)
	}
}

func TestSlugifyCWD(t *testing.T) {
	cases := []struct{ cwd, want string }{
		{"/Users/grippado", "Users-grippado"},
		{"/Users/grippado/www/isaac", "Users-grippado-www-isaac"},
		{"/Users/grippado/.notes", "Users-grippado-.notes"},
	}
	for _, c := range cases {
		if got := slugifyCWD(c.cwd); got != c.want {
			t.Errorf("slugifyCWD(%q) = %q, want %q", c.cwd, got, c.want)
		}
	}
}

func TestFindLatestTranscriptInWorkspace_OnlySearchesThatWorkspace(t *testing.T) {
	home := "/home/test"
	// A different, unrelated workspace with a MORE recently written
	// transcript — this must be ignored: findLatestTranscriptInWorkspace
	// is exactly the fix for the real bug where a global "most recent
	// anywhere" search attributed a different project's activity (or
	// Cursor IDE's own Agent panel) to this card.
	unrelated := filepath.Join(home, ".cursor", "projects", "Users-x-other-project", "agent-transcripts", "id-other", "id-other.jsonl")
	wanted := filepath.Join(home, ".cursor", "projects", "Users-x-www-isaac", "agent-transcripts", "id-mine", "id-mine.jsonl")

	f := &fakeReader{
		dirs: map[string][]string{
			filepath.Join(home, ".cursor", "projects", "Users-x-other-project", "agent-transcripts"):             {"id-other"},
			filepath.Join(home, ".cursor", "projects", "Users-x-other-project", "agent-transcripts", "id-other"): {"id-other.jsonl"},
			filepath.Join(home, ".cursor", "projects", "Users-x-www-isaac", "agent-transcripts"):                 {"id-mine"},
			filepath.Join(home, ".cursor", "projects", "Users-x-www-isaac", "agent-transcripts", "id-mine"):      {"id-mine.jsonl"},
		},
		files: map[string][]byte{
			unrelated: []byte(`{}`),
			wanted:    []byte(`{}`),
		},
		mtime: map[string]time.Time{
			unrelated: time.Unix(9999, 0), // much newer, but wrong workspace
			wanted:    time.Unix(1000, 0),
		},
	}
	withFakeReader(t, f)

	path, sessionID := findLatestTranscriptInWorkspace(home, "/Users/x/www/isaac")
	if path != wanted {
		t.Fatalf("path = %q, want the scoped workspace's own transcript %q (not the unrelated, newer one)", path, wanted)
	}
	if sessionID != "id-mine" {
		t.Fatalf("sessionID = %q, want %q", sessionID, "id-mine")
	}
}

func TestFindLatestTranscriptInWorkspace_NoWorkspaceDirReturnsEmpty(t *testing.T) {
	withFakeReader(t, &fakeReader{})
	path, sessionID := findLatestTranscriptInWorkspace("/home/test", "/Users/x/never-opened")
	if path != "" || sessionID != "" {
		t.Fatalf("expected empty results for a workspace with no agent-transcripts dir, got path=%q sessionID=%q", path, sessionID)
	}
}

func TestProcessCwd_MatchesRealProcessCwd(t *testing.T) {
	lsofPath, err := exec.LookPath("lsof")
	if err != nil {
		t.Skip("lsof not on PATH — skipping (processCwd degrades gracefully in production too)")
	}
	_ = lsofPath

	wantCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}

	gotCwd, ok := processCwd(context.Background(), os.Getpid())
	if !ok {
		t.Fatalf("expected processCwd to resolve this test process's own cwd via real lsof")
	}
	if gotCwd != wantCwd {
		t.Fatalf("processCwd = %q, want %q", gotCwd, wantCwd)
	}
}
