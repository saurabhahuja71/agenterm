package agent

import "testing"

func TestIsActionRequest(t *testing.T) {
	yes := []string{"can you do it", "do it", "apply the changes", "please implement it", "create a branch and commit"}
	no := []string{"hi", "can you read the readme yes or no", "what is SEO", "explain SEO friendly docs"}
	for _, s := range yes {
		if !isActionRequest(s) {
			t.Errorf("want action: %q", s)
		}
	}
	for _, s := range no {
		if isActionRequest(s) {
			t.Errorf("want non-action: %q", s)
		}
	}
}

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
