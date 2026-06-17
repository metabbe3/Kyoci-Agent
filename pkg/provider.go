package kyoci

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// ==============================================================================
// Provider Interface and Types
// ==============================================================================

// Provider is the interface that all LLM providers must implement.
// It defines the contract for interacting with language model services.
// Goroutine-safe: Implementations MUST be safe for concurrent use from multiple goroutines.
// The interface methods are called concurrently and must be properly synchronized.
type Provider interface {
	// Name returns the name of this provider (e.g., "openai", "anthropic", "ollama").
	// This is used for identification and logging purposes.
	Name() string

	// Complete performs a non-streaming completion request.
	// It blocks until the complete response is received or an error occurs.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout control
	//   - req: The completion request containing messages, model, tools, etc.
	//
	// Returns:
	//   - *CompletionResponse: The complete response including content, usage, and tool calls
	//   - error: Any error that occurred during the request
	//
	// Errors:
	//   - context.Canceled if ctx was canceled
	//   - context.DeadlineExceeded if timeout exceeded
	//   - *APIError if the provider API returns an error
	//   - *ConfigError if the request is invalid or provider not configured
	Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)

	// Stream performs a streaming completion request.
	// It returns a channel that receives StreamChunk values as they arrive.
	// The channel is closed when the stream is complete or an error occurs.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout control
	//   - req: The completion request (req.Stream is ignored, this always streams)
	//
	// Returns:
	//   - <-chan StreamChunk: Channel receiving stream chunks
	//   - error: Any error that occurred before streaming started
	//
	// The returned channel will receive multiple StreamChunk values.
	// The final chunk will have Done=true and may contain Usage and FinishReason.
	// If an error occurs during streaming, a chunk with Error set will be sent
	// and then the channel will be closed.
	//
	// Errors:
	//   - context.Canceled if ctx was canceled
	//   - context.DeadlineExceeded if timeout exceeded
	//   - *APIError if the provider API returns an error
	//   - *ConfigError if the request is invalid or provider not configured
	Stream(ctx context.Context, req CompletionRequest) (<-chan StreamChunk, error)

	// Models returns a list of available models from this provider.
	// The list should be cached where appropriate and refreshed periodically.
	//
	// Returns:
	//   - []ModelInfo: List of available models with their capabilities
	Models() []ModelInfo

	// IsAvailable checks if the provider is currently available and properly configured.
	// This may perform a lightweight health check (e.g., check API key validity).
	//
	// Returns:
	//   - bool: true if the provider is available, false otherwise
	IsAvailable() bool
}

// CompletionRequest represents a request to generate a completion.
// Goroutine-safe: CompletionRequest values should be treated as immutable after creation.
type CompletionRequest struct {
	// Messages is the conversation history including the user's current request
	Messages []Message
	// Model is the model ID to use (e.g., "gpt-4", "claude-3-opus")
	Model string
	// Temperature controls randomness (0.0 = deterministic, 1.0 = very random)
	Temperature float64
	// MaxTokens is the maximum number of tokens to generate (0 for provider default)
	MaxTokens int
	// Tools are the tools available for the model to call
	Tools []ToolDefinition
	// Stream indicates whether to stream the response (for Complete method)
	Stream bool
	// Metadata contains additional provider-specific metadata
	Metadata map[string]string
	// TopP is the nucleus sampling parameter (optional, defaults to 1.0)
	TopP float64
	// FrequencyPenalty decreases likelihood of repeating tokens (optional, defaults to 0.0)
	FrequencyPenalty float64
	// PresencePenalty decreases likelihood of repeating topics (optional, defaults to 0.0)
	PresencePenalty float64
	// StopSequences are sequences where generation will stop
	StopSequences []string
	// ToolChoice controls how the model uses the provided Tools. Valid values
	// (OpenAI-compatible): "auto" (model decides), "none" (never call tools),
	// "required" (must call at least one tool), or a specific tool object.
	//
	// Empty string = "use the client's default" — the OpenAI-compatible client
	// in internal/llm/client.go treats empty as "auto", preserving the
	// pre-existing behavior for all callers that don't set this field.
	//
	// The orchestrator worker sets "required" on iteration 0 (when a tool_hint
	// is present) to force evidence-gathering on the first turn, then "" (auto)
	// thereafter. The planner and synthesizer set "none" as a belt-and-suspenders
	// guard since they have no tools in their request anyway.
	ToolChoice string
}

// CompletionResponse represents the response from a completion request.
// Goroutine-safe: CompletionResponse values should be treated as immutable after creation.
type CompletionResponse struct {
	// Content is the generated text content
	Content string
	// Model is the model that generated the response
	Model string
	// Usage contains token usage statistics
	Usage TokenUsage
	// ToolCalls are any tool calls requested by the model
	ToolCalls []ToolCall
	// FinishReason indicates why the generation stopped
	FinishReason FinishReason
	// Metadata contains additional provider-specific response metadata
	Metadata map[string]string
}

