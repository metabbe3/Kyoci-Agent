package llm

import (
	"bufio"
	"bytes"
	"context"

	"github.com/nicholas/ai-agent/config"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OllamaProvider implements the Provider interface for Ollama
type OllamaProvider struct {
	cfg    config.ProviderConfig
	cache  *ResponseCache
	client *http.Client
}

// NewOllamaProvider creates a new Ollama provider with a 30-second timeout
func NewOllamaProvider(cfg config.ProviderConfig) *OllamaProvider {
	return &OllamaProvider{
		cfg: cfg,
		client: &http.Client{
			Timeout: 300 * time.Second, // 5 min
		},
	}
}

// SetCache sets the response cache for this provider
func (p *OllamaProvider) SetCache(cache *ResponseCache) {
	p.cache = cache
}

// Name returns the provider name
func (p *OllamaProvider) Name() string {
	return "ollama"
}

// Chat sends messages and returns a response
func (p *OllamaProvider) Chat(ctx context.Context, messages []Message, tools []ToolSchema) (*Response, error) {
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

	// Build request
	reqBody := map[string]interface{}{
		"model":       p.cfg.Model,
		"messages":    p.convertMessages(messages),
		"stream":      false,
	}

	if p.cfg.Temperature > 0 {
		reqBody["temperature"] = p.cfg.Temperature
	}

	if p.cfg.MaxTokens > 0 {
		reqBody["options"] = map[string]interface{}{
			"num_predict": p.cfg.MaxTokens,
		}
	}

	if len(tools) > 0 {
		reqBody["tools"] = p.convertTools(tools)
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	url := strings.TrimSuffix(p.cfg.BaseURL, "/") + "/api/chat"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

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
	var ollamaResp ollamaResponse
	if err := json.Unmarshal(body, &ollamaResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	response := &Response{
		Content:    ollamaResp.Message.Content,
		ToolCalls:  p.parseToolCalls(ollamaResp.Message.ToolCalls),
		StopReason: p.mapStopReason(ollamaResp.DoneReason),
		Model:      ollamaResp.Model,
	}

	if ollamaResp.PromptEvalCount > 0 || ollamaResp.EvalCount > 0 {
		response.Usage = Usage{
			InputTokens:  ollamaResp.PromptEvalCount,
			OutputTokens: ollamaResp.EvalCount,
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
func (p *OllamaProvider) Stream(ctx context.Context, messages []Message, tools []ToolSchema) (<-chan Chunk, error) {
	ch := make(chan Chunk, 10)

	go func() {
		defer close(ch)

		// Build request
		reqBody := map[string]interface{}{
			"model":       p.cfg.Model,
			"messages":    p.convertMessages(messages),
			"stream":      true,
		}

		if p.cfg.Temperature > 0 {
			reqBody["temperature"] = p.cfg.Temperature
		}

		if p.cfg.MaxTokens > 0 {
			reqBody["options"] = map[string]interface{}{
				"num_predict": p.cfg.MaxTokens,
			}
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
		url := strings.TrimSuffix(p.cfg.BaseURL, "/") + "/api/chat"
		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
		if err != nil {
			ch <- Chunk{Done: true}
			return
		}

		req.Header.Set("Content-Type", "application/json")

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

		// Parse NDJSON stream
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}

			var chunkResp ollamaStreamChunk
			if err := json.Unmarshal([]byte(line), &chunkResp); err != nil {
				continue
			}

			if chunkResp.Done {
				ch <- Chunk{Done: true}
				return
			}

			chunk := Chunk{}
			if chunkResp.Message.Content != "" {
				chunk.Content = chunkResp.Message.Content
			}

			if len(chunkResp.Message.ToolCalls) > 0 {
				for _, tc := range chunkResp.Message.ToolCalls {
					chunk.ToolCall = &ToolCall{
						Name:      tc.Function.Name,
						Arguments: string(tc.Function.Arguments),
					}
					ch <- chunk
					chunk.ToolCall = nil
				}
			} else if chunk.Content != "" {
				ch <- chunk
			}
		}

		if ctx.Err() != nil {
			return
		}

		ch <- Chunk{Done: true}
	}()

	return ch, nil
}

// Ollama-specific types

type ollamaRequest struct {
	Model       string              `json:"model"`
	Messages    []ollamaMessage     `json:"messages"`
	Stream      bool                `json:"stream"`
	Temperature float64             `json:"temperature,omitempty"`
	Options     map[string]interface{} `json:"options,omitempty"`
	Tools       []ollamaTool        `json:"tools,omitempty"`
}

type ollamaMessage struct {
	Role       string            `json:"role"`
	Content    string            `json:"content"`
	ToolCalls  []ollamaToolCall  `json:"tool_calls,omitempty"`
}

type ollamaTool struct {
	Type     string             `json:"type"`
	Function ollamaToolFunction `json:"function"`
}

type ollamaToolFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

type ollamaToolCall struct {
	Function ollamaToolCallFunction `json:"function"`
}

type ollamaToolCallFunction struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type ollamaResponse struct {
	Model         string        `json:"model"`
	CreatedAt     string        `json:"created_at"`
	Message       ollamaMessage `json:"message"`
	Done          bool          `json:"done"`
	DoneReason    string        `json:"done_reason,omitempty"`
	PromptEvalCount int         `json:"prompt_eval_count,omitempty"`
	EvalCount     int           `json:"eval_count,omitempty"`
}

type ollamaStreamChunk struct {
	Model         string        `json:"model"`
	CreatedAt     string        `json:"created_at"`
	Message       ollamaMessage `json:"message"`
	Done          bool          `json:"done"`
	DoneReason    string        `json:"done_reason,omitempty"`
}

// Helper methods

func (p *OllamaProvider) convertMessages(messages []Message) []ollamaMessage {
	result := make([]ollamaMessage, len(messages))
	for i, msg := range messages {
		result[i] = ollamaMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}

		if len(msg.ToolCalls) > 0 {
			result[i].ToolCalls = make([]ollamaToolCall, len(msg.ToolCalls))
			for j, tc := range msg.ToolCalls {
				result[i].ToolCalls[j] = ollamaToolCall{
					Function: ollamaToolCallFunction{
						Name:      tc.Name,
						Arguments: json.RawMessage(tc.Arguments),
					},
				}
			}
		}
	}
	return result
}

func (p *OllamaProvider) convertTools(tools []ToolSchema) []ollamaTool {
	result := make([]ollamaTool, len(tools))
	for i, tool := range tools {
		result[i] = ollamaTool{
			Type: "function",
			Function: ollamaToolFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.Parameters,
			},
		}
	}
	return result
}

func (p *OllamaProvider) parseToolCalls(calls []ollamaToolCall) []ToolCall {
	if len(calls) == 0 {
		return nil
	}

	result := make([]ToolCall, len(calls))
	for i, tc := range calls {
		result[i] = ToolCall{
			Name:      tc.Function.Name,
			Arguments: string(tc.Function.Arguments),
		}
	}
	return result
}

func (p *OllamaProvider) mapStopReason(reason string) string {
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