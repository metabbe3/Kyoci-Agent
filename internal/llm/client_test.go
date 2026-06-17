package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// ==============================================================================
// Test Helpers
// ==============================================================================

func createMockServer(response string, statusCode int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(response))
	}))
}

func createMockStreamingServer(chunks []string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
			return
		}

		for _, chunk := range chunks {
			w.Write([]byte("data: " + chunk + "\n\n"))
			flusher.Flush()
			time.Sleep(10 * time.Millisecond)
		}

		w.Write([]byte("data: [DONE]\n\n"))
		flusher.Flush()
	}))
}

// ==============================================================================
// OpenAIClient Tests
// ==============================================================================

func TestOpenAIClientCreation(t *testing.T) {
	config := kyoci.ProviderConfig{
		BaseURL:      "https://api.openai.com/v1",
		APIKey:       "test-key",
		DefaultModel: "gpt-4",
		MaxRetries:   3,
		Timeout:      60 * time.Second,
	}

	client, err := NewOpenAIClient("openai", config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	if client.Name() != "openai" {
		t.Errorf("Expected name 'openai', got '%s'", client.Name())
	}

	if !client.IsAvailable() {
		t.Error("Client should be available on creation")
	}
}

func TestOpenAIClientMissingBaseURL(t *testing.T) {
	config := kyoci.ProviderConfig{
		APIKey:       "test-key",
		DefaultModel: "gpt-4",
	}

	_, err := NewOpenAIClient("openai", config)
	if err == nil {
		t.Error("Expected error for missing base URL")
	}

	if !strings.Contains(err.Error(), "base_url") {
		t.Errorf("Expected error about base_url, got: %v", err)
	}
}

func TestOpenAIClientMissingAPIKey(t *testing.T) {
	config := kyoci.ProviderConfig{
		BaseURL:      "https://api.openai.com/v1",
		DefaultModel: "gpt-4",
	}

	_, err := NewOpenAIClient("openai", config)
	if err == nil {
		t.Error("Expected error for missing API key")
	}

	if !strings.Contains(err.Error(), "api_key") {
		t.Errorf("Expected error about api_key, got: %v", err)
	}
}

func TestOpenAIClientOllamaNoAPIKey(t *testing.T) {
	config := kyoci.ProviderConfig{
		BaseURL:      "http://localhost:11434/v1",
		DefaultModel: "llama2",
	}

	client, err := NewOpenAIClient("ollama", config)
	if err != nil {
		t.Fatalf("Ollama should not require API key: %v", err)
	}

	if client.Name() != "ollama" {
		t.Errorf("Expected name 'ollama', got '%s'", client.Name())
	}
}

// ==============================================================================
// Provider Factory Tests
// ==============================================================================

func TestProviderFactory(t *testing.T) {
	providers := []string{
		"openai", "anthropic", "ollama", "gemini", "zai",
		"groq", "mistral", "deepseek", "together", "fireworks", "xai",
	}

	for _, name := range providers {
		t.Run(name, func(t *testing.T) {
			config := kyoci.ProviderConfig{
				BaseURL:      "https://api.example.com/v1",
				APIKey:       "test-key",
				DefaultModel: "test-model",
				MaxRetries:   3,
				Timeout:      60 * time.Second,
			}

			if name == "ollama" {
				config.APIKey = ""
			}

			provider, err := NewProvider(name, config)
			if err != nil {
				t.Fatalf("Failed to create provider %s: %v", name, err)
			}

			if provider.Name() != name {
				t.Errorf("Expected name '%s', got '%s'", name, provider.Name())
			}

			if !provider.IsAvailable() {
				t.Errorf("Provider %s should be available", name)
			}

			models := provider.Models()
			if len(models) == 0 {
				t.Errorf("Provider %s should return at least one model", name)
			}

			if models[0].Provider != name {
				t.Errorf("Expected model provider '%s', got '%s'", name, models[0].Provider)
			}
		})
	}
}

func TestProviderFactoryAnthropicConfig(t *testing.T) {
	config := kyoci.ProviderConfig{
		BaseURL:      "https://api.anthropic.com/v1",
		APIKey:       "test-key",
		DefaultModel: "claude-3-opus",
	}

	provider, err := NewProvider("anthropic", config)
	if err != nil {
		t.Fatalf("Failed to create Anthropic provider: %v", err)
	}

	// Verify it's an OpenAIClient with x-api-key configuration
	if client, ok := provider.(*OpenAIClient); ok {
		if !client.useXAPIKey {
			t.Error("Anthropic provider should use x-api-key header")
		}
		if client.anthropicVersion != "2023-06-01" {
			t.Errorf("Expected anthropic-version '2023-06-01', got '%s'", client.anthropicVersion)
		}
	} else {
		t.Error("Provider should be an OpenAIClient")
	}
}

// ==============================================================================
// Provider Registry Tests
// ==============================================================================

func TestProviderRegistry(t *testing.T) {
	registry := NewProviderRegistry()

	// Test registration
	config := kyoci.ProviderConfig{
		BaseURL:      "https://api.openai.com/v1",
		APIKey:       "test-key",
		DefaultModel: "gpt-4",
	}

	provider, err := NewProvider("openai", config)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	err = registry.Register("openai", provider)
	if err != nil {
		t.Fatalf("Failed to register provider: %v", err)
	}

	// Test get
	retrieved, err := registry.Get("openai")
	if err != nil {
		t.Errorf("Failed to get provider: %v", err)
	}

	if retrieved.Name() != "openai" {
		t.Errorf("Expected name 'openai', got '%s'", retrieved.Name())
	}

	// Test list
	list := registry.List()
	if len(list) != 1 {
		t.Errorf("Expected 1 provider, got %d", len(list))
	}

	if _, ok := list["openai"]; !ok {
		t.Error("Expected 'openai' in list")
	}

	// Test get available
	available := registry.GetAvailable()
	if len(available) != 1 {
		t.Errorf("Expected 1 available provider, got %d", len(available))
	}

	// Test get non-existent
	_, err = registry.Get("nonexistent")
	if err == nil {
		t.Error("Expected error for non-existent provider")
	}
}

func TestProviderRegistryNilProvider(t *testing.T) {
	registry := NewProviderRegistry()

	err := registry.Register("test", nil)
	if err == nil {
		t.Error("Expected error for nil provider")
	}
}

// ==============================================================================
// Circuit Breaker Tests
// ==============================================================================

func TestCircuitBreakerClosedToOpen(t *testing.T) {
	cb := NewCircuitBreaker(3, 1*time.Second)

	if cb.State() != CircuitClosed {
		t.Errorf("Expected state Closed, got %v", cb.State())
	}

	if !cb.Allow() {
		t.Error("Circuit should allow requests when closed")
	}

	// Record failures to open the circuit
	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}

	if cb.State() != CircuitOpen {
		t.Errorf("Expected state Open, got %v", cb.State())
	}

	if cb.Allow() {
		t.Error("Circuit should not allow requests when open")
	}
}

