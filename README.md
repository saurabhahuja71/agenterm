# agenterm

**Snappy terminal AI agent** in Go — Grok-style TUI, works with **local or remote Ollama**, xAI, OpenAI, or any **OpenAI-compatible** API. Optional **MCP** tools.

```
You (TUI)
   │
   ▼
agenterm  ──►  Ollama / xAI / OpenAI  (/v1/chat/completions)
   │
   └── tools ──► built-in (files…) + MCP (e.g. mcp-demo)
```

## Features

- Full-screen **terminal UI** (Bubble Tea): streaming replies, tool traces, dark theme
- **Ollama-first** defaults (`http://127.0.0.1:11434/v1`)
- Config + flags for **local or remote** Ollama / cloud providers
- **Tool calling** loop (when the model supports tools)
- Optional **MCP** servers (HTTP streamable or stdio)
- Single static **Go binary** — easy for GitHub users

## Quick start

### 1. Ollama (Linux)

```bash
# install ollama, then:
ollama pull llama3.2
ollama serve   # if not already running
```

### 2. Build agenterm

```bash
git clone git@github.com:saurabhahuja71/agenterm.git
cd agenterm
go build -o agenterm ./cmd/agenterm
./agenterm init          # writes ~/.agenterm/config.toml
./agenterm --ping        # check endpoint
./agenterm               # open TUI
```

### 3. Chat

Type a message, press **Enter**.  
`/help` · `/model llama3.2` · `/clear` · **Ctrl+C** quit.

---

## Point at local or remote Ollama

Edit `~/.agenterm/config.toml`:

```toml
provider = "ollama-local"
model = "llama3.2"
base_url = "http://127.0.0.1:11434/v1"
api_key = "ollama"
```

**Remote Ollama** (another machine on your LAN):

```toml
provider = "ollama-remote"
model = "llama3.2"

[providers.ollama-remote]
base_url = "http://192.168.1.50:11434/v1"
api_key = "ollama"
model = "qwen2.5"
```

Or one-shot flags:

```bash
# local
./agenterm --base-url http://127.0.0.1:11434/v1 -m llama3.2

# remote
./agenterm --base-url http://gpu-box:11434/v1 -m qwen2.5

# xAI
export XAI_API_KEY=xai-...
./agenterm --provider xai -m grok-3

# OpenAI
export OPENAI_API_KEY=sk-...
./agenterm --provider openai -m gpt-4o-mini
```

Environment overrides:

| Env | Meaning |
|-----|---------|
| `AGENTERM_BASE_URL` | API root |
| `AGENTERM_MODEL` | Model id |
| `AGENTERM_PROVIDER` | Preset name |
| `AGENTERM_API_KEY` / `OLLAMA_API_KEY` / `XAI_API_KEY` / `OPENAI_API_KEY` | Auth |
| `AGENTERM_CONFIG` | Config path |

---

## MCP tools

Enable in config (e.g. your [mcp-demo](https://github.com/saurabhahuja71/mcp-demo) stack):

```toml
[[mcp_servers]]
name = "mcp-demo"
enabled = true
url = "http://127.0.0.1:8080/mcp"
```

Then ask: *“create a todo buy milk”* — the model can call `mcp-demo__create_todo` if tool-calling is supported.

Disable MCP for a session: `./agenterm --no-mcp`

---

## Built-in tools

| Tool | Notes |
|------|--------|
| `list_dir` | List directory |
| `read_file` | Read file |
| `write_file` | Write file |
| `run_shell` | Off by default; enable `enable_shell = true` or `--shell` |

---

## CLI

```bash
agenterm                  # TUI
agenterm init             # default config
agenterm --ping           # connectivity check
agenterm -m qwen2.5
agenterm --provider ollama-remote
agenterm --base-url http://host:11434/v1
agenterm --shell          # allow run_shell
agenterm --no-mcp
```

---

## Project layout

```
cmd/agenterm/          CLI entry
internal/config/       TOML + env + provider presets
internal/llm/          OpenAI-compatible client (stream + tools)
internal/agent/        Multi-turn tool loop
internal/tools/        Built-in tools
internal/mcp/          MCP client
internal/tui/          Bubble Tea UI
configs/config.example.toml
```

---

## Notes

- Ollama must expose the **OpenAI-compatible** API (`/v1/chat/completions`). Current Ollama does this by default.
- **Tool calling** quality depends on the model (`qwen2.5`, `llama3.1+`, etc. work better than tiny models).
- This is **not** affiliated with xAI/Grok; “Grok-style” means terminal agent UX only.

## License

MIT (or your choice when you publish).
