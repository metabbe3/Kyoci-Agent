package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/metabbe3/Kyoci-Agent/internal/apperr"
	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// ==============================================================================
// OpenAI-Compatible Client
// ==============================================================================

// OpenAIClient implements the kyoci.Provider interface for OpenAI-compatible APIs.
type OpenAIClient struct {
	name             string
	config           kyoci.ProviderConfig
	client           *http.Client
	circuitBreaker   *CircuitBreaker
	modelsCache      []kyoci.ModelInfo
	modelsCacheAt    int64
	mu               sync.RWMutex
	lastError        error
	logger           *slog.Logger
	customHeaders    map[string]string
	useXAPIKey       bool   // For Anthropic: use x-api-key header instead of Bearer
	anthropicVersion string // For Anthropic: anthropic-version header
	isOllama         bool   // Ollama needs tool history flattened to text
}

// NewOpenAIClient creates a new OpenAI-compatible client and returns it as the
// kyoci.Provider interface, so callers depend on the contract rather than the
// concrete type. Use AsOpenAIClient when the concrete type is genuinely needed
// (provider-specific fluent builders, or test introspection).
func NewOpenAIClient(name string, config kyoci.ProviderConfig) (kyoci.Provider, error) {
	if config.BaseURL == "" {
		return nil, fmt.Errorf("provider %s: base_url is required", name)
	}
	if config.APIKey == "" && name != "ollama" && name != "lmstudio" {
		return nil, fmt.Errorf("provider %s: api_key is required", name)
	}

	client := &http.Client{
		Timeout: config.Timeout,
	}

	// Default circuit breaker: opens after 5 consecutive failures, resets after 30s
	circuitBreaker := NewCircuitBreaker(5, 30*time.Second)

	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &OpenAIClient{
		name:             name,
		config:           config,
		client:           client,
		circuitBreaker:   circuitBreaker,
		logger:           logger,
		customHeaders:    config.Headers,
		useXAPIKey:       false,
		anthropicVersion: "",
		isOllama:         name == "ollama",
	}, nil
}

// WithCustomHeaders sets custom headers for the client.
func (c *OpenAIClient) WithCustomHeaders(headers map[string]string) *OpenAIClient {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.customHeaders = headers
	return c
}

// WithXAPIKey configures the client to use x-api-key header (for Anthropic).
func (c *OpenAIClient) WithXAPIKey(version string) *OpenAIClient {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.useXAPIKey = true
	c.anthropicVersion = version
	return c
}

// AsOpenAIClient returns the concrete *OpenAIClient behind a kyoci.Provider, or
// nil if the provider is a different type. Use it for provider-specific
// configuration (the fluent auth builders above) or test introspection; normal
// callers should keep working with the kyoci.Provider interface.
func AsOpenAIClient(p kyoci.Provider) *OpenAIClient {
	if c, ok := p.(*OpenAIClient); ok {
		return c
	}
	return nil
}

// Name returns the provider name.
func (c *OpenAIClient) Name() string {
	return c.name
}

// buildPayload constructs the OpenAI-compatible request body shared by Complete
// and Stream, removing the verbatim payload-builder duplication between them.
// stream selects the "stream" field. It returns the mutable payload map (the
// Ollama context-strip retry path mutates payload["messages"]) and the resolved
// model name (used by the per-method debug log).
func (c *OpenAIClient) buildPayload(req kyoci.CompletionRequest, stream bool) (map[string]any, string) {
	model := req.Model
	if model == "" {
		model = c.config.DefaultModel
	}
	payload := map[string]any{
		"model":    model,
		"messages": c.convertMessages(req.Messages),
		"stream":   stream,
	}
	if req.Temperature > 0 {
		payload["temperature"] = req.Temperature
	}
	if req.MaxTokens > 0 {
		payload["max_tokens"] = req.MaxTokens
	}
	if req.TopP > 0 {
		payload["top_p"] = req.TopP
	}
	if req.FrequencyPenalty > 0 {
		payload["frequency_penalty"] = req.FrequencyPenalty
	}
	if req.PresencePenalty > 0 {
		payload["presence_penalty"] = req.PresencePenalty
	}
	if len(req.StopSequences) > 0 {
		payload["stop"] = req.StopSequences
	}
	if len(req.Tools) > 0 {
		payload["tools"] = c.convertTools(req.Tools)
		// tool_choice: prefer the caller's explicit choice (e.g. "required" to
		// force at least one tool call, "none" to forbid). Default to "auto" —
		// many models won't use tools without this explicit signal.
		tc := req.ToolChoice
		if tc == "" {
			tc = "auto"
		}
		payload["tool_choice"] = tc
	}
	return payload, model
}