func TestCircuitBreakerHalfOpenToClosed(t *testing.T) {
	cb := NewCircuitBreaker(3, 100*time.Millisecond)

	// Open the circuit
	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}

	if cb.State() != CircuitOpen {
		t.Errorf("Expected state Open, got %v", cb.State())
	}

	// Wait for reset timeout
	time.Sleep(150 * time.Millisecond)

	// Next allow should put circuit in half-open
	if !cb.Allow() {
		t.Error("Circuit should allow request after reset timeout")
	}

	if cb.State() != CircuitHalfOpen {
		t.Errorf("Expected state HalfOpen, got %v", cb.State())
	}

	// Record success to close the circuit
	cb.RecordSuccess()

	if cb.State() != CircuitClosed {
		t.Errorf("Expected state Closed, got %v", cb.State())
	}

	// Failures should be reset
	if !cb.Allow() {
		t.Error("Circuit should allow requests after successful half-open")
	}
}

func TestCircuitBreakerReset(t *testing.T) {
	cb := NewCircuitBreaker(3, 1*time.Second)

	// Open the circuit
	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}

	if cb.State() != CircuitOpen {
		t.Errorf("Expected state Open, got %v", cb.State())
	}

	// Manually reset
	cb.Reset()

	if cb.State() != CircuitClosed {
		t.Errorf("Expected state Closed after reset, got %v", cb.State())
	}

	if !cb.Allow() {
		t.Error("Circuit should allow requests after reset")
	}
}

// ==============================================================================
// Router Tests
// ==============================================================================

