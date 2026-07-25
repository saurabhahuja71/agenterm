package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// runTests runs a project check command (default: go test ./... when go.mod exists).
type runTests struct {
	DefaultCmd string
}

func (runTests) Name() string { return "run_tests" }
func (r runTests) Description() string {
	def := r.DefaultCmd
	if def == "" {
		def = "auto (go test ./... if go.mod, else make test)"
	}
	return "Run project tests/checks. Default command: " + def + ". Use after edits to verify."
}
func (runTests) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "Override check command (bash -lc). Empty = configured/auto default.",
			},
		},
	}
}

func (r runTests) Run(ctx context.Context, argsJSON string) (string, error) {
	var in struct {
		Command string `json:"command"`
	}
	_ = json.Unmarshal([]byte(argsJSON), &in)
	cmdStr := strings.TrimSpace(in.Command)
	if cmdStr == "" {
		cmdStr = strings.TrimSpace(r.DefaultCmd)
	}
	if cmdStr == "" {
		cmdStr = autoTestCommand()
	}
	if cmdStr == "" {
		return "", fmt.Errorf("no test command configured (set test_command in config or pass command)")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "-lc", cmdStr)
	out, err := cmd.CombinedOutput()
	s := string(out)
	if len(s) > 40_000 {
		s = s[:40_000] + "\n…[truncated]…"
	}
	if err != nil {
		return fmt.Sprintf("$ %s\n%s\n[exit error: %v]", cmdStr, s, err), nil
	}
	return fmt.Sprintf("$ %s\n%s\nok", cmdStr, s), nil
}

func autoTestCommand() string {
	if _, err := os.Stat("go.mod"); err == nil {
		return "go test ./..."
	}
	if _, err := os.Stat("Makefile"); err == nil {
		return "make test"
	}
	if _, err := os.Stat("package.json"); err == nil {
		return "npm test --if-present"
	}
	return ""
}
