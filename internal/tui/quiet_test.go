package tui

import "testing"

func TestIsMostlyToolNoise(t *testing.T) {
	if !isMostlyToolNoise(`{"name": "read_file", "arguments": {"path":"/tmp/x"}}`) {
		t.Fatal("json tool")
	}
	if !isMostlyToolNoise(`Let's read the main README.md file.\n\n{"name":"read_file","arguments":{"path":"README.md"}}`) {
		t.Fatal("preamble+json")
	}
	if isMostlyToolNoise(`Yes, I can read the documentation in that folder.`) {
		t.Fatal("real answer should show")
	}
}

func TestFormatToolEndQuiet(t *testing.T) {
	s := formatToolEnd("read_file", stringsRepeat("x", 2048), false)
	if contains(s, "xxxx") {
		t.Fatalf("should not dump body: %s", s)
	}
	if !contains(s, "ok") {
		t.Fatalf("want ok summary: %s", s)
	}
}

func stringsRepeat(s string, n int) string {
	b := make([]byte, 0, n*len(s))
	for i := 0; i < n; i++ {
		b = append(b, s...)
	}
	return string(b)
}
func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
