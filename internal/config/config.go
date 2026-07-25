package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config is the user-facing agenterm settings file.
type Config struct {
	// Provider selects a named block under [providers.*], or "custom".
	Provider string `toml:"provider"`

	// Model is the chat model id (Ollama tag, OpenAI model, xAI model, …).
	Model string `toml:"model"`

	// BaseURL is the OpenAI-compatible API root, e.g.
	//   http://127.0.0.1:11434/v1   (local Ollama)
	//   http://192.168.1.10:11434/v1 (remote Ollama on LAN)
	//   https://api.x.ai/v1
	//   https://api.openai.com/v1
	BaseURL string `toml:"base_url"`

	// APIKey optional for Ollama; required for cloud providers.
	// Prefer env AGENTERM_API_KEY / OLLAMA_API_KEY / XAI_API_KEY / OPENAI_API_KEY.
	APIKey string `toml:"api_key"`

	// SystemPrompt prepended as a system message.
	SystemPrompt string `toml:"system_prompt"`

	// Temperature sampling (0–2). Ollama/OpenAI compatible.
	Temperature float64 `toml:"temperature"`

	// MaxTokens completion budget (0 = provider default).
	MaxTokens int `toml:"max_tokens"`

	// EnableTools allows function/tool calling when the model supports it.
	EnableTools bool `toml:"enable_tools"`

	// EnableShell allows the built-in run_shell tool (dangerous; off by default).
	EnableShell bool `toml:"enable_shell"`

	// MCPServers optional external MCP tool servers.
	MCPServers []MCPServer `toml:"mcp_servers"`

	// Providers optional named presets (ollama-local, ollama-remote, xai, …).
	Providers map[string]Provider `toml:"providers"`
}

// Provider is a reusable endpoint preset.
type Provider struct {
	BaseURL string `toml:"base_url"`
	APIKey  string `toml:"api_key"`
	Model   string `toml:"model"`
}

// MCPServer describes how to attach an MCP tool server.
type MCPServer struct {
	Name    string   `toml:"name"`
	Enabled bool     `toml:"enabled"`
	// URL for streamable HTTP, e.g. http://127.0.0.1:8080/mcp
	URL string `toml:"url"`
	// Command+Args for stdio transport (local process).
	Command string   `toml:"command"`
	Args    []string `toml:"args"`
}

// Default returns sensible local-Ollama defaults.
func Default() Config {
	return Config{
		Provider:    "ollama-local",
		// Best local default for Go + docs + tools (see docs/grok-parity-roadmap.md).
		Model:       "qwen2.5-coder:32b",
		BaseURL:     "http://127.0.0.1:11434/v1",
		APIKey:      "ollama",
		Temperature: 0.7,
		EnableTools: true,
		EnableShell: false,
		SystemPrompt: `You are agenterm, a fast terminal coding assistant.

Style:
- Be concise. Prefer short answers.
- Yes/no questions: answer Yes or No in the first sentence, then at most 1–2 short lines.
- Do NOT invent files, directories, paths, or file listings. If tools were not used or failed, say so.
- Do NOT paste long directory listings or full file contents into the chat unless the user asked to show them.
- After tools return, summarize only what is needed for the user's question.

Tools (list_dir, read_file, write_file, find_files, …):
- Do NOT call tools for greetings or small talk.
- When the user asks about a repo, README, file, or folder: use tools; never guess contents.
- Paths are relative to the workspace cwd (see workspace hint). Never invent roots like "repo/".
- Prefer the smallest useful tool action (find_files / list_dir / read_file).`,
		Providers: map[string]Provider{
			"ollama-local": {
				BaseURL: "http://127.0.0.1:11434/v1",
				APIKey:  "ollama",
				Model:   "qwen2.5-coder:32b",
			},
			"ollama-remote": {
				BaseURL: "http://127.0.0.1:11434/v1", // user should edit host
				APIKey:  "ollama",
				Model:   "qwen2.5-coder:32b",
			},
			"xai": {
				BaseURL: "https://api.x.ai/v1",
				Model:   "grok-3",
			},
			"openai": {
				BaseURL: "https://api.openai.com/v1",
				Model:   "gpt-4o-mini",
			},
		},
		MCPServers: []MCPServer{
			{
				Name:    "mcp-demo",
				Enabled: false,
				URL:     "http://127.0.0.1:8080/mcp",
			},
		},
	}
}

// Path returns the default config file path.
func Path() (string, error) {
	if p := os.Getenv("AGENTERM_CONFIG"); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".agenterm", "config.toml"), nil
}

