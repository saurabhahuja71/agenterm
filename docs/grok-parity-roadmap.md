# agenterm vs Grok UI — gap analysis & roadmap

**Status:** working notes for continued work (resume Monday)  
**Last updated:** 2026-07-25  
**Related:** [README](../README.md), [how-it-works](how-it-works.md)

---

## Why agenterm does not feel as good as Grok

**Grok (xAI app/web)** and **agenterm** are different products:

| | **Grok UI** | **agenterm today** |
|--|-------------|---------------------|
| Model | Grok models, trained + productized for chat/tools | Whatever Ollama tag you pick (quality varies a lot) |
| Backend | xAI infra, low latency, tools done right | Local/remote Ollama (often via SSH tunnel) |
| Agent brain | Large system: planning, retrieval, safety, memory | Thin multi-turn tool loop (`internal/agent`) |
| UI | Polished product UI, streaming, citations, modes | Minimal Bubble Tea chat |
| Context | Managed context, search, multimodal if enabled | Process cwd + a few file tools |

**Not the language.** Almost none of the pain is “Go instead of Rust.” The gap is **model quality + product depth**. agenterm is a thin client; Grok is a full product on a strong model.

### Issues we hit that are *not* Go-vs-Rust

| Issue | Real cause |
|-------|------------|
| “hi” slow / eager `list_dir` | Tools always attached + model eagerness |
| Tool call as plain JSON text | Ollama/Qwen not always using real `tool_calls` |
| Fake file listings (`config.go`, `db.go`…) | Model **hallucination** |
| Chat noise | UX + chatty models |
| Hang after `/model` | Ollama **loading** a large model |
| `strings.Builder` panic | Bubble Tea model copied by value (fixed in v0.1.4) |

Rewriting in Rust would not make models invent fewer paths or load 32B weights faster.

---

## What agenterm already has (MVP ~v0.1.x)

- Stream chat to OpenAI-compatible APIs (Ollama `/v1`, xAI, OpenAI, …)
- Tools: `list_dir`, `read_file`, `write_file`, `find_files`, optional `run_shell`
- Optional MCP client
- `/model`, `/models`, `/tools`, `/quiet`, `/verbose`
- Greetings skip tools; text tool-call recovery for Ollama
- Quiet TUI (compact tool lines); post-tool brief-answer hints
- Config + `scripts/install.sh` + smoke scripts
- Panic fix: stream buffer as `*strings.Builder`

**Positioning today:** open TUI + any OpenAI-compatible backend + thin tools — **not** a Grok clone.

```text
Grok UI     = world-class model + product + infra
Cursor/CC   = coding agent + IDE context + strong models
agenterm    = lightweight terminal agent shell
```

---

## What’s missing to feel closer to Grok / Cursor / Claude Code

### 1. Model & API quality (biggest feel gap)

| Missing | Why it matters |
|---------|----------------|
| First-class **xAI Grok** path (good defaults, streaming quirks) | Grok UI *is* Grok; agenterm is “any model” |
| Reliable **tool_choice** (`auto` / `none` / required) | Stops greetings + random tools |
| Parallel tools, better multi-step plans | Strong agents do multi-tool in one turn |
| Provider-specific adapters (Ollama vs OpenAI vs xAI) | One generic client is fragile on Ollama/Qwen |

### 2. Real coding agent loop

| Missing | Why |
|---------|-----|
| **Repo map** / project index (symbols, structure) | Knows tree without endless listing |
| **Semantic / grep search** (`grep`, `rg`, codebase search) | Find code without full-file reads |
| **Edit tools** (patch / apply_diff, not only whole-file write) | Surgical changes |
| **Plan → act → verify** | Explicit steps + checks |
| **Tests / build hooks** | Run tests after edit |
| **Git awareness** (diff, status, branch) | Safe changes |
| **Checkpoints / undo** | Revert bad edits |
| **Subagents / task split** | Explore vs implement |

### 3. Context management

| Missing | Why |
|---------|-----|
| Token budget + **auto-summarize** long chats | Less dump/ramble |
| **@file / @folder** attach | User points context (Cursor/Grok pattern) |
| Open buffers / recent files | IDE-like awareness |
| Ignore rules (`.gitignore`, secrets) | Don’t suck in junk |
| Image / PDF / multimodal | Grok has this; agenterm is text-only |

