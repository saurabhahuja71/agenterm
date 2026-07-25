//go:build ignore

// Smoke-test agenterm agent loop against live Ollama.
// Usage:
//
//	go run scripts/smoke_agent.go
//	MODEL=qwen3.6-plus:latest go run scripts/smoke_agent.go
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/saurabhahuja71/agenterm/internal/agent"
	"github.com/saurabhahuja71/agenterm/internal/config"
	"github.com/saurabhahuja71/agenterm/internal/llm"
	"github.com/saurabhahuja71/agenterm/internal/tools"
)

func main() {
	base := env("AGENTERM_BASE_URL", "http://127.0.0.1:11434/v1")
	model := env("MODEL", env("AGENTERM_MODEL", "qwen3.6-plus:latest"))

	cfg := config.Default()
	cfg.BaseURL = base
	cfg.Model = model
	cfg.Provider = "custom"
	cfg.EnableTools = true
	cfg.EnableShell = false
	cfg.Temperature = 0.2

	client := llm.New(base, "ollama")
	reg := tools.DefaultBuiltins(false)
	ag := agent.New(cfg.Effective(), client, reg)

	fmt.Printf("smoke: base=%s model=%s cwd=%s\n", base, model, mustCwd())

	// 0) list models
	{
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		ids, err := client.ListModels(ctx)
		cancel()
		check("ListModels", err == nil && len(ids) > 0, fmt.Sprintf("n=%d err=%v", len(ids), err))
	}

	// 1) pure chat "hi"
	runCase(ag, "hi (no tools)", "hi", 90*time.Second, func(evs []agent.Event, err error) bool {
		if err != nil {
			return false
		}
		text := tokens(evs)
		toolsN := countKind(evs, agent.EventToolStart)
		fmt.Printf("    reply=%q tools=%d\n", trunc(text, 80), toolsN)
		return strings.TrimSpace(text) != "" && toolsN == 0
	})

	// 2) list current directory via tools
	runCase(ag, "list dir", "List the files in the current directory using tools. Reply with just the names you found.", 180*time.Second, func(evs []agent.Event, err error) bool {
		if err != nil {
			return false
		}
		toolsN := countKind(evs, agent.EventToolStart)
		out := toolOuts(evs)
		text := tokens(evs)
		fmt.Printf("    tools=%d toolOutHasAgenterm=%v reply=%q\n", toolsN, strings.Contains(out, "agenterm") || strings.Contains(out, "go.mod") || strings.Contains(text, "go.mod") || strings.Contains(text, "README"), trunc(text, 100))
		// Prefer real tool use; accept text recovery that still produced tool ends
		return toolsN > 0 || strings.Contains(out+text, "go.mod") || strings.Contains(out+text, "README")
	})

	// 3) read README first lines
	runCase(ag, "read README", "Use read_file to read README.md in the current directory. Quote the first heading line.", 180*time.Second, func(evs []agent.Event, err error) bool {
		if err != nil {
			return false
		}
		toolsN := countKind(evs, agent.EventToolStart)
		blob := toolOuts(evs) + tokens(evs)
		fmt.Printf("    tools=%d hasAgenterm=%v\n", toolsN, strings.Contains(blob, "agenterm") || strings.Contains(blob, "Terminal"))
		return toolsN > 0 && (strings.Contains(blob, "agenterm") || strings.Contains(blob, "Terminal") || strings.Contains(blob, "Ollama"))
	})

	// 4) find_files
	runCase(ag, "find go.mod", "Use find_files to locate go.mod under . and tell me the path.", 180*time.Second, func(evs []agent.Event, err error) bool {
		if err != nil {
			return false
		}
		blob := toolOuts(evs) + tokens(evs)
		toolsN := countKind(evs, agent.EventToolStart)
		fmt.Printf("    tools=%d blob=%q\n", toolsN, trunc(blob, 120))
		return toolsN > 0 && strings.Contains(blob, "go.mod")
	})

	fmt.Println("smoke: done")
}

func runCase(ag *agent.Agent, name, prompt string, timeout time.Duration, ok func([]agent.Event, error) bool) {
	fmt.Printf("\n== %s ==\n  prompt: %s\n", name, prompt)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	var evs []agent.Event
	err := ag.RunUserMessage(ctx, prompt, func(ev agent.Event) {
		evs = append(evs, ev)
		switch ev.Kind {
		case agent.EventToolStart:
			fmt.Printf("    → tool %s(%s)\n", ev.Tool, trunc(ev.Text, 60))
		case agent.EventToolEnd:
			fmt.Printf("    ← tool %s: %s\n", ev.Tool, trunc(ev.ToolOut, 80))
		case agent.EventError:
			fmt.Printf("    ! error: %s\n", ev.Text)
		case agent.EventStatus:
			fmt.Printf("    · %s\n", trunc(ev.Text, 80))
		}
	})
	pass := ok(evs, err)
	if err != nil && !pass {
		fmt.Printf("  FAIL err=%v\n", err)
		os.Exit(1)
	}
	if !pass {
		fmt.Printf("  FAIL criteria not met (err=%v)\n", err)
		os.Exit(1)
	}
	fmt.Printf("  PASS\n")
}

func tokens(evs []agent.Event) string {
	var b strings.Builder
	for _, e := range evs {
		if e.Kind == agent.EventToken {
			b.WriteString(e.Text)
		}
	}
	return b.String()
}

func toolOuts(evs []agent.Event) string {
	var b strings.Builder
	for _, e := range evs {
		if e.Kind == agent.EventToolEnd {
			b.WriteString(e.ToolOut)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func countKind(evs []agent.Event, k agent.EventKind) int {
	n := 0
	for _, e := range evs {
		if e.Kind == k {
			n++
		}
	}
	return n
}

func check(name string, pass bool, detail string) {
	if pass {
		fmt.Printf("== %s == PASS (%s)\n", name, detail)
		return
	}
	fmt.Printf("== %s == FAIL (%s)\n", name, detail)
	os.Exit(1)
}

func trunc(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", "\\n")
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func mustCwd() string {
	c, _ := os.Getwd()
	return c
}