// ModelInfo represents information about an available model.
// Goroutine-safe: ModelInfo values should be treated as immutable after creation.
type ModelInfo struct {
	// ID is the unique model identifier (e.g., "gpt-4-turbo")
	ID string
	// Provider is the provider name (e.g., "openai", "anthropic")
	Provider string
	// ContextLength is the maximum context window size in tokens
	ContextLength int
	// SupportsTools indicates whether the model supports function/tool calling
	SupportsTools bool
	// SupportsStreaming indicates whether the model supports streaming responses
	SupportsStreaming bool
	// SupportsImages indicates whether the model supports image inputs
	SupportsImages bool
	// SupportsAudio indicates whether the model supports audio inputs/outputs
	SupportsAudio bool
	// MaxOutputTokens is the maximum number of tokens the model can generate
	MaxOutputTokens int
	// Description is a human-readable description of the model
	Description string
	// Tags are additional tags for filtering/models
	Tags []string
}

// ProviderConfig contains configuration for a provider.
// Goroutine-safe: ProviderConfig values should be treated as immutable after creation.
type ProviderConfig struct {
	// BaseURL is the base URL for the provider API (e.g., "https://api.openai.com/v1")
	BaseURL string
	// APIKey is the authentication API key
	APIKey string
	// DefaultModel is the default model to use if not specified
	DefaultModel string
	// MaxRetries is the maximum number of retries for failed requests
	MaxRetries int
	// Timeout is the timeout for individual requests
	Timeout time.Duration
	// Organization is the organization ID (for OpenAI)
	Organization string
	// Headers are additional headers to include in requests
	Headers map[string]string
	// TLSConfig is the TLS configuration (for custom certificate handling)
	TLSConfig interface{} // *tls.Config
	// EnableCompression enables request compression if supported
	EnableCompression bool
	// Logger is an optional custom logger (defaults to slog.Default())
	Logger *slog.Logger
}

// DefaultProviderConfig returns a provider config with sensible defaults.
func DefaultProviderConfig() ProviderConfig {
	return ProviderConfig{
		BaseURL:          "",
		APIKey:           "",
		DefaultModel:     "",
		MaxRetries:       3,
		Timeout:          60 * time.Second,
		Headers:          make(map[string]string),
		EnableCompression: true,
		Logger:           slog.Default(),
	}
}

// Validate checks if the provider config is valid.
func (c ProviderConfig) Validate() error {
	if c.APIKey == "" {
		return NewConfigError("api_key", "API key is required", nil)
	}
	if c.Timeout <= 0 {
		return NewConfigError("timeout", "timeout must be positive", nil)
	}
	if c.MaxRetries < 0 {
		return NewConfigError("max_retries", "max_retries cannot be negative", nil)
	}
	return nil
}

// ==============================================================================
// Provider Registry
// ==============================================================================

// ProviderRegistry manages a collection of providers.
// Goroutine-safe: All methods are safe for concurrent use.
// Uses internal synchronization (RWMutex) for thread-safe operations.
type ProviderRegistry struct {
	mu        sync.RWMutex
	providers map[string]Provider
	logger    *slog.Logger
}

// NewProviderRegistry creates a new provider registry.
func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		providers: make(map[string]Provider),
		logger:    slog.Default(),
	}
}

// Register adds a provider to the registry.
// If a provider with the same name already exists, it will be replaced.
//
// Parameters:
//   - provider: The provider to register
//
// Returns:
//   - error: nil on success, error if validation fails
func (r *ProviderRegistry) Register(provider Provider) error {
	if provider == nil {
		return NewValidationError("provider", "provider cannot be nil", nil)
	}

	name := provider.Name()
	if name == "" {
		return NewValidationError("name", "provider name cannot be empty", name)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[name] = provider
	r.logger.Info("provider registered", "name", name)
	return nil
}

// Get retrieves a provider by name.
//
// Parameters:
//   - name: The provider name
//
// Returns:
//   - Provider: The provider if found
//   - error: ErrProviderNotFound if not found
func (r *ProviderRegistry) Get(name string) (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	provider, ok := r.providers[name]
	if !ok {
		return nil, ErrProviderNotFound
	}
	return provider, nil
}

// List returns all registered provider names.
//
// Returns:
//   - []string: List of provider names
func (r *ProviderRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	return names
}

// Remove removes a provider from the registry.
//
// Parameters:
//   - name: The provider name to remove
//
// Returns:
//   - error: nil on success, error if not found
func (r *ProviderRegistry) Remove(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.providers[name]; !ok {
		return ErrProviderNotFound
	}
	delete(r.providers, name)
	r.logger.Info("provider removed", "name", name)
	return nil
}

// AvailableProviders returns the names of all providers that are currently available.
//
// Returns:
//   - []string: List of available provider names
func (r *ProviderRegistry) AvailableProviders() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	available := make([]string, 0)
	for name, provider := range r.providers {
		if provider.IsAvailable() {
			available = append(available, name)
		}
	}
	return available
}

// AllModels returns all models from all registered providers.
//
// Returns:
//   - []ModelInfo: List of all available models
func (r *ProviderRegistry) AllModels() []ModelInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	models := make([]ModelInfo, 0)
	for _, provider := range r.providers {
		models = append(models, provider.Models()...)
	}
	return models
}

// ErrProviderNotFound indicates that a provider was not found in the registry.
var ErrProviderNotFound = NewValidationError("provider", "provider not found in registry", nil)