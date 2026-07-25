package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/saurabhahuja71/agenterm/internal/agent"
)

var (
	colorMuted  = lipgloss.Color("#94a3b8")
	colorAccent = lipgloss.Color("#38bdf8")
	colorUser   = lipgloss.Color("#a78bfa")
	colorAsst   = lipgloss.Color("#4ade80")
	colorTool   = lipgloss.Color("#fbbf24")
	colorError  = lipgloss.Color("#f87171")
	colorBorder = lipgloss.Color("#1e293b")

	styleHeader = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Padding(0, 1)
	styleStatus = lipgloss.NewStyle().Foreground(colorMuted).Padding(0, 1)
	styleHelp   = lipgloss.NewStyle().Foreground(colorMuted).Padding(0, 1)
	styleUser   = lipgloss.NewStyle().Foreground(colorUser).Bold(true)
	styleAsst   = lipgloss.NewStyle().Foreground(colorAsst).Bold(true)
	styleTool   = lipgloss.NewStyle().Foreground(colorTool)
	styleErr    = lipgloss.NewStyle().Foreground(colorError)
	styleBox    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorBorder).Padding(0, 1)
)

// Deps wired from main.
type Deps struct {
	Title   string
	Summary string
	Agent   *agent.Agent
}

type chatLine struct {
	role string
	text string
}

type model struct {
	deps     Deps
	vp       viewport.Model
	ta       textarea.Model
	lines    []chatLine
	width    int
	height   int
	busy     bool
	status   string
	stream   strings.Builder
	renderer *glamour.TermRenderer
	cancel   context.CancelFunc
	events   <-chan agent.Event
}

type streamEvMsg agent.Event
type streamClosedMsg struct{}

func New(deps Deps) model {
	ta := textarea.New()
	ta.Placeholder = "Message…  /help  /model  /tools off"
	ta.Focus()
	ta.Prompt = "❯ "
	ta.CharLimit = 0
	ta.SetHeight(3)
	ta.ShowLineNumbers = false
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.BlurredStyle.CursorLine = lipgloss.NewStyle()
	// Enter sends (not newline). Use Alt+Enter for newline if needed later.
	ta.KeyMap.InsertNewline.SetEnabled(false)

	vp := viewport.New(80, 20)

	r, _ := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(80),
	)

	m := model{
		deps:     deps,
		vp:       vp,
		ta:       ta,
		status:   "ready",
		renderer: r,
		lines: []chatLine{
			{role: "system", text: "agenterm — snappy terminal agent (Ollama / OpenAI-compatible + MCP)"},
			{role: "system", text: "Endpoint: " + deps.Summary},
			{role: "system", text: "Enter send · Ctrl+C quit · /help · /model · /tools"},
		},
	}
	m.refreshViewport()
	return m
}

func (m model) Init() tea.Cmd {
	return textarea.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		headerH, helpH := 1, 1
		inputH := m.ta.Height() + 2
		vpH := msg.Height - headerH - helpH - inputH - 2
		if vpH < 5 {
			vpH = 5
		}
		m.vp.Width = max(20, msg.Width-2)
		m.vp.Height = vpH
		m.ta.SetWidth(max(20, msg.Width-4))
		if m.renderer != nil {
			m.renderer, _ = glamour.NewTermRenderer(
				glamour.WithAutoStyle(),
				glamour.WithWordWrap(max(40, msg.Width-8)),
			)
		}
		m.refreshViewport()

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit
		case "ctrl+l":
			if !m.busy {
				m.deps.Agent.Reset()
				m.lines = []chatLine{{role: "system", text: "history cleared"}}
				m.refreshViewport()
			}
		case "enter":
			if m.busy {
				return m, nil
			}
			text := strings.TrimSpace(m.ta.Value())
			if text == "" {
				return m, nil
			}
			m.ta.Reset()
			return m.handleSubmit(text)
		}

	case streamEvMsg:
		ev := agent.Event(msg)
		switch ev.Kind {
		case agent.EventToken:
			m.stream.WriteString(ev.Text)
			m.upsertStreamingAssistant(m.stream.String())
			m.refreshViewport()
		case agent.EventToolStart:
			m.flushStreamAsLine()
			m.lines = append(m.lines, chatLine{
				role: "tool",
				text: fmt.Sprintf("→ %s(%s)", ev.Tool, truncate(ev.Text, 120)),
			})
			m.status = "tool: " + ev.Tool
			m.refreshViewport()
		case agent.EventToolEnd:
			m.lines = append(m.lines, chatLine{
				role: "tool",
				text: fmt.Sprintf("← %s\n%s", ev.Tool, truncate(ev.ToolOut, 900)),
			})
			m.refreshViewport()
		case agent.EventStatus:
			if ev.Text != "" {
				m.status = ev.Text
			}
			m.refreshViewport()
		case agent.EventError:
			m.flushStreamAsLine()
			m.lines = append(m.lines, chatLine{role: "error", text: ev.Text})
			m.status = "error"
			m.refreshViewport()
		case agent.EventDone:
			m.flushStreamAsLine()
			m.busy = false
			m.status = "ready"
			m.cancel = nil
			m.refreshViewport()
		}
		// Keep reading the channel
		if m.events != nil {
			cmds = append(cmds, waitNext(m.events))
		}

	case streamClosedMsg:
		m.flushStreamAsLine()
		m.busy = false
		m.status = "ready"
		m.events = nil
		m.cancel = nil
		m.refreshViewport()
	}

	if !m.busy {
		var cmd tea.Cmd
		m.ta, cmd = m.ta.Update(msg)
		cmds = append(cmds, cmd)
	}
	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

