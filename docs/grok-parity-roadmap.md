# agenterm vs Grok UI — gap analysis & roadmap

**Status:** working notes for continued work (resume Monday)  
**Last updated:** 2026-07-25  
**Related:** [README](../README.md), [how-it-works](how-it-works.md)

**Default Ollama model (Go + docs agent work):** `qwen2.5-coder:32b`  
(set as project default in config; see [Recommended LLM](#recommended-llm-for-golang--documentation) below)

---

## Recommended LLM for Golang & documentation

Inventory from the lab host (`ollama list`, 2026-07-25):

| Name | Size | Role fit for **agenterm** |
|------|------|---------------------------|
| **qwen2.5-coder:32b** | 19 GB | **Best default** — coding + tools + structured file work |
| **qwen3-coder:30b** / `qwen3-coder:latest` | 18 GB | Strong alternate (same image id); excelled in our smoke tests |
| qwen3.6-plus:latest | 19 GB | Chatty; weak tool discipline; invents file trees — **avoid as agent default** |
| qwen3.6:latest | 23 GB | General; heavier; not first choice for tools |
| deepseek-r1:32b | 19 GB | Strong reasoning; slow TTFT / “thinking”; poor fit for snappy docs Q&A |
| deepseek-r1:latest | 5.2 GB | Lighter R1; same reasoning style, less capacity |
| gpt-oss:20b | 13 GB | Smaller general model; OK fallback if 32B is too slow |

### Decision (use this Monday)

| Use case | Model | Why |
|----------|--------|-----|
| **Default for agenterm** (Go code + README/docs + tools) | **`qwen2.5-coder:32b`** | Built as a **coder** model: better function/tool use, path/file discipline, Go-oriented edits, less “essay” noise than general chat models |
| Alternate if 2.5 misbehaves on tools | `qwen3-coder:30b` | Passed full smoke (`hi`, `list_dir`, `read_file`, `find_files`, sidb README) cleanly |
| Pure long reasoning (rare) | `deepseek-r1:32b` | Only when you want deep chain-of-thought; expect load + wait |
| Casual chat only | `qwen3.6-plus:latest` | Not for repo agent tasks |

### How to run with the default

```bash
# project default after config update
agenterm

# explicit
agenterm -m qwen2.5-coder:32b

# in TUI
/model qwen2.5-coder:32b
```

Config (`~/.agenterm/config.toml`):

```toml
provider = "ollama-local"
model = "qwen2.5-coder:32b"
base_url = "http://127.0.0.1:11434/v1"
```

If an old config still has `llama3.2` or `qwen3.6-plus`, either edit `model =` or:

```bash
agenterm init --force   # rewrites defaults (review first)
# then set model again if needed
```

### Why not qwen3.6-plus for this project?

We observed with agenterm + Ollama:

- Tool calls often printed as **plain JSON** instead of API `tool_calls`
- **Invented** directory listings (e.g. fake `config.go` / `db.go`) instead of tool ground truth
- Long preambles (“Let’s read…”) — more UI noise

Coder-tagged Qwen models behave better for **Golang + documentation agent** workflows.

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

1. ~~**@path** mentions + better repo root detection~~ **done (v0.2.0)**  
2. ~~**`grep` / `search_code` tool**~~ **done**  
3. ~~**tool_choice** auto/none~~ **done (basic)**  
4. ~~Quiet/collapsible tools~~ **done earlier**  
5. ~~**Regenerate** (`/retry`) / stop (Esc)~~ **done**; full “edit last prompt” still open  
6. ~~Session **history** on disk~~ **done** (`/save`, `/load`, `/sessions`, auto `last`)  

### Phase B — Real coding agent

7. ~~Diff-based **str_replace**~~ **done earlier** (full unified-diff apply still open)  
8. ~~**run_tests** / configurable `test_command`~~ **done**  
9. ~~Git status in `/status` + `git` tool~~ **done**  
10. Permission prompts for write/shell — **partial** (shell still opt-in; no interactive approve UI yet)  
11. ~~Context compaction~~ **done** (`/compact` + auto soft compact)  

### Phase C — Product polish

12. Multipane TUI (chat | files | diff) — **todo**  
13. ~~Project rules file (`AGENTS.md` / `.agenterm/rules`)~~ **done**  
14. First-class **xAI** preset with good defaults — **partial** (preset exists)  
15. Optional local embeddings for codebase Q&A — **todo**  

### Phase D — Optional “Grok-like” extras

16. Web search via MCP — **todo**  
17. Vision — **todo**  
18. Voice — **todo**  

### v0.2.0 shipped

| Feature | How to use |
|---------|------------|
| `@file` / `@dir` | `explain @README.md` or `@internal/agent` |
| `grep` | model tool or ask “search for Foo in *.go” |
| `run_tests` | after code edits; config `test_command` |
| `/retry` | regenerate last answer |
| `/save` `/load` `/sessions` | disk history under `~/.agenterm/sessions` |
| `/compact` | shrink tool history |
| `AGENTS.md` | auto-loaded into system prompt |
| Project root | shown in workspace hint |

### v0.3.0 shipped (Grok-like UX batch)

| Feature | How to use |
|---------|------------|
| Multi-line prompt | **Alt+Enter** newline; Enter sends |
| `/plan on\|off` | Plan-only (no tools), then implement |
| `/edit` | Edit last user prompt & resend |
| `/copy` | Last agent reply → clipboard + `~/.agenterm/last_reply.txt` |
| `/undo` | Revert last `write_file` / `str_replace` |
| `/stop` | Cancel generation |
| `repo_map` tool | Compact project tree |
| Link check | Built-in grep+fetch report (skips localhost / `$VAR`) |
| `/model` picker | Tab / Enter / Esc |

---

## Honest goals

| Goal | Realistic? |
|------|------------|
| **Best lightweight Ollama/xAI terminal agent** | Yes — focus Phases A–B |
| **Clone of Grok UI** | No — needs xAI model + product + infra |

**Prefer for tool work:** **`qwen2.5-coder:32b`** (default) or `qwen3-coder:30b` — not chatty “plus” general models.

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
| Default model | `qwen2.5-coder:32b` for Go + docs |
| First-launch OSC junk | Fixed dark glamour (no `WithAutoStyle` color probe); scrub `]11;rgb:…` from input |
| Apply changes (not only chat) | `str_replace` + allowlisted `git` tools; action-mode nudge on “do it” / apply / commit |

---

## Monday resume checklist

- [ ] Re-read this doc + skim `internal/agent`, `internal/tui`, `internal/tools`
- [ ] Confirm Ollama has **`qwen2.5-coder:32b`** loaded; use as default (`-m` or config)
- [ ] Pick **Phase A** items (suggest: `@file` + `grep` + session history)
- [ ] Optional later: first-class xAI Grok API preset (cloud), keep Ollama coder for local
- [ ] Optional: tag release so install script gets binaries without source build
- [ ] Manual: from `dboper/` cwd with `qwen2.5-coder:32b` — “can you read sidb/oracle-database-operator/README.md? yes or no”

---

## One-line summary

> **agenterm is a thin Go TUI agent over OpenAI-compatible APIs; Grok feels better because of the model and a deep product stack—not because agenterm used Go instead of Rust. Close the gap with search/edit/context/sessions and stronger tool discipline, not a rewrite.**