// Complete performs a non-streaming completion request.
func (c *OpenAIClient) Complete(ctx context.Context, req kyoci.CompletionRequest) (*kyoci.CompletionResponse, error) {
	startTime := time.Now()

	// Check circuit breaker
	if !c.circuitBreaker.Allow() {
		return nil, fmt.Errorf("provider %s: %w", c.name, apperr.ErrCircuitOpen)
	}

	// Prepare request
	payload, model := c.buildPayload(req, false)

	c.logger.Debug("sending completion request",
		"provider", c.name,
		"model", model,
		"messages", len(req.Messages),
		"stream", false)

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("provider %s: failed to marshal request: %w", c.name, err)
	}

	// Make request with retry logic
	var resp *http.Response
	var lastErr error
	var attempts int

	for attempts = 0; attempts <= c.config.MaxRetries; attempts++ {
		if attempts > 0 {
			// Exponential backoff with cap
			backoff := time.Duration(1<<uint(attempts-1)) * time.Second
			if backoff > 5*time.Second {
				backoff = 5 * time.Second // cap at 5s
			}

			// Check if we have enough budget left to retry
			deadline, ok := ctx.Deadline()
			if ok {
				remaining := time.Until(deadline)
				if remaining < backoff+5*time.Second {
					// Not enough time left — return last error instead of wasting budget
					if lastErr != nil {
						return nil, fmt.Errorf("provider %s: context deadline approaching (remaining %v), last error: %w", c.name, remaining.Round(time.Second), lastErr)
					}
					return nil, fmt.Errorf("provider %s: context deadline approaching (remaining %v)", c.name, remaining.Round(time.Second))
				}
			}

			c.logger.Info("retrying request",
				"provider", c.name,
				"attempt", attempts,
				"backoff", backoff)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return nil, fmt.Errorf("provider %s: context canceled during backoff: %w", c.name, ctx.Err())
			}
		}

		httpReq, err := http.NewRequestWithContext(ctx, "POST", c.config.BaseURL+"/chat/completions", bytes.NewBuffer(jsonPayload))
		if err != nil {
			lastErr = fmt.Errorf("provider %s: failed to create request: %w", c.name, err)
			continue
		}

		httpReq.Header.Set("Content-Type", "application/json")

		// Set authentication header
		if c.useXAPIKey {
			httpReq.Header.Set("x-api-key", c.config.APIKey)
			if c.anthropicVersion != "" {
				httpReq.Header.Set("anthropic-version", c.anthropicVersion)
			}
		} else if c.config.APIKey != "" {
			httpReq.Header.Set("Authorization", "Bearer "+c.config.APIKey)
		}

		// Set custom headers
		c.mu.RLock()
		for key, value := range c.customHeaders {
			httpReq.Header.Set(key, value)
		}
		c.mu.RUnlock()

		resp, err = c.client.Do(httpReq)
		if err != nil {
			lastErr = fmt.Errorf("provider %s: request failed: %w", c.name, err)
			continue
		}

		// Check for rate limiting
		if resp.StatusCode == http.StatusTooManyRequests {
			retryAfter := c.parseRetryAfter(resp.Header.Get("Retry-After"))
			if retryAfter > 0 {
				c.logger.Warn("rate limited, will retry",
					"provider", c.name,
					"retry_after", retryAfter)
				resp.Body.Close()
				time.Sleep(retryAfter)
				continue
			}
		}

		// Ollama XML parser crash — retry with stripped context
		// This happens when conversation history contains HTML/CSS/code
		// that breaks Ollama's internal XML template for tool calling
		if c.isOllama && resp.StatusCode == http.StatusInternalServerError && attempts < c.config.MaxRetries {
			errorBody, _ := io.ReadAll(resp.Body)
			errStr := string(errorBody)
			resp.Body.Close()

			if strings.Contains(errStr, "XML syntax error") {
				c.logger.Warn("ollama XML error — retrying with stripped context",
					"provider", c.name,
					"attempt", attempts+1,
					"original_msgs", len(req.Messages))

				// Strip to bare minimum: system + last user message only
				stripped := c.stripToMinimal(req.Messages)
				req.Messages = stripped

				// Rebuild payload
				payload["messages"] = c.convertMessages(stripped)
				jsonPayload, err = json.Marshal(payload)
				if err != nil {
					return nil, fmt.Errorf("provider %s: failed to marshal stripped request: %w", c.name, err)
				}
				continue // retry with stripped context
			}

			// Other 500 error — record and return
			c.circuitBreaker.RecordFailure()
			lastErr = fmt.Errorf("provider %s: API error (status %d): %s", c.name, resp.StatusCode, errStr)
			c.mu.Lock()
			c.lastError = lastErr
			c.mu.Unlock()
			return nil, lastErr
		}

		break
	}

	if resp == nil {
		c.circuitBreaker.RecordFailure()
		c.mu.Lock()
		c.lastError = lastErr
		c.mu.Unlock()
		return nil, lastErr
	}
	defer resp.Body.Close()

	// Parse response
	if resp.StatusCode != http.StatusOK {
		c.circuitBreaker.RecordFailure()
		errorBody, _ := io.ReadAll(resp.Body)
		lastErr = fmt.Errorf("provider %s: API error (status %d): %s", c.name, resp.StatusCode, string(errorBody))
		c.mu.Lock()
		c.lastError = lastErr
		c.mu.Unlock()
		return nil, lastErr
	}

	var apiResponse ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResponse); err != nil {
		c.circuitBreaker.RecordFailure()
		lastErr = fmt.Errorf("provider %s: failed to decode response: %w", c.name, err)
		c.mu.Lock()
		c.lastError = lastErr
		c.mu.Unlock()
		return nil, lastErr
	}

	// Record success
	c.circuitBreaker.RecordSuccess()
	c.mu.Lock()
	c.lastError = nil
	c.mu.Unlock()

	// Convert response
	kyociResp := c.convertResponse(&apiResponse)

	duration := time.Since(startTime)
	c.logger.Info("completion request successful",
		"provider", c.name,
		"model", apiResponse.Model,
		"duration", duration,
		"prompt_tokens", apiResponse.Usage.PromptTokens,
		"completion_tokens", apiResponse.Usage.CompletionTokens)

	return kyociResp, nil
}

