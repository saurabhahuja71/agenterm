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
