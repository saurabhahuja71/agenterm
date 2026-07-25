package tui

import "testing"

func TestScrubTerminalGarbage(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{`]11;rgb:fafa/fafa/fdfd\`, ""},
		{"hello", "hello"},
		{"]11;rgb:fafa/fafa/fdfd\\hello", "hello"},
		{"\x1b]11;rgb:1111/2222/3333\x07hi", "hi"},
		{"rgb:fafa/fafa/fdfd\\", ""},
	}
	for _, c := range cases {
		got := scrubTerminalGarbage(c.in)
		if got != c.want {
			t.Errorf("scrub(%q) = %q want %q", c.in, got, c.want)
		}
	}
}
