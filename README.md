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

## Two ways to run (pick one)

| Mode | Best when | Limitations |
|------|-----------|-------------|
| **Native binary** | Daily use, full host access, no container networking quirks | Need a matching OS/arch binary (or Go toolchain) |
| **Podman / Docker** | “Just run an image”, locked-down hosts, CI | Needs `-it`; host Ollama needs `--network=host` (Linux) or a remote URL |

Both share the same config (`~/.agenterm/config.toml`) and CLI flags.

### A) Install native binary (recommended)

**From GitHub Releases** (after [v0.1.0](https://github.com/saurabhahuja71/agenterm/releases)):

```bash
# auto-detect OS/arch → ~/.local/bin/agenterm
curl -fsSL https://raw.githubusercontent.com/saurabhahuja71/agenterm/main/scripts/install.sh | bash

agenterm --version
agenterm init
agenterm --ping
agenterm
```

Manual download:

```bash
# example: Linux amd64
curl -fsSL -o agenterm \
  https://github.com/saurabhahuja71/agenterm/releases/latest/download/agenterm-linux-amd64
chmod +x agenterm && ./agenterm
```

**From source:**

```bash
git clone https://github.com/saurabhahuja71/agenterm.git
cd agenterm
make build          # → ./agenterm
# or: go install github.com/saurabhahuja71/agenterm/cmd/agenterm@latest
./agenterm
```

### B) Container (Podman / Docker)

Run the **TUI inside a container** so users only need Podman or Docker + a terminal. Ollama can stay on the host (or any remote URL).

**Published image** (after release):

```bash
podman run --rm -it --network=host \
  -e AGENTERM_BASE_URL=http://127.0.0.1:11434/v1 \
  -v "$HOME/.agenterm:/home/agenterm/.agenterm:Z" \
  ghcr.io/saurabhahuja71/agenterm:v0.1.0
```

## Features

- Full-screen **terminal UI** (Bubble Tea): streaming replies, tool traces, dark theme
- **Ollama-first** defaults (`http://127.0.0.1:11434/v1`)
- Config + flags for **local or remote** Ollama / cloud providers
- **Tool calling** loop (when the model supports tools)
- Optional **MCP** servers (HTTP streamable or stdio)
- **Native binaries** + **container image** for every release

## Quick start (Podman / Docker from source)

Yes: build the image locally if you have not pulled from GHCR yet.

### 1. Ollama on the host (or remote)

```bash
ollama pull llama3.2
ollama serve   # listens on :11434
```

### 2. Run agenterm in a container

```bash
git clone git@github.com:saurabhahuja71/agenterm.git
cd agenterm

# one-liner helper (builds image if needed, -it TUI, host network)
chmod +x scripts/run-podman.sh
./scripts/run-podman.sh --ping
./scripts/run-podman.sh
```

**Manual Podman** (Linux — host Ollama on localhost):

```bash
podman build -t agenterm:latest .

# Ensure user socket if you use compose elsewhere:
# systemctl --user enable --now podman.socket

podman run --rm -it --network=host \
  -e AGENTERM_BASE_URL=http://127.0.0.1:11434/v1 \
  -e AGENTERM_MODEL=llama3.2 \
  -v "$HOME/.agenterm:/home/agenterm/.agenterm:Z" \
  agenterm:latest
```

**Compose:**

```bash
export DOCKER_HOST=unix:///run/user/$(id -u)/podman/podman.sock   # rootless Podman
podman compose run --rm agenterm
# or: docker compose run --rm agenterm
```

| Flag / need | Why |
|-------------|-----|
| `-it` / `tty: true` | Interactive **TUI** needs a real terminal |
| `--network=host` (Linux) | Container can reach **host** Ollama at `127.0.0.1:11434` |
| volume `~/.agenterm` | Persist config |

**Remote Ollama** (no host network required for the LLM):

```bash
./scripts/run-podman.sh --base-url http://192.168.1.50:11434/v1 -m qwen2.5
# or
AGENTERM_BASE_URL=http://gpu-box:11434/v1 ./scripts/run-podman.sh
```

**macOS Docker:** host Ollama is not `127.0.0.1` inside the container — use  
`http://host.docker.internal:11434/v1` (the run script sets this on Darwin).

### 3. Chat

Type a message, press **Enter**.  
`/help` · `/model llama3.2` · `/clear` · **Ctrl+C** quit.

---

## Quick start (native Go binary)

```bash
git clone git@github.com:saurabhahuja71/agenterm.git
cd agenterm
go build -o agenterm ./cmd/agenterm
./agenterm init
./agenterm --ping
./agenterm
```

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
| `AGENTERM_ENABLE_TOOLS` | `0`/`false` to disable tools; `1`/`true` to enable |
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
agenterm init             # default config (use --force to overwrite)
agenterm --ping           # connectivity check
agenterm -m qwen2.5
agenterm --provider ollama-remote
agenterm --base-url http://host:11434/v1
agenterm --no-tools       # pure chat (no function tools; faster replies)
agenterm --shell          # allow run_shell
agenterm --no-mcp
```

**Snappy chat over Ollama:** greetings never attach tools (no pointless `list_dir` on “hi”).
Use `/tools off` in the TUI or `enable_tools = false` / `AGENTERM_ENABLE_TOOLS=0` for fully tool-free sessions.

**Switch model mid-chat (Grok-style):**

```text
/model                         # list models from the server (* = current)
/model qwen2.5-coder:32b       # switch for the next messages
/models                        # alias for /model
```

Start with a model: `agenterm -m qwen3.6-plus:latest`

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

## Tech stack (for contributors & curious users)

This section explains **what agenterm is written in** and how the pieces fit together.

### Core language

| Piece | Tech |
|--------|------|
| **Language** | **Go** (Golang) |
| **Module** | `github.com/saurabhahuja71/agenterm` |
| **Entry point** | `cmd/agenterm/main.go` → single binary `agenterm` |
| **Go version** | 1.25.x in CI / Docker builds |

agenterm is a **compiled CLI**. There is **no Python/Node runtime** required to run the released binary. Users download a release asset, `go install`, or run a container.

### Major libraries

| Layer | Library | Role |
|--------|---------|------|
| **CLI flags / subcommands** | [Cobra](https://github.com/spf13/cobra) | `agenterm`, `init`, `--model`, `--ping`, … |
| **Config** | [BurntSushi/toml](https://github.com/BurntSushi/toml) | `~/.agenterm/config.toml` |
| **Terminal UI** | [Bubble Tea](https://github.com/charmbracelet/bubbletea) + [Bubbles](https://github.com/charmbracelet/bubbles) + [Lip Gloss](https://github.com/charmbracelet/lipgloss) | Full-screen TUI (scrollback, input, status) |
| **Markdown rendering** | [Glamour](https://github.com/charmbracelet/glamour) | Pretty-print assistant replies |
| **LLM HTTP client** | Custom (`internal/llm`) | OpenAI-compatible `POST /v1/chat/completions` with SSE streaming |
| **MCP** | [official Go MCP SDK](https://github.com/modelcontextprotocol/go-sdk) | Connect to MCP servers (HTTP streamable or stdio) |
| **Built-in tools** | Go standard library (`os`, `os/exec`, …) | `list_dir`, `read_file`, `write_file`, optional `run_shell` |

### Architecture

```
┌─────────────────────────────────────┐
│  TUI (Bubble Tea / Lip Gloss)       │  ← terminal UI
└─────────────────┬───────────────────┘
                  │
┌─────────────────▼───────────────────┐
│  Agent loop (internal/agent)        │  ← chat history + tool rounds
└─────────────┬───────────┬───────────┘
              │           │
              ▼           ▼
     LLM client      Tools registry
  (OpenAI-compat)    + MCP client
              │
              ▼
   Ollama / xAI / OpenAI / any /v1 API
```

| Package | Responsibility |
|---------|----------------|
| `internal/tui` | Screen, input, streaming tokens, `/help` |
| `internal/agent` | User message → model → tools → model again |
| `internal/llm` | HTTP to chat APIs (stream + `tool_calls`) |
| `internal/config` | Providers, base URL, model, MCP list |
| `internal/tools` | Built-in tools |
| `internal/mcp` | External MCP servers exposed as tools |

**Important:** chat **replies** come from the **LLM**. MCP only provides **tools** the model may call. agenterm is an **MCP client**; projects like [mcp-demo](https://github.com/saurabhahuja71/mcp-demo) are **MCP servers**.

### External systems (not embedded in the binary)

| System | How agenterm talks to it |
|--------|---------------------------|
| **Ollama** (local or remote) | OpenAI-compatible HTTP, e.g. `http://127.0.0.1:11434/v1` |
| **xAI / OpenAI / vLLM / …** | Same `/v1/chat/completions` style API |
| **MCP servers** | MCP over **stdio** or **streamable HTTP** |

Models are **not** shipped inside the binary. The binary is only: **UI + agent loop + HTTP client + tools/MCP**.

### Packaging & DevOps

| Concern | Tech |
|---------|------|
| **Native install** | Static Go binary (linux / darwin / windows × amd64 / arm64) |
| **Container** | Multi-stage **Dockerfile** (`golang` builder → `alpine`) |
| **Compose** | `docker-compose.yml` (Podman-compatible) |
| **CI/CD** | **GitHub Actions** (`.github/workflows/release.yml`) |
| **Artifacts** | GitHub **Releases** + **GHCR** (`ghcr.io/saurabhahuja71/agenterm`) |
| **Build helpers** | `Makefile`, `scripts/install.sh`, `scripts/run-podman.sh` |

### How this compares to related demos

| Project | Stack |
|---------|--------|
| [reactapp](https://github.com/saurabhahuja71/reactapp) | React + FastAPI (Python) + Postgres |
| [react-java-todo](https://github.com/saurabhahuja71/react-java-todo) | React + Spring Boot (Java) + Postgres |
| [mcp-demo](https://github.com/saurabhahuja71/mcp-demo) | **Go MCP server** |
| **agenterm** (this repo) | **Go MCP client + TUI + LLM agent** |

### One-line summary

> **agenterm is a Go terminal application** that uses a **Bubble Tea TUI**, calls **OpenAI-compatible LLM APIs** (especially Ollama), runs an **agent tool loop**, and can load extra tools via **MCP**—distributed as **static binaries** and a **Docker/Podman image**.

See also [`docs/how-it-works.md`](docs/how-it-works.md) for request flow details.

---

## Notes

- Ollama must expose the **OpenAI-compatible** API (`/v1/chat/completions`). Current Ollama does this by default.
- **Tool calling** quality depends on the model (`qwen2.5`, `llama3.1+`, etc. work better than tiny models).
- This is **not** affiliated with xAI/Grok; “Grok-style” means terminal agent UX only.

## License

MIT (or your choice when you publish).

---

## Releases

GitHub Actions builds **multi-platform binaries** and a **GHCR container** on every version tag:

| Asset | Platforms |
|-------|-----------|
| `agenterm-linux-amd64` | Linux x86_64 |
| `agenterm-linux-arm64` | Linux aarch64 |
| `agenterm-darwin-amd64` | macOS Intel |
| `agenterm-darwin-arm64` | macOS Apple Silicon |
| `agenterm-windows-amd64.exe` | Windows |
| `ghcr.io/saurabhahuja71/agenterm:vX.Y.Z` | Container |

Maintainers:

```bash
# cut a release
git tag v0.1.0
git push origin v0.1.0
# Actions → Release workflow publishes assets

# local cross-build only
make dist VERSION=0.1.0
```

Published container:

```bash
podman pull ghcr.io/saurabhahuja71/agenterm:v0.1.0
podman run --rm -it --network=host \
  -v "$HOME/.agenterm:/home/agenterm/.agenterm:Z" \
  ghcr.io/saurabhahuja71/agenterm:v0.1.0
```
