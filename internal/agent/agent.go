package agent

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/saurabhahuja71/agenterm/internal/config"
	"github.com/saurabhahuja71/agenterm/internal/llm"
	"github.com/saurabhahuja71/agenterm/internal/tools"
)

// Event kinds streamed to the TUI.
type EventKind int

const (
	EventToken EventKind = iota
	EventToolStart
	EventToolEnd
	EventError
	EventDone
	EventStatus
)

type Event struct {
	Kind    EventKind
	Text    string
	Tool    string
	ToolOut string
}

// Agent runs the multi-turn tool loop against an OpenAI-compatible model.
type Agent struct {
	Cfg    config.Config
	Client *llm.Client
	Tools  *tools.Registry
	// History is the full conversation (including system).
	History []llm.Message
	// MaxToolRounds prevents infinite tool loops.
	MaxToolRounds int
}

func New(cfg config.Config, client *llm.Client, reg *tools.Registry) *Agent {
	e := cfg.Effective()
	hist := []llm.Message{}
	sys := strings.TrimSpace(e.SystemPrompt)
	if sys != "" {
		sys = sys + "\n\n" + workspaceHint()
		hist = append(hist, llm.Message{Role: llm.RoleSystem, Content: sys})
	} else {
		hist = append(hist, llm.Message{Role: llm.RoleSystem, Content: workspaceHint()})
	}
	return &Agent{
		Cfg:           e,
		Client:        client,
		Tools:         reg,
		History:       hist,
		MaxToolRounds: 8,
	}
}

// workspaceHint tells the model where tools resolve paths (critical for repo reads).
func workspaceHint() string {
	cwd, err := os.Getwd()
	if err != nil || cwd == "" {
		cwd = "."
	}
	return strings.TrimSpace(fmt.Sprintf(`
Workspace (tool paths resolve here):
- Current working directory: %s
- Paths for read_file / list_dir / write_file / find_files are relative to that cwd, or absolute.
- Do NOT invent prefixes like "repo/". Use real paths under the cwd.
- If the user names a project (e.g. dboper/sidb/oracle-database-operator), first list_dir or find_files to locate README.md, then read_file.
- Prefer find_files with name "README.md" under a likely root when the exact path is unknown.
- Use the API tool-calling interface. If you must emit JSON, use exactly: {"name":"read_file","arguments":{"path":"..."}}
`, cwd))
}

// Reset clears chat history but keeps system prompt.
func (a *Agent) Reset() {
	sys := ""
	if len(a.History) > 0 && a.History[0].Role == llm.RoleSystem {
		sys = a.History[0].Content
	}
	a.History = nil
	if sys != "" {
		a.History = []llm.Message{{Role: llm.RoleSystem, Content: sys}}
	}
}

