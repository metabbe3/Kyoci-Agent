package llm
import (
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

// AnthropicProvider implements the Provider interface for Anthropic Claude API
type AnthropicProvider struct {
	cfg   config.ProviderConfig
	cache *ResponseCache
	client *http.Client
}

// NewAnthropicProvider creates a new Anthropic provider with a 30-second timeout
func NewAnthropicProvider(cfg config.ProviderConfig) *AnthropicProvider {
	return &AnthropicProvider{
		cfg: cfg,
		client: &http.Client{
			Timeout: 300 * time.Second, // 5 min
		},
	}
}

// SetCache sets the response cache for this provider
func (p *AnthropicProvider) SetCache(cache *ResponseCache) {
	p.cache = cache
}

// Name returns the provider name
func (p *AnthropicProvider) Name() string {
	return "anthropic"
}

// Chat sends messages and returns a response
func (p *AnthropicProvider) Chat(ctx context.Context, messages []Message, tools []ToolSchema) (*Response, error) {
	// Extract system message
	var systemPrompt string
	var filteredMessages []Message
	for _, msg := range messages {
		if msg.Role == "system" {
			systemPrompt = msg.Content
		} else {
			filteredMessages = append(filteredMessages, msg)
		}
	}

	// Check cache first (skip if tools are present as they're dynamic)
	if p.cache != nil && len(tools) == 0 {
		if cachedResp, found := p.cache.Get(systemPrompt, filteredMessages); found {
			return cachedResp, nil
		}
	}

	// Build request
	reqBody := map[string]interface{}{
		"model":       p.cfg.Model,
		"messages":    p.convertMessages(filteredMessages),
		"max_tokens":  p.cfg.MaxTokens,
	}

	if systemPrompt != "" {
		// Apply Anthropic cache control to system prompt
		if IsCacheableContext(systemPrompt, 1000) {
			reqBody["system"] = map[string]interface{}{
				"type":  "text",
				"text":  systemPrompt,
				"cache_control": map[string]string{
					"type": "ephemeral",
				},
			}
		} else {
			reqBody["system"] = systemPrompt
		}
	}

	if p.cfg.Temperature > 0 {
		reqBody["temperature"] = p.cfg.Temperature
	}

	if len(tools) > 0 {
		reqBody["tools"] = p.convertTools(tools)
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	url := strings.TrimSuffix(p.cfg.BaseURL, "/") + "/v1/messages"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.cfg.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

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
	var anthropicResp anthropicResponse
	if err := json.Unmarshal(body, &anthropicResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	response := &Response{
		Content:    p.extractContent(anthropicResp.Content),
		ToolCalls:  p.parseToolCalls(anthropicResp.Content),
		StopReason: p.mapStopReason(anthropicResp.StopReason),
		Model:      anthropicResp.Model,
	}

	if anthropicResp.Usage.InputTokens > 0 || anthropicResp.Usage.OutputTokens > 0 {
		response.Usage = Usage{
			InputTokens:  anthropicResp.Usage.InputTokens,
			OutputTokens: anthropicResp.Usage.OutputTokens,
		}
	}

	// Store in cache (skip if tools are present)
	if p.cache != nil && len(tools) == 0 {
		// Determine TTL based on whether this looks like AST context
		isASTContext := IsCacheableContext(systemPrompt, 1000)
		ttl := time.Duration(GetDefaultTTL(isASTContext)) * time.Second
		p.cache.Set(systemPrompt, filteredMessages, response, ttl)
	}

	return response, nil
}

// Stream sends messages and returns a channel of chunks
func (p *AnthropicProvider) Stream(ctx context.Context, messages []Message, tools []ToolSchema) (<-chan Chunk, error) {
	ch := make(chan Chunk, 10)

	go func() {
		defer close(ch)

		// Extract system message
		var systemPrompt string
		var filteredMessages []Message
		for _, msg := range messages {
			if msg.Role == "system" {
				systemPrompt = msg.Content
			} else {
				filteredMessages = append(filteredMessages, msg)
			}
		}

		// Build request
		reqBody := map[string]interface{}{
			"model":       p.cfg.Model,
			"messages":    p.convertMessages(filteredMessages),
			"max_tokens":  p.cfg.MaxTokens,
			"stream":      true,
		}

		if systemPrompt != "" {
			reqBody["system"] = systemPrompt
		}

		if p.cfg.Temperature > 0 {
			reqBody["temperature"] = p.cfg.Temperature
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
		url := strings.TrimSuffix(p.cfg.BaseURL, "/") + "/v1/messages"
		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
		if err != nil {
			ch <- Chunk{Done: true}
			return
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-api-key", p.cfg.APIKey)
		req.Header.Set("anthropic-version", "2023-06-01")

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

		// Anthropic streaming uses SSE format
		decoder := json.NewDecoder(resp.Body)
		for {
			var event anthropicStreamEvent
			if err := decoder.Decode(&event); err != nil {
				if err == io.EOF {
					break
				}
				continue
			}

			if event.Type == "content_block_delta" {
				chunk := Chunk{}
				if event.Delta.Type == "text_delta" {
					chunk.Content = event.Delta.Text
				}
				ch <- chunk
			} else if event.Type == "content_block_stop" {
				ch <- Chunk{Done: true}
				return
			}
		}

		if ctx.Err() != nil {
			return
		}

		ch <- Chunk{Done: true}
	}()

	return ch, nil
}

// Anthropic-specific types

type anthropicRequest struct {
	Model       string                  `json:"model"`
	Messages    []anthropicMessage      `json:"messages"`
	MaxTokens   int                     `json:"max_tokens"`
	System      string                  `json:"system,omitempty"`
	Temperature float64                 `json:"temperature,omitempty"`
	Stream      bool                    `json:"stream,omitempty"`
	Tools       []anthropicTool         `json:"tools,omitempty"`
}

type anthropicMessage struct {
	Role    string                   `json:"role"`
	Content []anthropicContentBlock  `json:"content"`
}

type anthropicContentBlock struct {
	Type       string                `json:"type"`
	Text       string                `json:"text,omitempty"`
	ID         string                `json:"id,omitempty"`
	Name       string                `json:"name,omitempty"`
	Input      map[string]interface{} `json:"input,omitempty"`
	ToolUseID  string                `json:"tool_use_id,omitempty"`
	Content    string                `json:"content,omitempty"`
}

type anthropicTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

type anthropicResponse struct {
	ID         string                    `json:"id"`
	Type       string                    `json:"type"`
	Role       string                    `json:"role"`
	Content    []anthropicContentBlock   `json:"content"`
	StopReason string                    `json:"stop_reason"`
	Model      string                    `json:"model"`
	Usage      anthropicUsage            `json:"usage"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type anthropicStreamEvent struct {
	Type string `json:"type"`
	Delta struct {
		Type string `json:"type,omitempty"`
		Text string `json:"text,omitempty"`
	} `json:"delta,omitempty"`
}

// Helper methods

func (p *AnthropicProvider) convertMessages(messages []Message) []anthropicMessage {
	result := make([]anthropicMessage, len(messages))
	for i, msg := range messages {
		result[i] = anthropicMessage{
			Role:    msg.Role,
			Content: p.convertContent(msg),
		}
	}
	return result
}

func (p *AnthropicProvider) convertContent(msg Message) []anthropicContentBlock {
	if msg.Role == "tool" {
		return []anthropicContentBlock{
			{
				Type:      "tool_result",
				ToolUseID: msg.ToolCallID,
				Content:   msg.Content,
			},
		}
	}

	if len(msg.ToolCalls) > 0 {
		blocks := make([]anthropicContentBlock, 0, len(msg.ToolCalls)+1)
		if msg.Content != "" {
			blocks = append(blocks, anthropicContentBlock{
				Type: "text",
				Text: msg.Content,
			})
		}

		for _, tc := range msg.ToolCalls {
			var args map[string]interface{}
			json.Unmarshal([]byte(tc.Arguments), &args)

			blocks = append(blocks, anthropicContentBlock{
				Type:  "tool_use",
				ID:    tc.ID,
				Name:  tc.Name,
				Input: args,
			})
		}

		return blocks
	}

	return []anthropicContentBlock{
		{
			Type: "text",
			Text: msg.Content,
		},
	}
}

func (p *AnthropicProvider) convertTools(tools []ToolSchema) []anthropicTool {
	result := make([]anthropicTool, len(tools))
	for i, tool := range tools {
		result[i] = anthropicTool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.Parameters,
		}
	}
	return result
}

func (p *AnthropicProvider) extractContent(blocks []anthropicContentBlock) string {
	var content strings.Builder
	for _, block := range blocks {
		if block.Type == "text" {
			content.WriteString(block.Text)
		}
	}
	return content.String()
}

func (p *AnthropicProvider) parseToolCalls(blocks []anthropicContentBlock) []ToolCall {
	if len(blocks) == 0 {
		return nil
	}

	var calls []ToolCall
	for _, block := range blocks {
		if block.Type == "tool_use" {
			args, _ := json.Marshal(block.Input)
			calls = append(calls, ToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: string(args),
			})
		}
	}

	return calls
}

func (p *AnthropicProvider) mapStopReason(reason string) string {
	switch reason {
	case "end_turn":
		return "stop"
	case "tool_use":
		return "tool_use"
	case "max_tokens":
		return "max_tokens"
	default:
		return "stop"
	}
}