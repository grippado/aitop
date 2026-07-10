package claude

import (
	"strings"
	"testing"
)

func TestSummarizeLastAction(t *testing.T) {
	toolBlock := contentBlock{Type: "tool_use", Name: "Bash"}
	toolBlock.Input.Command = "go test ./... with a very long command that should get clamped down to size"
	textBlock := contentBlock{Type: "text", Text: "thinking about   the   next   step\nacross lines"}
	emptyText := contentBlock{Type: "text", Text: "   "}

	if got := summarizeLastAction(nil); got != "" {
		t.Fatalf("expected empty for no blocks, got %q", got)
	}
	if got := summarizeLastAction([]contentBlock{emptyText}); got != "" {
		t.Fatalf("expected empty for blank text, got %q", got)
	}
	if got := summarizeLastAction([]contentBlock{textBlock}); got != "💭 thinking about the next step across lines" {
		t.Fatalf("unexpected text summary: %q", got)
	}
	if got := summarizeLastAction([]contentBlock{textBlock, toolBlock}); !strings.HasPrefix(got, "🔧") {
		t.Fatalf("expected the LAST block (tool_use) to win over an earlier text block, got %q", got)
	}
}

func TestIngest_CapturesAiTitleAndLastAction(t *testing.T) {
	tr := newTranscriptTracker()
	data := []byte(
		`{"type":"ai-title","aiTitle":"Resolver conflitos do wsync"}` + "\n" +
			transcriptLineFor("claude-sonnet-5", 5, 10, 20, 30) + // no content -> no action
			`{"message":{"content":[{"type":"tool_use","name":"Read","input":{"file_path":"/x/y.go"}}]}}` + "\n" + // action, no usage
			`{"type":"ai-title","aiTitle":"aitop-tui-monitoring"}` + "\n", // title updates again
	)
	tr.ingest("sX", data)

	usage, ok := tr.get("sX")
	if !ok {
		t.Fatal("expected a reading")
	}
	if usage.Title != "aitop-tui-monitoring" {
		t.Fatalf("expected the LATEST ai-title to win, got %q", usage.Title)
	}
	if usage.LastAction != "🔧 Read: /x/y.go" {
		t.Fatalf("unexpected last action: %q", usage.LastAction)
	}
	if usage.InputTokens != 5 {
		t.Fatalf("expected usage fields from the one line that had them, got %+v", usage)
	}
}

func transcriptLineFor(model string, in, cacheCreate, cacheRead, out int64) string {
	return `{"message":{"model":"` + model + `","usage":{"input_tokens":` + itoa64(in) +
		`,"cache_creation_input_tokens":` + itoa64(cacheCreate) +
		`,"cache_read_input_tokens":` + itoa64(cacheRead) +
		`,"output_tokens":` + itoa64(out) + `}}}` + "\n"
}