// Stream performs a streaming completion request.
func (c *OpenAIClient) Stream(ctx context.Context, req kyoci.CompletionRequest) (<-chan kyoci.StreamChunk, error) {
	// Check circuit breaker
	if !c.circuitBreaker.Allow() {
		return nil, fmt.Errorf("provider %s: %w", c.name, apperr.ErrCircuitOpen)
	}

	// Prepare request
	payload, model := c.buildPayload(req, true)

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("provider %s: failed to marshal request: %w", c.name, err)
	}

	c.logger.Debug("sending streaming request",
		"provider", c.name,
		"model", model,
		"messages", len(req.Messages),
		"stream", true)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.config.BaseURL+"/chat/completions", bytes.NewBuffer(jsonPayload))
	if err != nil {
		return nil, fmt.Errorf("provider %s: failed to create request: %w", c.name, err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	// Set authentication header
	if c.useXAPIKey {
		httpReq.Header.Set("x-api-key", c.config.APIKey)
		if c.anthropicVersion != "" {
			httpReq.Header.Set("anthropic-version", c.anthropicVersion)
		}
	} else if c.config.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	}

	// Set custom headers
	c.mu.RLock()
	for key, value := range c.customHeaders {
		httpReq.Header.Set(key, value)
	}
	c.mu.RUnlock()

	resp, err := c.client.Do(httpReq)
	if err != nil {
		c.circuitBreaker.RecordFailure()
		return nil, fmt.Errorf("provider %s: request failed: %w", c.name, err)
	}

	if resp.StatusCode != http.StatusOK {
		c.circuitBreaker.RecordFailure()
		errorBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("provider %s: API error (status %d): %s", c.name, resp.StatusCode, string(errorBody))
	}

	// Create streaming channel
	ch := make(chan kyoci.StreamChunk, 10)

	go func() {
		defer close(ch)
		defer resp.Body.Close()

		// Cancel reader: closing the body on ctx cancellation unblocks
		// scanner.Scan (which otherwise blocks on the network read until EOF).
		// streamDone lets the watcher exit cleanly when the reader finishes
		// normally, so it never leaks.
		streamDone := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				resp.Body.Close()
			case <-streamDone:
			}
		}()
		defer close(streamDone)

		scanner := bufio.NewScanner(resp.Body)
		var fullContent strings.Builder
		var totalPromptTokens, totalCompletionTokens int

		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				break
			}

			var chunk ChatCompletionChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				c.logger.Error("failed to decode streaming chunk", "provider", c.name, "error", err)
				ch <- kyoci.StreamError(fmt.Errorf("provider %s: failed to decode chunk: %w", c.name, err))
				c.circuitBreaker.RecordFailure()
				return
			}

			// Process delta
			if len(chunk.Choices) > 0 {
				delta := chunk.Choices[0].Delta
				finishReason := chunk.Choices[0].FinishReason

				if delta.Content != "" {
					fullContent.WriteString(delta.Content)
					ch <- kyoci.ContentChunk(delta.Content)
				}

				if finishReason != "" {
					// Final chunk
					totalCompletionTokens = chunk.Usage.CompletionTokens
					totalPromptTokens = chunk.Usage.PromptTokens

					usage := kyoci.TokenUsage{
						PromptTokens:     totalPromptTokens,
						CompletionTokens: totalCompletionTokens,
						TotalTokens:      totalPromptTokens + totalCompletionTokens,
					}

					ch <- kyoci.FinalChunk(fullContent.String(), usage, kyoci.FinishReason(finishReason))
					c.circuitBreaker.RecordSuccess()
					return
				}
			}
		}

		if ctx.Err() != nil {
			// Stream was canceled (caller deadline/cancel) — surface it rather
			// than the secondary "read on closed body" error from scanner.Err.
			ch <- kyoci.StreamError(fmt.Errorf("provider %s: stream canceled: %w", c.name, ctx.Err()))
			return
		}
		if err := scanner.Err(); err != nil {
			c.logger.Error("streaming error", "provider", c.name, "error", err)
			ch <- kyoci.StreamError(fmt.Errorf("provider %s: streaming error: %w", c.name, err))
			c.circuitBreaker.RecordFailure()
		}
	}()

	return ch, nil
}

