package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/saurabhahuja71/agenterm/internal/agent"
	"github.com/saurabhahuja71/agenterm/internal/config"
	"github.com/saurabhahuja71/agenterm/internal/llm"
	mcpclient "github.com/saurabhahuja71/agenterm/internal/mcp"
	"github.com/saurabhahuja71/agenterm/internal/tools"
	"github.com/saurabhahuja71/agenterm/internal/tui"
	"github.com/spf13/cobra"
)

var (
	version = "0.1.7"
	flagProvider string
	flagModel    string
	flagBaseURL  string
	flagAPIKey   string
	flagConfig   string
	flagNoMCP    bool
	flagNoTools  bool
	flagShell    bool
	flagPing     bool
)

func main() {
	root := &cobra.Command{
		Use:     "agenterm",
		Short:   "Snappy terminal AI agent (Ollama / OpenAI-compatible + MCP)",
		Long:    "agenterm is a Grok-style terminal chat agent. Point it at local or remote Ollama, xAI, OpenAI, or any OpenAI-compatible server.",
		Version: version,
		RunE:    runTUI,
	}
	root.Flags().StringVar(&flagProvider, "provider", "", "provider preset: ollama-local | ollama-remote | xai | openai | custom")
	root.Flags().StringVarP(&flagModel, "model", "m", "", "model id (e.g. qwen2.5-coder:32b, qwen3-coder:30b, grok-3)")
	root.Flags().StringVar(&flagBaseURL, "base-url", "", "OpenAI-compatible API base (e.g. http://127.0.0.1:11434/v1)")
	root.Flags().StringVar(&flagAPIKey, "api-key", "", "API key (optional for Ollama)")
	root.Flags().StringVar(&flagConfig, "config", "", "path to config.toml (default ~/.agenterm/config.toml)")
	root.Flags().BoolVar(&flagNoMCP, "no-mcp", false, "do not connect MCP servers from config")
	root.Flags().BoolVar(&flagNoTools, "no-tools", false, "disable function/tool calling for this session (faster chat)")
	root.Flags().BoolVar(&flagShell, "shell", false, "enable run_shell tool for this session")
	root.Flags().BoolVar(&flagPing, "ping", false, "check LLM endpoint and exit")

	var initForce bool
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Write default config to ~/.agenterm/config.toml",
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := config.Path()
			if err != nil {
				return err
			}
			if flagConfig != "" {
				path = flagConfig
			}
			if !initForce {
				if _, err := os.Stat(path); err == nil {
					return fmt.Errorf("%s already exists (use: agenterm init --force)", path)
				}
			}
			cfg := config.Default()
			if err := config.Save(cfg, path); err != nil {
				return err
			}
			fmt.Println("wrote", path)
			fmt.Println("tips:")
			fmt.Println("  • Ollama default: http://127.0.0.1:11434/v1 (SSH tunnel OK)")
			fmt.Println("  • Fast chat:     agenterm --no-tools")
			fmt.Println("  • Ping:          agenterm --ping")
			return nil
		},
	}
	initCmd.Flags().BoolVar(&initForce, "force", false, "overwrite existing config with defaults")
	root.AddCommand(initCmd)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func runTUI(cmd *cobra.Command, args []string) error {
	if flagConfig != "" {
		_ = os.Setenv("AGENTERM_CONFIG", flagConfig)
	}

	cfg, path, err := config.Load()
	if err != nil {
		return err
	}

	// CLI overrides
	if flagProvider != "" {
		cfg.Provider = flagProvider
	}
	if flagModel != "" {
		cfg.Model = flagModel
	}
	if flagBaseURL != "" {
		cfg.BaseURL = flagBaseURL
		cfg.Provider = "custom"
	}
	if flagAPIKey != "" {
		cfg.APIKey = flagAPIKey
	}
	if flagShell {
		cfg.EnableShell = true
	}
	if flagNoTools {
		cfg.EnableTools = false
	}

	eff := cfg.Effective()
	client := llm.New(eff.BaseURL, eff.APIKey)

	if flagPing {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := client.Ping(ctx); err != nil {
			return fmt.Errorf("ping %s: %w", eff.BaseURL, err)
		}
		fmt.Printf("ok  provider=%s model=%s url=%s\n", eff.Provider, eff.Model, eff.BaseURL)
		return nil
	}

	reg := tools.DefaultBuiltins(eff.EnableShell)

	var mcpMgr *mcpclient.Manager
	if !flagNoMCP {
		mcpMgr = mcpclient.NewManager()
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		if err := mcpMgr.ConnectAll(ctx, eff.MCPServers); err != nil {
			// non-fatal: show in TUI via stderr
			fmt.Fprintf(os.Stderr, "mcp: %v\n", err)
		}
		cancel()
		mcpMgr.RegisterOnto(reg)
		defer mcpMgr.Close()
	}

	ag := agent.New(eff, client, reg)

	fmt.Fprintf(os.Stderr, "agenterm %s  config=%s\n", version, path)
	fmt.Fprintf(os.Stderr, "  %s\n", eff.Summary())

	return tui.Run(tui.Deps{
		Title:   "agenterm",
		Summary: eff.Summary(),
		Agent:   ag,
	})
}
