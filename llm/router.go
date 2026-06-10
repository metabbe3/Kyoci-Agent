package llm

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/nicholas/ai-agent/config"
)

// Router manages multiple providers with fallback and routing
type Router struct {
	providers      map[string]Provider
	cache          *ResponseCache
	defaultName    string
	fallbackOrder  []string
	mu             sync.RWMutex
}

// NewRouter creates a new LLM router from config
func NewRouter(cfg *config.Config) *Router {
	r := &Router{
		providers:     make(map[string]Provider),
		defaultName:   cfg.LLM.DefaultProvider,
		fallbackOrder: cfg.LLM.Fallback,
	}

	// Initialize cache (optional - if cacheDir is empty, no cache is used)
	cacheDir := "data/cache"
	if cacheDir != "" {
		cache, err := NewResponseCache(cacheDir)
		if err != nil {
			slog.Error("failed to initialize cache", "error", err)
		} else {
			r.cache = cache
			slog.Info("response cache initialized", "cacheDir", cacheDir)
		}
	}

	for name, pcfg := range cfg.LLM.Providers {
		var provider Provider

		switch name {
		case "openai":
			provider = NewOpenAIProvider(pcfg)
		case "anthropic":
			provider = NewAnthropicProvider(pcfg)
		case "ollama":
			op := NewOllamaProvider(pcfg)
			op.SetQueue(NewOllamaQueue(32)) // serial queue, depth 32
			provider = op
		case "google":
			provider = NewGoogleProvider(pcfg)
		default:
			slog.Warn("unknown provider, skipping", "provider", name)
			continue
		}

		// Set cache if available and provider supports it
		if r.cache != nil {
			if cacheable, ok := provider.(interface{ SetCache(*ResponseCache) }); ok {
				cacheable.SetCache(r.cache)
			}
		}

		r.providers[name] = provider
		slog.Info("registered provider", "provider", name, "model", pcfg.Model)
	}

	return r
}

// Chat sends a request to the default provider, with automatic fallback
func (r *Router) Chat(ctx context.Context, messages []Message, tools []ToolSchema) (*Response, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Try default provider first
	providers := []string{r.defaultName}
	providers = append(providers, r.fallbackOrder...)

	var lastErr error
	for _, name := range providers {
		p, ok := r.providers[name]
		if !ok {
			continue
		}

		resp, err := p.Chat(ctx, messages, tools)
		if err != nil {
			slog.Error("provider failed, trying next", "provider", name, "error", err)
			lastErr = err
			continue
		}

		return resp, nil
	}

	return nil, fmt.Errorf("all providers failed, last error: %w", lastErr)
}

// Stream sends a streaming request to the default provider
func (r *Router) Stream(ctx context.Context, messages []Message, tools []ToolSchema) (<-chan Chunk, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.providers[r.defaultName]
	if !ok {
		return nil, fmt.Errorf("default provider not found: %s", r.defaultName)
	}
	return p.Stream(ctx, messages, tools)
}

// ChatWith forces a specific provider
func (r *Router) ChatWith(ctx context.Context, providerName string, messages []Message, tools []ToolSchema) (*Response, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.providers[providerName]
	if !ok {
		return nil, fmt.Errorf("provider not found: %s", providerName)
	}
	return p.Chat(ctx, messages, tools)
}

// GetProvider returns a specific provider
func (r *Router) GetProvider(name string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[name]
	return p, ok
}

// ListProviders returns names of all registered providers
func (r *Router) ListProviders() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	return names
}

// SetDefault changes the default provider
func (r *Router) SetDefault(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.providers[name]; !ok {
		return fmt.Errorf("provider not found: %s", name)
	}
	r.defaultName = name
	return nil
}

// CacheStats returns the current cache statistics
func (r *Router) CacheStats() CacheStats {
	if r.cache == nil {
		return CacheStats{}
	}
	return r.cache.Stats()
}

// InvalidateCache removes cache entries whose hash matches the given prefix
func (r *Router) InvalidateCache(prefix string) {
	if r.cache != nil {
		r.cache.Invalidate(prefix)
	}
}

// ClearCache removes all entries from the cache
func (r *Router) ClearCache() error {
	if r.cache == nil {
		return nil
	}
	return r.cache.Clear()
}

// OllamaQueueStats returns queue statistics from the Ollama provider if available.
// Returns nil if Ollama provider is not configured or has no queue.
func (r *Router) OllamaQueueStats() map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Try to get Ollama provider
	op, ok := r.providers["ollama"]
	if !ok {
		return nil
	}

	// Type assert to OllamaProvider to access its queue
	ollamaProvider, ok := op.(*OllamaProvider)
	if !ok {
		return nil
	}

	// Return nil if queue is not configured
	if ollamaProvider.queue == nil {
		return nil
	}

	return ollamaProvider.queue.Stats()
}