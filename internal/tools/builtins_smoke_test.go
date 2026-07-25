package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuiltinsBasic(t *testing.T) {
	// Run from module root so relative paths match real agenterm usage.
	root := findModuleRoot(t)
	cwd, _ := os.Getwd()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	r := DefaultBuiltins(false)
	out, err := r.Run(context.Background(), "list_dir", `{"path":"."}`)
	if err != nil || !strings.Contains(out, "go.mod") {
		t.Fatalf("list_dir: %v %q", err, out)
	}
	out, err = r.Run(context.Background(), "read_file", `{"path":"README.md"}`)
	if err != nil || !strings.Contains(out, "agenterm") {
		t.Fatalf("read_file: %v %q", err, trunc(out, 80))
	}
	out, err = r.Run(context.Background(), "find_files", `{"name":"go.mod","root":"."}`)
	if err != nil || !strings.Contains(out, "go.mod") {
		t.Fatalf("find_files: %v %q", err, out)
	}
	// Invented "repo/" prefix should still resolve when README.md exists at cwd.
	out, err = r.Run(context.Background(), "read_file", `{"path":"repo/README.md"}`)
	if err != nil || !strings.Contains(out, "agenterm") {
		t.Fatalf("resolve repo/README: %v %q", err, trunc(out, 80))
	}
}

func findModuleRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
