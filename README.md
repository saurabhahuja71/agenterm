# agenterm — Terminal AI Agent for Ollama, OpenAI, xAI & MCP

**agenterm** is a free, open-source **terminal AI agent** and **CLI chat TUI** written in **Go**.  
Chat with **local or remote [Ollama](https://ollama.com)** models, **xAI Grok**, **OpenAI**, or any **OpenAI-compatible API** (vLLM, LocalAI, OpenRouter-compatible endpoints, and more) from your shell—with optional **file tools** and **[Model Context Protocol (MCP)](https://modelcontextprotocol.io)** servers.

> **Keywords & discoverability:** terminal AI assistant · Ollama terminal client · CLI LLM agent · Grok-style TUI · local LLM chat · remote Ollama over SSH tunnel · OpenAI-compatible `/v1/chat/completions` · Bubble Tea terminal UI · MCP client in Go · developer coding agent in the terminal

[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go)](https://go.dev/)
[![Ollama](https://img.shields.io/badge/Ollama-compatible-black)](https://ollama.com)
[![MCP](https://img.shields.io/badge/MCP-client-purple)](https://modelcontextprotocol.io)
[![Release](https://img.shields.io/github/v/release/saurabhahuja71/agenterm?include_prereleases)](https://github.com/saurabhahuja71/agenterm/releases)
[![Install](https://img.shields.io/badge/install-one--line-brightgreen)](#install-agenterm-native-binary-recommended)

| | |
|---|---|
| **Homepage / source** | [github.com/saurabhahuja71/agenterm](https://github.com/saurabhahuja71/agenterm) |
| **What it is** | Snappy **terminal AI agent** (not a web UI) |
| **Best for** | Developers who want **Ollama in the terminal**, coding help, and tool use without leaving the shell |
| **Runs on** | Linux, macOS, Windows (static binary) · Docker / Podman |
| **Talks to** | Ollama · xAI · OpenAI · any `/v1` chat API |

```
You (terminal TUI)
   │
   ▼
agenterm  ──►  Ollama / xAI / OpenAI  (POST /v1/chat/completions)
   │
   └── tools ──► built-in (files, optional shell) + MCP servers
```

---

## Table of contents

- [Why agenterm?](#why-agenterm)
- [Features](#features-terminal-ai-agent--ollama-tui)
- [Install agenterm](#install-agenterm-native-binary-recommended)
- [Quick start with Ollama](#quick-start-chat-with-ollama-in-the-terminal)
- [Local vs remote Ollama](#local-vs-remote-ollama-and-ssh-tunnels)
- [Docker / Podman](#run-agenterm-in-docker-or-podman)
- [CLI reference](#cli-reference-and-in-chat-commands)
- [Configuration](#configuration-env-and-configtoml)
- [Built-in tools & MCP](#built-in-tools-and-mcp-integration)
- [FAQ](#faq-terminal-ai-ollama-and-agenterm)
- [Architecture & tech stack](#architecture-and-tech-stack)
- [Roadmap vs Grok UI](#roadmap-vs-grok-ui)
- [Releases](#releases-and-downloads)
- [License](#license)

---

## Why agenterm?

Many people search for a **terminal AI chatbot**, an **Ollama CLI with a real TUI**, or a **Grok-like agent in the terminal** without sending all code to a browser.

**agenterm** fills that gap:

| Need | How agenterm helps |
|------|---------------------|
| Chat with **local LLMs** | Default endpoint is Ollama at `http://127.0.0.1:11434/v1` |
| Use a **GPU box over the network** | Point `--base-url` at remote Ollama or an SSH tunnel on localhost |
| **Coding agent** in the shell | Tool loop: list/read/write files; optional shell; optional MCP |
| Switch models mid-session | `/model` and `/models` (Grok-style) while chatting |
| Fast small talk | Greetings skip tools so “hi” is one round-trip, not a `list_dir` storm |
| No heavy runtime | Single **static Go binary**—no Python/Node required to run releases |

**Not affiliated with xAI.** “Grok-style” describes the **terminal agent UX** only.

---

## Features: terminal AI agent & Ollama TUI

- Full-screen **terminal UI** (Bubble Tea): streaming replies, tool traces, dark theme  
- **Ollama-first** defaults with OpenAI-compatible HTTP  
- **Local and remote Ollama**, xAI, OpenAI, vLLM, LocalAI, and similar APIs  
- **Multi-turn agent loop** with function/tool calling when the model supports it  
- **Mid-chat model switch:** `/model`, `/models`  
- **Tools on/off:** `--no-tools`, `/tools off`, `AGENTERM_ENABLE_TOOLS`  
- Optional **MCP** (HTTP streamable or stdio) as extra tools  
- Install via **one-line script**, GitHub Releases, `go install`, or **container**  

---

## Install agenterm (native binary, recommended)

### One-line install (Linux / macOS)

```bash
curl -fsSL https://raw.githubusercontent.com/saurabhahuja71/agenterm/main/scripts/install.sh | bash
```

Installs to `~/.local/bin/agenterm` by default. Ensure that directory is on your `PATH`.

```bash
agenterm --version
agenterm init          # writes ~/.agenterm/config.toml
agenterm --ping
agenterm
```

Environment options for the installer:

| Variable | Meaning |
|----------|---------|
| `INSTALL_DIR` | Install path (default `~/.local/bin`) |
| `AGENTERM_VERSION` | Release tag or `latest` |
| `AGENTERM_FROM_SOURCE=1` | Build with Go if no release asset (or prefer source) |
| `AGENTERM_SKIP_INIT=1` | Do not create config |

### Manual download from GitHub Releases

```bash
# example: Linux amd64
curl -fsSL -o agenterm \
  https://github.com/saurabhahuja71/agenterm/releases/latest/download/agenterm-linux-amd64
chmod +x agenterm && ./agenterm
```

Also published: `linux-arm64`, `darwin-amd64`, `darwin-arm64`, `windows-amd64.exe`.

### Build from source (developers)

```bash
git clone https://github.com/saurabhahuja71/agenterm.git
cd agenterm
make build          # → ./agenterm
# or:
go install github.com/saurabhahuja71/agenterm/cmd/agenterm@latest
```

---

## Quick start: chat with Ollama in the terminal

### 1. Run Ollama (local or tunnel)

```bash
ollama pull llama3.2
ollama serve   # http://127.0.0.1:11434
```

If Ollama runs on another machine, forward it (SSH tunnel is enough):

```bash
ssh -N -L 11434:127.0.0.1:11434 user@gpu-host
# then agenterm still uses http://127.0.0.1:11434/v1
```

### 2. Start agenterm

```bash
agenterm --ping
agenterm -m llama3.2
# pure chat (fastest):
agenterm --no-tools -m llama3.2
```

### 3. In the TUI

| Action | How |
|--------|-----|
| Send message | **Enter** |
| Help | `/help` |
| List models on server | `/model` |
| Switch model (like Grok) | `/model qwen2.5-coder:32b` |
| Disable tools this session | `/tools off` |
| Clear history | `/clear` or **Ctrl+L** |
| Quit | **Ctrl+C** |

---

## Local vs remote Ollama (and SSH tunnels)

### Config file (`~/.agenterm/config.toml`)

```toml
provider = "ollama-local"
model = "llama3.2"
base_url = "http://127.0.0.1:11434/v1"
api_key = "ollama"
```

**Remote Ollama** on the LAN:

```toml
provider = "ollama-remote"

[providers.ollama-remote]
base_url = "http://192.168.1.50:11434/v1"
api_key = "ollama"
model = "qwen2.5"
```

### One-shot CLI

```bash
# local or SSH tunnel
agenterm --base-url http://127.0.0.1:11434/v1 -m llama3.2

# remote host
agenterm --base-url http://gpu-box:11434/v1 -m qwen2.5

# xAI Grok API
export XAI_API_KEY=xai-...
agenterm --provider xai -m grok-3

# OpenAI
export OPENAI_API_KEY=sk-...
agenterm --provider openai -m gpt-4o-mini
```

Always include **`/v1`** on Ollama base URLs so OpenAI-compatible routes work (`/v1/chat/completions`, `/v1/models`).

---

## Run agenterm in Docker or Podman

Use a container when you want a locked-down runtime; keep Ollama on the host or remote.

```bash
podman run --rm -it --network=host \
  -e AGENTERM_BASE_URL=http://127.0.0.1:11434/v1 \
  -v "$HOME/.agenterm:/home/agenterm/.agenterm:Z" \
  ghcr.io/saurabhahuja71/agenterm:latest
```

From this repo:

```bash
git clone https://github.com/saurabhahuja71/agenterm.git
cd agenterm
./scripts/run-podman.sh --ping
./scripts/run-podman.sh
```

| Need | Why |
|------|-----|
| `-it` / TTY | Interactive **TUI** |
| `--network=host` (Linux) | Reach host Ollama at `127.0.0.1:11434` |
| Volume `~/.agenterm` | Persist config |

**Remote Ollama from a container:**

```bash
AGENTERM_BASE_URL=http://gpu-box:11434/v1 ./scripts/run-podman.sh
```

**macOS Docker Desktop:** use `http://host.docker.internal:11434/v1` for host Ollama.

**Compose:**

```bash
export DOCKER_HOST=unix:///run/user/$(id -u)/podman/podman.sock   # rootless Podman
podman compose run --rm agenterm
# or: docker compose run --rm agenterm
```

| Mode | Best when | Limitations |
|------|-----------|-------------|
| **Native binary** | Daily use, full host access | Need OS/arch binary or Go |
| **Podman / Docker** | CI, locked-down hosts | TTY + networking setup |

---

## CLI reference and in-chat commands

### Flags

```bash
agenterm                  # open TUI
agenterm init             # default config (use --force to overwrite)
agenterm --ping           # connectivity check
agenterm -m qwen2.5       # start with a model
agenterm --provider ollama-remote
agenterm --base-url http://host:11434/v1
agenterm --no-tools       # pure chat (no function tools; faster replies)
agenterm --shell          # allow run_shell
agenterm --no-mcp
```

### In-chat commands (session)

```text
/help
/status
/model                         # list models from the server (* = current)
/model qwen2.5-coder:32b       # switch model for next messages
/models                        # alias for /model
/tools on | /tools off
/clear
/quit
```

**Snappy Ollama chat:** greetings do not attach tools (avoids pointless `list_dir` on “hi”).  
For fully tool-free sessions: `/tools off`, `enable_tools = false`, or `AGENTERM_ENABLE_TOOLS=0`.

---

## Configuration, env, and config.toml

| Env | Meaning |
|-----|---------|
| `AGENTERM_BASE_URL` | API root (e.g. `http://127.0.0.1:11434/v1`) |
| `AGENTERM_MODEL` | Model id / Ollama tag |
| `AGENTERM_PROVIDER` | Preset: `ollama-local`, `ollama-remote`, `xai`, `openai`, `custom` |
| `AGENTERM_ENABLE_TOOLS` | `0`/`false` off · `1`/`true` on |
| `AGENTERM_API_KEY` / `OLLAMA_API_KEY` / `XAI_API_KEY` / `OPENAI_API_KEY` | Auth |
| `AGENTERM_CONFIG` | Path to config file |

Example file: [`configs/config.example.toml`](configs/config.example.toml).

---

## Built-in tools and MCP integration

### Built-in tools

| Tool | Notes |
|------|--------|
| `list_dir` | List directory (path relative to process cwd) |
| `read_file` | Read file; resolves common bad paths (`repo/…`, `dbope`→`dboper`) |
| `write_file` | Write file |
| `find_files` | Locate `README.md` / project folders when the full path is unknown |
| `run_shell` | Off by default; `enable_shell = true` or `--shell` |

**Tip:** start agenterm from the workspace root you care about (e.g. `cd …/dboper && agenterm`). Paths and tools use that directory.

### MCP tools

agenterm is an **MCP client**. Enable servers in config (example: [mcp-demo](https://github.com/saurabhahuja71/mcp-demo)):

```toml
[[mcp_servers]]
name = "mcp-demo"
enabled = true
url = "http://127.0.0.1:8080/mcp"
```

Disable MCP for a session: `agenterm --no-mcp`.

---

## FAQ: terminal AI, Ollama, and agenterm

### What is agenterm?

**agenterm** is an open-source **terminal AI agent** in Go: a full-screen TUI that streams chat from Ollama or any OpenAI-compatible API and can run tools (files, shell, MCP).

### Is agenterm an Ollama GUI or CLI?

It is a **terminal TUI** (text UI in the terminal), not a web or desktop GUI. It is a great **Ollama terminal client** when you want chat + agent tools without leaving the shell.

### Can I use remote Ollama or an SSH tunnel?

Yes. Set `base_url` / `--base-url` to the remote host, or tunnel remote Ollama to `127.0.0.1:11434` and use the default local URL.

### Does it work with xAI Grok and OpenAI?

Yes. Use `--provider xai` or `--provider openai` with the matching API key environment variables, or any custom `--base-url` that speaks OpenAI chat completions.

### How do I change the model during a chat?

Type `/model` to list models, then `/model <name>` (for example `/model qwen2.5-coder:32b`). Same idea as switching models mid-conversation in other agent UIs.

### Why was “hi” slow before?

Some models eagerly called tools (e.g. `list_dir`) on greetings, causing extra LLM round-trips over the network. agenterm now skips tools for trivial chat and supports `--no-tools` / `/tools off`.

### Is Python required?

**No** for release binaries. agenterm is a **static Go binary**. Python/Node are not needed to run it.

### Is this related to xAI Grok?

No affiliation. The product is independent; “Grok-style” only means a snappy terminal agent experience.

### Where is the config stored?

Default: `~/.agenterm/config.toml`. Override with `AGENTERM_CONFIG` or `--config`.

---

## Architecture and tech stack

### Project layout

```
cmd/agenterm/          CLI entry
internal/config/       TOML + env + provider presets
internal/llm/          OpenAI-compatible client (stream + tools)
internal/agent/        Multi-turn tool loop
internal/tools/        Built-in tools
internal/mcp/          MCP client
internal/tui/          Bubble Tea UI
configs/config.example.toml
scripts/install.sh     End-user installer
```

### Core stack

| Piece | Tech |
|--------|------|
| Language | **Go** (Golang) |
| Module | `github.com/saurabhahuja71/agenterm` |
| CLI | [Cobra](https://github.com/spf13/cobra) |
| Config | [BurntSushi/toml](https://github.com/BurntSushi/toml) |
| TUI | [Bubble Tea](https://github.com/charmbracelet/bubbletea) · Bubbles · Lip Gloss |
| Markdown | [Glamour](https://github.com/charmbracelet/glamour) |
| LLM HTTP | Custom `internal/llm` → `POST /v1/chat/completions` (SSE) |
| MCP | [official Go MCP SDK](https://github.com/modelcontextprotocol/go-sdk) |

```
┌─────────────────────────────────────┐
│  TUI (Bubble Tea / Lip Gloss)       │
└─────────────────┬───────────────────┘
                  │
┌─────────────────▼───────────────────┐
│  Agent loop (internal/agent)        │
└─────────────┬───────────┬───────────┘
              │           │
              ▼           ▼
     LLM client      Tools + MCP
              │
              ▼
   Ollama / xAI / OpenAI / any /v1 API
```

Chat **replies** come from the LLM. MCP only supplies **tools**. Models are **not** embedded in the binary.

### Related projects

| Project | Stack |
|---------|--------|
| [reactapp](https://github.com/saurabhahuja71/reactapp) | React + FastAPI + Postgres |
| [react-java-todo](https://github.com/saurabhahuja71/react-java-todo) | React + Spring Boot + Postgres |
| [mcp-demo](https://github.com/saurabhahuja71/mcp-demo) | Go **MCP server** |
| **agenterm** | Go **MCP client + TUI + LLM agent** |

More detail: [`docs/how-it-works.md`](docs/how-it-works.md).

### Roadmap vs Grok UI

Notes on what agenterm has, what Grok-class UIs have, and phased work (resume later):

→ **[`docs/grok-parity-roadmap.md`](docs/grok-parity-roadmap.md)**

### One-line summary (for crawlers & humans)

> **agenterm** is a **Go terminal AI agent** with a **Bubble Tea TUI**, **Ollama / OpenAI-compatible** chat, a **tool-calling agent loop**, and optional **MCP**—shipped as **static binaries** and a **Docker/Podman** image.

---

## Notes

- Ollama must expose the **OpenAI-compatible** API (`/v1/chat/completions`). Current Ollama does this by default.
- Tool-calling quality depends on the model (e.g. `qwen2.5`, `llama3.1+` work better than tiny models).
- Not affiliated with xAI, OpenAI, or Ollama.

---

## Releases and downloads

GitHub Actions builds **multi-platform binaries** and a **GHCR** image on version tags:

| Asset | Platforms |
|-------|-----------|
| `agenterm-linux-amd64` | Linux x86_64 |
| `agenterm-linux-arm64` | Linux aarch64 |
| `agenterm-darwin-amd64` | macOS Intel |
| `agenterm-darwin-arm64` | macOS Apple Silicon |
| `agenterm-windows-amd64.exe` | Windows |
| `ghcr.io/saurabhahuja71/agenterm:vX.Y.Z` | Container |

```bash
# cut a release
git tag v0.1.1
git push origin v0.1.1

# local cross-build
make dist VERSION=0.1.1
```

```bash
podman pull ghcr.io/saurabhahuja71/agenterm:latest
podman run --rm -it --network=host \
  -v "$HOME/.agenterm:/home/agenterm/.agenterm:Z" \
  ghcr.io/saurabhahuja71/agenterm:latest
```

---

## License

MIT — free to use, modify, and distribute. See repository license file when present.

---

## Topics (GitHub discovery)

Suggested repository topics for search and SEO:

`ai` · `terminal` · `cli` · `tui` · `ollama` · `llm` · `openai` · `xai` · `grok` · `mcp` · `agent` · `golang` · `bubbletea` · `coding-assistant` · `local-llm` · `chatbot`

If you maintain this repo on GitHub: **Settings → General → Topics**, or the gear next to About on the repo home page.
