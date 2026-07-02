package cursor

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// openTestComposerDB creates a fresh temp SQLite file with the minimal
// subset of Cursor's real state.vscdb schema this package reads, and
// returns a composerStore pointed at it directly (bypassing
// composerDBPath, which only knows the real ~/Library/... location) —
// mirrors the opencode adapter's own openTestDB convention: SQL query
// behavior is tested against a real temporary SQLite file, not a faked
// byte-level reader.
func openTestComposerDB(t *testing.T) *composerStore {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.vscdb")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open temp db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	schema := `
		CREATE TABLE ItemTable (key TEXT UNIQUE ON CONFLICT REPLACE, value BLOB);
		CREATE TABLE cursorDiskKV (key TEXT UNIQUE ON CONFLICT REPLACE, value BLOB);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return &composerStore{db: db}
}

func setComposerHeaders(t *testing.T, s *composerStore, json string) {
	t.Helper()
	if _, err := s.db.Exec(`INSERT INTO ItemTable (key, value) VALUES ('composer.composerHeaders', ?)`, json); err != nil {
		t.Fatalf("insert composerHeaders: %v", err)
	}
}

func TestBestComposerForWorkspace_MatchesByFolderBasename(t *testing.T) {
	s := openTestComposerDB(t)
	setComposerHeaders(t, s, `{"allComposers":[
		{"composerId":"c1","name":"Old backoffice work","lastUpdatedAt":1000,"workspaceIdentifier":{"uri":{"fsPath":"/Users/x/www/isaac/backoffice"}}},
		{"composerId":"c2","name":"Newer backoffice work","lastUpdatedAt":2000,"workspaceIdentifier":{"uri":{"fsPath":"/Users/x/www/isaac/backoffice"}}},
		{"composerId":"c3","name":"Different project","lastUpdatedAt":3000,"workspaceIdentifier":{"uri":{"fsPath":"/Users/x/www/personal/aitop"}}}
	]}`)

	c, ok := s.bestComposerForWorkspace("backoffice")
	if !ok {
		t.Fatalf("expected a match")
	}
	if c.ComposerID != "c2" {
		t.Fatalf("ComposerID = %q, want %q (the more recently updated of the two backoffice composers)", c.ComposerID, "c2")
	}
}

func TestBestComposerForWorkspace_NoLabelPicksGlobalMostRecent(t *testing.T) {
	s := openTestComposerDB(t)
	setComposerHeaders(t, s, `{"allComposers":[
		{"composerId":"c1","name":"A","lastUpdatedAt":1000,"workspaceIdentifier":{"uri":{"fsPath":"/Users/x/a"}}},
		{"composerId":"c2","name":"B","lastUpdatedAt":5000,"workspaceIdentifier":{"uri":{"fsPath":"/Users/x/b"}}}
	]}`)

	c, ok := s.bestComposerForWorkspace("")
	if !ok || c.ComposerID != "c2" {
		t.Fatalf("bestComposerForWorkspace(\"\") = (%+v, %v), want c2", c, ok)
	}
}

func TestBestComposerForWorkspace_SkipsArchivedAndDraft(t *testing.T) {
	s := openTestComposerDB(t)
	setComposerHeaders(t, s, `{"allComposers":[
		{"composerId":"c1","name":"Archived","lastUpdatedAt":9000,"isArchived":true,"workspaceIdentifier":{"uri":{"fsPath":"/Users/x/proj"}}},
		{"composerId":"c2","name":"Draft","lastUpdatedAt":8000,"isDraft":true,"workspaceIdentifier":{"uri":{"fsPath":"/Users/x/proj"}}},
		{"composerId":"c3","name":"Real one","lastUpdatedAt":1000,"workspaceIdentifier":{"uri":{"fsPath":"/Users/x/proj"}}}
	]}`)

	c, ok := s.bestComposerForWorkspace("proj")
	if !ok || c.ComposerID != "c3" {
		t.Fatalf("expected the non-archived, non-draft composer to win despite lower lastUpdatedAt, got %+v ok=%v", c, ok)
	}
}

func TestBestComposerForWorkspace_UnnamedComposerStillWins(t *testing.T) {
	// A real, currently-active session just hasn't been auto-titled yet —
	// this must NOT be filtered out in favor of an older, named one; the
	// UI's own fallbackTitle handles an empty Title gracefully.
	s := openTestComposerDB(t)
	setComposerHeaders(t, s, `{"allComposers":[
		{"composerId":"c1","name":"Old named session","lastUpdatedAt":1000,"workspaceIdentifier":{"uri":{"fsPath":"/Users/x/proj"}}},
		{"composerId":"c2","lastUpdatedAt":5000,"workspaceIdentifier":{"uri":{"fsPath":"/Users/x/proj"}}}
	]}`)

	c, ok := s.bestComposerForWorkspace("proj")
	if !ok || c.ComposerID != "c2" {
		t.Fatalf("expected the unnamed but more recent composer to win, got %+v ok=%v", c, ok)
	}
}

func TestUsageForComposer_TracksLastActionAndLastNonZeroTokens(t *testing.T) {
	s := openTestComposerDB(t)
	bubbles := []struct {
		id, data string
	}{
		{"b1", `{"type":1,"text":"faça o merge"}`},
		{"b2", `{"type":2,"toolFormerData":{"name":"run_terminal_cmd"},"tokenCount":{"inputTokens":100,"outputTokens":20}}`},
		{"b3", `{"type":2,"text":"Merge concluído.","tokenCount":{"inputTokens":0,"outputTokens":0}}`},
	}
	for _, b := range bubbles {
		if _, err := s.db.Exec(`INSERT INTO cursorDiskKV (key, value) VALUES (?, ?)`, "bubbleId:comp1:"+b.id, b.data); err != nil {
			t.Fatalf("insert bubble %s: %v", b.id, err)
		}
	}

	u, ok := s.usageForComposer("comp1")
	if !ok {
		t.Fatalf("expected a usage reading")
	}
	if u.LastAction != "💭 Merge concluído." {
		t.Fatalf("LastAction = %q, want the last bubble's text", u.LastAction)
	}
	if u.TokensIn != 100 || u.TokensOut != 20 {
		t.Fatalf("tokens = (%d, %d), want the last NON-ZERO reading (100, 20), not the final zeroed-out bubble", u.TokensIn, u.TokensOut)
	}
}

func TestUsageForComposer_NoBubblesReturnsNotOK(t *testing.T) {
	s := openTestComposerDB(t)
	if _, ok := s.usageForComposer("no-such-composer"); ok {
		t.Fatalf("expected ok=false for a composer with no bubbles")
	}
}

func TestSummarizeBubble(t *testing.T) {
	cases := []struct {
		name string
		b    bubble
		want string
	}{
		{"tool call", bubble{Type: 2, ToolFormerData: &struct {
			Name string `json:"name"`
		}{Name: "edit_file"}}, "🔧 edit_file"},
		{"assistant text", bubble{Type: 2, Text: "Pronto, alterado."}, "💭 Pronto, alterado."},
		{"user text (assistant hasn't replied yet)", bubble{Type: 1, Text: "faça isso"}, "👤 faça isso"},
		{"empty text, no tool", bubble{Type: 2, Text: ""}, ""},
	}
	for _, c := range cases {
		if got := summarizeBubble(c.b); got != c.want {
			t.Errorf("%s: summarizeBubble() = %q, want %q", c.name, got, c.want)
		}
	}
}
