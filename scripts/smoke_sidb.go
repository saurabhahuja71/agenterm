//go:build ignore

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/saurabhahuja71/agenterm/internal/agent"
	"github.com/saurabhahuja71/agenterm/internal/config"
	"github.com/saurabhahuja71/agenterm/internal/llm"
	"github.com/saurabhahuja71/agenterm/internal/tools"
)

func main() {
	// Run from dboper so sidb/... paths resolve like the user's workflow.
	dboper := filepath.Clean(filepath.Join(mustCwd(), ".."))
	if err := os.Chdir(dboper); err != nil {
		panic(err)
	}
	fmt.Println("cwd:", mustCwd())

	cfg := config.Default()
	cfg.Model = env("MODEL", "qwen3-coder:latest")
	cfg.BaseURL = env("AGENTERM_BASE_URL", "http://127.0.0.1:11434/v1")
	cfg.EnableTools = true
	ag := agent.New(cfg.Effective(), llm.New(cfg.BaseURL, "ollama"), tools.DefaultBuiltins(false))

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	var toolsN int
	var blob strings.Builder
	err := ag.RunUserMessage(ctx,
		"Read sidb/oracle-database-operator/README.md using tools and say in one short sentence whether it looks SEO-friendly.",
		func(ev agent.Event) {
			switch ev.Kind {
			case agent.EventToolStart:
				toolsN++
				fmt.Printf("→ %s %s\n", ev.Tool, trunc(ev.Text, 100))
			case agent.EventToolEnd:
				fmt.Printf("← %s %s\n", ev.Tool, trunc(ev.ToolOut, 120))
				blob.WriteString(ev.ToolOut)
			case agent.EventToken:
				blob.WriteString(ev.Text)
				fmt.Print(ev.Text)
			case agent.EventError:
				fmt.Println("ERR", ev.Text)
			case agent.EventStatus:
				fmt.Println("·", trunc(ev.Text, 90))
			}
		})
	fmt.Printf("\n--- tools=%d err=%v ---\n", toolsN, err)
	if err != nil || toolsN == 0 {
		os.Exit(1)
	}
	fmt.Println("PASS")
}

func mustCwd() string { c, _ := os.Getwd(); return c }
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func trunc(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
