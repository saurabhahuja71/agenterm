# How agenterm works

## Architecture

```
┌─────────────────────────────────────────────┐
│  TUI (Bubble Tea)                           │
│  scrollback · input · /commands             │
└─────────────────┬───────────────────────────┘
                  │ user message
                  ▼
┌─────────────────────────────────────────────┐
│  Agent loop                                 │
│  history → LLM → tokens / tool_calls        │
│            ↑_______________|                │
│            tool results                     │
└───────┬─────────────────────┬───────────────┘
        │                     │
        ▼                     ▼
  Built-in tools         MCP client
  list/read/write        mcp-demo, …
        │
        ▼
  OpenAI-compatible API
  Ollama local/remote · xAI · OpenAI · …
```

## Why OpenAI-compatible?

Almost every local stack (Ollama, vLLM, LocalAI) and many clouds speak:

`POST {base_url}/chat/completions`

So one client supports:

| Target | Example base_url |
|--------|------------------|
| Local Ollama | `http://127.0.0.1:11434/v1` |
| Remote Ollama | `http://192.168.1.50:11434/v1` |
| xAI | `https://api.x.ai/v1` |
| OpenAI | `https://api.openai.com/v1` |

## Config resolution order

1. Defaults  
2. `~/.agenterm/config.toml`  
3. Provider preset (`providers.<name>`)  
4. Environment variables  
5. CLI flags  

## Chat vs MCP

- **Chat replies** come from the LLM (streamed into the TUI).  
- **MCP** only provides **tools** the model may call.  
- agenterm is the **MCP client**; mcp-demo is an **MCP server**.