// Models queries the provider's /v1/models endpoint for all available models.
// Results cached for 60 seconds to avoid spamming the provider.
func (c *OpenAIClient) Models() []kyoci.ModelInfo {
	// Return cached if fresh (60s TTL).
	now := time.Now().Unix()
	if c.modelsCache != nil && (now - c.modelsCacheAt) < 60 {
		return c.modelsCache
	}
	result := c.fetchModels()
	c.modelsCache = result
	c.modelsCacheAt = now
	return result
}

func (c *OpenAIClient) fetchModels() []kyoci.ModelInfo {
	type modelsResponse struct {
		Data []struct {
			ID      string `json:"id"`
			OwnedBy string `json:"owned_by,omitempty"`
		} `json:"data"`
	}
	url := strings.TrimRight(c.config.BaseURL, "/") + "/models"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return c.fallbackModels()
	}
	if c.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return c.fallbackModels()
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return c.fallbackModels()
	}
	var mr modelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&mr); err != nil {
		return c.fallbackModels()
	}
	out := make([]kyoci.ModelInfo, 0, len(mr.Data))
	for _, m := range mr.Data {
		isDefault := m.ID == c.config.DefaultModel
		tags := []string{}
		if isDefault { tags = append(tags, "default") }
		out = append(out, kyoci.ModelInfo{
			ID: m.ID, Provider: c.name, ContextLength: 128000,
			SupportsTools: true, SupportsStreaming: true, MaxOutputTokens: 8192,
			Description: m.OwnedBy, Tags: tags,
		})
	}
	if len(out) == 0 { return c.fallbackModels() }
	return out
}