func itoa64(n int64) string {
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

func TestTranscriptTracker_ResolvesByEncodedCWDGuessAndReadsLatestUsage(t *testing.T) {
	configDir := "/home/test/.claude"
	sessionID := "s1"
	transcriptPath := "/home/test/.claude/projects/-Users-demo/s1.jsonl"

	f := &fakeReader{
		files: map[string][]byte{
			transcriptPath: []byte(
				transcriptLineFor("claude-sonnet-5", 2, 100, 500, 50) +
					`{"type":"bridge-session","sessionId":"s1"}` + "\n" + // no usage — must be skipped
					transcriptLineFor("claude-sonnet-5", 5, 200, 1000, 300)), // latest — must win
		},
	}
	withFakeReader(t, f)

	tr := newTranscriptTracker()
	usage, ok := tr.usageFor(configDir, "/Users/demo", sessionID)
	if !ok {
		t.Fatal("expected a usage reading")
	}
	if usage.InputTokens != 5 || usage.CacheCreationInputTokens != 200 || usage.CacheReadInputTokens != 1000 || usage.OutputTokens != 300 {
		t.Fatalf("expected the LATEST usage line to win, got %+v", usage)
	}
}

func TestTranscriptTracker_FallsBackToScanningProjectsWhenGuessMisses(t *testing.T) {
	configDir := "/home/test/.claude"
	realPath := "/home/test/.claude/projects/some-other-encoding/s2.jsonl"

	f := &fakeReader{
		dirs: map[string][]string{
			configDir + "/projects":                     {"some-other-encoding"},
			configDir + "/projects/some-other-encoding": {}, // presence marks it as a directory, see ReadDir's isDir lookup
		},
		files: map[string][]byte{
			realPath: []byte(transcriptLineFor("claude-opus-4-8", 10, 20, 30, 40)),
		},
	}
	withFakeReader(t, f)

	tr := newTranscriptTracker()
	usage, ok := tr.usageFor(configDir, "/Users/demo/unencoded/path", "s2")
	if !ok {
		t.Fatal("expected fallback scan to find the transcript")
	}
	if usage.InputTokens != 10 {
		t.Fatalf("unexpected usage: %+v", usage)
	}
}

func TestTranscriptTracker_OnlyReadsNewBytesOnSecondCall(t *testing.T) {
	configDir := "/home/test/.claude"
	path := "/home/test/.claude/projects/-Users-demo/s3.jsonl"

	f := &fakeReader{
		files: map[string][]byte{
			path: []byte(transcriptLineFor("claude-sonnet-5", 1, 1, 1, 1)),
		},
	}
	withFakeReader(t, f)

	tr := newTranscriptTracker()
	first, _ := tr.usageFor(configDir, "/Users/demo", "s3")
	if first.InputTokens != 1 {
		t.Fatalf("unexpected first read: %+v", first)
	}

	// Simulate the transcript growing with a new turn.
	f.files[path] = append(f.files[path], []byte(transcriptLineFor("claude-sonnet-5", 9, 9, 9, 9))...)
	second, _ := tr.usageFor(configDir, "/Users/demo", "s3")
	if second.InputTokens != 9 {
		t.Fatalf("expected the new turn's usage after growth, got %+v", second)
	}
}

func TestDeriveTokenFields_SuppressesPctOverAssumedWindow(t *testing.T) {
	// The window guess was corrected from 200k to 1M (see
	// contextWindowForModel's doc comment) after 200k proved wrong in
	// practice — this fixture now needs to exceed 1M to still exercise
	// suppression at all.
	usage := transcriptUsage{Model: "claude-sonnet-5", InputTokens: 2, CacheCreationInputTokens: 100000, CacheReadInputTokens: 2000000, OutputTokens: 1122}
	tokensIn, tokensOut, ctxPct, hasCtx := deriveTokenFields(usage)
	if tokensIn == 0 || tokensOut == 0 {
		t.Fatalf("expected real token counts regardless of the window guess, got in=%d out=%d", tokensIn, tokensOut)
	}
	if hasCtx || ctxPct != 0 {
		t.Fatalf("expected ContextUsedPct suppressed when the computed pct exceeds 100%%, got hasCtx=%v pct=%v", hasCtx, ctxPct)
	}
}

// TestSessions_PerSessionTokensDontMixAcrossSessions is the exact bug
// reported in practice: two different Claude Code sessions showed
// identical token counts because Usage() picked one "best" session and
// applied it tool-wide to every card. Tokens now come from each
// session's own transcript directly.
func TestSessions_PerSessionTokensDontMixAcrossSessions(t *testing.T) {
	configDir := "/home/test/.claude"
	f := &fakeReader{
		dirs: map[string][]string{
			configDir + "/sessions": {"111.json", "222.json"},
		},
		files: map[string][]byte{
			configDir + "/sessions/111.json":               []byte(`{"pid":1,"sessionId":"sA","cwd":"/Users/demo/a","status":"busy","updatedAt":1000}`),
			configDir + "/sessions/222.json":               []byte(`{"pid":1,"sessionId":"sB","cwd":"/Users/demo/b","status":"idle","updatedAt":1000}`),
			configDir + "/projects/-Users-demo-a/sA.jsonl": []byte(transcriptLineFor("claude-sonnet-5", 10, 0, 0, 5)),
			configDir + "/projects/-Users-demo-b/sB.jsonl": []byte(transcriptLineFor("claude-sonnet-5", 999, 0, 0, 888)),
		},
	}
	withFakeReader(t, f)
	// PID 1 is alive, so Sessions() runs the PPID walk; feed a synthetic tree
	// so it never touches live gopsutil (golden invariant 3).
	withFakePpid(t, map[int]int{})

	a := &Adapter{configDir: configDir, transcript: newTranscriptTracker()}
	sessions, err := a.Sessions(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	byID := map[string]int64{}
	for _, s := range sessions {
		byID[s.ID] = s.TokensIn
	}
	if byID["sA"] != 10 {
		t.Fatalf("session sA TokensIn = %d, want 10", byID["sA"])
	}
	if byID["sB"] != 999 {
		t.Fatalf("session sB TokensIn = %d, want 999", byID["sB"])
	}
	if byID["sA"] == byID["sB"] {
		t.Fatalf("sessions must not share the same token reading")
	}
}
