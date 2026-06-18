package llm

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/metabbe3/Kyoci-Agent/internal/apperr"
	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// ==============================================================================
// Router Strategy
// ==============================================================================

// RouteStrategy defines the routing strategy for provider selection.
type RouteStrategy int

const (
	// StrategyFastest routes to the provider with lowest latency.
	StrategyFastest RouteStrategy = iota
	// StrategyCheapest routes to the provider with lowest cost.
	StrategyCheapest
	// StrategyFallback routes to preferred provider, then falls back.
	StrategyFallback
	// StrategyRoundRobin rotates through all available providers.
	StrategyRoundRobin
)

// String returns a string representation of the route strategy.
func (s RouteStrategy) String() string {
	switch s {
	case StrategyFastest:
		return "fastest"
	case StrategyCheapest:
		return "cheapest"
	case StrategyFallback:
		return "fallback"
	case StrategyRoundRobin:
		return "round_robin"
	default:
		return "unknown"
	}
}

// ==============================================================================
// Router
// ==============================================================================

// Router implements intelligent routing for LLM requests.
type Router struct {
	registry       *ProviderRegistry
	strategy       RouteStrategy
	latencyTracker map[string]time.Duration
	costTracker    map[string]float64
	roundRobinIdx  int
	mu             sync.RWMutex
	logger         *slog.Logger
}

// NewRouter creates a new router with the given provider registry.
func NewRouter(registry *ProviderRegistry, strategy RouteStrategy) *Router {
	return &Router{
		registry:       registry,
		strategy:       strategy,
		latencyTracker: make(map[string]time.Duration),
		costTracker:    make(map[string]float64),
		roundRobinIdx:  0,
		logger:         slog.Default(),
	}
}

// SetStrategy updates the routing strategy.
func (r *Router) SetStrategy(strategy RouteStrategy) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.strategy = strategy
	r.logger.Info("router strategy updated", "strategy", strategy.String())
}

// Route performs a non-streaming completion request with intelligent routing.
func (r *Router) Route(ctx context.Context, req kyoci.CompletionRequest, preferredProvider string) (*kyoci.CompletionResponse, error) {
	// Tier 0: Check if this can be handled without LLM (zero-AI)
	// This would be implemented in a higher layer (orchestrator/role)
	// For now, we always proceed to LLM

	availableProviders := r.registry.GetAvailable()
	if len(availableProviders) == 0 {
		return nil, apperr.ErrNoAvailableProviders
	}

	// Select provider based on strategy
	provider, err := r.selectProvider(availableProviders, preferredProvider)
	if err != nil {
		return nil, apperr.Wrap("llm.select_failed", apperr.KindUnavailable, err, "failed to select provider")
	}

	// Execute request
	startTime := time.Now()
	resp, err := provider.Complete(ctx, req)
	duration := time.Since(startTime)

	if err != nil {
		// Record failure
		r.recordFailure(provider.Name())
		return nil, apperr.Wrapf("llm.provider_failed", apperr.KindUnavailable, err, "provider %s", provider.Name())
	}

	// Record success metrics
	r.recordSuccess(provider.Name(), duration, resp)

	r.logger.Info("request routed successfully",
		"provider", provider.Name(),
		"model", resp.Model,
		"duration", duration,
		"tokens", resp.Usage.TotalTokens)

	return resp, nil
}

// RouteStream performs a streaming completion request with intelligent routing.
func (r *Router) RouteStream(ctx context.Context, req kyoci.CompletionRequest, preferredProvider string) (<-chan kyoci.StreamChunk, error) {
	availableProviders := r.registry.GetAvailable()
	if len(availableProviders) == 0 {
		return nil, apperr.ErrNoAvailableProviders
	}

	// Select provider based on strategy
	provider, err := r.selectProvider(availableProviders, preferredProvider)
	if err != nil {
		return nil, apperr.Wrap("llm.select_failed", apperr.KindUnavailable, err, "failed to select provider")
	}

	// Execute streaming request
	startTime := time.Now()
	ch, err := provider.Stream(ctx, req)
	if err != nil {
		r.recordFailure(provider.Name())
		return nil, apperr.Wrapf("llm.provider_failed", apperr.KindUnavailable, err, "provider %s", provider.Name())
	}

	// Create wrapped channel to track completion
	wrappedCh := make(chan kyoci.StreamChunk, 10)

	go func() {
		defer close(wrappedCh)

		duration := time.Since(startTime)
		var totalTokens int

		for chunk := range ch {
			if chunk.Done && chunk.Usage != nil {
				totalTokens = chunk.Usage.TotalTokens
				// Record success metrics on final chunk
				r.recordSuccess(provider.Name(), duration, &kyoci.CompletionResponse{
					Usage: *chunk.Usage,
				})
			}
			wrappedCh <- chunk
		}

		r.logger.Info("streaming request routed successfully",
			"provider", provider.Name(),
			"duration", duration,
			"tokens", totalTokens)
	}()

	return wrappedCh, nil
}

