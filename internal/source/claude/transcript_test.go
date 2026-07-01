package claude

import "testing"

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

func TestUsage_ContextPctSuppressedWhenOverAssumedWindow(t *testing.T) {
	setFakeCacheDir(t, "/home/test/.cache")
	configDir := "/home/test/.claude"
	sessionID := "s4"
	transcriptPath := configDir + "/projects/-Users-demo/s4.jsonl"

	// pid 1 (init/launchd) is alive on any Unix test runner — needed
	// because populateTranscriptUsage only considers Alive sessions, and
	// Alive is a real gopsutil process-existence check, not mockable here.
	f := &fakeReader{
		dirs: map[string][]string{
			configDir + "/sessions": {"111.json"},
		},
		files: map[string][]byte{
			configDir + "/sessions/111.json": []byte(`{"pid":1,"sessionId":"` + sessionID + `","cwd":"/Users/demo","status":"busy","updatedAt":1000}`),
			// cache_read alone (569869-style) blows way past a 200k window.
			transcriptPath: []byte(transcriptLineFor("claude-sonnet-5", 2, 1737, 569869, 1122)),
		},
	}
	withFakeReader(t, f)

	a := &Adapter{configDir: configDir, transcript: newTranscriptTracker()}
	u, err := a.Usage(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u.TokensIn == 0 || u.TokensOut == 0 {
		t.Fatalf("expected real token counts regardless of the window guess, got %+v", u)
	}
	if u.ContextUsedPct != 0 {
		t.Fatalf("expected ContextUsedPct suppressed (0/absent) when the computed pct exceeds 100%%, got %v", u.ContextUsedPct)
	}
	if !u.Available {
		t.Fatalf("expected Available=true: token counts are real data even though context%% was suppressed")
	}
}
