package tui

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/saurabhahuja71/agenterm/internal/agent"
)

// OSC / color-query replies that leak into stdin when libraries probe the TTY
// (classic first-launch garbage: "]11;rgb:fafa/fafa/fdfd\").
var reOSCLeak = regexp.MustCompile(
	`(?:\x1b)?\]\d+;[^\x07\x1b\\]*(?:\x07|\x1b\\)?` +
		`|\]\d+;rgb:[0-9a-fA-F/\\]+` +
		`|rgb:[0-9a-fA-F]{2,4}/[0-9a-fA-F]{2,4}/[0-9a-fA-F]{2,4}\\?`,
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
	deps       Deps
	vp         viewport.Model
	ta         textarea.Model
	lines      []chatLine
	width      int
	height     int
	busy      bool
	status    string
	// stream accumulates assistant tokens. Must be a pointer: Bubble Tea
	// copies model by value; a non-empty strings.Builder must not be copied.
	stream    *strings.Builder
	renderer  *glamour.TermRenderer
	cancel    context.CancelFunc
	events    <-chan agent.Event
	busySince time.Time
	gotToken  bool
	waitSecs  int
	// verbose shows full tool I/O and model preambles; default is quiet/compact.
	verbose bool
	// scrubLeft: remaining startup OSC scrub passes (color-query junk).
	scrubLeft int
}

type streamEvMsg agent.Event
type streamClosedMsg struct{}
type busyTickMsg time.Time

func New(deps Deps) model {
	ta := textarea.New()
	ta.Placeholder = "Message…  /help  /model  /quiet  /verbose"
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

	// Fixed dark style — WithAutoStyle() queries OSC 11 (bg color) and the
	// reply often appears as garbage in the input line on first launch.
	r := newGlamourRenderer(80)

	m := model{
		deps:     deps,
		vp:       vp,
		ta:       ta,
		status:   "ready",
		stream:   &strings.Builder{},
		renderer: r,
		verbose:   false, // compact tools by default
		scrubLeft: 5,     // a few startup passes to catch late OSC replies
		lines: []chatLine{
			{role: "system", text: "agenterm — snappy terminal agent (Ollama / OpenAI-compatible + MCP)"},
			{role: "system", text: "Endpoint: " + deps.Summary},
			{role: "system", text: "Quiet mode on · /verbose for full tool output · /help"},
		},
	}
	m.refreshViewport()
	return m
}

func (m *model) ensureStream() *strings.Builder {
	if m.stream == nil {
		m.stream = &strings.Builder{}
	}
	return m.stream
}

func (m model) Init() tea.Cmd {
	// Scrub any OSC color-query junk already sitting in the input buffer.
	return tea.Batch(textarea.Blink, scrubInputCmd())
}

type scrubInputMsg struct{}

func scrubInputCmd() tea.Cmd {
	return func() tea.Msg { return scrubInputMsg{} }
}

func newGlamourRenderer(width int) *glamour.TermRenderer {
	if width < 20 {
		width = 80
	}
	// Prefer explicit dark theme over AutoStyle (no TTY color probes).
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		r, _ = glamour.NewTermRenderer(glamour.WithWordWrap(width))
	}
	return r
}

// scrubTerminalGarbage removes leaked OSC / rgb color-query fragments.
func scrubTerminalGarbage(s string) string {
	if s == "" {
		return s
	}
	out := reOSCLeak.ReplaceAllString(s, "")
	// Common partials if ESC was consumed already
	out = strings.ReplaceAll(out, "]11;", "")
	out = strings.ReplaceAll(out, "]10;", "")
	// ST is often a bare trailing backslash after rgb:... was stripped
	out = strings.Trim(out, " \t\r\n\\")
	// If almost nothing left but punctuation from probes, drop it
	if out != "" && !hasLetterOrDigit(out) && (strings.Contains(s, "rgb:") || strings.Contains(s, "]11") || strings.Contains(s, "]10")) {
		return ""
	}
	return strings.TrimSpace(out)
}

func hasLetterOrDigit(s string) bool {
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return true
		}
	}
	return false
}

