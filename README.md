# agenterm — Terminal AI Agent Tutorial (Ollama, OpenAI, xAI + MCP Client)

**Terminal AI agent** and **coding assistant in the terminal** for [Ollama](https://ollama.com), [xAI](https://x.ai), [OpenAI](https://openai.com), and any **OpenAI-compatible** API. Full-screen **TUI**, optional **file/shell tools**, and **[Model Context Protocol (MCP)](https://modelcontextprotocol.io)** client support—one static **Go** binary.

> **Lab 1** in the [AI · Agents · MCP learning path](https://github.com/saurabhahuja71/learning-path#7-ai--agents--mcp) · Audience: intermediate · Time: ~1 hour to first chat · Level: intermediate

**SEO keywords:** *terminal AI agent*, *Ollama TUI*, *OpenAI compatible CLI agent*, *MCP client Go*, *local LLM coding agent*, *agenterm tutorial*, *AI pair programmer terminal*.

[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go)](https://go.dev/)
[![Ollama](https://img.shields.io/badge/Ollama-compatible-black)](https://ollama.com)
[![MCP](https://img.shields.io/badge/MCP-client-purple)](https://modelcontextprotocol.io)
[![Release](https://img.shields.io/github/v/release/saurabhahuja71/agenterm?include_prereleases)](https://github.com/saurabhahuja71/agenterm/releases)

| | |
|---|---|
| **Best for** | Developers who want Ollama (or any `/v1` chat API) in the terminal with optional coding tools |
| **Runs on** | Linux, macOS, Windows · Docker / Podman |
| **Not** | A web UI or desktop app — pure terminal |

```
You (TUI)
   │
   ▼
agenterm  ──►  Ollama / xAI / OpenAI  (POST /v1/chat/completions)
   │
   └── tools ──► built-in (files, optional shell) + MCP servers
```

---

## Get started in 60 seconds

**1. Install** (Linux / macOS):

```bash
curl -fsSL https://raw.githubusercontent.com/saurabhahuja71/agenterm/main/scripts/install.sh | bash
```

Binary goes to `~/.local/bin/agenterm`. If the shell cannot find it:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

**2. Start Ollama** (local or already tunneled to your machine):

```bash
ollama pull qwen2.5-coder:32b
ollama serve   # http://127.0.0.1:11434
```

**3. Chat:**

```bash
agenterm init          # first time: writes ~/.agenterm/config.toml
agenterm --ping        # check the API
agenterm               # open the TUI
```

In the TUI: type a message and press **Enter**. Try `/help`, `/model`, or `/tools off` for pure chat.

---

## Table of contents

- [Why agenterm?](#why-agenterm)
- [Install](#install)
- [Quick start](#quick-start)
- [Local vs remote Ollama](#local-vs-remote-ollama)
- [Docker / Podman](#docker--podman)
- [CLI and in-chat commands](#cli-and-in-chat-commands)
- [Configuration](#configuration)
- [Tools and MCP](#tools-and-mcp)
- [FAQ](#faq)
- [Architecture](#architecture)
- [Releases](#releases)
- [License](#license)

---

## Why agenterm?

| You want… | agenterm does… |
|-----------|----------------|
| Chat with **local LLMs** | Defaults to Ollama at `http://127.0.0.1:11434/v1` |
| A **GPU box over the network** | `--base-url` or an SSH tunnel to localhost |
| A **coding agent** in the shell | Tool loop: list/read/write files; optional shell; MCP |
| Switch models mid-session | `/model` while chatting |
| Fast small talk | Greetings skip tools; use `--no-tools` for pure chat |
| No Python/Node runtime | One **static Go binary** |

Not affiliated with xAI. “Grok-style” only means a snappy terminal agent UX.

---

## Install

### One-line (recommended)

```bash
curl -fsSL https://raw.githubusercontent.com/saurabhahuja71/agenterm/main/scripts/install.sh | bash
agenterm --version
```

| Variable | Meaning |
|----------|---------|
| `INSTALL_DIR` | Install path (default `~/.local/bin`) |
| `AGENTERM_VERSION` | Release tag or `latest` |
| `AGENTERM_FROM_SOURCE=1` | Build with Go instead of downloading a release |
| `AGENTERM_SKIP_INIT=1` | Do not create config |

### GitHub Releases

```bash
curl -fsSL -o agenterm \
  https://github.com/saurabhahuja71/agenterm/releases/latest/download/agenterm-linux-amd64
chmod +x agenterm && ./agenterm
```

Also published: `linux-arm64`, `darwin-amd64`, `darwin-arm64`, `windows-amd64.exe`.

### From source

```bash
git clone https://github.com/saurabhahuja71/agenterm.git
cd agenterm
make build          # → ./agenterm
make install        # → $(go env GOPATH)/bin (or $GOBIN)
# or:
go install github.com/saurabhahuja71/agenterm/cmd/agenterm@latest
```

---

## Quick start

### Ollama on this machine

```bash
ollama pull qwen2.5-coder:32b
agenterm --ping
agenterm -m qwen2.5-coder:32b
# pure chat (no function tools):
agenterm --no-tools -m qwen2.5-coder:32b
```

### Ollama on another host (SSH tunnel)

```bash
ssh -N -L 11434:127.0.0.1:11434 user@gpu-host
# agenterm still uses http://127.0.0.1:11434/v1
agenterm
```

### In the TUI

| Action | How |
|--------|-----|
| Send message | **Enter** |
| Help | `/help` |
| List / switch model | `/model` · `/model qwen2.5-coder:32b` |
| Disable tools this session | `/tools off` |
| Clear history | `/clear` or **Ctrl+L** |
| Quit | **Ctrl+C** |

Run agenterm from the project directory you care about (`cd myrepo && agenterm`) so file tools use that workspace.

---

## Local vs remote Ollama

### Config (`~/.agenterm/config.toml`)

```toml
provider = "ollama-local"
model = "qwen2.5-coder:32b"
base_url = "http://127.0.0.1:11434/v1"
api_key = "ollama"
```

Remote host on the LAN:

```toml
provider = "ollama-remote"

[providers.ollama-remote]
base_url = "http://192.168.1.50:11434/v1"
api_key = "ollama"
model = "qwen2.5"
```

### One-shot CLI

```bash
agenterm --base-url http://127.0.0.1:11434/v1 -m qwen2.5-coder:32b
agenterm --base-url http://gpu-box:11434/v1 -m qwen2.5

export XAI_API_KEY=xai-...
agenterm --provider xai -m grok-3

export OPENAI_API_KEY=sk-...
agenterm --provider openai -m gpt-4o-mini
```

Always include **`/v1`** on Ollama base URLs (`/v1/chat/completions`, `/v1/models`).

Full example: [`configs/config.example.toml`](configs/config.example.toml).

---

## Docker / Podman

Prefer the **native binary** for daily use. Use a container when you want a locked-down runtime.

```bash
podman run --rm -it --network=host \
  -e AGENTERM_BASE_URL=http://127.0.0.1:11434/v1 \
  -v "$HOME/.agenterm:/home/agenterm/.agenterm:Z" \
  ghcr.io/saurabhahuja71/agenterm:latest
```

From this repo:

```bash
./scripts/run-podman.sh --ping
./scripts/run-podman.sh
```

| Need | Why |
|------|-----|
| `-it` / TTY | Interactive TUI |
| `--network=host` (Linux) | Reach host Ollama at `127.0.0.1:11434` |
| Volume `~/.agenterm` | Persist config |

On **macOS Docker Desktop**, use `http://host.docker.internal:11434/v1` for host Ollama.

```bash
export DOCKER_HOST=unix:///run/user/$(id -u)/podman/podman.sock   # rootless Podman
podman compose run --rm agenterm
```

---

## CLI and in-chat commands

### Flags

```bash
agenterm                  # open TUI
agenterm init             # default config (--force to overwrite)
agenterm --ping           # connectivity check
agenterm -m qwen2.5       # start with a model
agenterm --provider ollama-remote
agenterm --base-url http://host:11434/v1
agenterm --no-tools       # pure chat (faster)
agenterm --shell          # allow run_shell
agenterm --no-mcp
```

### In-chat

```text
/help
/status
/model                         # list models (* = current)
/model qwen2.5-coder:32b       # switch model
/models                        # alias for /model
/tools on | /tools off
/clear
/quit
```

Greetings skip tools automatically. For fully tool-free sessions: `/tools off`, `enable_tools = false`, or `AGENTERM_ENABLE_TOOLS=0`.

---

## Configuration

| Env | Meaning |
|-----|---------|
| `AGENTERM_BASE_URL` | API root (e.g. `http://127.0.0.1:11434/v1`) |
| `AGENTERM_MODEL` | Model id / Ollama tag |
| `AGENTERM_PROVIDER` | `ollama-local`, `ollama-remote`, `xai`, `openai`, `custom` |
| `AGENTERM_ENABLE_TOOLS` | `0`/`false` off · `1`/`true` on |
| `AGENTERM_API_KEY` / `OLLAMA_API_KEY` / `XAI_API_KEY` / `OPENAI_API_KEY` | Auth |
| `AGENTERM_CONFIG` | Path to config file |

Default file: `~/.agenterm/config.toml`.

---

## Tools and MCP

### Built-in tools

| Tool | Notes |
|------|--------|
| `list_dir` | List a directory (relative to process cwd) |
| `read_file` | Read a file |
| `write_file` | Write a full file |
| `str_replace` | Surgical edit (preferred for patches) |
| `find_files` | Locate files when the path is unknown |
| `git` | Allowlisted git (`status`, `checkout`, `add`, `commit`, …) |
| `run_shell` | Off by default; `--shell` or `enable_shell = true` |

When you ask to **apply / implement / do it**, agenterm uses tools—not only printed shell recipes.

### MCP

agenterm is an **MCP client**. Example:

```toml
[[mcp_servers]]
name = "mcp-demo"
enabled = true
url = "http://127.0.0.1:8080/mcp"
```

Disable for a session: `agenterm --no-mcp`. Demo server: [mcp-demo](https://github.com/saurabhahuja71/mcp-demo).

---

## FAQ

### What is agenterm?

An open-source **terminal AI agent** in Go: a full-screen TUI that streams chat from Ollama or any OpenAI-compatible API, with optional tools (files, shell, MCP).

### Is it an Ollama GUI?

No—it is a **terminal TUI**, not a web or desktop GUI. Use it when you want chat and agent tools without leaving the shell.

### Can I use remote Ollama or an SSH tunnel?

Yes. Point `base_url` / `--base-url` at the host, or tunnel remote Ollama to `127.0.0.1:11434` and keep the default URL.

### Does it work with xAI Grok and OpenAI?

Yes. `--provider xai` or `--provider openai` with the matching API key env vars, or any custom `--base-url` that speaks OpenAI chat completions.

### How do I change the model during a chat?

`/model` to list, then `/model <name>` (for example `/model qwen2.5-coder:32b`).

### Why was “hi” slow before?

Some models called tools (e.g. `list_dir`) on greetings. Trivial chat now skips tools; use `--no-tools` or `/tools off` for pure chat.

### Is Python required?

No for release binaries. agenterm is a **static Go binary**.

### Where is the config?

Default: `~/.agenterm/config.toml`. Override with `AGENTERM_CONFIG` or `--config`.

### Is this related to xAI Grok?

No affiliation. Independent project.

---

## Architecture

```
cmd/agenterm/          CLI entry
internal/config/       TOML + env + provider presets
internal/llm/          OpenAI-compatible client (stream + tools)
internal/agent/        Multi-turn tool loop
internal/tools/        Built-in tools
internal/mcp/          MCP client
internal/tui/          Bubble Tea UI
scripts/install.sh     End-user installer
```

| Piece | Tech |
|--------|------|
| Language | Go |
| CLI | [Cobra](https://github.com/spf13/cobra) |
| TUI | [Bubble Tea](https://github.com/charmbracelet/bubbletea) · Bubbles · Lip Gloss |
| Markdown | [Glamour](https://github.com/charmbracelet/glamour) |
| MCP | [official Go MCP SDK](https://github.com/modelcontextprotocol/go-sdk) |

Chat **replies** come from the LLM. MCP only supplies **tools**. Models are **not** embedded in the binary.

More detail: [`docs/how-it-works.md`](docs/how-it-works.md).  
Roadmap (Grok-class UX): [`docs/grok-parity-roadmap.md`](docs/grok-parity-roadmap.md).

**Notes:** Ollama must expose the OpenAI-compatible API (default). Tool quality depends on the model (`qwen2.5`, `llama3.1+` work better than tiny models).

---

## Releases

On version tags, GitHub Actions publishes multi-platform binaries and a GHCR image:

| Asset | Platforms |
|-------|-----------|
| `agenterm-linux-amd64` | Linux x86_64 |
| `agenterm-linux-arm64` | Linux aarch64 |
| `agenterm-darwin-amd64` | macOS Intel |
| `agenterm-darwin-arm64` | macOS Apple Silicon |
| `agenterm-windows-amd64.exe` | Windows |
| `ghcr.io/saurabhahuja71/agenterm:vX.Y.Z` | Container |

```bash
git tag v0.2.1 && git push origin v0.2.1
make dist VERSION=0.2.1   # local cross-build
```

---



## Learning path — AI / Agents / MCP

| # | Lab | Focus |
|---|-----|--------|
| 0 | [mcp-demo](https://github.com/saurabhahuja71/mcp-demo) | Build MCP server tools |
| **1 (this)** | agenterm | Terminal agent + MCP client |
| 2 | [agentic-ai-sample](https://github.com/saurabhahuja71/agentic-ai-sample) | LangChain ReAct research agent |
| 3 | [workshop-redis-ai](https://github.com/saurabhahuja71/workshop-redis-ai) | Redis vector search + RAG workshop |

Hub: [learning-path](https://github.com/saurabhahuja71/learning-path)

## Topics / SEO tags

`ai-agent` `ollama` `mcp` `golang` `tui` `openai` `xai` `terminal` `llm` `coding-agent` `tutorial` `education`


## License

MIT — free to use, modify, and distribute.

---

## GitHub topics (maintainers)

Suggested topics for the About sidebar:

`ai` · `terminal` · `cli` · `tui` · `ollama` · `llm` · `openai` · `mcp` · `agent` · `golang` · `bubbletea` · `coding-assistant` · `local-llm`
