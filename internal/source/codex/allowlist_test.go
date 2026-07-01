package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoDirectoryWalkOverCodexHome is the CI guard from the design doc: it
// greps this package's own source for filepath.Walk/filepath.Glob and fails
// the build if either appears. Enumerating ~/.codex is exactly what would
// let a future change accidentally slurp auth.json.
func TestNoDirectoryWalkOverCodexHome(t *testing.T) {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	forbidden := []string{"filepath.Walk", "filepath.Glob", "os.ReadDir(filepath.Join(home"}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		src := string(raw)
		for _, f := range forbidden {
			if strings.Contains(src, f) {
				t.Fatalf("%s contains forbidden directory-walk pattern %q — the codex adapter must only ever open exact allowlisted paths (see allowlist.go)", e.Name(), f)
			}
		}
	}
}

// TestAuthJSONNeverAllowlisted fixes the hard security constraint: auth.json
// (plaintext OpenAI API key) must never appear as an allowlisted path.
func TestAuthJSONNeverAllowlisted(t *testing.T) {
	for key, rel := range allowedRelPaths {
		if strings.Contains(rel, "auth.json") {
			t.Fatalf("allowedRelPaths[%q] = %q must never reference auth.json", key, rel)
		}
	}
}