func busyTick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return busyTickMsg(t)
	})
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case scrubInputMsg:
		if v := m.ta.Value(); v != "" {
			if cleaned := scrubTerminalGarbage(v); cleaned != v {
				m.ta.SetValue(cleaned)
				m.ta.CursorEnd()
			}
		}
		// Limited startup retries — OSC replies can arrive a few frames late.
		if m.scrubLeft > 0 {
			m.scrubLeft--
			return m, tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg {
				return scrubInputMsg{}
			})
		}

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
		m.renderer = newGlamourRenderer(max(40, msg.Width-8))
		m.refreshViewport()
		// Resize often coincides with first paint; scrub leaked OSC once more.
		if v := m.ta.Value(); v != "" {
			if cleaned := scrubTerminalGarbage(v); cleaned != v {
				m.ta.SetValue(cleaned)
				m.ta.CursorEnd()
			}
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			// Cancel in-flight generation without quitting the TUI.
			if m.busy && m.cancel != nil {
				m.cancel()
				m.status = "cancelling… (Esc)"
				m.refreshViewport()
				return m, nil
			}
		case "ctrl+c":
			if m.busy && m.cancel != nil {
				// First Ctrl+C cancels generation; second (when idle) quits.
				m.cancel()
				m.status = "cancelling… (Ctrl+C again to quit)"
				m.refreshViewport()
				return m, nil
			}
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

	case busyTickMsg:
		if m.busy && !m.gotToken {
			m.waitSecs = int(time.Since(m.busySince).Seconds())
			m.status = fmt.Sprintf("waiting for %s… %ds (Ollama may be loading model; Esc cancel)", m.deps.Agent.Cfg.Model, m.waitSecs)
			m.refreshViewport()
			return m, busyTick()
		}

	case streamEvMsg:
		ev := agent.Event(msg)
		switch ev.Kind {
		case agent.EventToken:
			m.gotToken = true
			// Quiet mode: buffer tokens but don't paint pure tool-JSON noise live.
			m.ensureStream().WriteString(ev.Text)
			cur := m.stream.String()
			if m.verbose || !isMostlyToolNoise(cur) {
				m.upsertStreamingAssistant(cur)
			} else {
				// Keep status only; drop any partial tool-JSON bubble.
				m.dropStreamingAssistant()
			}
			m.status = "streaming…"
			m.refreshViewport()
		case agent.EventToolStart:
			m.gotToken = true
			if m.verbose {
				m.flushStreamAsLine()
			} else {
				// Hide "Let's read…" preambles and raw {"name":"read_file"...} dumps.
				m.dropStreamIfNoise()
			}
			m.lines = append(m.lines, chatLine{
				role: "tool",
				text: formatToolStart(ev.Tool, ev.Text, m.verbose),
			})
			m.status = "tool: " + ev.Tool
			m.refreshViewport()
		case agent.EventToolEnd:
			m.lines = append(m.lines, chatLine{
				role: "tool",
				text: formatToolEnd(ev.Tool, ev.ToolOut, m.verbose),
			})
			m.refreshViewport()
		case agent.EventStatus:
			// Status bar only — do not spam the chat transcript.
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
			// Final answer: show stream unless it's leftover tool JSON.
			if m.verbose {
				m.flushStreamAsLine()
			} else {
				m.flushStreamAsLineQuiet()
			}
			m.busy = false
			m.gotToken = false
			m.waitSecs = 0
			m.status = "ready"
			m.cancel = nil
			m.refreshViewport()
		}
		// Keep reading the channel
		if m.events != nil {
			cmds = append(cmds, waitNext(m.events))
		}

	case streamClosedMsg:
		if m.verbose {
			m.flushStreamAsLine()
		} else {
			m.flushStreamAsLineQuiet()
		}
		m.busy = false
		m.gotToken = false
		m.waitSecs = 0
		m.status = "ready"
		m.events = nil
		m.cancel = nil
		m.refreshViewport()
	}

	if !m.busy {
		var cmd tea.Cmd
		m.ta, cmd = m.ta.Update(msg)
		cmds = append(cmds, cmd)
		// Drop OSC junk if it landed as "typed" characters.
		if v := m.ta.Value(); v != "" && (strings.Contains(v, "]11;") || strings.Contains(v, "rgb:") || strings.Contains(v, "\x1b]")) {
			if cleaned := scrubTerminalGarbage(v); cleaned != v {
				m.ta.SetValue(cleaned)
				m.ta.CursorEnd()
			}
		}
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
	m.ensureStream().Reset()
	m.busy = true
	m.gotToken = false
	m.waitSecs = 0
	m.busySince = time.Now()
	m.status = fmt.Sprintf("thinking… (%s)", m.deps.Agent.Cfg.Model)
	m.refreshViewport()

	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	ch := make(chan agent.Event, 256)
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

	return m, tea.Batch(waitNext(ch), busyTick())
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
	case "/quit", "/exit":
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
	case "/verbose", "/v":
		m.verbose = true
		m.lines = append(m.lines, chatLine{role: "system", text: "verbose on — full tool args/output and model preambles"})
	case "/quiet":
		m.verbose = false
		m.lines = append(m.lines, chatLine{role: "system", text: "quiet mode — compact tools (default). /verbose for full traces"})
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
  /quiet             Compact UI (default): short tool lines, hide tool JSON dumps
  /verbose           Full tool args/output and model “let me read…” text
  /quit              Exit (or Ctrl+C when idle)

Examples:
  /model qwen2.5-coder:32b
  /tools off
  /quiet

Keys:
  Enter           Send
  Esc             Cancel in-flight reply (does not quit)
  Ctrl+C          Cancel if busy; quit when idle
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
		msg = fmt.Sprintf(
			"model: %s → %s\n"+
				"Next message uses the new model.\n"+
				"Note: Ollama often unloads the old model and loads the new one on first request —\n"+
				"32B models can sit on “thinking…” for 30s–several minutes with no tokens yet.\n"+
				"Status bar shows a wait timer; Esc cancels. Tip: /clear if chat history is huge.",
			prev, name,
		)
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
	sb := m.ensureStream()
	if sb.Len() == 0 {
		if len(m.lines) > 0 && m.lines[len(m.lines)-1].role == "assistant-stream" {
			m.lines[len(m.lines)-1].role = "assistant"
		}
		return
	}
	content := sb.String()
	sb.Reset()
	if len(m.lines) > 0 && m.lines[len(m.lines)-1].role == "assistant-stream" {
		m.lines[len(m.lines)-1] = chatLine{role: "assistant", text: content}
		return
	}
	m.lines = append(m.lines, chatLine{role: "assistant", text: content})
}

