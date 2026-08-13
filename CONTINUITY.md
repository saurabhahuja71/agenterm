# Continuity

- Summary: SSH execution now probes the local hostname, `whoami`, and current kubectl context; matching targets execute locally. The TUI keeps one active assistant bubble per turn and removes the duplicate standalone busy Agent panel.
- Files modified: `internal/tools/ssh.go`, `internal/tools/ssh_test.go`, `internal/config/config.go`, `internal/tui/app.go`, `internal/tui/transcript_test.go`.
- Decisions: `run_shell` remains the local execution path for kubectl/watch/logs; `ssh_execute` is reserved for clearly different hosts and still supports SSH config aliases. Thinking, streaming, and final assistant text update one indexed transcript line; tools/errors remain cards.
- Checks: `gofmt` passed; focused and full `go test` passed; `make build` passed with `GOCACHE=/tmp/agenterm-go-build` because the default Go cache is read-only in this environment.
- TODOs/blockers: none known.
- Suggested next task: exercise `ssh_execute` against a real same-host alias and a deliberately unauthenticated host to verify the user-facing tool transcript.
