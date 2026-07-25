package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/saurabhahuja71/agenterm/internal/llm"
)

// Runner executes a named tool with JSON arguments.
type Runner interface {
	Name() string
	Description() string
	Schema() map[string]any
	Run(ctx context.Context, argsJSON string) (string, error)
}

// Registry maps tool name → runner and builds LLM tool schemas.
type Registry struct {
	runners map[string]Runner
}

func NewRegistry() *Registry {
	return &Registry{runners: map[string]Runner{}}
}

func (r *Registry) Register(t Runner) {
	r.runners[t.Name()] = t
}

func (r *Registry) LLMTools() []llm.Tool {
	out := make([]llm.Tool, 0, len(r.runners))
	for _, t := range r.runners {
		out = append(out, llm.Tool{
			Type: "function",
			Function: llm.ToolFunctionSchema{
				Name:        t.Name(),
				Description: t.Description(),
				Parameters:  t.Schema(),
			},
		})
	}
	return out
}

func (r *Registry) Run(ctx context.Context, name, argsJSON string) (string, error) {
	t, ok := r.runners[name]
	if !ok {
		return "", fmt.Errorf("unknown tool %q", name)
	}
	return t.Run(ctx, argsJSON)
}

func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.runners))
	for n := range r.runners {
		names = append(names, n)
	}
	return names
}

// --- built-in tools ---

type listDir struct{}

func (listDir) Name() string { return "list_dir" }
func (listDir) Description() string {
	return "List files and directories. path is relative to the process cwd or absolute. Use '.' for workspace root."
}
func (listDir) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string", "description": "Directory path (default .)"},
		},
	}
}
func (listDir) Run(_ context.Context, argsJSON string) (string, error) {
	var in struct {
		Path string `json:"path"`
	}
	_ = json.Unmarshal([]byte(argsJSON), &in)
	if in.Path == "" {
		in.Path = "."
	}
	entries, err := os.ReadDir(in.Path)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, e := range entries {
		suffix := ""
		if e.IsDir() {
			suffix = "/"
		}
		fmt.Fprintf(&b, "%s%s\n", e.Name(), suffix)
	}
	return b.String(), nil
}

type readFile struct{}

func (readFile) Name() string { return "read_file" }
func (readFile) Description() string {
	return "Read a text file (truncated for large files). path is relative to cwd or absolute. Example: sidb/oracle-database-operator/README.md"
}
func (readFile) Schema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"required":             []string{"path"},
		"additionalProperties": false,
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "File path relative to cwd or absolute (not repo/... unless that folder exists)",
			},
		},
	}
}
func (readFile) Run(_ context.Context, argsJSON string) (string, error) {
	var in struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &in); err != nil || in.Path == "" {
		return "", fmt.Errorf("path required")
	}
	path, err := resolveExistingFile(in.Path)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	// Keep reads useful but not so large that models re-dump them into chat.
	const max = 24_000
	if len(data) > max {
		return string(data[:max]) + "\n…[truncated]…", nil
	}
	if path != in.Path {
		return fmt.Sprintf("[resolved path: %s]\n%s", path, string(data)), nil
	}
	return string(data), nil
}

type writeFile struct{}

func (writeFile) Name() string { return "write_file" }
func (writeFile) Description() string {
	return "Write content to a file (creates parent dirs)"
}
func (writeFile) Schema() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"path", "content"},
		"properties": map[string]any{
			"path":    map[string]any{"type": "string"},
			"content": map[string]any{"type": "string"},
		},
	}
}
func (writeFile) Run(_ context.Context, argsJSON string) (string, error) {
	var in struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &in); err != nil {
		return "", err
	}
	if in.Path == "" {
		return "", fmt.Errorf("path required")
	}
	if err := os.MkdirAll(filepath.Dir(in.Path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(in.Path, []byte(in.Content), 0o644); err != nil {
		return "", err
	}
	return fmt.Sprintf("wrote %d bytes to %s", len(in.Content), in.Path), nil
}

type runShell struct{}

func (runShell) Name() string { return "run_shell" }
func (runShell) Description() string {
	return "Run a shell command (bash -lc). Use carefully."
}
func (runShell) Schema() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"command"},
		"properties": map[string]any{
			"command": map[string]any{"type": "string"},
		},
	}
}
func (runShell) Run(ctx context.Context, argsJSON string) (string, error) {
	var in struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &in); err != nil || strings.TrimSpace(in.Command) == "" {
		return "", fmt.Errorf("command required")
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "-lc", in.Command)
	out, err := cmd.CombinedOutput()
	s := string(out)
	if len(s) > 50_000 {
		s = s[:50_000] + "\n…[truncated]…"
	}
	if err != nil {
		return fmt.Sprintf("%s\n[exit error: %v]", s, err), nil
	}
	return s, nil
}

