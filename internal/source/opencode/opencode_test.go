package opencode

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// fakeReader never touches a real ~/.local/share/opencode or
// ~/.cache/opencode directory — every path this package's Detect()/
// models.json reader can request is served from an in-memory map instead.
type fakeReader struct {
	files map[string][]byte
}

func (f *fakeReader) ReadFile(path string) ([]byte, error) {
	if b, ok := f.files[path]; ok {
		return b, nil
	}
	return nil, os.ErrNotExist
}

func (f *fakeReader) Stat(path string) (os.FileInfo, error) {
	if _, ok := f.files[path]; ok {
		return fakeFileInfo{name: filepath.Base(path)}, nil
	}
	return nil, os.ErrNotExist
}

type fakeFileInfo struct{ name string }

func (f fakeFileInfo) Name() string           { return f.name }
func (f fakeFileInfo) Size() int64            { return 0 }
func (f fakeFileInfo) Mode() os.FileMode      { return 0 }
func (f fakeFileInfo) ModTime() (t time.Time) { return }
func (f fakeFileInfo) IsDir() bool            { return false }
func (f fakeFileInfo) Sys() interface{}       { return nil }

func withFakeReader(t *testing.T, f *fakeReader) {
	t.Helper()
	orig := reader
	reader = f
	t.Cleanup(func() { reader = orig })
}

func TestDetect_ChecksDBFileExistence(t *testing.T) {
	home := "/home/test"
	dbPath := filepath.Join(home, ".local", "share", "opencode", "opencode.db")
	withFakeReader(t, &fakeReader{files: map[string][]byte{dbPath: []byte("")}})

	a := &Adapter{home: home}
	if !a.Detect(context.Background()) {
		t.Fatalf("expected Detect=true when opencode.db exists")
	}
}

func TestDetect_FalseWhenDBMissing(t *testing.T) {
	withFakeReader(t, &fakeReader{})
	a := &Adapter{home: "/home/test"}
	if a.Detect(context.Background()) {
		t.Fatalf("expected Detect=false when opencode.db doesn't exist")
	}
}

func TestIsOpencodeProcess(t *testing.T) {
	cases := []struct {
		name, cmdline string
		want          bool
	}{
		{"opencode", "opencode", true},
		{"", "/opt/homebrew/bin/opencode", true},
		{"", "opencode run something", true},
		// A real false-positive trap: an unrelated command whose ARGUMENTS
		// merely mention "opencode" must never match — mirrors Codex's own
		// isCodexProcess test for the same reason.
		{"zsh", "/bin/zsh -c cd ~/www/personal/aitop/internal/source/opencode && go test", false},
		{"vim", "vim internal/source/opencode/opencode.go", false},
		{"", "echo building opencode adapter", false},
	}
	for _, c := range cases {
		if got := isOpencodeProcess(c.name, c.cmdline); got != c.want {
			t.Errorf("isOpencodeProcess(%q, %q) = %v, want %v", c.name, c.cmdline, got, c.want)
		}
	}
}

// openTestDB creates a fresh temp SQLite file with the minimal subset of
// opencode's real schema this adapter reads, so loadLatestSession/
// lastAction can be tested without a live opencode process or its real
// ~/.local/share/opencode/opencode.db.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "opencode.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open temp db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	schema := `
		CREATE TABLE session (
			id text PRIMARY KEY, directory text, title text, model text,
			tokens_input integer, tokens_output integer,
			tokens_cache_read integer, tokens_cache_write integer,
			time_updated integer
		);
		CREATE TABLE message (id text PRIMARY KEY, session_id text, time_created integer);
		CREATE TABLE part (id text PRIMARY KEY, message_id text, time_created integer, data text);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return db
}

func TestLoadLatestSession_PicksMostRecentlyUpdated(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	_, err := db.ExecContext(ctx, `INSERT INTO session
		(id, directory, title, model, tokens_input, tokens_output, tokens_cache_read, tokens_cache_write, time_updated)
		VALUES
		('ses_old', '/old', 'Old session', '{}', 10, 5, 0, 0, 1000),
		('ses_new', '/Users/grippado', 'Benchmark opencode vs claude code', '{"id":"deepseek-v4-flash-free","providerID":"opencode"}', 118298, 4006, 0, 0, 2000)`)
	if err != nil {
		t.Fatalf("insert sessions: %v", err)
	}

	row, ok := loadLatestSession(ctx, db)
	if !ok {
		t.Fatalf("expected a session row")
	}
	if row.ID != "ses_new" {
		t.Fatalf("ID = %q, want the more recently updated %q", row.ID, "ses_new")
	}
	if row.Directory != "/Users/grippado" || row.Title != "Benchmark opencode vs claude code" {
		t.Fatalf("unexpected row: %+v", row)
	}
	if row.TokensIn != 118298 || row.TokensOut != 4006 {
		t.Fatalf("unexpected token fields: %+v", row)
	}
}

func TestLoadLatestSession_EmptyTableReturnsNotOK(t *testing.T) {
	db := openTestDB(t)
	if _, ok := loadLatestSession(context.Background(), db); ok {
		t.Fatalf("expected ok=false for an empty session table")
	}
}

func TestLastAction_SkipsStepBookkeepingAndFindsLatestToolCall(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if _, err := db.ExecContext(ctx, `INSERT INTO message (id, session_id, time_created) VALUES ('msg1', 'ses1', 100)`); err != nil {
		t.Fatalf("insert message: %v", err)
	}
	parts := []struct {
		id   string
		t    int64
		data string
	}{
		{"p1", 1, `{"type":"step-start"}`},
		{"p2", 2, `{"type":"reasoning","text":"thinking about it"}`},
		{"p3", 3, `{"type":"tool","tool":"bash","state":{"input":{"command":"go test ./..."}}}`},
		{"p4", 4, `{"type":"step-finish"}`},
	}
	for _, p := range parts {
		if _, err := db.ExecContext(ctx, `INSERT INTO part (id, message_id, time_created, data) VALUES (?, 'msg1', ?, ?)`, p.id, p.t, p.data); err != nil {
			t.Fatalf("insert part %s: %v", p.id, err)
		}
	}

	a := &Adapter{}
	got := a.lastAction(ctx, db, "ses1")
	want := "🔧 bash: go test ./..."
	if got != want {
		t.Fatalf("lastAction = %q, want %q", got, want)
	}
}

func TestLastAction_NoMessagesReturnsEmpty(t *testing.T) {
	db := openTestDB(t)
	a := &Adapter{}
	if got := a.lastAction(context.Background(), db, "no-such-session"); got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}