func (m model) handleSubmit(text string) (tea.Model, tea.Cmd) {
	if strings.HasPrefix(text, "/") {
		var cmd tea.Cmd
		m, cmd = m.handleSlash(text)
		return m, cmd
	}
	m.lines = append(m.lines, chatLine{role: "user", text: text})
	m.stream.Reset()
	m.busy = true
	m.status = "thinking…"
	m.refreshViewport()

	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	ch := make(chan agent.Event, 128)
	m.events = ch
	ag := m.deps.Agent

	go func() {
		_ = ag.RunUserMessage(ctx, text, func(ev agent.Event) {
			select {
			case ch <- ev:
			case <-ctx.Done():
			}
		})
		close(ch)
	}()

	return m, waitNext(ch)
}

func waitNext(ch <-chan agent.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return streamClosedMsg{}
		}
		return streamEvMsg(ev)
	}
}

func (m model) handleSlash(text string) (model, tea.Cmd) {
	parts := strings.Fields(text)
	cmd := strings.ToLower(parts[0])
	switch cmd {
	case "/quit", "/exit", "/q":
		return m, tea.Quit
	case "/help", "/h":
		m.lines = append(m.lines, chatLine{role: "system", text: helpText()})
	case "/clear":
		m.deps.Agent.Reset()
		m.lines = []chatLine{{role: "system", text: "history cleared"}}
	case "/status":
		m.lines = append(m.lines, chatLine{role: "system", text: m.deps.Summary + "\nstatus: " + m.status})
	case "/model", "/models":
		return m.handleModelCmd(parts)
	case "/tools":
		if len(parts) < 2 {
			state := "on"
			if !m.deps.Agent.Cfg.EnableTools {
				state = "off"
			}
			m.lines = append(m.lines, chatLine{role: "system", text: "tools: " + state + "\nusage: /tools on | /tools off"})
		} else {
			switch strings.ToLower(parts[1]) {
			case "on", "true", "1", "enable":
				m.deps.Agent.Cfg.EnableTools = true
				m.lines = append(m.lines, chatLine{role: "system", text: "tools enabled (used only when the task needs them)"})
			case "off", "false", "0", "disable":
				m.deps.Agent.Cfg.EnableTools = false
				m.lines = append(m.lines, chatLine{role: "system", text: "tools disabled for this session (faster chat)"})
			default:
				m.lines = append(m.lines, chatLine{role: "error", text: "usage: /tools on | /tools off"})
			}
		}
	default:
		m.lines = append(m.lines, chatLine{role: "error", text: "unknown command " + cmd + " — try /help"})
	}
	m.refreshViewport()
	return m, nil
}

func helpText() string {
	return strings.TrimSpace(`
Commands:
  /help              This help
  /clear             Clear conversation
  /status            Provider · model · URL
  /model             List models from the server (current marked *)
  /model <name>      Switch model for this chat (immediate, like Grok)
  /models            Same as /model
  /tools on|off      Toggle function tools (session; off = faster chat)
  /quit              Hint to exit (Ctrl+C)

Examples:
  /model
  /model qwen2.5-coder:32b
  /model qwen3.6-plus:latest
  /tools off

Keys:
  Enter           Send
  Ctrl+C          Quit
  Ctrl+L          Clear history
  PgUp / PgDn     Scroll

Ollama:
  Local default   http://127.0.0.1:11434/v1 (SSH tunnel to remote is fine)
  Edit            ~/.agenterm/config.toml
  Fast chat       agenterm --no-tools   or   enable_tools = false
  Remote example  providers.ollama-remote.base_url = "http://gpu-host:11434/v1"
`)
}