// flushStreamAsLineQuiet drops tool-JSON / empty stream; keeps real answers.
func (m *model) flushStreamAsLineQuiet() {
	sb := m.ensureStream()
	content := strings.TrimSpace(sb.String())
	sb.Reset()
	m.dropStreamingAssistant()
	if content == "" || isMostlyToolNoise(content) {
		return
	}
	m.lines = append(m.lines, chatLine{role: "assistant", text: content})
}

func (m *model) dropStreamingAssistant() {
	if len(m.lines) > 0 && m.lines[len(m.lines)-1].role == "assistant-stream" {
		m.lines = m.lines[:len(m.lines)-1]
	}
}

func (m *model) dropStreamIfNoise() {
	sb := m.ensureStream()
	content := strings.TrimSpace(sb.String())
	sb.Reset()
	m.dropStreamingAssistant()
	// Keep non-noise preamble only in verbose (caller already branched).
	_ = content
}

// isMostlyToolNoise detects model dumps of tool-call JSON and short “let me tool” chatter.
func isMostlyToolNoise(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return true
	}
	// Raw / fenced tool JSON
	if strings.Contains(s, `"name"`) && (strings.Contains(s, `"arguments"`) || strings.Contains(s, `"parameters"`)) {
		// If almost the whole message is JSON-ish, hide it.
		withoutSpace := strings.Map(func(r rune) rune {
			if r == ' ' || r == '\n' || r == '\t' {
				return -1
			}
			return r
		}, s)
		if strings.HasPrefix(withoutSpace, "{") || strings.Contains(s, "```") {
			return true
		}
		// JSON object is majority of text
		if i := strings.Index(s, "{"); i >= 0 {
			jsonPart := s[i:]
			if len(jsonPart) > len(s)*2/3 {
				return true
			}
		}
	}
	low := strings.ToLower(s)
	if len(s) < 400 {
		for _, p := range []string{
			"let's read", "lets read", "i'll read", "i will read",
			"let me read", "reading the", "i'll use", "i will use",
			"using the read_file", "call read_file", "invoke",
		} {
			if strings.Contains(low, p) {
				return true
			}
		}
	}
	return false
}

func formatToolStart(name, args string, verbose bool) string {
	if verbose {
		return fmt.Sprintf("→ %s(%s)", name, truncate(args, 200))
	}
	path := toolArgHint(args)
	if path != "" {
		return fmt.Sprintf("→ %s · %s", name, truncate(path, 72))
	}
	return fmt.Sprintf("→ %s", name)
}

func formatToolEnd(name, out string, verbose bool) string {
	if strings.HasPrefix(out, "error:") {
		return fmt.Sprintf("← %s · %s", name, truncate(out, 160))
	}
	if verbose {
		return fmt.Sprintf("← %s\n%s", name, truncate(out, 900))
	}
	n := len(out)
	unit := "B"
	sz := float64(n)
	if n >= 1024 {
		sz = float64(n) / 1024
		unit = "KB"
	}
	// One-line confirmation; model will summarize in the final answer.
	return fmt.Sprintf("← %s · ok (%.1f %s)", name, sz, unit)
}

func toolArgHint(argsJSON string) string {
	argsJSON = strings.TrimSpace(argsJSON)
	if argsJSON == "" {
		return ""
	}
	// Prefer "path" then "name" then "command"
	for _, key := range []string{"path", "name", "root", "command"} {
		// cheap extract: "path": "..."
		needle := `"` + key + `"`
		i := strings.Index(argsJSON, needle)
		if i < 0 {
			continue
		}
		rest := argsJSON[i+len(needle):]
		// skip : and spaces
		rest = strings.TrimLeft(rest, " \t\n:")
		if len(rest) == 0 || rest[0] != '"' {
			continue
		}
		rest = rest[1:]
		j := strings.Index(rest, `"`)
		if j < 0 {
			continue
		}
		return rest[:j]
	}
	return truncate(argsJSON, 48)
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
	mode := "quiet"
	if m.verbose {
		mode = "verbose"
	}
	help := styleHelp.Render("enter send · esc cancel · /" + mode + " · /help · /model · ctrl+l clear")
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
	// Discourage libraries from probing terminal fg/bg via OSC (leaks into stdin).
	// Fixed styles above are the main fix; these env hints help termenv/glamour.
	if os.Getenv("GLAMOUR_STYLE") == "" {
		_ = os.Setenv("GLAMOUR_STYLE", "dark")
	}
	p := tea.NewProgram(New(deps), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