### 4. UI / UX product

| Missing | Why Grok feels better |
|---------|------------------------|
| Clean **message cards** (user / assistant / tools collapsible) | Less wall of text |
| Reliable markdown + copy code blocks | Product polish |
| **Thinking** collapsed by default | R1-style without noise |
| **Stop / regenerate / edit & resend** | Expected in modern chat UIs |
| **History** sessions (sidebar, resume) | Not only `/clear` in one session |
| **Permissions** UI (approve shell / write) | Trust |
| Multi-line editor (Shift+Enter), paste large files | Daily use |
| Themes, density, keyboard-first nav | “App” feel |
| Citations / sources when browsing | Grok web feature |

### 5. Reliability & agent discipline

| Missing | Why |
|---------|-----|
| Verifier: ground answers in tool output only | Stops fake listings |
| Structured final answer for simple Qs | “Yes/No + one line” |
| Retries / fallback model | When Ollama fails tool format |
| Offline eval / golden tasks | Catch regressions |
| Stronger Ollama tool recovery + schema prompting | Qwen-specific |

### 6. Platform features Grok has as a service

| Missing | Notes |
|---------|--------|
| Web search / X search | Grok differentiator |
| Image gen / vision | Not in agenterm |
| Accounts, sync, mobile | Product, not CLI |
| Hosted low-latency infra | vs SSH tunnel + 32B load |

You will not match **hosted Grok** by only polishing the TUI.

---

## Roadmap (resume Monday)

### Phase A — Feel snappy & trustworthy (high ROI)

1. **@path** mentions + better repo root detection  
2. **`grep` / `search_code` tool**  
3. **tool_choice** + stronger Ollama tool prompts  
4. Collapsible tools always; emphasize final answer  
5. **Regenerate / stop / edit last**  
6. Session **history** on disk  

### Phase B — Real coding agent

7. Diff-based **edit_file**  
8. **run_tests** / configurable check command  
9. Git status + proposed diff preview  
10. Permission prompts for write/shell  
11. Context compaction  

### Phase C — Product polish

12. Multipane TUI (chat | files | diff)  
13. Project rules file (`AGENTS.md` / `.agenterm/rules`)  
14. First-class **xAI** preset with good defaults  
15. Optional local embeddings for codebase Q&A  

### Phase D — Optional “Grok-like” extras

16. Web search via MCP  
17. Vision  
18. Voice (usually out of scope for terminal)  

---

## Honest goals

| Goal | Realistic? |
|------|------------|
| **Best lightweight Ollama/xAI terminal agent** | Yes — focus Phases A–B |
| **Clone of Grok UI** | No — needs xAI model + product + infra |

**Prefer for tool work:** models like `qwen3-coder:*` over chatty “plus” general models when using Ollama.

---

## Recent fixes already on `main` (context for Monday)

| Commit / theme | What |
|----------------|------|
| Tool eagerness / greetings | Skip tools on trivial chat; better system prompt |
| Text tool-call recovery | Run tools when models print JSON instead of `tool_calls` |
| `/model` wait UX | Timer + Esc cancel during Ollama load |
| Builder panic | `*strings.Builder` for Bubble Tea |
| Quiet TUI | Compact tools; `/verbose` for full dumps |
| Post-tool brief answers | Cap tool payload, cooler/shorter synthesis, less invented listings |
| Smoke scripts | `scripts/smoke_agent.go`, `scripts/smoke_sidb.go` |

---

## Monday resume checklist

- [ ] Re-read this doc + skim `internal/agent`, `internal/tui`, `internal/tools`
- [ ] Pick **Phase A** items (suggest: `@file` + `grep` + session history)
- [ ] Decide default model story: Ollama coder vs xAI Grok API
- [ ] Optional: tag release `v0.1.6+` so install script gets binaries without source build
- [ ] Manual: from `dboper/` cwd, “can you read sidb/oracle-database-operator/README.md? yes or no”

---

## One-line summary

> **agenterm is a thin Go TUI agent over OpenAI-compatible APIs; Grok feels better because of the model and a deep product stack—not because agenterm used Go instead of Rust. Close the gap with search/edit/context/sessions and stronger tool discipline, not a rewrite.**