// handleModelCmd implements Grok-style mid-chat model switch and listing.
func (m model) handleModelCmd(parts []string) (model, tea.Cmd) {
	cur := m.deps.Agent.Cfg.Model
	// /model  or  /model list  → show available models from the endpoint
	if len(parts) < 2 || strings.EqualFold(parts[1], "list") || strings.EqualFold(parts[1], "ls") {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		ids, err := m.deps.Agent.Client.ListModels(ctx)
		if err != nil {
			m.lines = append(m.lines, chatLine{
				role: "error",
				text: fmt.Sprintf("could not list models: %v\ncurrent: %s\nusage: /model <name>", err, cur),
			})
			m.refreshViewport()
			return m, nil
		}
		if len(ids) == 0 {
			m.lines = append(m.lines, chatLine{
				role: "system",
				text: "no models reported by server\ncurrent: " + cur + "\nusage: /model <name>",
			})
			m.refreshViewport()
			return m, nil
		}
		var b strings.Builder
		b.WriteString("Models at ")
		b.WriteString(m.deps.Agent.Cfg.BaseURL)
		b.WriteString("\n")
		b.WriteString("current: ")
		b.WriteString(cur)
		b.WriteString("\n\n")
		for _, id := range ids {
			mark := "  "
			if id == cur {
				mark = "* "
			}
			b.WriteString(mark)
			b.WriteString(id)
			b.WriteByte('\n')
		}
		b.WriteString("\nSwitch: /model <name>")
		m.lines = append(m.lines, chatLine{role: "system", text: strings.TrimRight(b.String(), "\n")})
		m.refreshViewport()
		return m, nil
	}

	// /model <name> — tags may include ":" (e.g. qwen2.5-coder:32b); join rest.
	name := strings.TrimSpace(strings.Join(parts[1:], " "))
	if name == "" {
		m.lines = append(m.lines, chatLine{role: "error", text: "usage: /model <name>"})
		m.refreshViewport()
		return m, nil
	}
	prev := cur
	m.deps.Agent.Cfg.Model = name
	m.deps.Summary = fmt.Sprintf("%s · %s · %s", m.deps.Agent.Cfg.Provider, m.deps.Agent.Cfg.Model, m.deps.Agent.Cfg.BaseURL)
	m.status = "model: " + name
	msg := fmt.Sprintf("model set to %s", name)
	if prev != "" && prev != name {
		msg = fmt.Sprintf("model: %s → %s  (applies to next message)", prev, name)
	}
	m.lines = append(m.lines, chatLine{role: "system", text: msg})
	m.refreshViewport()
	return m, nil
}

func (m *model) upsertStreamingAssistant(content string) {
	if len(m.lines) > 0 && m.lines[len(m.lines)-1].role == "assistant-stream" {
		m.lines[len(m.lines)-1].text = content
		return
	}
	m.lines = append(m.lines, chatLine{role: "assistant-stream", text: content})
}

func (m *model) flushStreamAsLine() {
	if m.stream.Len() == 0 {
		if len(m.lines) > 0 && m.lines[len(m.lines)-1].role == "assistant-stream" {
			m.lines[len(m.lines)-1].role = "assistant"
		}
		return
	}
	content := m.stream.String()
	m.stream.Reset()
	if len(m.lines) > 0 && m.lines[len(m.lines)-1].role == "assistant-stream" {
		m.lines[len(m.lines)-1] = chatLine{role: "assistant", text: content}
		return
	}
	m.lines = append(m.lines, chatLine{role: "assistant", text: content})
}

func (m *model) refreshViewport() {
	var b strings.Builder
	width := m.vp.Width
	if width < 20 {
		width = 80
	}
	for _, ln := range m.lines {
		switch ln.role {
		case "user":
			b.WriteString(styleUser.Render("You") + "\n")
			b.WriteString(wrap(ln.text, width) + "\n\n")
		case "assistant", "assistant-stream":
			label := "Agent"
			if ln.role == "assistant-stream" {
				label = "Agent …"
			}
			b.WriteString(styleAsst.Render(label) + "\n")
			rendered := ln.text
			if m.renderer != nil && ln.role == "assistant" && len(ln.text) > 0 {
				if out, err := m.renderer.Render(ln.text); err == nil {
					rendered = strings.TrimRight(out, "\n")
				}
			}
			b.WriteString(rendered + "\n\n")
		case "tool":
			b.WriteString(styleTool.Render("Tool") + "\n")
			b.WriteString(styleTool.Render(wrap(ln.text, width)) + "\n\n")
		case "error":
			b.WriteString(styleErr.Render("Error") + "\n")
			b.WriteString(styleErr.Render(wrap(ln.text, width)) + "\n\n")
		default:
			b.WriteString(styleStatus.Render(wrap(ln.text, width)) + "\n\n")
		}
	}
	m.vp.SetContent(b.String())
	m.vp.GotoBottom()
}

func (m model) View() string {
	w := m.width
	if w == 0 {
		w = 80
	}
	header := styleHeader.Render(" agenterm ") + styleStatus.Render(m.deps.Summary)
	if m.busy {
		header += styleStatus.Render("  ·  " + m.status + "  " + spinnerFrame())
	} else {
		header += styleStatus.Render("  ·  " + m.status)
	}
	help := styleHelp.Render("enter send · ctrl+c quit · /help · ctrl+l clear")
	body := styleBox.Width(max(10, w-2)).Render(m.vp.View())
	input := styleBox.Width(max(10, w-2)).Render(m.ta.View())
	return lipgloss.JoinVertical(lipgloss.Left, header, body, input, help)
}

func wrap(s string, width int) string {
	if width < 20 {
		return s
	}
	var b strings.Builder
	for _, line := range strings.Split(s, "\n") {
		for len(line) > width {
			b.WriteString(line[:width])
			b.WriteByte('\n')
			line = line[width:]
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func spinnerFrame() string {
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	return frames[int(time.Now().UnixNano()/1e8)%len(frames)]
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Run launches the full-screen TUI.
func Run(deps Deps) error {
	p := tea.NewProgram(New(deps), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
