package cursor

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
