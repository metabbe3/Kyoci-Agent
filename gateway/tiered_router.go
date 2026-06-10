package gateway

import (
	"log/slog"
	"sync"
	"time"

	"github.com/nicholas/ai-agent/config"
)

// Tier represents the routing tier level
type Tier int

const (
	Tier0 Tier = 0 // Code/Deterministic
	Tier1 Tier = 1 // Cheap AI/Local
	Tier2 Tier = 2 // Complex AI/Cloud
)

// String returns string representation of Tier
func (t Tier) String() string {
	switch t {
	case Tier0:
		return "Tier0"
	case Tier1:
		return "Tier1"
	case Tier2:
		return "Tier2"
	default:
		return "Unknown"
	}
}

// Default timeouts for each tier
var TierTimeouts = map[Tier]time.Duration{
	Tier0: 5 * time.Second,   // Code operations
	Tier1: 150 * time.Second, // Cheap AI/Local (2.5 min)
	Tier2: 300 * time.Second, // Complex AI/Cloud (5 min)
}

// Provider represents a service provider
type Provider struct {
	Name      string
	Model     string
	Timeout   time.Duration
	MaxTokens int
	Tier      Tier
}

// TieredRouter implements cascading tiered routing with circuit breakers
type TieredRouter struct {
	providers      map[Tier][]Provider    // tier -> list of providers
	cb             map[string]*CircuitBreaker // per-provider circuit breaker
	tierBindings   map[int]string          // tier -> provider name binding
	tier1Fallback  string                  // tier1 fallback provider or "reject"
	tier2Fallback  string                  // tier2 fallback provider or "reject"
	mu             sync.RWMutex
}

// NewTieredRouter creates a new tiered router from config
func NewTieredRouter(cfg *config.Config) *TieredRouter {
	tr := &TieredRouter{
		providers:     make(map[Tier][]Provider),
		cb:            make(map[string]*CircuitBreaker),
		tierBindings:  make(map[int]string),
		tier1Fallback: cfg.Routing.TierBindings.Tier1Fallback,
		tier2Fallback: cfg.Routing.TierBindings.Tier2Fallback,
	}

	// Set up tier bindings from config
	tr.tierBindings[0] = cfg.Routing.TierBindings.Tier0
	tr.tierBindings[1] = cfg.Routing.TierBindings.Tier1
	tr.tierBindings[2] = cfg.Routing.TierBindings.Tier2

	// Populate providers from config
	// Default tier mapping - can be extended with config-based tier assignment
	for name, pCfg := range cfg.LLM.Providers {
		provider := Provider{
			Name:      name,
			Model:     pCfg.Model,
			MaxTokens: pCfg.MaxTokens,
		}

		// Assign tier based on provider name (configurable)
		switch name {
		case "ollama", "local":
			provider.Tier = Tier1
			provider.Timeout = TierTimeouts[Tier1]
		case "openai", "anthropic", "google", "claude":
			provider.Tier = Tier2
			provider.Timeout = TierTimeouts[Tier2]
		default:
			provider.Tier = Tier2
			provider.Timeout = TierTimeouts[Tier2]
		}

		tr.providers[provider.Tier] = append(tr.providers[provider.Tier], provider)
		// Create circuit breaker for each provider
		tr.cb[name] = NewCircuitBreaker(
			name,
			WithFailureThreshold(5),
			WithSuccessThreshold(3),
			WithTimeout(30*time.Second),
		)
	}

	return tr
}

// getProviderByName returns a provider by name across all tiers
func (tr *TieredRouter) getProviderByName(name string) (*Provider, bool) {
	for _, providers := range tr.providers {
		for i, provider := range providers {
			if provider.Name == name {
				return &providers[i], true
			}
		}
	}
	return nil, false
}

// isProviderAvailable checks if a provider's circuit is not open
func (tr *TieredRouter) isProviderAvailable(name string) bool {
	cb, exists := tr.cb[name]
	if !exists {
		return true
	}
	return cb.State() != StateOpen
}

