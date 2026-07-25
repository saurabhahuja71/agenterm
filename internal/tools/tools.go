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
	return "APPLY changes: write full file contents to disk (creates parent dirs). Use when the user asks you to implement/edit/update a file — do not only describe the change."
}
func (writeFile) Schema() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"path", "content"},
		"properties": map[string]any{
			"path":    map[string]any{"type": "string", "description": "File path relative to cwd or absolute"},
			"content": map[string]any{"type": "string", "description": "Full new file contents"},
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

// strReplace edits a file by replacing an exact old_string with new_string (once or all).
type strReplace struct{}

func (strReplace) Name() string { return "str_replace" }
func (strReplace) Description() string {
	return "APPLY a surgical edit: replace exact text in an existing file. Prefer this over rewrite for README/docs/code patches. old_string must match exactly once unless replace_all is true."
}
func (strReplace) Schema() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"path", "old_string", "new_string"},
		"properties": map[string]any{
			"path":        map[string]any{"type": "string"},
			"old_string":  map[string]any{"type": "string", "description": "Exact text to find"},
			"new_string":  map[string]any{"type": "string", "description": "Replacement text"},
			"replace_all": map[string]any{"type": "boolean", "description": "Replace every occurrence (default false)"},
		},
	}
}
func (strReplace) Run(_ context.Context, argsJSON string) (string, error) {
	var in struct {
		Path       string `json:"path"`
		OldString  string `json:"old_string"`
		NewString  string `json:"new_string"`
		ReplaceAll bool   `json:"replace_all"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &in); err != nil {
		return "", err
	}
	if in.Path == "" || in.OldString == "" {
		return "", fmt.Errorf("path and old_string required")
	}
	path, err := resolveExistingFile(in.Path)
	if err != nil {
		// allow creating? no — str_replace needs existing
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	content := string(data)
	count := strings.Count(content, in.OldString)
	if count == 0 {
		return "", fmt.Errorf("old_string not found in %s", path)
	}
	if count > 1 && !in.ReplaceAll {
		return "", fmt.Errorf("old_string found %d times in %s (set replace_all=true or use a more unique old_string)", count, path)
	}
	var next string
	if in.ReplaceAll {
		next = strings.ReplaceAll(content, in.OldString, in.NewString)
	} else {
		next = strings.Replace(content, in.OldString, in.NewString, 1)
	}
	if err := os.WriteFile(path, []byte(next), 0o644); err != nil {
		return "", err
	}
	n := 1
	if in.ReplaceAll {
		n = count
	}
	return fmt.Sprintf("updated %s (%d replacement(s), now %d bytes)", path, n, len(next)), nil
}

// gitCmd runs a small allowlisted set of git commands (no full shell).
type gitCmd struct{}

func (gitCmd) Name() string { return "git" }
func (gitCmd) Description() string {
	return `Run a safe git subcommand. Pass either:
- args as array: {"args":["checkout","-b","seo-readme"]}
- args as string: {"args":"checkout -b seo-readme"}
- command string: {"command":"checkout -b seo-readme"} or {"command":"git status"}
Do not nest JSON arrays as strings. Prefer simple string form if unsure.`
}
func (gitCmd) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"args": map[string]any{
				"description": "Git argv after 'git'. Prefer array [\"status\"] or string \"status -sb\". Also accepts JSON-encoded array string.",
				// no type constraint — models send array OR string
			},
			"command": map[string]any{
				"type":        "string",
				"description": "Alternate: full git command without or with leading 'git', e.g. \"checkout -b seo-readme\"",
			},
		},
	}
}
func (gitCmd) Run(ctx context.Context, argsJSON string) (string, error) {
	args, err := parseGitArgsJSON(argsJSON)
	if err != nil {
		return "", err
	}
	if err := validateGitArgs(args); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	out, err := cmd.CombinedOutput()
	s := string(out)
	if len(s) > 40_000 {
		s = s[:40_000] + "\n…[truncated]…"
	}
	if err != nil {
		return fmt.Sprintf("%s\n[exit error: %v]", s, err), nil
	}
	if strings.TrimSpace(s) == "" {
		return "ok (no output)", nil
	}
	return s, nil
}

// parseGitArgsJSON accepts many shapes models actually emit.
func parseGitArgsJSON(argsJSON string) ([]string, error) {
	raw := strings.TrimSpace(argsJSON)
	if raw == "" || raw == "{}" || raw == "null" {
		return nil, fmt.Errorf("args required (e.g. {\"args\":\"checkout -b seo-readme\"})")
	}

	// Whole payload is a JSON array: ["checkout","-b","seo"]
	if strings.HasPrefix(raw, "[") {
		var arr []string
		if err := json.Unmarshal([]byte(raw), &arr); err == nil && len(arr) > 0 {
			return normalizeGitArgv(arr), nil
		}
	}

	// Generic object map
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		// bare string command?
		return splitGitCommand(raw), nil
	}

	// Prefer args, then command, then argv
	for _, key := range []string{"args", "command", "argv", "cmd"} {
		v, ok := m[key]
		if !ok || v == nil {
			continue
		}
		switch t := v.(type) {
		case []any:
			out := make([]string, 0, len(t))
			for _, x := range t {
				out = append(out, fmt.Sprint(x))
			}
			if len(out) > 0 {
				return normalizeGitArgv(out), nil
			}
		case []string:
			if len(t) > 0 {
				return normalizeGitArgv(t), nil
			}
		case string:
			s := strings.TrimSpace(t)
			if s == "" {
				continue
			}
			// Double-encoded JSON array as string: "[\"checkout\",\"-b\",\"x\"]"
			if strings.HasPrefix(s, "[") {
				var arr []string
				if err := json.Unmarshal([]byte(s), &arr); err == nil && len(arr) > 0 {
					return normalizeGitArgv(arr), nil
				}
			}
			return splitGitCommand(s), nil
		}
	}

	// Single field "subcommand" + rest
	if sub, ok := m["subcommand"].(string); ok && sub != "" {
		rest, _ := m["options"].(string)
		return splitGitCommand(strings.TrimSpace(sub + " " + rest)), nil
	}

	return nil, fmt.Errorf("could not parse git args from %s — use {\"args\":\"checkout -b branch\"}", truncateLine(raw, 120))
}

func normalizeGitArgv(args []string) []string {
	if len(args) == 0 {
		return args
	}
	// Drop leading "git" if model included it
	if strings.EqualFold(args[0], "git") {
		args = args[1:]
	}
	return args
}

func splitGitCommand(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "git ")
	s = strings.TrimPrefix(s, "git\t")
	if s == "" {
		return nil
	}
	// simple split respecting basic quotes
	var out []string
	var cur strings.Builder
	inQ := byte(0)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inQ != 0 {
			if c == inQ {
				inQ = 0
			} else {
				cur.WriteByte(c)
			}
			continue
		}
		if c == '"' || c == '\'' {
			inQ = c
			continue
		}
		if c == ' ' || c == '\t' {
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
			continue
		}
		cur.WriteByte(c)
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return normalizeGitArgv(out)
}

func validateGitArgs(args []string) error {
	sub := strings.ToLower(args[0])
	allowed := map[string]bool{
		"status": true, "diff": true, "log": true, "show": true,
		"branch": true, "checkout": true, "switch": true, "add": true,
		"commit": true, "push": true, "pull": true, "fetch": true,
		"stash": true, "rev-parse": true, "remote": true,
	}
	if !allowed[sub] {
		return fmt.Errorf("git subcommand %q not allowed (use status/diff/branch/checkout/add/commit/push/…)", sub)
	}
	// Block shell injection via git -c core.editor etc. is ok; block obvious footguns
	joined := strings.Join(args, " ")
	if strings.Contains(joined, ";") || strings.Contains(joined, "|") || strings.Contains(joined, "`") || strings.Contains(joined, "$(") {
		return fmt.Errorf("git args contain shell metacharacters")
	}
	// push --force requires explicit env opt-in
	if sub == "push" {
		for _, a := range args[1:] {
			al := strings.ToLower(a)
			if al == "--force" || al == "-f" || al == "--force-with-lease" {
				if os.Getenv("AGENTERM_ALLOW_GIT_FORCE") != "1" {
					return fmt.Errorf("git push --force blocked (set AGENTERM_ALLOW_GIT_FORCE=1 to allow)")
				}
			}
		}
	}
	return nil
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

// BuiltinOpts configures optional tools.
type BuiltinOpts struct {
	EnableShell bool
	TestCommand string
}

// DefaultBuiltins registers safe tools; shell optional.
// str_replace + git + grep are always on so the agent can explore and apply changes.
func DefaultBuiltins(enableShell bool) *Registry {
	return DefaultBuiltinsOpts(BuiltinOpts{EnableShell: enableShell})
}

// DefaultBuiltinsOpts registers tools with options.
func DefaultBuiltinsOpts(opts BuiltinOpts) *Registry {
	r := NewRegistry()
	r.Register(listDir{})
	r.Register(readFile{})
	r.Register(writeFile{})
	r.Register(strReplace{})
	r.Register(findFiles{})
	r.Register(grepTool{})
	r.Register(gitCmd{})
	r.Register(runTests{DefaultCmd: opts.TestCommand})
	if opts.EnableShell {
		r.Register(runShell{})
	}
	return r
}
