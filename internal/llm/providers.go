package llm

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/metabbe3/Kyoci-Agent/internal/config"
	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// ==============================================================================
// Provider Factory
// ==============================================================================

// NewProvider creates a new provider instance based on the provider name.
func NewProvider(name string, cfg kyoci.ProviderConfig) (kyoci.Provider, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("provider %s: base_url is required", name)
	}

	client, err := NewOpenAIClient(name, cfg)
	if err != nil {
		return nil, err
	}

	// Configure provider-specific settings
	switch name {
	case "anthropic":
		// Anthropic uses x-api-key header instead of Bearer
		client.WithXAPIKey("2023-06-01")
	case "ollama":
		// Ollama doesn't require auth, no special config needed
	default:
		// All other providers use standard Bearer token auth
	}

	return client, nil
}

// ==============================================================================
// Provider Registry
// ==============================================================================

// ProviderRegistry manages a collection of providers.
type ProviderRegistry struct {
	mu        sync.RWMutex
	providers map[string]kyoci.Provider
	logger    *slog.Logger
}

// NewProviderRegistry creates a new provider registry.
func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		providers: make(map[string]kyoci.Provider),
		logger:    slog.Default(),
	}
}

// Register adds a provider to the registry.
func (r *ProviderRegistry) Register(name string, provider kyoci.Provider) error {
	if provider == nil {
		return fmt.Errorf("cannot register nil provider for %s", name)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.providers[name] = provider
	r.logger.Info("provider registered", "name", name)
	return nil
}

// Get retrieves a provider by name.
func (r *ProviderRegistry) Get(name string) (kyoci.Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	provider, ok := r.providers[name]
	if !ok {
		return nil, fmt.Errorf("provider %s not found in registry", name)
	}
	return provider, nil
}

// List returns all registered providers.
func (r *ProviderRegistry) List() map[string]kyoci.Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]kyoci.Provider, len(r.providers))
	for name, provider := range r.providers {
		result[name] = provider
	}
	return result
}

// GetAvailable returns all providers that are currently available.
func (r *ProviderRegistry) GetAvailable() []kyoci.Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()

	available := make([]kyoci.Provider, 0)
	for _, provider := range r.providers {
		if provider.IsAvailable() {
			available = append(available, provider)
		}
	}
	return available
}

// ==============================================================================
// Provider Initialization
// ==============================================================================

// InitProviders creates and initializes all configured providers.
func InitProviders(cfg *config.Config) (*ProviderRegistry, error) {
	registry := NewProviderRegistry()

	// Provider base URLs
	providerBaseURLs := map[string]string{
		"openai":    "https://api.openai.com/v1",
		"anthropic": "https://api.anthropic.com/v1",
		"ollama":    "http://localhost:11434/v1",
		"gemini":    "https://generativelanguage.googleapis.com/v1beta/openai",
		"zai":       "https://open.bigmodel.cn/api/paas/v4",
		"groq":      "https://api.groq.com/openai/v1",
		"mistral":   "https://api.mistral.ai/v1",
		"deepseek":  "https://api.deepseek.com/v1",
		"together":  "https://api.together.ai/v1",
		"fireworks": "https://api.fireworks.ai/inference/v1",
		"xai":       "https://api.x.ai/v1",
	}

	// Default models for each provider
	defaultModels := map[string]string{
		"openai":    "gpt-4-turbo-preview",
		"anthropic": "claude-3-opus-20240229",
		"ollama":    "llama2",
		"gemini":    "gemini-pro",
		"zai":       "glm-4",
		"groq":      "llama2-70b-4096",
		"mistral":   "mistral-large-latest",
		"deepseek":  "deepseek-chat",
		"together":  "mistralai/Mixtral-8x7B-Instruct-v0.1",
		"fireworks": "accounts/fireworks/models/llama-v2-7b-chat",
		"xai":       "grok-beta",
	}

	// Create providers from config
	for name, providerCfg := range cfg.Providers {
		if !providerCfg.Enabled {
			continue
		}

		// Get base URL from defaults if not specified
		baseURL := providerCfg.GetBaseURL()
		if baseURL == "" {
			if defaultURL, ok := providerBaseURLs[name]; ok {
				baseURL = defaultURL
			} else {
				// Skip provider without base URL
				continue
			}
		}

		// Get default model from defaults if not specified
		defaultModel := providerCfg.GetDefaultModel()
		if defaultModel == "" {
			defaultModel = defaultModels[name]
		}

		// Convert config.ProviderConfig to kyoci.ProviderConfig
		kyociConfig := kyoci.ProviderConfig{
			BaseURL:      baseURL,
			APIKey:       providerCfg.GetAPIKey(),
			DefaultModel: defaultModel,
			MaxRetries:   providerCfg.GetMaxRetries(),
			Timeout:      providerCfg.GetTimeout(),
			Headers:      make(map[string]string),
			Logger:       slog.Default(),
		}

		provider, err := NewProvider(name, kyociConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create provider %s: %w", name, err)
		}

		if err := registry.Register(name, provider); err != nil {
			return nil, fmt.Errorf("failed to register provider %s: %w", name, err)
		}
	}

	return registry, nil
}