// selectProvider selects a provider based on the current strategy.
func (r *Router) selectProvider(providers []kyoci.Provider, preferredProvider string) (kyoci.Provider, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	switch r.strategy {
	case StrategyFastest:
		return r.selectFastest(providers)
	case StrategyCheapest:
		return r.selectCheapest(providers)
	case StrategyFallback:
		return r.selectFallback(providers, preferredProvider)
	case StrategyRoundRobin:
		return r.selectRoundRobin(providers)
	default:
		return r.selectFallback(providers, preferredProvider)
	}
}

// selectFastest selects the provider with lowest average latency.
func (r *Router) selectFastest(providers []kyoci.Provider) (kyoci.Provider, error) {
	if len(providers) == 0 {
		return nil, apperr.ErrNoProviders
	}

	var fastest kyoci.Provider
	minLatency := time.Duration(1<<63 - 1) // Max duration

	for _, provider := range providers {
		latency, ok := r.latencyTracker[provider.Name()]
		if !ok {
			// No latency data, use this provider
			return provider, nil
		}

		if latency < minLatency {
			minLatency = latency
			fastest = provider
		}
	}

	if fastest == nil {
		return providers[0], nil
	}

	return fastest, nil
}

// selectCheapest selects the provider with lowest average cost.
func (r *Router) selectCheapest(providers []kyoci.Provider) (kyoci.Provider, error) {
	if len(providers) == 0 {
		return nil, apperr.ErrNoProviders
	}

	var cheapest kyoci.Provider
	minCost := float64(-1)

	for _, provider := range providers {
		cost, ok := r.costTracker[provider.Name()]
		if !ok {
			// No cost data, use this provider
			return provider, nil
		}

		if minCost < 0 || cost < minCost {
			minCost = cost
			cheapest = provider
		}
	}

	if cheapest == nil {
		return providers[0], nil
	}

	return cheapest, nil
}

// selectFallback tries preferred provider, then falls back to others.
func (r *Router) selectFallback(providers []kyoci.Provider, preferredProvider string) (kyoci.Provider, error) {
	if len(providers) == 0 {
		return nil, apperr.ErrNoProviders
	}

	// Check if preferred provider exists and is available
	if preferredProvider != "" {
		provider, err := r.registry.Get(preferredProvider)
		if err == nil && provider.IsAvailable() {
			return provider, nil
		}
	}

	// Fall back to first available provider
	return providers[0], nil
}

// selectRoundRobin selects the next provider in round-robin order.
func (r *Router) selectRoundRobin(providers []kyoci.Provider) (kyoci.Provider, error) {
	if len(providers) == 0 {
		return nil, apperr.ErrNoProviders
	}

	idx := r.roundRobinIdx % len(providers)
	r.roundRobinIdx++

	return providers[idx], nil
}

// recordSuccess updates metrics for a successful request.
func (r *Router) recordSuccess(providerName string, duration time.Duration, resp *kyoci.CompletionResponse) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Update latency tracker (exponential moving average)
	oldLatency, ok := r.latencyTracker[providerName]
	if !ok {
		r.latencyTracker[providerName] = duration
	} else {
		// EMA with alpha=0.3
		r.latencyTracker[providerName] = time.Duration(float64(oldLatency)*0.7 + float64(duration)*0.3)
	}

	// Update cost tracker (simple estimate: $0.0001 per 1K tokens)
	// In production, this should use actual provider pricing
	cost := float64(resp.Usage.TotalTokens) / 10000.0
	oldCost, ok := r.costTracker[providerName]
	if !ok {
		r.costTracker[providerName] = cost
	} else {
		r.costTracker[providerName] = oldCost*0.7 + cost*0.3
	}
}

// recordFailure marks a provider as having failed (circuit breaker handles state).
func (r *Router) recordFailure(providerName string) {
	r.logger.Warn("provider request failed", "provider", providerName)
}

// GetLatency returns the average latency for a provider.
func (r *Router) GetLatency(providerName string) time.Duration {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.latencyTracker[providerName]
}

// GetCost returns the average cost for a provider.
func (r *Router) GetCost(providerName string) float64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.costTracker[providerName]
}

// GetStrategy returns the current routing strategy.
func (r *Router) GetStrategy() RouteStrategy {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.strategy
}