type findFiles struct{}

func (findFiles) Name() string { return "find_files" }
func (findFiles) Description() string {
	return "Find files by exact name or substring under a root (default .). Use to locate README.md or a project folder when the full path is unknown."
}
func (findFiles) Schema() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"name"},
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "File or directory name / substring, e.g. README.md or oracle-database-operator",
			},
			"root": map[string]any{
				"type":        "string",
				"description": "Search root relative to cwd (default .)",
			},
			"max_results": map[string]any{
				"type":        "integer",
				"description": "Max paths to return (default 30)",
			},
		},
	}
}
func (findFiles) Run(_ context.Context, argsJSON string) (string, error) {
	var in struct {
		Name       string `json:"name"`
		Root       string `json:"root"`
		MaxResults int    `json:"max_results"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &in); err != nil || strings.TrimSpace(in.Name) == "" {
		return "", fmt.Errorf("name required")
	}
	root := in.Root
	if root == "" {
		root = "."
	}
	if in.MaxResults <= 0 {
		in.MaxResults = 30
	}
	needle := strings.ToLower(strings.TrimSpace(in.Name))
	var hits []string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		// Skip huge / noisy trees
		base := d.Name()
		if d.IsDir() {
			switch base {
			case ".git", "node_modules", "vendor", "dist", "target", ".idea", ".cache":
				return filepath.SkipDir
			}
		}
		if strings.Contains(strings.ToLower(base), needle) {
			hits = append(hits, path)
			if len(hits) >= in.MaxResults {
				return fmt.Errorf("done")
			}
		}
		// Cap walk depth roughly by path segments under root
		rel, relErr := filepath.Rel(root, path)
		if relErr == nil && rel != "." {
			if len(strings.Split(rel, string(os.PathSeparator))) > 8 {
				if d.IsDir() {
					return filepath.SkipDir
				}
			}
		}
		return nil
	})
	if len(hits) == 0 {
		return fmt.Sprintf("no matches for %q under %s", in.Name, root), nil
	}
	return strings.Join(hits, "\n"), nil
}

// resolveExistingFile tries the path and common corrections models invent.
func resolveExistingFile(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", fmt.Errorf("path required")
	}
	candidates := []string{p}
	// Models invent "repo/..." prefixes
	for _, prefix := range []string{"repo/", "repos/", "./repo/"} {
		if strings.HasPrefix(p, prefix) {
			candidates = append(candidates, strings.TrimPrefix(p, prefix))
		}
	}
	// Common typo: dbope vs dboper
	if strings.Contains(p, "dbope") {
		candidates = append(candidates, strings.ReplaceAll(p, "dbope", "dboper"))
		if strings.HasPrefix(p, "repo/") {
			candidates = append(candidates, strings.ReplaceAll(strings.TrimPrefix(p, "repo/"), "dbope", "dboper"))
		}
	}
	// If looking for README under a project name, try sibling patterns
	// e.g. path ends with oracle-database-operator/README.md
	seen := map[string]struct{}{}
	for _, c := range candidates {
		c = filepath.Clean(c)
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c, nil
		}
	}

	// Last resort: search for basename under cwd (shallow-ish via WalkDir capped)
	base := filepath.Base(p)
	if base != "" && base != "." && base != string(filepath.Separator) {
		var found string
		_ = filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				if d != nil && d.IsDir() {
					switch d.Name() {
					case ".git", "node_modules", "vendor":
						return filepath.SkipDir
					}
				}
				return nil
			}
			if strings.EqualFold(d.Name(), base) {
				// Prefer paths that also contain parent dir name if present
				parent := filepath.Base(filepath.Dir(p))
				if parent != "" && parent != "." && !strings.Contains(path, parent) {
					if found == "" {
						found = path
					}
					return nil
				}
				found = path
				return fmt.Errorf("done")
			}
			rel, relErr := filepath.Rel(".", path)
			if relErr == nil && len(strings.Split(rel, string(os.PathSeparator))) > 10 {
				return nil
			}
			return nil
		})
		if found != "" {
			return found, nil
		}
	}
	return "", fmt.Errorf("file not found: %s (cwd-relative; try find_files or list_dir)", p)
}

// DefaultBuiltins registers safe tools; shell optional.
func DefaultBuiltins(enableShell bool) *Registry {
	r := NewRegistry()
	r.Register(listDir{})
	r.Register(readFile{})
	r.Register(writeFile{})
	r.Register(findFiles{})
	if enableShell {
		r.Register(runShell{})
	}
	return r
}
