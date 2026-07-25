package agent

import (
	"context"
	"fmt"
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
	if strings.TrimSpace(e.SystemPrompt) != "" {
		hist = append(hist, llm.Message{Role: llm.RoleSystem, Content: e.SystemPrompt})
	}
	return &Agent{
		Cfg:           e,
		Client:        client,
		Tools:         reg,
		History:       hist,
		MaxToolRounds: 8,
	}
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

	var toolSchemas []llm.Tool
	if a.Cfg.EnableTools && a.Tools != nil {
		toolSchemas = a.Tools.LLMTools()
	}

	for round := 0; round < a.MaxToolRounds; round++ {
		req := llm.ChatRequest{
			Model:       a.Cfg.Model,
			Messages:    a.History,
			Temperature: a.Cfg.Temperature,
			MaxTokens:   a.Cfg.MaxTokens,
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
