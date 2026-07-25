package agent

import "testing"

func TestIsTrivialChat(t *testing.T) {
	yes := []string{
		"hi", "Hi!", "hello", "hey there", "thanks", "good morning",
		"how are you?", "what's up", "yo",
	}
	no := []string{
		"list the files here",
		"read README.md",
		"hi, please list dir",
		"fix the bug in main.go",
		"what files are in this folder",
		"",
	}
	for _, s := range yes {
		if !isTrivialChat(s) {
			t.Errorf("expected trivial: %q", s)
		}
	}
	for _, s := range no {
		if isTrivialChat(s) {
			t.Errorf("expected non-trivial: %q", s)
		}
	}
}
