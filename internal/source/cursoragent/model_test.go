package cursoragent

import "testing"

func TestFriendlyModelName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"claude-sonnet-5", "sonnet 5"},
		{"claude-opus-4-8", "opus 4.8"},
		{"composer-2.5", "composer 2.5"},
		{"", ""},
	}
	for _, c := range cases {
		if got := friendlyModelName(c.in); got != c.want {
			t.Errorf("friendlyModelName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCurrentModel_ReadsCliConfig(t *testing.T) {
	f := &fakeReader{files: map[string][]byte{
		"/home/test/.cursor/cli-config.json": []byte(`{"model":{"modelId":"claude-sonnet-5","displayName":"Sonnet 5 300K High"}}`),
	}}
	withFakeReader(t, f)

	if got := currentModel("/home/test"); got != "claude-sonnet-5" {
		t.Fatalf("currentModel() = %q, want %q", got, "claude-sonnet-5")
	}
}

func TestCurrentModel_MissingFileReturnsEmpty(t *testing.T) {
	withFakeReader(t, &fakeReader{})
	if got := currentModel("/home/test"); got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}
