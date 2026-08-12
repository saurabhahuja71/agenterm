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

type sshExecute struct{}

func (sshExecute) Name() string { return "ssh_execute" }
func (sshExecute) Description() string {
	return "Execute a literal command on an SSH config alias (for example podman8 or podman9) and return raw stdout/stderr. Use for all remote work; never substitute run_shell."
}
func (sshExecute) Schema() map[string]any {
	return map[string]any{"type": "object", "required": []string{"host", "command"}, "additionalProperties": false, "properties": map[string]any{
		"host": map[string]any{"type": "string", "description": "SSH config alias"},
		"command": map[string]any{"type": "string", "description": "Literal remote command"},
		"timeout": map[string]any{"type": "number", "default": 30},
	}}
}
func (sshExecute) Run(ctx context.Context, argsJSON string) (string, error) {
	var in struct { Host string `json:"host"`; Command string `json:"command"`; Timeout float64 `json:"timeout"` }
	if err := json.Unmarshal([]byte(argsJSON), &in); err != nil { return "", err }
	if strings.TrimSpace(in.Host) == "" || strings.TrimSpace(in.Command) == "" { return "", fmt.Errorf("host and command required") }
	if in.Timeout <= 0 { in.Timeout = 30 }
	ctx, cancel := context.WithTimeout(ctx, time.Duration(in.Timeout*float64(time.Second))); defer cancel()
	config := os.Getenv("SSH_CONFIG_PATH")
	args := []string{"-T", "-o", "BatchMode=yes", "-o", "ConnectTimeout=10"}
	if config != "" { args = append(args, "-F", config) }
	args = append(args, in.Host, in.Command)
	out, err := exec.CommandContext(ctx, "ssh", args...).CombinedOutput()
	if ctx.Err() != nil { return string(out), fmt.Errorf("ssh timed out after %.0fs", in.Timeout) }
	if err != nil { return string(out), fmt.Errorf("ssh failed: %w", err) }
	return string(out), nil
}
