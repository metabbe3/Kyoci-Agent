package config

import (
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config holds all configuration for the AI agent system
type Config struct {
	Agent    AgentConfig    `yaml:"agent"`
	LLM      LLMConfig      `yaml:"llm"`
	Memory   MemoryConfig   `yaml:"memory"`
	Server   ServerConfig   `yaml:"server"`
	Tools    ToolsConfig    `yaml:"tools"`
	Routing  RoutingConfig  `yaml:"routing"`
}

type AgentConfig struct {
	MaxIterations int    `yaml:"max_iterations"` // Max ReAct loops
	SystemPrompt  string `yaml:"system_prompt"`
	Temperature   float64 `yaml:"temperature"`
	MaxTokens     int    `yaml:"max_tokens"`
	Verbose       bool   `yaml:"verbose"` // Log reasoning steps
	Template      string `yaml:"template"` // Prompt template: system, coder, researcher, analyst, browser, creative
	DataDir       string `yaml:"data_dir"` // Directory for self-improvement data
}

type LLMConfig struct {
	DefaultProvider string            `yaml:"default_provider"`
	Providers       map[string]ProviderConfig `yaml:"providers"`
	Fallback        []string          `yaml:"fallback"` // Provider fallback chain
}

type ProviderConfig struct {
	APIKey     string  `yaml:"api_key"`
	BaseURL    string  `yaml:"base_url"`
	Model      string  `yaml:"model"`
	MaxTokens  int     `yaml:"max_tokens"`
	Temperature float64 `yaml:"temperature"`
}

type MemoryConfig struct {
	Type         string `yaml:"type"`          // "conversation", "summary", "vector"
	MaxTokens    int    `yaml:"max_tokens"`    // Max context window
	BufferSize   int    `yaml:"buffer_size"`   // Number of messages to keep
	LongTermPath string `yaml:"long_term_path"` // Path to long-term memory JSON
	CompactionThreshold float64 `yaml:"compaction_threshold"` // 0.0-1.0, when to compact
}

type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type ToolsConfig struct {
	Enabled  []string `yaml:"enabled"`
	Sandbox  bool     `yaml:"sandbox"` // Run tools in sandbox
	WorkDir  string   `yaml:"work_dir"`
}

type TierBinding struct {
	Tier0         string `yaml:"tier0"`          // "builtin"
	Tier1         string `yaml:"tier1"`          // "ollama"
	Tier2         string `yaml:"tier2"`          // "openai"
	Tier1Fallback string `yaml:"tier1_fallback"` // "reject" or provider name
	Tier2Fallback string `yaml:"tier2_fallback"` // "anthropic" or provider name
}

type RoutingConfig struct {
	TierBindings TierBinding `yaml:"tier_bindings"`
}

// DefaultConfig returns a sensible default configuration
func DefaultConfig() *Config {
	return &Config{
		Agent: AgentConfig{
			MaxIterations: 10,
			SystemPrompt:  "You are a helpful AI assistant. Use tools when needed to complete tasks accurately.",
			Temperature:   0.7,
			MaxTokens:     4096,
			Verbose:       true,
		},
		LLM: LLMConfig{
			DefaultProvider: "openai",
			Fallback:        []string{"anthropic", "ollama"},
			Providers: map[string]ProviderConfig{
				"openai": {
					BaseURL:    "https://api.openai.com/v1",
					Model:      "gpt-4o",
					MaxTokens:  4096,
					Temperature: 0.7,
				},
				"anthropic": {
					BaseURL:    "https://api.anthropic.com",
					Model:      "claude-sonnet-4-20250514",
					MaxTokens:  4096,
					Temperature: 0.7,
				},
				"ollama": {
					BaseURL:    "http://localhost:11434",
					Model:      "llama3",
					MaxTokens:  4096,
					Temperature: 0.7,
				},
				"google": {
					BaseURL:    "https://generativelanguage.googleapis.com/v1beta",
					Model:      "gemini-2.0-flash",
					MaxTokens:  4096,
					Temperature: 0.7,
				},
			},
		},
		Memory: MemoryConfig{
			Type:       "conversation",
			MaxTokens:  8192,
			BufferSize: 20,
		},
		Server: ServerConfig{
			Host: "0.0.0.0",
			Port: 8080,
		},
		Tools: ToolsConfig{
			Enabled: []string{"web_search", "calculator", "file_handler"},
			Sandbox: false,
		},
		Routing: RoutingConfig{
			TierBindings: TierBinding{
				Tier0:         "builtin",
				Tier1:         "ollama",
				Tier2:         "openai",
				Tier1Fallback: "reject",
				Tier2Fallback: "anthropic",
			},
		},
	}
}

// Load reads config from a YAML file, then overlays env vars
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return overlayEnv(cfg), nil
		}
		return nil, err
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return overlayEnv(cfg), nil
}

// overlayEnv replaces config values with environment variables if set
func overlayEnv(cfg *Config) *Config {
	// API keys from env (highest priority)
	if v := os.Getenv("OPENAI_API_KEY"); v != "" {
		p := cfg.LLM.Providers["openai"]
		p.APIKey = v
		cfg.LLM.Providers["openai"] = p
	}
	if v := os.Getenv("ANTHROPIC_API_KEY"); v != "" {
		p := cfg.LLM.Providers["anthropic"]
		p.APIKey = v
		cfg.LLM.Providers["anthropic"] = p
	}
	if v := os.Getenv("GOOGLE_API_KEY"); v != "" {
		p := cfg.LLM.Providers["google"]
		p.APIKey = v
		cfg.LLM.Providers["google"] = p
	}

	// Override base URLs for custom endpoints
	if v := os.Getenv("OLLAMA_BASE_URL"); v != "" {
		p := cfg.LLM.Providers["ollama"]
		p.BaseURL = v
		cfg.LLM.Providers["ollama"] = p
	}
	if v := os.Getenv("OPENAI_BASE_URL"); v != "" {
		p := cfg.LLM.Providers["openai"]
		p.BaseURL = v
		cfg.LLM.Providers["openai"] = p
	}

	// Default provider override
	if v := os.Getenv("AI_DEFAULT_PROVIDER"); v != "" {
		cfg.LLM.DefaultProvider = v
	}

	// Override models
	for _, name := range []string{"openai", "anthropic", "ollama", "google"} {
		env := "AI_MODEL_" + strings.ToUpper(name)
		if v := os.Getenv(env); v != "" {
			if p, ok := cfg.LLM.Providers[name]; ok {
				p.Model = v
				cfg.LLM.Providers[name] = p
			}
		}
	}

	return cfg
}