func (c *OpenAIClient) fallbackModels() []kyoci.ModelInfo {
	return []kyoci.ModelInfo{
		{ID: c.config.DefaultModel, Provider: c.name, ContextLength: 128000,
			SupportsTools: true, SupportsStreaming: true, MaxOutputTokens: 8192,
			Description: fmt.Sprintf("Default for %s", c.name), Tags: []string{"default"}},
	}
}

// IsAvailable checks if the provider is available.
func (c *OpenAIClient) IsAvailable() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastError == nil && c.circuitBreaker.State() != CircuitOpen
}

// parseRetryAfter parses the Retry-After header.
func (c *OpenAIClient) parseRetryAfter(header string) time.Duration {
	if header == "" {
		return 0
	}

	// Try parsing as integer seconds
	if seconds, err := strconv.Atoi(header); err == nil {
		return time.Duration(seconds) * time.Second
	}

	// Try parsing as HTTP date
	if t, err := http.ParseTime(header); err == nil {
		return time.Until(t)
	}

	return 0
}

// convertMessages converts kyoci messages to API format.
// For Ollama, tool calls and tool results in conversation history are flattened
// to text because Ollama's OpenAI-compatible API chokes on structured tool_calls
// in the message history (error: "expected element type <function> but have <parameter>").
func (c *OpenAIClient) convertMessages(messages []kyoci.Message) []APIMessage {
	if !c.isOllama {
		// Standard OpenAI format — works for OpenAI, Anthropic, etc.
		apiMessages := make([]APIMessage, len(messages))
		for i, msg := range messages {
			apiMessages[i] = APIMessage{
				Role:    msg.Role.String(),
				Content: msg.Content,
			}
			if msg.Name != "" {
				apiMessages[i].Name = msg.Name
			}
			if msg.ToolCallID != "" {
				apiMessages[i].ToolCallID = msg.ToolCallID
			}
			if len(msg.ToolCalls) > 0 {
				apiMessages[i].ToolCalls = c.convertToolCalls(msg.ToolCalls)
			}
		}
		return apiMessages
	}

	// Ollama mode: flatten tool history to plain text
	// IMPORTANT: Ollama uses XML internally for tool calling with qwen models.
	// Raw HTML/CSS/JS in tool results contains <, >, & that crash Ollama's XML parser.
	// We must: (1) escape XML chars, (2) truncate long results, (3) use fenced blocks.
	apiMessages := make([]APIMessage, 0, len(messages))
	for _, msg := range messages {
		switch msg.Role {
		case kyoci.RoleSystem:
			apiMessages = append(apiMessages, APIMessage{
				Role:    "system",
				Content: sanitizeForOllama(msg.Content, 8000),
			})

		case kyoci.RoleUser:
			apiMessages = append(apiMessages, APIMessage{
				Role:    "user",
				Content: sanitizeForOllama(msg.Content, 8000),
			})

		case kyoci.RoleAssistant:
			// Flatten assistant messages: if they have tool calls, embed as text
			content := sanitizeForOllama(msg.Content, 4000)
			if len(msg.ToolCalls) > 0 {
				for _, tc := range msg.ToolCalls {
					// Escape arguments too — JSON with HTML inside breaks Ollama
					argEscaped := sanitizeForOllama(tc.Arguments, 500)
					content += "\n\n[Tool Call: " + tc.Name + "(" + argEscaped + ")]"
				}
			}
			apiMessages = append(apiMessages, APIMessage{
				Role:    "assistant",
				Content: content,
			})

		case kyoci.RoleTool:
			// Convert tool results to user messages with text
			// Truncate aggressively — Ollama chokes on large payloads
			resultContent := sanitizeForOllama(msg.Content, 1500)
			apiMessages = append(apiMessages, APIMessage{
				Role:    "user",
				Content: "[Tool Result for " + msg.ToolCallID + "]: " + resultContent,
			})

		default:
			apiMessages = append(apiMessages, APIMessage{
				Role:    msg.Role.String(),
				Content: sanitizeForOllama(msg.Content, 4000),
			})
		}
	}
	return apiMessages
}

