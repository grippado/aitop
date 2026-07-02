package codex

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
)

// sessionMeta mirrors the first line of a rollout-*.jsonl file, a
// {"type":"session_meta","payload":{"id":...,"cwd":...}} event — confirmed
// on this machine's real rollout files. Reading just the first line (not
// the whole transcript) is enough to get the session's cwd.
type sessionMetaLine struct {
	Type    string `json:"type"`
	Payload struct {
		ID  string `json:"id"`
		CWD string `json:"cwd"`
	} `json:"payload"`
}

// cwdResolver finds a Codex session's cwd by locating its rollout file
// under sessions/YYYY/MM/DD/rollout-<ts>-<sessionID>.jsonl — the filename
// itself carries the session ID, so only one file is ever opened per
// lookup. Results are cached: a session's cwd never changes once set, and
// re-scanning the whole sessions/ tree every tick would be wasteful.
type cwdResolver struct {
	mu    sync.Mutex
	cache map[string]string // sessionID -> cwd (may be "" if genuinely not found)
	tried map[string]bool
}

func newCWDResolver() *cwdResolver {
	return &cwdResolver{cache: map[string]string{}, tried: map[string]bool{}}
}

func (r *cwdResolver) resolve(configDir, sessionID string) string {
	r.mu.Lock()
	if r.tried[sessionID] {
		v := r.cache[sessionID]
		r.mu.Unlock()
		return v
	}
	r.mu.Unlock()

	cwd := findSessionCWD(configDir, sessionID)

	r.mu.Lock()
	r.cache[sessionID] = cwd
	r.tried[sessionID] = true
	r.mu.Unlock()
	return cwd
}

// findSessionCWD walks sessions/<year>/<month>/<day>/ (never the ~/.codex
// root — see runner.go) looking for a filename containing sessionID, then
// reads only that one file's first line for its cwd.
func findSessionCWD(configDir, sessionID string) string {
	path := findSessionRolloutPath(configDir, sessionID)
	if path == "" {
		return ""
	}
	return readFirstLineCWD(path)
}

// findSessionRolloutPath is the shared directory-cascade lookup behind
// findSessionCWD (reads just the first line) and the full tail-follow
// transcript reader in usage.go (reads the whole growing file) — both
// need the same rollout-*.jsonl path for a given session ID.
func findSessionRolloutPath(configDir, sessionID string) string {
	sessionsRoot := filepath.Join(configDir, "sessions")
	years, err := reader.ReadDir(sessionsRoot)
	if err != nil {
		return ""
	}
	for _, y := range years {
		if !y.IsDir() {
			continue
		}
		yearDir := filepath.Join(sessionsRoot, y.Name())
		months, err := reader.ReadDir(yearDir)
		if err != nil {
			continue
		}
		for _, m := range months {
			if !m.IsDir() {
				continue
			}
			monthDir := filepath.Join(yearDir, m.Name())
			days, err := reader.ReadDir(monthDir)
			if err != nil {
				continue
			}
			for _, d := range days {
				if !d.IsDir() {
					continue
				}
				dayDir := filepath.Join(monthDir, d.Name())
				files, err := reader.ReadDir(dayDir)
				if err != nil {
					continue
				}
				for _, f := range files {
					if strings.Contains(f.Name(), sessionID) {
						return filepath.Join(dayDir, f.Name())
					}
				}
			}
		}
	}
	return ""
}

func readFirstLineCWD(path string) string {
	raw, err := reader.ReadFile(path)
	if err != nil {
		return ""
	}
	nl := indexByte(raw, '\n')
	if nl >= 0 {
		raw = raw[:nl]
	}
	var meta sessionMetaLine
	if err := json.Unmarshal(raw, &meta); err != nil {
		return ""
	}
	return meta.Payload.CWD
}

func indexByte(b []byte, c byte) int {
	for i, v := range b {
		if v == c {
			return i
		}
	}
	return -1
}

// rolloutUUIDLen is the fixed length of a UUID (8-4-4-4-12 hex, with
// dashes): 36 characters, regardless of the timestamp prefix in a
// rollout filename.
const rolloutUUIDLen = 36

// findLatestRolloutSessionID finds the most recently written rollout file
// across the whole sessions/ tree and extracts its session ID from the
// filename itself (rollout-<timestamp>-<uuid>.jsonl — the UUID is always
// the last 36 characters before ".jsonl"). Year/month/day directory names
// and the filename's embedded timestamp are all zero-padded/ISO-like, so
// picking the lexicographically last entry at each level is equivalent to
// picking the chronologically latest one.
//
// Used when a live Codex process can't be correlated to any session via
// history.jsonl/chat_processes.json (e.g. a session started moments ago,
// before history.jsonl caught up) — the alternative is leaving that
// session permanently unidentified, which blocks every transcript-based
// reading (tokens, context%, last action all key off session ID).
func findLatestRolloutSessionID(configDir string) (string, bool) {
	sessionsRoot := filepath.Join(configDir, "sessions")
	year, ok := latestDirEntry(sessionsRoot)
	if !ok {
		return "", false
	}
	yearDir := filepath.Join(sessionsRoot, year)
	month, ok := latestDirEntry(yearDir)
	if !ok {
		return "", false
	}
	monthDir := filepath.Join(yearDir, month)
	day, ok := latestDirEntry(monthDir)
	if !ok {
		return "", false
	}
	dayDir := filepath.Join(monthDir, day)
	file, ok := latestDirEntry(dayDir)
	if !ok {
		return "", false
	}

	name := strings.TrimSuffix(file, ".jsonl")
	if len(name) < rolloutUUIDLen {
		return "", false
	}
	return name[len(name)-rolloutUUIDLen:], true
}

// latestDirEntry returns the lexicographically last entry name in dir
// (works for both subdirectories and files — every level of the
// sessions/ tree only ever contains one kind).
func latestDirEntry(dir string) (string, bool) {
	entries, err := reader.ReadDir(dir)
	if err != nil {
		return "", false
	}
	best := ""
	for _, e := range entries {
		if e.Name() > best {
			best = e.Name()
		}
	}
	return best, best != ""
}
