package codex

import "testing"

func TestSynthesizeCodexTitle(t *testing.T) {
	cases := []struct{ in, want string }{
		{"procure no cangaco/ aqui dentro desta pasta\ne junto ao scc", "procure no cangaco/ aqui dentro desta pasta"},
		{"  short   with   extra   spaces  ", "short with extra spaces"},
	}
	for _, c := range cases {
		if got := synthesizeCodexTitle(c.in); got != c.want {
			t.Errorf("synthesizeCodexTitle(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestIngest_SkipsWrapperTagsForTitleButKeepsFirstGenuineMessage is the
// exact real-world case found on this machine: Codex injects
// <environment_context> first, then sometimes <user_shell_command> for
// shell output the user ran themselves -- neither is a real request and
// must not become the card's title. The first later message with plain
// text is, and it's never overwritten by subsequent turns.
func TestIngest_SkipsWrapperTagsForTitleButKeepsFirstGenuineMessage(t *testing.T) {
	tr := newCodexTranscriptTracker()
	data := []byte(
		`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"<environment_context>\n<cwd>/x</cwd>\n</environment_context>"}]}}` + "\n" +
			`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"<user_shell_command>\n<command>ls</command>\n</user_shell_command>"}]}}` + "\n" +
			`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"crie o alias scodex pra abrir o codex com bypass"}]}}` + "\n" +
			`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"esse texto nao deveria aparecer, o titulo ja foi fixado"}]}}` + "\n",
	)
	tr.ingest("s1", data)

	usage, ok := tr.get("s1")
	if !ok {
		t.Fatal("expected a reading")
	}
	want := "crie o alias scodex pra abrir o codex com bypass"
	if usage.Title != want {
		t.Fatalf("Title = %q, want %q", usage.Title, want)
	}
}

func TestIngest_TokenCountUsesAuthoritativeContextWindow(t *testing.T) {
	tr := newCodexTranscriptTracker()
	data := []byte(
		`{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":198688,"output_tokens":2773,"total_tokens":201461},"model_context_window":258400}}}` + "\n",
	)
	tr.ingest("s2", data)

	usage, ok := tr.get("s2")
	if !ok {
		t.Fatal("expected a reading")
	}
	if usage.TokensIn != 198688 || usage.TokensOut != 2773 {
		t.Fatalf("unexpected tokens: %+v", usage)
	}
	if !usage.HasContext {
		t.Fatal("expected HasContext=true: model_context_window is authoritative, not a guess")
	}
	wantPct := float64(201461) / float64(258400) * 100
	if usage.ContextUsedPct != wantPct {
		t.Fatalf("ContextUsedPct = %v, want %v", usage.ContextUsedPct, wantPct)
	}
}
