package llm

import (
	"bufio"
	"bytes"
	"context"

	"encoding/json"

	"github.com/nicholas/ai-agent/config"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OpenAIProvider implements the Provider interface for OpenAI-compatible APIs
type OpenAIProvider struct {
	cfg    config.ProviderConfig
	cache  *ResponseCache
	client *http.Client
}

// NewOpenAIProvider creates a new OpenAI provider with a 30-second timeout
func NewOpenAIProvider(cfg config.ProviderConfig) *OpenAIProvider {
	return &OpenAIProvider{
		cfg: cfg,
		client: &http.Client{
			Timeout: 300 * time.Second, // 5 min
		},
	}
}

// SetCache sets the response cache for this provider
func (p *OpenAIProvider) SetCache(cache *ResponseCache) {
	p.cache = cache
}

// Name returns the provider name
func (p *OpenAIProvider) Name() string {
	return "openai"
}

// Chat sends messages and returns a response
func (p *OpenAIProvider) Chat(ctx context.Context, messages []Message, tools []ToolSchema) (*Response, error) {
	// Extract system message for caching
	var systemPrompt string
	for _, msg := range messages {
		if msg.Role == "system" {
			systemPrompt = msg.Content
			break
		}
	}

	// Check cache first (skip if tools are present as they're dynamic)
	if p.cache != nil && len(tools) == 0 {
		if cachedResp, found := p.cache.Get(systemPrompt, messages); found {
			return cachedResp, nil
		}
	}

	// Normalize messages for consistent caching (ensures stable order)
	_, normalizedMessages := NormalizeForOpenAICache(systemPrompt, messages)

	// Build request
	reqBody := map[string]interface{}{
		"model":       p.cfg.Model,
		"messages":    p.convertMessages(normalizedMessages),
		"max_tokens":  p.cfg.MaxTokens,
		"temperature": p.cfg.Temperature,
	}

	if len(tools) > 0 {
		reqBody["tools"] = p.convertTools(tools)
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	url := strings.TrimSuffix(p.cfg.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if p.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	}

	// Send request
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	// Parse response
	var openaiResp openAIResponse
	if err := json.Unmarshal(body, &openaiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(openaiResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	choice := openaiResp.Choices[0]
	response := &Response{
		Content:    choice.Message.Content,
		ToolCalls:  p.parseToolCalls(choice.Message.ToolCalls),
		StopReason: p.mapStopReason(choice.FinishReason),
		Model:      openaiResp.Model,
	}

	if openaiResp.Usage.PromptTokens > 0 || openaiResp.Usage.CompletionTokens > 0 {
		response.Usage = Usage{
			InputTokens:  openaiResp.Usage.PromptTokens,
			OutputTokens: openaiResp.Usage.CompletionTokens,
		}
	}

	// Store in cache (skip if tools are present)
	if p.cache != nil && len(tools) == 0 {
		// Determine TTL based on whether this looks like AST context
		isASTContext := IsCacheableContext(systemPrompt, 1000)
		ttl := time.Duration(GetDefaultTTL(isASTContext)) * time.Second
		p.cache.Set(systemPrompt, messages, response, ttl)
	}

	return response, nil
}

// Stream sends messages and returns a channel of chunks
func (p *OpenAIProvider) Stream(ctx context.Context, messages []Message, tools []ToolSchema) (<-chan Chunk, error) {
	ch := make(chan Chunk, 10)

	go func() {
		defer close(ch)

		// Build request
		reqBody := map[string]interface{}{
			"model":       p.cfg.Model,
			"messages":    p.convertMessages(messages),
			"max_tokens":  p.cfg.MaxTokens,
			"temperature": p.cfg.Temperature,
			"stream":      true,
		}

		if len(tools) > 0 {
			reqBody["tools"] = p.convertTools(tools)
		}

		jsonBody, err := json.Marshal(reqBody)
		if err != nil {
			ch <- Chunk{Done: true}
			return
		}

		// Create HTTP request
		url := strings.TrimSuffix(p.cfg.BaseURL, "/") + "/chat/completions"
		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
		if err != nil {
			ch <- Chunk{Done: true}
			return
		}

		req.Header.Set("Content-Type", "application/json")
		if p.cfg.APIKey != "" {
			req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
		}

		// Send request
		resp, err := p.client.Do(req)
		if err != nil {
			ch <- Chunk{Done: true}
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			ch <- Chunk{Done: true}
			return
		}

		// Parse SSE stream
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				ch <- Chunk{Done: true}
				return
			}

			var chunkResp openAIStreamChunk
			if err := json.Unmarshal([]byte(data), &chunkResp); err != nil {
				continue
			}

			if len(chunkResp.Choices) > 0 {
				choice := chunkResp.Choices[0]
				chunk := Chunk{}

				if len(choice.Delta.Content) > 0 {
					chunk.Content = choice.Delta.Content
				}

				if len(choice.Delta.ToolCalls) > 0 {
					for _, tc := range choice.Delta.ToolCalls {
						chunk.ToolCall = &ToolCall{
							ID:       tc.ID,
							Name:     tc.Function.Name,
							Arguments: tc.Function.Arguments,
						}
						ch <- chunk
						chunk.ToolCall = nil
					}
				} else if chunk.Content != "" || chunk.ToolCall != nil {
					ch <- chunk
				}
			}
		}

		if ctx.Err() != nil {
			return
		}

		ch <- Chunk{Done: true}
	}()

	return ch, nil
}

// OpenAI-specific types

type openAIRequest struct {
	Model       string              `json:"model"`
	Messages    []openAIMessage     `json:"messages"`
	MaxTokens   int                 `json:"max_tokens"`
	Temperature float64             `json:"temperature"`
	Stream      bool                `json:"stream,omitempty"`
	Tools       []openAITool        `json:"tools,omitempty"`
}

type openAIMessage struct {
	Role       string              `json:"role"`
	Content    string              `json:"content"`
	ToolCalls  []openAIToolCall    `json:"tool_calls,omitempty"`
	ToolCallID string              `json:"tool_call_id,omitempty"`
}

type openAITool struct {
	Type     string            `json:"type"`
	Function openAIToolFunction `json:"function"`
}

type openAIToolFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters"`
}

type openAIToolCall struct {
	ID       string                `json:"id"`
	Type     string                `json:"type"`
	Function openAIToolCallFunction `json:"function"`
}

type openAIToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openAIResponse struct {
	ID      string             `json:"id"`
	Object  string             `json:"object"`
	Created int64              `json:"created"`
	Model   string             `json:"model"`
	Choices []openAIChoice     `json:"choices"`
	Usage   openAIUsage        `json:"usage"`
}

type openAIChoice struct {
	Index        int             `json:"index"`
	Message      openAIMessage   `json:"message"`
	FinishReason string          `json:"finish_reason"`
}

type openAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type openAIStreamChunk struct {
	ID      string              `json:"id"`
	Object  string              `json:"object"`
	Created int64               `json:"created"`
	Model   string              `json:"model"`
	Choices []openAIStreamChoice `json:"choices"`
}

type openAIStreamChoice struct {
	Index int             `json:"index"`
	Delta openAIDelta     `json:"delta"`
	FinishReason *string  `json:"finish_reason"`
}

type openAIDelta struct {
	Content   string                `json:"content,omitempty"`
	ToolCalls []openAIToolCall      `json:"tool_calls,omitempty"`
}

// Helper methods

func (p *OpenAIProvider) convertMessages(messages []Message) []openAIMessage {
	result := make([]openAIMessage, len(messages))
	for i, msg := range messages {
		result[i] = openAIMessage{
			Role:       msg.Role,
			Content:    msg.Content,
			ToolCallID: msg.ToolCallID,
		}

		if len(msg.ToolCalls) > 0 {
			result[i].ToolCalls = make([]openAIToolCall, len(msg.ToolCalls))
			for j, tc := range msg.ToolCalls {
				result[i].ToolCalls[j] = openAIToolCall{
					ID:   tc.ID,
					Type: "function",
					Function: openAIToolCallFunction{
						Name:      tc.Name,
						Arguments: tc.Arguments,
					},
				}
			}
		}
	}
	return result
}

func (p *OpenAIProvider) convertTools(tools []ToolSchema) []openAITool {
	result := make([]openAITool, len(tools))
	for i, tool := range tools {
		result[i] = openAITool{
			Type: "function",
			Function: openAIToolFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.Parameters,
			},
		}
	}
	return result
}

func (p *OpenAIProvider) parseToolCalls(calls []openAIToolCall) []ToolCall {
	if len(calls) == 0 {
		return nil
	}

	result := make([]ToolCall, len(calls))
	for i, tc := range calls {
		result[i] = ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		}
	}
	return result
}

func (p *OpenAIProvider) mapStopReason(reason string) string {
	switch reason {
	case "stop":
		return "stop"
	case "tool_calls":
		return "tool_use"
	case "length":
		return "max_tokens"
	default:
		return "stop"
	}
}