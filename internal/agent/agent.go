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
- Paths for read_file / list_dir / write_file / str_replace / find_files / git are relative to that cwd, or absolute.
- Do NOT invent prefixes like "repo/" or invent file names (no fake main.go/config.go lists).
- Only report paths that appeared in tool results.
- If the user names a project (e.g. sidb/oracle-database-operator), find_files or list_dir first, then read_file.
- For "can you read X?" after a successful read_file: answer "Yes" (or "No" + why) in one short sentence.
- For "can you do it" / apply / implement: use str_replace or write_file (and git if needed). Do not only print a plan.
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
	// Action requests: nudge the model in the same user turn so it executes tools.
	payload := user
	if isActionRequest(user) {
		payload = user + "\n\n[agenterm] Execute now with tools (str_replace/write_file/git). Do not only print steps."
		emit(Event{Kind: EventStatus, Text: "action mode: will apply changes via tools"})
	}
	a.History = append(a.History, llm.Message{Role: llm.RoleUser, Content: payload})

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

	maxRounds := a.MaxToolRounds
	if isActionRequest(user) && maxRounds < 12 {
		maxRounds = 12 // branch + edit + commit may need more steps
	}

	toolsUsed := 0
	for round := 0; round < maxRounds; round++ {
		if err := ctx.Err(); err != nil {
			emit(Event{Kind: EventError, Text: "cancelled"})
			emit(Event{Kind: EventDone})
			return err
		}

		// Build request messages; after tools, add a non-persisted brief-answer nudge.
		msgs := a.History
		if toolsUsed > 0 {
			msgs = append(append([]llm.Message{}, a.History...), llm.Message{
				Role:    llm.RoleSystem,
				Content: afterToolsAnswerHint(user, toolsUsed),
			})
		} else if isActionRequest(user) && round == 0 {
			msgs = append(append([]llm.Message{}, a.History...), llm.Message{
				Role: llm.RoleSystem,
				Content: `The user wants real on-disk changes. Call tools now:
1) read_file if needed, 2) str_replace or write_file, 3) git add/commit/push only if they asked.
Do not answer with only a markdown plan or shell snippets.`,
			})
		}

		req := llm.ChatRequest{
			Model:       a.Cfg.Model,
			Messages:    msgs,
			Temperature: a.Cfg.Temperature,
			MaxTokens:   a.Cfg.MaxTokens,
		}
		// After tools: cooler sampling + shorter completion → less rambling / fake lists.
		if toolsUsed > 0 {
			if req.Temperature <= 0 || req.Temperature > 0.3 {
				req.Temperature = 0.2
			}
			if req.MaxTokens == 0 || req.MaxTokens > 600 {
				req.MaxTokens = 600
			}
		}

		// Multi-step tools allowed; action tasks get more tool rounds before we force an answer.
		toolCap := 4
		if isActionRequest(user) {
			toolCap = 10
		}
		roundTools := toolSchemas
		if round > 0 && a.Cfg.EnableTools && a.Tools != nil && toolsUsed < toolCap && round < toolCap {
			roundTools = a.Tools.LLMTools()
		}
		if toolsUsed >= toolCap || round >= toolCap {
			roundTools = nil // force plain-text answer
			emit(Event{Kind: EventStatus, Text: "final answer (no more tools)"})
		}
		if len(roundTools) > 0 {
			req.Tools = roundTools
		}

		emit(Event{Kind: EventStatus, Text: fmt.Sprintf("calling %s (round %d)…", a.Cfg.Model, round+1)})
		handler := &streamBridge{emit: emit}
		msg, err := a.Client.ChatStream(ctx, req, handler)
		if err != nil {
			if ctx.Err() != nil {
				emit(Event{Kind: EventError, Text: "cancelled"})
			} else {
				emit(Event{Kind: EventError, Text: err.Error()})
			}
			emit(Event{Kind: EventDone})
			return err
		}

		// Ollama/Qwen often print tools as plain JSON content — recover and run them.
		if len(msg.ToolCalls) == 0 && len(roundTools) > 0 && a.Tools != nil {
			if recovered, rest := extractToolCallsFromContent(msg.Content, toolNameSet(a.Tools)); len(recovered) > 0 {
				emit(Event{Kind: EventStatus, Text: fmt.Sprintf("recovered %d tool call(s) from text", len(recovered))})
				msg.ToolCalls = recovered
				msg.Content = rest
			}
		}

		// Persist assistant turn (without the ephemeral after-tools system nudge)
		a.History = append(a.History, msg)

		if len(msg.ToolCalls) == 0 {
			// Some models return only whitespace after a long load — surface it.
			if strings.TrimSpace(msg.Content) == "" {
				emit(Event{Kind: EventToken, Text: "(empty reply — model may still be loading; try again or /model list)"})
			}
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
			// Cap what the model sees so it does not re-dump huge listings into chat.
			outForModel := capToolResult(out, 10_000)
			emit(Event{Kind: EventToolEnd, Tool: name, ToolOut: out}) // TUI uses compact formatter
			a.History = append(a.History, llm.Message{
				Role:       llm.RoleTool,
				ToolCallID: tc.ID,
				Content:    outForModel,
				Name:       name,
			})
			toolsUsed++
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

func (s *streamBridge) OnStatus(text string) {
	if text != "" {
		s.emit(Event{Kind: EventStatus, Text: text})
	}
}

// afterToolsAnswerHint is injected only into the next model request (not history).
func afterToolsAnswerHint(userQuestion string, toolsUsed int) string {
	if isActionRequest(userQuestion) {
		return fmt.Sprintf(`You have used %d tool call(s). Continue the user's request using ONLY real tool results.
- If file/git changes are still incomplete, call more tools (str_replace/write_file/git).
- If work is done, give a short confirmation of what changed on disk (paths + git result). Do not invent success.
- Do not paste full file bodies or long plans.`, toolsUsed)
	}
	base := `You now have tool results above. Answer the user's latest question using ONLY those results.
Rules:
- Do not invent files, folders, or paths that did not appear in tool output.
- Do not paste large listings or full file bodies unless the user asked to show them.
- Prefer a short direct answer (a few sentences max).`
	uq := strings.TrimSpace(userQuestion)
	low := strings.ToLower(uq)
	// "can you do it" is action, not yes/no — handled above.
	if !isActionRequest(userQuestion) &&
		((strings.Contains(low, "yes") && strings.Contains(low, "no")) ||
			strings.Contains(low, "yes or no") || strings.Contains(low, "y/n") ||
			(strings.HasPrefix(low, "can you") && strings.Contains(low, "read")) ||
			strings.Contains(low, "able to read")) {
		base += "\n- This is a yes/no style question: start with Yes or No, then one short line of detail."
	}
	return base
}

// isActionRequest is true when the user wants real on-disk / git changes, not advice only.
func isActionRequest(user string) bool {
	s := strings.ToLower(strings.TrimSpace(user))
	if s == "" {
		return false
	}
	// Short "do it" / "apply" follow-ups
	switch strings.Join(strings.Fields(strings.Trim(s, "!.?")), " ") {
	case "do it", "do this", "please do it", "go ahead", "apply it", "apply",
		"make the change", "make the changes", "implement it", "just do it",
		"can you do it", "could you do it", "yes do it", "ok do it", "yes apply":
		return true
	}
	needles := []string{
		"do it", "apply the", "apply these", "apply this", "make the change",
		"implement ", "implement it", "write the", "update the readme", "update readme",
		"edit the", "fix the", "create a branch", "create branch", "commit ",
		"git commit", "git push", "push the", "refactor ", "add a section",
		"improve the readme", "improve readme", "please apply", "go ahead and",
		"make these changes", "make those changes",
	}
	for _, n := range needles {
		if strings.Contains(s, n) {
			return true
		}
	}
	return false
}

func capToolResult(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "\n…[truncated for model; do not invent the rest]…"
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