// sanitizeForOllama makes text safe for Ollama's internal XML parser.
// - Escapes raw < > & that would break XML parsing
// - Truncates to maxLen characters (Ollama truncates long messages anyway,
//   and mid-truncation is what causes "unexpected EOF")
func sanitizeForOllama(s string, maxLen int) string {
	if s == "" {
		return s
	}
	// Escape XML-sensitive characters
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	// Truncate if too long
	if len(s) > maxLen {
		s = s[:maxLen] + "\n... [truncated for context limit]"
	}
	return s
}

// stripToMinimal reduces conversation history to system prompt + last user message.
// Used as a fallback when Ollama's XML parser crashes on complex context.
func (c *OpenAIClient) stripToMinimal(messages []kyoci.Message) []kyoci.Message {
	if len(messages) <= 2 {
		return messages
	}

	var result []kyoci.Message

	// Keep system message if present
	if messages[0].Role == kyoci.RoleSystem {
		result = append(result, messages[0])
	}

	// Find the last user message
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == kyoci.RoleUser {
			result = append(result, messages[i])
			break
		}
	}

	if len(result) == 0 {
		// Fallback: just return the last message
		return messages[len(messages)-1:]
	}

	return result
}

// convertTools converts kyoci tool definitions to API format.
func (c *OpenAIClient) convertTools(tools []kyoci.ToolDefinition) []APITool {
	apiTools := make([]APITool, len(tools))
	for i, tool := range tools {
		apiTools[i] = APITool{
			Type: "function",
			Function: APIToolFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.ToJSONSchema(),
			},
		}
	}
	return apiTools
}

// convertToolCalls converts kyoci tool calls to API format.
func (c *OpenAIClient) convertToolCalls(toolCalls []kyoci.ToolCall) []APIToolCall {
	apiCalls := make([]APIToolCall, len(toolCalls))
	for i, tc := range toolCalls {
		apiCalls[i] = APIToolCall{
			ID:       tc.ID,
			Type:     "function",
			Function: APIToolCallFunction{Name: tc.Name, Arguments: tc.Arguments},
		}
	}
	return apiCalls
}

