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
	return "List files and directories in a path (relative to cwd or absolute)"
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
	return "Read a text file (truncated for large files)"
}
func (readFile) Schema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"required":             []string{"path"},
		"additionalProperties": false,
		"properties": map[string]any{
			"path": map[string]any{"type": "string"},
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
	data, err := os.ReadFile(in.Path)
	if err != nil {
		return "", err
	}
	const max = 80_000
	if len(data) > max {
		return string(data[:max]) + "\n…[truncated]…", nil
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

// DefaultBuiltins registers safe tools; shell optional.
func DefaultBuiltins(enableShell bool) *Registry {
	r := NewRegistry()
	r.Register(listDir{})
	r.Register(readFile{})
	r.Register(writeFile{})
	if enableShell {
		r.Register(runShell{})
	}
	return r
}
