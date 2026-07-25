package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExpandMentions(t *testing.T) {
	dir := t.TempDir()
	cwd, _ := os.Getwd()
	_ = os.Chdir(dir)
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	_ = os.WriteFile(filepath.Join(dir, "hello.md"), []byte("# Hi\n"), 0o644)

	aug, names := expandMentions("look at @hello.md please")
	if !strings.Contains(names, "hello.md") {
		t.Fatalf("names=%q", names)
	}
	if !strings.Contains(aug, "# Hi") {
		t.Fatalf("aug missing content: %s", aug)
	}
}
