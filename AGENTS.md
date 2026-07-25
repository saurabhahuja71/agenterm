# agenterm project rules

- Language: Go. Module `github.com/saurabhahuja71/agenterm`.
- Prefer small, focused changes. Keep the TUI quiet by default (`/verbose` for full tool dumps).
- Default local model: `qwen2.5-coder:32b` (see `docs/grok-parity-roadmap.md`).
- When applying user-requested edits: use `str_replace` / `write_file` / `git` tools — do not only print plans.
- Tests: `go test ./...` and `make build`.
- Do not invent file paths; use `grep`, `find_files`, or `list_dir` first.