func TestRouterWithPreferredProvider(t *testing.T) {
	registry := NewProviderRegistry()

	config := kyoci.ProviderConfig{
		BaseURL:      "https://api.openai.com/v1",
		APIKey:       "test-key",
		DefaultModel: "gpt-4",
	}

	provider, _ := NewProvider("openai", config)
	registry.Register("openai", provider)

	router := NewRouter(registry, StrategyFallback)

	// We can't actually test the request without a mock server,
	// but we can test provider selection
	available := registry.GetAvailable()
	if len(available) == 0 {
		t.Error("Should have at least one available provider")
	}

	selected, err := router.selectFallback(available, "openai")
	if err != nil {
		t.Fatalf("Failed to select provider: %v", err)
	}

	if selected.Name() != "openai" {
		t.Errorf("Expected 'openai', got '%s'", selected.Name())
	}
}

func TestRouterRoundRobin(t *testing.T) {
	registry := NewProviderRegistry()

	// Register 3 providers
	for _, name := range []string{"openai", "anthropic", "ollama"} {
		config := kyoci.ProviderConfig{
			BaseURL:      "https://api.example.com/v1",
			APIKey:       "test-key",
			DefaultModel: "test-model",
		}
		if name == "ollama" {
			config.APIKey = ""
		}

		provider, _ := NewProvider(name, config)
		registry.Register(name, provider)
	}

	router := NewRouter(registry, StrategyRoundRobin)

	available := registry.GetAvailable()

	// Test round-robin selection
	providers := make(map[string]bool)
	for i := 0; i < 3; i++ {
		selected, err := router.selectRoundRobin(available)
		if err != nil {
			t.Fatalf("Failed to select provider: %v", err)
		}
		providers[selected.Name()] = true
	}

	// Should have selected all 3 providers
	for _, name := range []string{"openai", "anthropic", "ollama"} {
		if !providers[name] {
			t.Errorf("Expected to select provider '%s' in round-robin", name)
		}
	}
}

func TestRouterStrategy(t *testing.T) {
	registry := NewProviderRegistry()

	config := kyoci.ProviderConfig{
		BaseURL:      "https://api.openai.com/v1",
		APIKey:       "test-key",
		DefaultModel: "gpt-4",
	}

	provider, _ := NewProvider("openai", config)
	registry.Register("openai", provider)

	router := NewRouter(registry, StrategyFallback)

	if router.GetStrategy() != StrategyFallback {
		t.Errorf("Expected strategy %v, got %v", StrategyFallback, router.GetStrategy())
	}

	router.SetStrategy(StrategyFastest)
	if router.GetStrategy() != StrategyFastest {
		t.Errorf("Expected strategy %v, got %v", StrategyFastest, router.GetStrategy())
	}
}

func TestRouterNoAvailableProviders(t *testing.T) {
	registry := NewProviderRegistry()
	router := NewRouter(registry, StrategyFallback)

	req := kyoci.CompletionRequest{
		Messages: []kyoci.Message{
			{Role: kyoci.RoleUser, Content: "Hello"},
		},
	}

	_, err := router.Route(context.Background(), req, "openai")
	if err == nil {
		t.Error("Expected error when no providers available")
	}

	if !strings.Contains(err.Error(), "no available providers") {
		t.Errorf("Expected error about no available providers, got: %v", err)
	}
}

// ==============================================================================
// HTTP Request Tests with Mock Server
// ==============================================================================

func TestCompleteRequest(t *testing.T) {
	mockResponse := `{
		"id": "chatcmpl-123",
		"object": "chat.completion",
		"created": 1699012345,
		"model": "gpt-4",
		"choices": [{
			"index": 0,
			"message": {
				"role": "assistant",
				"content": "Hello, world!"
			},
			"finish_reason": "stop"
		}],
		"usage": {
			"prompt_tokens": 10,
			"completion_tokens": 5,
			"total_tokens": 15
		}
	}`

	server := createMockServer(mockResponse, http.StatusOK)
	defer server.Close()

	config := kyoci.ProviderConfig{
		BaseURL:      server.URL,
		APIKey:       "test-key",
		DefaultModel: "gpt-4",
		Timeout:      10 * time.Second,
	}

	client, err := NewOpenAIClient("openai", config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	req := kyoci.CompletionRequest{
		Messages: []kyoci.Message{
			{Role: kyoci.RoleUser, Content: "Hello"},
		},
	}

	resp, err := client.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("Complete request failed: %v", err)
	}

	if resp.Content != "Hello, world!" {
		t.Errorf("Expected content 'Hello, world!', got '%s'", resp.Content)
	}

	if resp.Model != "gpt-4" {
		t.Errorf("Expected model 'gpt-4', got '%s'", resp.Model)
	}

	if resp.Usage.TotalTokens != 15 {
		t.Errorf("Expected 15 total tokens, got %d", resp.Usage.TotalTokens)
	}
}