// Load reads config from disk (or creates defaults).
func Load() (Config, string, error) {
	path, err := Path()
	if err != nil {
		return Config{}, "", err
	}
	cfg := Default()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := Save(cfg, path); err != nil {
			return cfg, path, fmt.Errorf("create default config: %w", err)
		}
		return applyEnv(cfg), path, nil
	}
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return Config{}, path, fmt.Errorf("parse config %s: %w", path, err)
	}
	// Merge defaults for missing provider map
	if cfg.Providers == nil {
		cfg.Providers = Default().Providers
	}
	return applyEnv(cfg), path, nil
}

// Save writes config atomically-ish.
func Save(cfg Config, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := toml.NewEncoder(f)
	return enc.Encode(cfg)
}

// Resolve applies provider preset + env overrides into effective BaseURL/Model/APIKey.
func (c Config) Resolve() Config {
	out := c
	if c.Provider != "" && c.Provider != "custom" {
		if p, ok := c.Providers[c.Provider]; ok {
			if out.BaseURL == "" || out.BaseURL == Default().BaseURL && c.Provider != "ollama-local" {
				// Prefer explicit top-level fields; fill from provider when empty-ish
			}
			if p.BaseURL != "" && (c.BaseURL == "" || providerOwns(c)) {
				out.BaseURL = p.BaseURL
			}
			if p.Model != "" && (c.Model == "" || providerOwns(c)) {
				// Only override model from provider if user still on defaults for that provider
			}
			// Simpler rule: if provider set, provider fields fill blanks; top-level wins when non-empty after load.
			if out.BaseURL == "" {
				out.BaseURL = p.BaseURL
			}
			if out.Model == "" {
				out.Model = p.Model
			}
			if out.APIKey == "" {
				out.APIKey = p.APIKey
			}
			// When provider is selected, use its base_url/model unless top-level was customized away from default preset.
			// Practical approach used below in applyProvider.
			out = applyProvider(c, p)
		}
	}
	if out.BaseURL == "" {
		out.BaseURL = "http://127.0.0.1:11434/v1"
	}
	out.BaseURL = strings.TrimRight(out.BaseURL, "/")
	if out.Model == "" {
		out.Model = "qwen2.5-coder:32b"
	}
	if out.APIKey == "" {
		out.APIKey = "ollama"
	}
	return out
}

func providerOwns(c Config) bool {
	return true
}

func applyProvider(c Config, p Provider) Config {
	out := c
	// Top-level config always wins if set; provider fills when loading defaults.
	// Users set provider = "ollama-remote" and edit providers.ollama-remote.base_url.
	if p.BaseURL != "" {
		// Use provider base when provider is named (typical)
		out.BaseURL = p.BaseURL
	}
	if p.Model != "" {
		// Prefer top-level model if user set it and provider model is just default
		if c.Model != "" {
			out.Model = c.Model
		} else {
			out.Model = p.Model
		}
	}
	if p.APIKey != "" && c.APIKey == "" {
		out.APIKey = p.APIKey
	} else if p.APIKey != "" && (c.APIKey == "ollama" || c.APIKey == "") {
		out.APIKey = p.APIKey
	}
	// If top-level base_url was explicitly different and provider is custom-ish, keep top-level.
	// For named providers we intentionally use provider.base_url so remote Ollama works:
	// set [providers.ollama-remote] base_url = "http://gpu-box:11434/v1"
	if c.Provider != "" && c.Provider != "custom" && p.BaseURL != "" {
		out.BaseURL = p.BaseURL
	}
	if c.Provider == "custom" && c.BaseURL != "" {
		out.BaseURL = c.BaseURL
	}
	// Always allow top-level model override
	if c.Model != "" {
		out.Model = c.Model
	}
	return out
}

func applyEnv(c Config) Config {
	if v := os.Getenv("AGENTERM_PROVIDER"); v != "" {
		c.Provider = v
	}
	if v := os.Getenv("AGENTERM_MODEL"); v != "" {
		c.Model = v
	}
	if v := os.Getenv("AGENTERM_BASE_URL"); v != "" {
		c.BaseURL = v
	}
	if v := firstEnv("AGENTERM_API_KEY", "OLLAMA_API_KEY", "XAI_API_KEY", "OPENAI_API_KEY"); v != "" {
		c.APIKey = v
	}
	// AGENTERM_ENABLE_TOOLS=0|false|off disables tools; 1|true|on enables.
	if v := strings.TrimSpace(os.Getenv("AGENTERM_ENABLE_TOOLS")); v != "" {
		switch strings.ToLower(v) {
		case "0", "false", "no", "off":
			c.EnableTools = false
		case "1", "true", "yes", "on":
			c.EnableTools = true
		}
	}
	return c
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

// Effective returns resolved runtime settings.
func (c Config) Effective() Config {
	return applyEnv(c).Resolve()
}

// Summary is a one-line status for the TUI header.
func (c Config) Summary() string {
	e := c.Effective()
	return fmt.Sprintf("%s · %s · %s", e.Provider, e.Model, e.BaseURL)
}