// convertResponse converts API response to kyoci response.
func (c *OpenAIClient) convertResponse(apiResp *ChatCompletionResponse) *kyoci.CompletionResponse {
	if len(apiResp.Choices) == 0 {
		return &kyoci.CompletionResponse{
			Content:      "",
			Model:        apiResp.Model,
			Usage:        kyoci.TokenUsage{},
			ToolCalls:    []kyoci.ToolCall{},
			FinishReason: kyoci.FinishStop,
		}
	}

	choice := apiResp.Choices[0]

	// Convert tool calls
	toolCalls := make([]kyoci.ToolCall, len(choice.Message.ToolCalls))
	for i, tc := range choice.Message.ToolCalls {
		toolCalls[i] = kyoci.ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		}
	}

	// Model-agnostic content normalization: some models (Gemma, o1-style) emit
	// their answer in a `reasoning` field and leave `content` empty. Fall back
	// to reasoning so the caller still sees the model's output.
	content := normalizeContent(choice.Message.Content, choice.Message.Reasoning)

	// Ollama mode: the model may echo [Tool Call: ...] text patterns that it
	// saw in the flattened conversation history. These are NOT real tool calls —
	// they're artifacts. Strip them from content so they don't leak to the user.
	// Also try to parse them into structured tool calls if none were returned natively.
	if c.isOllama {
		var parsedCalls []kyoci.ToolCall
		content, parsedCalls = extractTextToolCalls(content)
		if len(parsedCalls) > 0 && len(toolCalls) == 0 {
			toolCalls = parsedCalls
		}

		// Qwen and a few other Ollama models emit tool calls as bare JSON objects
		// in `content` (e.g. `{"name":"file","arguments":{...}}`) instead of in
		// the structured `tool_calls` field. Parse them out so the dispatcher can
		// execute them. Only do this when no native tool_calls were returned —
		// native calls are authoritative.
		if len(toolCalls) == 0 {
			var bareCalls []kyoci.ToolCall
			content, bareCalls = extractBareJSONToolCalls(content)
			if len(bareCalls) > 0 {
				toolCalls = bareCalls
			}
		}
	}

	return &kyoci.CompletionResponse{
		Content: content,
		Model:   apiResp.Model,
		Usage: kyoci.TokenUsage{
			PromptTokens:     apiResp.Usage.PromptTokens,
			CompletionTokens: apiResp.Usage.CompletionTokens,
			TotalTokens:      apiResp.Usage.TotalTokens,
		},
		ToolCalls:    toolCalls,
		FinishReason: kyoci.FinishReason(choice.FinishReason),
		Metadata:     make(map[string]string),
	}
}


// ==============================================================================
// API Types (OpenAI-compatible format)
// ==============================================================================

// ChatCompletionResponse represents a non-streaming chat completion response.
type ChatCompletionResponse struct {
	ID      string             `json:"id"`
	Object  string             `json:"object"`
	Created int64              `json:"created"`
	Model   string             `json:"model"`
	Choices []CompletionChoice `json:"choices"`
	Usage   CompletionUsage    `json:"usage"`
}

// CompletionChoice represents a completion choice.
type CompletionChoice struct {
	Index        int               `json:"index"`
	Message      CompletionMessage `json:"message"`
	FinishReason string            `json:"finish_reason"`
}

// CompletionMessage represents a message in a completion response.
type CompletionMessage struct {
	Role      string        `json:"role"`
	Content   string        `json:"content"`
	Reasoning string        `json:"reasoning,omitempty"` // Gemma/o1-style chain-of-thought field
	ToolCalls []APIToolCall `json:"tool_calls,omitempty"`
}

// CompletionUsage represents token usage.
type CompletionUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ChatCompletionChunk represents a streaming chat completion chunk.
type ChatCompletionChunk struct {
	ID      string                  `json:"id"`
	Object  string                  `json:"object"`
	Created int64                   `json:"created"`
	Model   string                  `json:"model"`
	Choices []CompletionChunkChoice `json:"choices"`
	Usage   CompletionUsage         `json:"usage,omitempty"`
}

// CompletionChunkChoice represents a choice in a streaming completion.
type CompletionChunkChoice struct {
	Index        int                    `json:"index"`
	Delta        CompletionMessageDelta `json:"delta"`
	FinishReason string                 `json:"finish_reason"`
}

// CompletionMessageDelta represents a delta in a streaming completion.
type CompletionMessageDelta struct {
	Role      string        `json:"role,omitempty"`
	Content   string        `json:"content,omitempty"`
	ToolCalls []APIToolCall `json:"tool_calls,omitempty"`
}

// APIMessage represents a message in the API format.
type APIMessage struct {
	Role       string        `json:"role"`
	Content    string        `json:"content"`
	Name       string        `json:"name,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
	ToolCalls  []APIToolCall `json:"tool_calls,omitempty"`
}

// APITool represents a tool definition in the API format.
type APITool struct {
	Type     string          `json:"type"`
	Function APIToolFunction `json:"function"`
}

// APIToolFunction represents a tool function definition.
type APIToolFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// APIToolCall represents a tool call in the API format.
type APIToolCall struct {
	ID       string              `json:"id"`
	Type     string              `json:"type"`
	Function APIToolCallFunction `json:"function"`
}

// APIToolCallFunction represents a tool call function.
type APIToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}
