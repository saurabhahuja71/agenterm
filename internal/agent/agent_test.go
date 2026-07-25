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

func TestIsLinkCheckRequest(t *testing.T) {
	if !isLinkCheckRequest("check links in this repo and it shud be workin ones") {
		t.Fatal("expected link check")
	}
	if !isLinkCheckRequest("check links in this documentation it should work") {
		t.Fatal("expected documentation link check")
	}
	if isLinkCheckRequest("how does linking work in go") {
		t.Fatal("not a link check")
	}
}

func TestExtractHTTPURLs(t *testing.T) {
	text := `see https://example.com/foo and http://golang.org/pkg. also (https://x.test/a).`
	got := extractHTTPURLs(text)
	if len(got) < 2 {
		t.Fatalf("urls: %v", got)
	}
}

func TestIsLocalOrPrivateURL(t *testing.T) {
	locals := []string{
		"http://localhost:8080/ords",
		"http://127.0.0.1:8443/health",
		"https://[::1]:8484/x",
		"http://192.168.1.5/app",
	}
	for _, u := range locals {
		if !isLocalOrPrivateURL(u) {
			t.Errorf("want local: %s", u)
		}
	}
	if isLocalOrPrivateURL("https://github.com/oracle/example") {
		t.Fatal("github should be external")
	}
}

func TestIsTemplatePlaceholderURL(t *testing.T) {
	if !isTemplatePlaceholderURL("https://$PROMETHEUS_SVC/api/v1/query") {
		t.Fatal("want template")
	}
	if isTemplatePlaceholderURL("https://prometheus.io/docs") {
		t.Fatal("real host")
	}
}

func TestIsShellOnlyAssistantText(t *testing.T) {
	bare := `grep https?:\/\/ . --files-with-matches | xargs -I {} grep -oP 'https?://' {} | sort -u`
	if !isShellOnlyAssistantText(bare) {
		t.Fatal("expected shell-only")
	}
	if isShellOnlyAssistantText("I checked three URLs and two are fine.") {
		t.Fatal("prose should not be shell-only")
	}
}

func TestRecoverShellishToGrep(t *testing.T) {
	known := map[string]struct{}{"grep": {}, "run_shell": {}, "fetch": {}}
	bare := `grep https?:\/\/ . --files-with-matches | xargs -I {} grep -oP 'https?://' {} | sort -u`
	calls, rest, note := recoverShellishContent(bare, known)
	if len(calls) != 1 || calls[0].Function.Name != "grep" {
		t.Fatalf("want grep recovery, got %+v note=%q", calls, note)
	}
	if rest != "" {
		t.Fatalf("rest should be empty, got %q", rest)
	}
}
