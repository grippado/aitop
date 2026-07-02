package widgets

import (
	"reflect"
	"testing"
)

func TestWrap(t *testing.T) {
	cases := []struct {
		name         string
		text         string
		width, maxLn int
		want         []string
	}{
		{"empty", "", 10, 2, nil},
		{"fits one line", "hello world", 20, 2, []string{"hello world"}},
		{
			"wraps across exactly maxLines with no leftover",
			"one two three four", 8, 2,
			[]string{"one two", "three…"},
		},
		{
			"fewer than maxLines when short",
			"short text", 20, 4,
			[]string{"short text"},
		},
		{
			"truncates with ellipsis when text exceeds maxLines*width",
			"aaaa bbbb cccc dddd eeee ffff", 9, 2,
			[]string{"aaaa bbbb", "cccc ddd…"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Wrap(c.text, c.width, c.maxLn)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("Wrap(%q, %d, %d) = %#v, want %#v", c.text, c.width, c.maxLn, got, c.want)
			}
		})
	}
}