// Route picks the best available provider for the given tier
// Enforces strict tier-to-provider binding per config
// Returns error if no provider is available in the tier
func (tr *TieredRouter) Route(level int) (*Provider, error) {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	tier := Tier(level)

	// Tier 0: execute zero-AI code (no provider needed)
	if tier == Tier0 {
		// Return a special placeholder or nil to indicate no AI provider needed
		return nil, &NoProviderAvailableError{Tier: tier, Message: "Tier0 requires no AI provider"}
	}

	// Get the bound provider name for this tier
	boundProvider, hasBinding := tr.tierBindings[int(tier)]
	if !hasBinding {
		return nil, &NoProviderAvailableError{Tier: tier, Message: "no tier binding configured"}
	}

	// Log when enforcing tier binding
	slog.Debug("tier bound to provider", "tier", tier, "provider", boundProvider)

	// Find the bound provider
	provider, found := tr.getProviderByName(boundProvider)
	if !found {
		return nil, &NoProviderAvailableError{Tier: tier, Message: "bound provider not found: " + boundProvider}
	}

	// Check if the bound provider is available (circuit not open)
	if tr.isProviderAvailable(boundProvider) {
		return provider, nil
	}

	// Provider is down - check for fallback
	var fallbackProvider string
	switch tier {
	case Tier1:
		fallbackProvider = tr.tier1Fallback
	case Tier2:
		fallbackProvider = tr.tier2Fallback
	}

	// If fallback is "reject", return error
	if fallbackProvider == "reject" {
		return nil, &NoProviderAvailableError{Tier: tier, Message: "bound provider '" + boundProvider + "' is unavailable and fallback is set to reject"}
	}

	slog.Debug("bound provider unavailable, trying fallback", "tier", tier, "bound_provider", boundProvider, "fallback", fallbackProvider)

	// Try the fallback provider
	fbProvider, found := tr.getProviderByName(fallbackProvider)
	if !found {
		return nil, &NoProviderAvailableError{Tier: tier, Message: "fallback provider not found: " + fallbackProvider}
	}

	if tr.isProviderAvailable(fallbackProvider) {
		slog.Debug("tier using fallback provider", "tier", tier, "fallback", fallbackProvider)
		return fbProvider, nil
	}

	// Both bound and fallback providers are unavailable
	return nil, &NoProviderAvailableError{Tier: tier, Message: "both bound provider '" + boundProvider + "' and fallback '" + fallbackProvider + "' are unavailable"}
}

// RouteWithFallback picks provider from given tier, falls back to lower tiers if unavailable
func (tr *TieredRouter) RouteWithFallback(level int) (*Provider, Tier, error) {
	provider, err := tr.Route(level)
	if err != nil {
		return nil, Tier(level), err
	}
	return provider, Tier(level), nil
}

// ReportFailure records a failure for a provider
func (tr *TieredRouter) ReportFailure(name string) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	if cb, exists := tr.cb[name]; exists {
		cb.RecordFailure()
	}
}

// RecordSuccess records a success for a provider
func (tr *TieredRouter) RecordSuccess(name string) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	if cb, exists := tr.cb[name]; exists {
		cb.RecordSuccess()
	}
}

// AvailableTiers returns all tiers that have at least one available provider
func (tr *TieredRouter) AvailableTiers() []Tier {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	var tiers []Tier
	for tier := Tier0; tier <= Tier2; tier++ {
		for _, provider := range tr.providers[tier] {
			if tr.isProviderAvailable(provider.Name) {
				tiers = append(tiers, tier)
				break
			}
		}
	}
	return tiers
}

// GetCircuitBreaker returns the circuit breaker for a provider
func (tr *TieredRouter) GetCircuitBreaker(name string) *CircuitBreaker {
	tr.mu.RLock()
	defer tr.mu.RUnlock()
	return tr.cb[name]
}

// GetProvider returns a provider by name
func (tr *TieredRouter) GetProvider(name string) (*Provider, bool) {
	tr.mu.RLock()
	defer tr.mu.RUnlock()
	return tr.getProviderByName(name)
}

// AddProvider adds a provider to a tier
func (tr *TieredRouter) AddProvider(tier Tier, provider Provider) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.providers[tier] = append(tr.providers[tier], provider)
}

// RemoveProvider removes a provider
func (tr *TieredRouter) RemoveProvider(name string) bool {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	for tier, providers := range tr.providers {
		for i, provider := range providers {
			if provider.Name == name {
				tr.providers[tier] = append(providers[:i], providers[i+1:]...)
				delete(tr.cb, name)
				return true
			}
		}
	}
	return false
}

// GetProvidersInTier returns all providers in a tier
func (tr *TieredRouter) GetProvidersInTier(level int) []Provider {
	tr.mu.RLock()
	defer tr.mu.RUnlock()
	tier := Tier(level)
	providers, exists := tr.providers[tier]
	if !exists {
		return []Provider{}
	}
	result := make([]Provider, len(providers))
	copy(result, providers)
	return result
}

// NoProviderAvailableError is returned when no provider is available for a tier
type NoProviderAvailableError struct {
	Tier    Tier
	Message string
}

func (e *NoProviderAvailableError) Error() string {
	return e.Message
}