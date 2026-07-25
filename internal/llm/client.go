package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client talks to any OpenAI-compatible Chat Completions API
// (Ollama, xAI, OpenAI, vLLM, LocalAI, …).
type Client struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

func New(baseURL, apiKey string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  apiKey,
		HTTPClient: &http.Client{
			Timeout: 0, // streaming can be long; use context
		},
	}
}

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type Message struct {
	Role       Role       `json:"role"`
	Content    string     `json:"content,omitempty"`
	Name       string     `json:"name,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type Tool struct {
	Type     string             `json:"type"` // "function"
	Function ToolFunctionSchema `json:"function"`
}

type ToolFunctionSchema struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Tools       []Tool    `json:"tools,omitempty"`
	Temperature float64   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Stream      bool      `json:"stream"`
}

type ChatResponse struct {
	Choices []struct {
		Message      Message `json:"message"`
		FinishReason string  `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content   string     `json:"content"`
			ToolCalls []ToolCall `json:"tool_calls"`
			Role      string     `json:"role"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// StreamHandler receives partial content and final message.
type StreamHandler interface {
	OnToken(token string)
	OnToolCallDelta(index int, tc ToolCall)
}

// ChatStream streams a completion; returns the assembled assistant message.
func (c *Client) ChatStream(ctx context.Context, req ChatRequest, h StreamHandler) (Message, error) {
	req.Stream = true
	body, err := json.Marshal(req)
	if err != nil {
		return Message{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return Message{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	}

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return Message{}, fmt.Errorf("chat request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return Message{}, fmt.Errorf("API %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}

	// Non-SSE JSON fallback (some servers ignore stream)
	ct := resp.Header.Get("Content-Type")
	if strings.Contains(ct, "application/json") && !strings.Contains(ct, "event-stream") {
		var full ChatResponse
		if err := json.NewDecoder(resp.Body).Decode(&full); err != nil {
			return Message{}, err
		}
		if full.Error != nil {
			return Message{}, fmt.Errorf("API error: %s", full.Error.Message)
		}
		if len(full.Choices) == 0 {
			return Message{}, fmt.Errorf("empty choices")
		}
		msg := full.Choices[0].Message
		if h != nil && msg.Content != "" {
			h.OnToken(msg.Content)
		}
		return msg, nil
	}

	msg := Message{Role: RoleAssistant}
	// Accumulate tool call fragments by index
	toolAcc := map[int]*ToolCall{}

	sc := bufio.NewScanner(resp.Body)
	// Increase buffer for large tool payloads
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 1024*1024)

	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var chunk streamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if chunk.Error != nil {
			return Message{}, fmt.Errorf("API error: %s", chunk.Error.Message)
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta
		if delta.Content != "" {
			msg.Content += delta.Content
			if h != nil {
				h.OnToken(delta.Content)
			}
		}
		for i, tc := range delta.ToolCalls {
			// OpenAI streams tool_calls with index field sometimes; use loop index if missing
			idx := i
			// try parse index from raw — many servers put "index" on tool call object
			var raw map[string]any
			b, _ := json.Marshal(tc)
			_ = json.Unmarshal(b, &raw)
			if v, ok := raw["index"].(float64); ok {
				idx = int(v)
			}
			acc, ok := toolAcc[idx]
			if !ok {
				acc = &ToolCall{Type: "function"}
				toolAcc[idx] = acc
			}
			if tc.ID != "" {
				acc.ID = tc.ID
			}
			if tc.Type != "" {
				acc.Type = tc.Type
			}
			if tc.Function.Name != "" {
				acc.Function.Name += tc.Function.Name
			}
			if tc.Function.Arguments != "" {
				acc.Function.Arguments += tc.Function.Arguments
			}
			if h != nil {
				h.OnToolCallDelta(idx, *acc)
			}
		}
	}
	if err := sc.Err(); err != nil {
		return Message{}, err
	}

	if len(toolAcc) > 0 {
		// stable order by index
		max := -1
		for i := range toolAcc {
			if i > max {
				max = i
			}
		}
		for i := 0; i <= max; i++ {
			if tc, ok := toolAcc[i]; ok {
				if tc.ID == "" {
					tc.ID = fmt.Sprintf("call_%d_%d", time.Now().UnixNano(), i)
				}
				if tc.Type == "" {
					tc.Type = "function"
				}
				msg.ToolCalls = append(msg.ToolCalls, *tc)
			}
		}
	}
	return msg, nil
}

// Chat non-streaming convenience.
func (c *Client) Chat(ctx context.Context, req ChatRequest) (Message, error) {
	req.Stream = false
	body, err := json.Marshal(req)
	if err != nil {
		return Message{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return Message{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	// finite timeout for non-stream
	cli := *c.HTTPClient
	cli.Timeout = 10 * time.Minute
	resp, err := cli.Do(httpReq)
	if err != nil {
		return Message{}, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return Message{}, fmt.Errorf("API %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	var full ChatResponse
	if err := json.Unmarshal(b, &full); err != nil {
		return Message{}, err
	}
	if full.Error != nil {
		return Message{}, fmt.Errorf("API error: %s", full.Error.Message)
	}
	if len(full.Choices) == 0 {
		return Message{}, fmt.Errorf("empty choices")
	}
	return full.Choices[0].Message, nil
}

// Ping checks the server is reachable (Ollama tags or models list).
func (c *Client) Ping(ctx context.Context) error {
	// Try OpenAI-compatible /models
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/models", nil)
	if err != nil {
		return err
	}
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	cli := &http.Client{Timeout: 5 * time.Second}
	resp, err := cli.Do(req)
	if err != nil {
		// Ollama root without /v1
		return fmt.Errorf("cannot reach %s: %w", c.BaseURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("server error: %s", resp.Status)
	}
	return nil
}
