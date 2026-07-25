package agent

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/saurabhahuja71/agenterm/internal/llm"
)

// Many Ollama models (e.g. some Qwen builds) print tool invocations as plain
// text JSON instead of OpenAI tool_calls. Recover those so the agent loop runs.

var (
	reJSONObject = regexp.MustCompile(`(?s)\{[^{}]*(?:\{[^{}]*\}[^{}]*)*\}`)
	reFencedJSON = regexp.MustCompile("(?s)```(?:json|tool)?\\s*(\\{.*?\\})\\s*```")
)

type textToolCall struct {
	Name      string          `json:"name"`
	Tool      string          `json:"tool"`
	Function  string          `json:"function"`
	Arguments json.RawMessage `json:"arguments"`
	Params    json.RawMessage `json:"parameters"`
	Args      json.RawMessage `json:"args"`
	Path      string          `json:"path"` // sometimes flattened
}

// extractToolCallsFromContent parses pseudo tool-calls from assistant text.
// Returns recovered calls and remaining non-tool text (may be empty).
func extractToolCallsFromContent(content string, knownTools map[string]struct{}) ([]llm.ToolCall, string) {
	content = strings.TrimSpace(content)
	if content == "" || knownTools == nil || len(knownTools) == 0 {
		return nil, content
	}

	var calls []llm.ToolCall
	rest := content

	// Prefer fenced blocks first.
	if locs := reFencedJSON.FindAllStringSubmatchIndex(content, -1); len(locs) > 0 {
		var b strings.Builder
		last := 0
		for _, loc := range locs {
			// loc: full start, full end, group1 start, group1 end
			if len(loc) < 4 {
				continue
			}
			b.WriteString(content[last:loc[0]])
			raw := content[loc[2]:loc[3]]
			if tc, ok := parseOneToolJSON(raw, knownTools); ok {
				calls = append(calls, tc)
			} else {
				b.WriteString(content[loc[0]:loc[1]])
			}
			last = loc[1]
		}
		b.WriteString(content[last:])
		rest = strings.TrimSpace(b.String())
		if len(calls) > 0 {
			return calls, rest
		}
	}

	// Whole message is a single JSON tool call.
	if tc, ok := parseOneToolJSON(content, knownTools); ok {
		return []llm.ToolCall{tc}, ""
	}

	// Scan for JSON objects that look like tool calls.
	matches := reJSONObject.FindAllStringIndex(content, -1)
	if len(matches) == 0 {
		return nil, content
	}
	var b strings.Builder
	last := 0
	for _, m := range matches {
		raw := content[m[0]:m[1]]
		if tc, ok := parseOneToolJSON(raw, knownTools); ok {
			b.WriteString(content[last:m[0]])
			calls = append(calls, tc)
			last = m[1]
			continue
		}
	}
	b.WriteString(content[last:])
	rest = strings.TrimSpace(b.String())
	if len(calls) == 0 {
		return nil, content
	}
	return calls, rest
}

func parseOneToolJSON(raw string, knownTools map[string]struct{}) (llm.ToolCall, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw[0] != '{' {
		return llm.ToolCall{}, false
	}
	var t textToolCall
	if err := json.Unmarshal([]byte(raw), &t); err != nil {
		return llm.ToolCall{}, false
	}
	name := firstNonEmpty(t.Name, t.Tool, t.Function)
	name = strings.TrimSpace(name)
	if name == "" {
		return llm.ToolCall{}, false
	}
	// Normalize common aliases
	switch name {
	case "read", "Read", "ReadFile", "read-file":
		name = "read_file"
	case "list", "List", "ListDir", "list-dir", "ls":
		name = "list_dir"
	case "write", "Write", "WriteFile", "write-file":
		name = "write_file"
	case "find", "Find", "FindFiles", "find-files", "search":
		name = "find_files"
	case "shell", "bash", "run":
		name = "run_shell"
	}
	if _, ok := knownTools[name]; !ok {
		return llm.ToolCall{}, false
	}

	argsRaw := firstRaw(t.Arguments, t.Params, t.Args)
	args := "{}"
	if len(argsRaw) > 0 {
		args = string(argsRaw)
		// arguments sometimes double-encoded as a string
		var asStr string
		if err := json.Unmarshal(argsRaw, &asStr); err == nil && strings.TrimSpace(asStr) != "" {
			args = asStr
		}
	} else if t.Path != "" {
		b, _ := json.Marshal(map[string]string{"path": t.Path})
		args = string(b)
	}
	// Must look like JSON object for tool runners
	if !json.Valid([]byte(args)) {
		// wrap as path-only if bare string path
		b, _ := json.Marshal(map[string]string{"path": strings.Trim(args, `"`)})
		args = string(b)
	}

	id := fmt.Sprintf("textcall_%d", time.Now().UnixNano())
	return llm.ToolCall{
		ID:   id,
		Type: "function",
		Function: llm.FunctionCall{
			Name:      name,
			Arguments: args,
		},
	}, true
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

func firstRaw(parts ...json.RawMessage) json.RawMessage {
	for _, p := range parts {
		if len(p) > 0 && string(p) != "null" {
			return p
		}
	}
	return nil
}

func toolNameSet(reg interface{ Names() []string }) map[string]struct{} {
	if reg == nil {
		return nil
	}
	names := reg.Names()
	m := make(map[string]struct{}, len(names))
	for _, n := range names {
		m[n] = struct{}{}
	}
	return m
}