// RunUserMessage appends a user message and runs the agent loop, emitting events.
func (a *Agent) RunUserMessage(ctx context.Context, user string, emit func(Event)) error {
	a.History = append(a.History, llm.Message{Role: llm.RoleUser, Content: user})

	// Attach tools only when enabled and the turn is not pure small-talk.
	// Skipping tools for greetings avoids a pointless second LLM round-trip
	// (common with Ollama models that eagerly call list_dir on "hi").
	var toolSchemas []llm.Tool
	attachTools := a.Cfg.EnableTools && a.Tools != nil && !isTrivialChat(user)
	if attachTools {
		toolSchemas = a.Tools.LLMTools()
	} else if a.Cfg.EnableTools && a.Tools != nil && isTrivialChat(user) {
		emit(Event{Kind: EventStatus, Text: "tools skipped (chat-only turn)"})
	}

	for round := 0; round < a.MaxToolRounds; round++ {
		req := llm.ChatRequest{
			Model:       a.Cfg.Model,
			Messages:    a.History,
			Temperature: a.Cfg.Temperature,
			MaxTokens:   a.Cfg.MaxTokens,
		}
		// After a tool result, re-enable tools so multi-step work continues.
		if round > 0 && a.Cfg.EnableTools && a.Tools != nil {
			toolSchemas = a.Tools.LLMTools()
		}
		if len(toolSchemas) > 0 {
			req.Tools = toolSchemas
		}

		handler := &streamBridge{emit: emit}
		msg, err := a.Client.ChatStream(ctx, req, handler)
		if err != nil {
			emit(Event{Kind: EventError, Text: err.Error()})
			return err
		}

		// Ollama/Qwen often print tools as plain JSON content — recover and run them.
		if len(msg.ToolCalls) == 0 && a.Cfg.EnableTools && a.Tools != nil {
			if recovered, rest := extractToolCallsFromContent(msg.Content, toolNameSet(a.Tools)); len(recovered) > 0 {
				emit(Event{Kind: EventStatus, Text: fmt.Sprintf("recovered %d tool call(s) from text", len(recovered))})
				msg.ToolCalls = recovered
				msg.Content = rest
			}
		}

		// Persist assistant turn
		a.History = append(a.History, msg)

		if len(msg.ToolCalls) == 0 {
			emit(Event{Kind: EventDone})
			return nil
		}

		// Execute tools sequentially
		for _, tc := range msg.ToolCalls {
			name := tc.Function.Name
			args := tc.Function.Arguments
			emit(Event{Kind: EventToolStart, Tool: name, Text: args})
			out, err := a.Tools.Run(ctx, name, args)
			if err != nil {
				out = fmt.Sprintf("error: %v\n%s", err, out)
			}
			emit(Event{Kind: EventToolEnd, Tool: name, ToolOut: out})
			a.History = append(a.History, llm.Message{
				Role:       llm.RoleTool,
				ToolCallID: tc.ID,
				Content:    out,
				Name:       name,
			})
		}
		// loop: model continues with tool results
	}
	emit(Event{Kind: EventError, Text: "max tool rounds reached"})
	emit(Event{Kind: EventDone})
	return nil
}

type streamBridge struct {
	emit func(Event)
}

func (s *streamBridge) OnToken(token string) {
	s.emit(Event{Kind: EventToken, Text: token})
}

func (s *streamBridge) OnToolCallDelta(index int, tc llm.ToolCall) {
	// optional: show streaming tool assembly
	_ = index
	_ = tc
}

// isTrivialChat is true for short greetings / small-talk that should not
// trigger tool schemas (faster first reply over Ollama, local or tunneled).
func isTrivialChat(user string) bool {
	s := strings.TrimSpace(strings.ToLower(user))
	if s == "" {
		return false
	}
	// Strip common punctuation for matching.
	s = strings.Map(func(r rune) rune {
		switch r {
		case '!', '?', '.', ',', ';', ':', '"', '\'':
			return -1
		default:
			return r
		}
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 48 {
		return false
	}
	// Paths / shell-ish → not trivial.
	if strings.ContainsAny(s, "/\\") {
		return false
	}
	for _, needle := range []string{
		"list ", "read ", "write ", "create ", "delete ", "open ", "show ",
		"file", "dir", "folder", "code", "repo", "path", "run ", "exec",
		"cd ", "ls ", "cat ", "grep", "find ", "fix", "function", "bug",
		"error", "fix", "implement", "refactor", "debug",
	} {
		if strings.Contains(s, needle) {
			return false
		}
	}
	switch s {
	case "hi", "hello", "hey", "yo", "sup", "howdy", "hola",
		"hi there", "hello there", "hey there",
		"good morning", "good afternoon", "good evening", "good night",
		"thanks", "thank you", "thx", "ty",
		"ok", "okay", "k", "cool", "nice", "great",
		"bye", "goodbye", "see you", "cya",
		"how are you", "how r you", "whats up", "what's up", "what up",
		"who are you", "what are you", "help":
		return true
	}
	// Very short 1–2 word greetings with common openers.
	words := strings.Fields(s)
	if len(words) <= 2 {
		switch words[0] {
		case "hi", "hello", "hey", "yo", "sup", "howdy", "hola", "thanks", "bye":
			return true
		}
	}
	return false
}
