package tui

import "testing"

func TestIsMostlyToolNoise(t *testing.T) {
	if !isMostlyToolNoise(`{"name": "read_file", "arguments": {"path":"/tmp/x"}}`) {
		t.Fatal("json tool")
	}
	if !isMostlyToolNoise("Let's read the main README.md file.\n\n{\"name\":\"read_file\",\"arguments\":{\"path\":\"README.md\"}}") {
		t.Fatal("preamble+json")
	}
	if !isMostlyToolNoise(`<tool_call>{"name":"list_dir"}</tool_call>`) {
		t.Fatal("xml tool_call")
	}
	if !isMostlyToolNoise(`Let me check the files first.`) {
		t.Fatal("let me check")
	}
	if !isMostlyToolNoise(`find_files README.md`) {
		t.Fatal("bare tool sketch")
	}
	if isMostlyToolNoise(`Yes, I can read the documentation in that folder.`) {
		t.Fatal("real answer should show")
	}
	// Must not hide normal long replies (this was wiping SEO/doc answers).
	long := "I need to optimise the README for SEO. First, improve the title and headings, then add a clear install section and FAQ."
	if isMostlyToolNoise(long) {
		t.Fatal("long prose must not be treated as tool noise")
	}
	// Long run_shell crawl dump must still be noise (was shown as hung "Agent" reply).
	crawl := `run_shell {"command": "wget -qO- https://github.com/x | grep -oP href | xargs -n 1 curl --head --silent"}`
	if !isMostlyToolNoise(crawl) {
		t.Fatal("run_shell crawl dump must be noise")
	}
	// Bare shell the model pastes as "Agent" answer
	bare := `grep https?:\/\/ . --files-with-matches | xargs -I {} grep -oP 'https?://\K[\w.-\/]+' {} | sort -u`
	if !isMostlyToolNoise(bare) {
		t.Fatal("bare grep|xargs pipeline must be noise")
	}
}

func TestFlushKeepsLongAnswer(t *testing.T) {
	// regression: quiet flush must not drop multi-sentence answers
	long := "I need to optimise the README for SEO. First, improve the title and headings."
	if isMostlyToolNoise(long) {
		t.Fatal("should keep")
	}
	if stripLeadingToolNoise(`{"name":"read_file","arguments":{"path":"x"}}`+"\n\n"+long) != long {
		t.Fatal("strip should leave prose")
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