func TestCompleteRequestError(t *testing.T) {
	mockResponse := `{
		"error": {
			"message": "Invalid API key",
			"type": "invalid_request_error",
			"code": "invalid_api_key"
		}
	}`

	server := createMockServer(mockResponse, http.StatusUnauthorized)
	defer server.Close()

	config := kyoci.ProviderConfig{
		BaseURL:      server.URL,
		APIKey:       "invalid-key",
		DefaultModel: "gpt-4",
		Timeout:      10 * time.Second,
	}

	client, err := NewOpenAIClient("openai", config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	req := kyoci.CompletionRequest{
		Messages: []kyoci.Message{
			{Role: kyoci.RoleUser, Content: "Hello"},
		},
	}

	// Make 5 requests to trigger circuit breaker opening (threshold is 5)
	for i := 0; i < 5; i++ {
		_, err = client.Complete(context.Background(), req)
		if err == nil {
			t.Error("Expected error for invalid request")
		}
	}

	if !strings.Contains(err.Error(), "API error") {
		t.Errorf("Expected API error, got: %v", err)
	}

	// Circuit breaker should be affected after 5 failures
	if client.circuitBreaker.State() != CircuitOpen {
		t.Error("Circuit breaker should be open after 5 failures")
	}
}

func TestStreamingRequest(t *testing.T) {
	chunk1 := `{"id":"chatcmpl-123","object":"chat.completion.chunk","created":1699012345,"model":"gpt-4","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}`

	chunk2 := `{"id":"chatcmpl-123","object":"chat.completion.chunk","created":1699012345,"model":"gpt-4","choices":[{"index":0,"delta":{"content":", world!"},"finish_reason":null}]}`

	chunk3 := `{"id":"chatcmpl-123","object":"chat.completion.chunk","created":1699012345,"model":"gpt-4","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`

	server := createMockStreamingServer([]string{chunk1, chunk2, chunk3})
	defer server.Close()

	config := kyoci.ProviderConfig{
		BaseURL:      server.URL,
		APIKey:       "test-key",
		DefaultModel: "gpt-4",
		Timeout:      10 * time.Second,
	}

	client, err := NewOpenAIClient("openai", config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	req := kyoci.CompletionRequest{
		Messages: []kyoci.Message{
			{Role: kyoci.RoleUser, Content: "Hello"},
		},
	}

	ch, err := client.Stream(context.Background(), req)
	if err != nil {
		t.Fatalf("Stream request failed: %v", err)
	}

	var fullContent strings.Builder
	chunkCount := 0
	for chunk := range ch {
		chunkCount++

		if chunk.Error != nil {
			t.Fatalf("Received error chunk: %v", chunk.Error)
		}

		if chunk.Content != "" && !chunk.Done {
			fullContent.WriteString(chunk.Content)
		}

		if chunk.Done {
			if chunk.Usage == nil {
				t.Error("Final chunk should have usage info")
			} else if chunk.Usage.TotalTokens != 15 {
				t.Errorf("Expected 15 total tokens, got %d", chunk.Usage.TotalTokens)
			}
		}
	}

	if chunkCount == 0 {
		t.Error("Expected at least one chunk")
	}

	if fullContent.String() != "Hello, world!" {
		t.Errorf("Expected content 'Hello, world!', got '%s'", fullContent.String())
	}
}

func TestRateLimitHandling(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	config := kyoci.ProviderConfig{
		BaseURL:      server.URL,
		APIKey:       "test-key",
		DefaultModel: "gpt-4",
		Timeout:      10 * time.Second,
		MaxRetries:   1,
	}

	client, err := NewOpenAIClient("openai", config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	req := kyoci.CompletionRequest{
		Messages: []kyoci.Message{
			{Role: kyoci.RoleUser, Content: "Hello"},
		},
	}

	start := time.Now()
	_, err = client.Complete(context.Background(), req)
	duration := time.Since(start)

	if err == nil {
		t.Error("Expected error for rate limit")
	}

	// Should have waited at least 1 second for retry-after
	if duration < 1*time.Second {
		t.Errorf("Expected to wait at least 1 second for rate limit retry, got %v", duration)
	}
}

func TestContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := kyoci.ProviderConfig{
		BaseURL:      server.URL,
		APIKey:       "test-key",
		DefaultModel: "gpt-4",
		Timeout:      10 * time.Second,
	}

	client, err := NewOpenAIClient("openai", config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	req := kyoci.CompletionRequest{
		Messages: []kyoci.Message{
			{Role: kyoci.RoleUser, Content: "Hello"},
		},
	}

	_, err = client.Complete(ctx, req)
	if err == nil {
		t.Error("Expected error for context cancellation")
	}

	if !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("Expected context canceled error, got: %v", err)
	}
}

// ==============================================================================
// Message/Tool Conversion Tests
// ==============================================================================

func TestConvertMessages(t *testing.T) {
	config := kyoci.ProviderConfig{
		BaseURL:      "https://api.openai.com/v1",
		APIKey:       "test-key",
		DefaultModel: "gpt-4",
	}

	client, err := NewOpenAIClient("openai", config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	messages := []kyoci.Message{
		{Role: kyoci.RoleSystem, Content: "You are a helpful assistant"},
		{Role: kyoci.RoleUser, Content: "Hello"},
		{Role: kyoci.RoleAssistant, Content: "Hi there!"},
	}

	apiMessages := client.convertMessages(messages)

	if len(apiMessages) != 3 {
		t.Errorf("Expected 3 messages, got %d", len(apiMessages))
	}

	if apiMessages[0].Role != "system" {
		t.Errorf("Expected role 'system', got '%s'", apiMessages[0].Role)
	}

	if apiMessages[1].Role != "user" {
		t.Errorf("Expected role 'user', got '%s'", apiMessages[1].Role)
	}

	if apiMessages[2].Role != "assistant" {
		t.Errorf("Expected role 'assistant', got '%s'", apiMessages[2].Role)
	}
}

func TestConvertTools(t *testing.T) {
	config := kyoci.ProviderConfig{
		BaseURL:      "https://api.openai.com/v1",
		APIKey:       "test-key",
		DefaultModel: "gpt-4",
	}

	client, err := NewOpenAIClient("openai", config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	tools := []kyoci.ToolDefinition{
		{
			Name:        "get_weather",
			Description: "Get the current weather",
			Parameters: []kyoci.ToolParameter{
				{Name: "location", Type: "string", Description: "City name", Required: true},
			},
		},
	}

	apiTools := client.convertTools(tools)

	if len(apiTools) != 1 {
		t.Errorf("Expected 1 tool, got %d", len(apiTools))
	}

	if apiTools[0].Type != "function" {
		t.Errorf("Expected type 'function', got '%s'", apiTools[0].Type)
	}

	if apiTools[0].Function.Name != "get_weather" {
		t.Errorf("Expected name 'get_weather', got '%s'", apiTools[0].Function.Name)
	}
}

// ==============================================================================
// Retry Logic Tests
// ==============================================================================

func TestRetryLogic(t *testing.T) {
	attemptCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		if attemptCount < 3 {
			// Return 429 (rate limit) to trigger retry
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"id": "chatcmpl-123",
			"object": "chat.completion",
			"created": 1699012345,
			"model": "gpt-4",
			"choices": [{
				"index": 0,
				"message": {"role": "assistant", "content": "Success"},
				"finish_reason": "stop"
			}],
			"usage": {"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}
		}`))
	}))
	defer server.Close()

	config := kyoci.ProviderConfig{
		BaseURL:      server.URL,
		APIKey:       "test-key",
		DefaultModel: "gpt-4",
		Timeout:      10 * time.Second,
		MaxRetries:   5,
	}

	client, err := NewOpenAIClient("openai", config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	req := kyoci.CompletionRequest{
		Messages: []kyoci.Message{
			{Role: kyoci.RoleUser, Content: "Hello"},
		},
	}

	resp, err := client.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("Complete request failed: %v", err)
	}

	if resp.Content != "Success" {
		t.Errorf("Expected content 'Success', got '%s'", resp.Content)
	}

	// Should have retried twice (initial attempt + 2 retries = 3 total)
	if attemptCount != 3 {
		t.Errorf("Expected 3 attempts (2 retries), got %d", attemptCount)
	}
}
