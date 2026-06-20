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
	"strings"
	"sync"
	"time"

	"github.com/metabbe3/Kyoci-Agent/internal/apperr"
	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// AnthropicClient is a kyoci.Provider that speaks the Anthropic Messages API
// (POST /v1/messages). It enables Anthropic-native endpoints — including
// OpenAI/Anthropic-compat gateways like z.ai's /api/anthropic — which the
// OpenAI-compatible client cannot reach (that client only does /chat/completions).
//
// It translates Kyoci's OpenAI-shaped CompletionRequest (system/user/assistant/
// tool messages, OpenAI tool schemas) to/from Anthropic's Messages format
// (top-level system, alternating user/assistant turns, tool_use/tool_result
// content blocks, input_schema tools).
type AnthropicClient struct {
	name           string
	config         kyoci.ProviderConfig
	httpClient     *http.Client
	circuitBreaker *CircuitBreaker
	logger         *slog.Logger
	mu             sync.RWMutex
	lastError      error
}

// NewAnthropicClient creates an Anthropic Messages API client.
func NewAnthropicClient(name string, config kyoci.ProviderConfig) (kyoci.Provider, error) {
	if config.BaseURL == "" {
		return nil, fmt.Errorf("provider %s: base_url is required", name)
	}
	if config.APIKey == "" {
		return nil, fmt.Errorf("provider %s: api_key is required", name)
	}
	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &AnthropicClient{
		name:           name,
		config:         config,
		httpClient:     &http.Client{Timeout: config.Timeout},
		circuitBreaker: NewCircuitBreaker(5, 30*time.Second),
		logger:         logger,
	}, nil
}

// AsAnthropicClient returns the concrete *AnthropicClient behind a provider.
func AsAnthropicClient(p kyoci.Provider) *AnthropicClient {
	if c, ok := p.(*AnthropicClient); ok {
		return c
	}
	return nil
}

func (c *AnthropicClient) Name() string     { return c.name }
func (c *AnthropicClient) IsAvailable() bool { return c.config.APIKey != "" && c.circuitBreaker.Allow() }

func (c *AnthropicClient) Models() []kyoci.ModelInfo {
	// Query /v1/models endpoint (z.ai supports this).
	type m struct{ ID string `json:"id"` }
	type resp struct{ Data []m `json:"data"` }
	url := strings.TrimRight(c.config.BaseURL, "/") + "/v1/models"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return c.fallbackModels()
	}
	req.Header.Set("x-api-key", c.config.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	respBody, err := c.httpClient.Do(req)
	if err != nil {
		return c.fallbackModels()
	}
	defer respBody.Body.Close()
	if respBody.StatusCode != 200 {
		return c.fallbackModels()
	}
	var r resp
	if err := json.NewDecoder(respBody.Body).Decode(&r); err != nil {
		return c.fallbackModels()
	}
	out := make([]kyoci.ModelInfo, 0, len(r.Data))
	for _, m := range r.Data {
		isDefault := m.ID == c.config.DefaultModel
		tags := []string{}
		if isDefault { tags = append(tags, "default") }
		out = append(out, kyoci.ModelInfo{
			ID: m.ID, Provider: c.name, ContextLength: 128000,
			SupportsTools: true, SupportsStreaming: true, MaxOutputTokens: 8192,
			Description: m.ID, Tags: tags,
		})
	}
	if len(out) == 0 { return c.fallbackModels() }
	return out
}

func (c *AnthropicClient) fallbackModels() []kyoci.ModelInfo {
	return []kyoci.ModelInfo{{
		ID: c.config.DefaultModel, Provider: c.name, ContextLength: 128000,
		SupportsTools: true, SupportsStreaming: true, MaxOutputTokens: 8192,
		Description: "Default: " + c.config.DefaultModel, Tags: []string{"default"},
	}}
}

func (c *AnthropicClient) defaultModel(req kyoci.CompletionRequest) string {
	if req.Model != "" {
		return req.Model
	}
	return c.config.DefaultModel
}

func (c *AnthropicClient) checkCircuit() error {
	if !c.circuitBreaker.Allow() {
		return fmt.Errorf("provider %s: %w", c.name, apperr.ErrCircuitOpen)
	}
	return nil
}

// ---- Anthropic wire types ----

type anthropicContentBlock struct {
	Type string `json:"type"`            // "text" | "tool_use" | "tool_result"
	Text string `json:"text,omitempty"`  // for text
	// tool_use:
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
	// tool_result:
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   any    `json:"content,omitempty"` // string or []block
}

type anthropicMessage struct {
	Role    string `json:"role"` // "user" | "assistant"
	Content any    `json:"content"`
}

type anthropicTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

type anthropicToolChoice struct {
	Type string `json:"type"`           // "auto" | "any" | "tool"
	Name string `json:"name,omitempty"` // when type=="tool"
}

type anthropicRequest struct {
	Model       string             `json:"model"`
	MaxTokens   int                `json:"max_tokens"`
	System      string             `json:"system,omitempty"`
	Messages    []anthropicMessage `json:"messages"`
	Temperature float64            `json:"temperature,omitempty"`
	Tools       []anthropicTool    `json:"tools,omitempty"`
	ToolChoice  *anthropicToolChoice `json:"tool_choice,omitempty"`
	Stream      bool               `json:"stream,omitempty"`
}

type anthropicResponse struct {
	ID          string                 `json:"id"`
	Model       string                 `json:"model"`
	Content     []anthropicContentBlock `json:"content"`
	StopReason  string                 `json:"stop_reason"`
	Usage       struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// ---- translation: Kyoci CompletionRequest -> Anthropic messages ----

// buildMessages converts Kyoci's OpenAI-style message list into Anthropic's
// (system string, alternating user/assistant turns with tool_use/tool_result
// content blocks). Consecutive same-role turns are merged so the result
// strictly alternates (Anthropic requires this).
func (c *AnthropicClient) buildMessages(req kyoci.CompletionRequest) (string, []anthropicMessage) {
	var systemParts []string
	var msgs []anthropicMessage

	flushText := func(role, text string) {
		text = strings.TrimSpace(text)
		if text == "" {
			return
		}
		if n := len(msgs); n > 0 && msgs[n-1].Role == role {
			// merge into previous same-role message's text content
			msgs[n-1] = mergeText(msgs[n-1], text)
		} else {
			msgs = append(msgs, anthropicMessage{Role: role, Content: text})
		}
	}

	for _, m := range req.Messages {
		switch m.Role {
		case kyoci.RoleSystem:
			if strings.TrimSpace(m.Content) != "" {
				systemParts = append(systemParts, m.Content)
			}
		case kyoci.RoleUser:
			flushText("user", m.Content)
		case kyoci.RoleAssistant:
			if len(m.ToolCalls) > 0 {
				var blocks []anthropicContentBlock
				if strings.TrimSpace(m.Content) != "" {
					blocks = append(blocks, anthropicContentBlock{Type: "text", Text: m.Content})
				}
				for _, tc := range m.ToolCalls {
					var input json.RawMessage
					if tc.Arguments != "" {
						input = json.RawMessage(tc.Arguments)
					} else {
						input = json.RawMessage("{}")
					}
					blocks = append(blocks, anthropicContentBlock{Type: "tool_use", ID: tc.ID, Name: tc.Name, Input: input})
				}
				msgs = append(msgs, anthropicMessage{Role: "assistant", Content: blocks})
			} else {
				flushText("assistant", m.Content)
			}
		case kyoci.RoleTool:
			// Tool result → user message with a tool_result block. Merge into a
			// preceding user(tool_result) message if adjacent.
			block := anthropicContentBlock{Type: "tool_result", ToolUseID: m.ToolCallID, Content: m.Content}
			if n := len(msgs); n > 0 && msgs[n-1].Role == "user" {
				if existing, ok := msgs[n-1].Content.([]anthropicContentBlock); ok {
					msgs[n-1].Content = append(existing, block)
					continue
				}
			}
			msgs = append(msgs, anthropicMessage{Role: "user", Content: []anthropicContentBlock{block}})
		}
	}
	return strings.Join(systemParts, "\n\n"), msgs
}

// mergeText appends text to a message whose Content is a string.
func mergeText(m anthropicMessage, text string) anthropicMessage {
	if s, ok := m.Content.(string); ok {
		m.Content = s + "\n" + text
	}
	return m
}

func (c *AnthropicClient) buildTools(req kyoci.CompletionRequest) []anthropicTool {
	if len(req.Tools) == 0 {
		return nil
	}
	tools := make([]anthropicTool, 0, len(req.Tools))
	for _, t := range req.Tools {
		tools = append(tools, anthropicTool{
			Name: t.Name, Description: t.Description, InputSchema: t.ToJSONSchema(),
		})
	}
	return tools
}

func (c *AnthropicClient) buildToolChoice(req kyoci.CompletionRequest) *anthropicToolChoice {
	switch req.ToolChoice {
	case "required":
		return &anthropicToolChoice{Type: "any"}
	case "none":
		return &anthropicToolChoice{Type: "auto"} // Anthropic has no "none"; caller should omit tools
	default: // "" or "auto"
		return &anthropicToolChoice{Type: "auto"}
	}
}

func (c *AnthropicClient) buildRequest(req kyoci.CompletionRequest, stream bool) (*anthropicRequest, error) {
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	system, msgs := c.buildMessages(req)
	if len(msgs) == 0 {
		return nil, fmt.Errorf("provider %s: no messages", c.name)
	}
	ar := &anthropicRequest{
		Model:       c.defaultModel(req),
		MaxTokens:   maxTokens,
		System:      system,
		Messages:    msgs,
		Tools:       c.buildTools(req),
		ToolChoice:  c.buildToolChoice(req),
		Stream:      stream,
	}
	if req.Temperature > 0 {
		ar.Temperature = req.Temperature
	}
	return ar, nil
}

func (c *AnthropicClient) setHeaders(h http.Header) {
	h.Set("x-api-key", c.config.APIKey)
	h.Set("anthropic-version", "2023-06-01")
	h.Set("Content-Type", "application/json")
	c.mu.RLock()
	for k, v := range c.config.Headers {
		h.Set(k, v)
	}
	c.mu.RUnlock()
}

// ---- Complete ----

// Complete performs a non-streaming Anthropic Messages request with retry on
// transient failures (5xx, 429, network). Client errors (4xx except 429) are
// not retried.
func (c *AnthropicClient) Complete(ctx context.Context, req kyoci.CompletionRequest) (*kyoci.CompletionResponse, error) {
	if err := c.checkCircuit(); err != nil {
		return nil, err
	}
	ar, err := c.buildRequest(req, false)
	if err != nil {
		return nil, err
	}
	body, _ := json.Marshal(ar)

	var lastErr error
	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			if backoff > 5*time.Second {
				backoff = 5 * time.Second
			}
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return nil, fmt.Errorf("provider %s: canceled during retry backoff: %w", c.name, ctx.Err())
			}
		}
		r, retryable, err := c.doOnce(ctx, body)
		if err == nil {
			c.circuitBreaker.RecordSuccess()
			return c.toCompletionResponse(r), nil
		}
		lastErr = err
		if !retryable || attempt == c.config.MaxRetries {
			break
		}
		c.logger.Warn("anthropic request failed, retrying", "provider", c.name, "attempt", attempt+1, "error", err)
	}
	c.recordFailure()
	return nil, lastErr
}

// doOnce performs a single non-streaming /v1/messages POST. Returns the parsed
// response, whether the error is retryable (5xx / 429 / network), and the error.
func (c *AnthropicClient) doOnce(ctx context.Context, body []byte) (*anthropicResponse, bool, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.config.BaseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, false, fmt.Errorf("provider %s: create request: %w", c.name, err)
	}
	c.setHeaders(httpReq.Header)
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, true, fmt.Errorf("provider %s: request failed: %w", c.name, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == 429 || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("provider %s: API error (status %d): %s", c.name, resp.StatusCode, truncateBody(string(respBody), 300))
	}
	if resp.StatusCode >= 400 {
		return nil, false, fmt.Errorf("provider %s: API error (status %d): %s", c.name, resp.StatusCode, truncateBody(string(respBody), 300))
	}
	var ar anthropicResponse
	if err := json.Unmarshal(respBody, &ar); err != nil {
		return nil, false, fmt.Errorf("provider %s: decode response: %w", c.name, err)
	}
	if ar.Error != nil {
		return nil, false, fmt.Errorf("provider %s: %s", c.name, ar.Error.Message)
	}
	return &ar, false, nil
}

func (c *AnthropicClient) toCompletionResponse(r *anthropicResponse) *kyoci.CompletionResponse {
	out := &kyoci.CompletionResponse{Model: r.Model, FinishReason: mapStopReason(r.StopReason)}
	out.Usage = kyoci.TokenUsage{PromptTokens: r.Usage.InputTokens, CompletionTokens: r.Usage.OutputTokens, TotalTokens: r.Usage.InputTokens + r.Usage.OutputTokens}
	var toolCalls []kyoci.ToolCall
	for _, b := range r.Content {
		switch b.Type {
		case "text":
			out.Content += b.Text
		case "tool_use":
			args := "{}"
			if len(b.Input) > 0 {
				args = string(b.Input)
			}
			toolCalls = append(toolCalls, kyoci.ToolCall{ID: b.ID, Name: b.Name, Arguments: args})
		}
	}
	out.ToolCalls = toolCalls
	if len(toolCalls) > 0 && out.FinishReason == "" {
		out.FinishReason = kyoci.FinishToolCall
	}
	return out
}

func mapStopReason(r string) kyoci.FinishReason {
	switch r {
	case "end_turn", "stop_sequence":
		return kyoci.FinishStop
	case "tool_use":
		return kyoci.FinishToolCall
	case "max_tokens":
		return kyoci.FinishMaxTokens
	default:
		return ""
	}
}

// ---- Stream ----

// Stream performs a streaming Anthropic Messages request (SSE) and emits
// kyoci.StreamChunk values.
func (c *AnthropicClient) Stream(ctx context.Context, req kyoci.CompletionRequest) (<-chan kyoci.StreamChunk, error) {
	if err := c.checkCircuit(); err != nil {
		return nil, err
	}
	ar, err := c.buildRequest(req, true)
	if err != nil {
		return nil, err
	}
	body, _ := json.Marshal(ar)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.config.BaseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("provider %s: failed to create request: %w", c.name, err)
	}
	c.setHeaders(httpReq.Header)
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.recordFailure()
		return nil, fmt.Errorf("provider %s: request failed: %w", c.name, err)
	}
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		c.recordFailure()
		return nil, fmt.Errorf("provider %s: API error (status %d): %s", c.name, resp.StatusCode, truncateBody(string(respBody), 500))
	}

	ch := make(chan kyoci.StreamChunk, 16)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		// Cancel reader: closing the body on ctx cancellation unblocks the scanner.
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
		scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		var (
			totalIn, totalOut int
			currentToolID     string
			currentToolName   string
			toolJSON          strings.Builder
			finishReason      kyoci.FinishReason
		)
		emitErr := func(msg string) {
			ch <- kyoci.StreamError(fmt.Errorf("provider %s: %s", c.name, msg))
		}
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			var ev struct {
				Type  string          `json:"type"`
				Delta json.RawMessage `json:"delta,omitempty"`
				Message *anthropicResponse `json:"message,omitempty"`
				ContentBlock *anthropicContentBlock `json:"content_block,omitempty"`
				Usage struct {
					InputTokens  int `json:"input_tokens"`
					OutputTokens int `json:"output_tokens"`
				} `json:"usage,omitempty"`
			}
			if err := json.Unmarshal([]byte(data), &ev); err != nil {
				continue
			}
			switch ev.Type {
			case "message_start":
				if ev.Message != nil {
					totalIn = ev.Message.Usage.InputTokens
				}
			case "content_block_start":
				if ev.ContentBlock != nil && ev.ContentBlock.Type == "tool_use" {
					currentToolID = ev.ContentBlock.ID
					currentToolName = ev.ContentBlock.Name
					toolJSON.Reset()
					if len(ev.ContentBlock.Input) > 0 {
						toolJSON.Write(ev.ContentBlock.Input)
					}
				}
			case "content_block_delta":
				if len(ev.Delta) == 0 {
					continue
				}
				var d struct {
					Type string `json:"type"`
					Text string `json:"text"`
					PartialJSON string `json:"partial_json"`
				}
				if json.Unmarshal(ev.Delta, &d) != nil {
					continue
				}
				switch d.Type {
				case "text_delta":
					if d.Text != "" {
						ch <- kyoci.ContentChunk(d.Text)
					}
				case "input_json_delta":
					toolJSON.WriteString(d.PartialJSON)
				}
			case "content_block_stop":
				if currentToolID != "" {
					args := toolJSON.String()
					if args == "" {
						args = "{}"
					}
					ch <- kyoci.ToolCallChunk(kyoci.ToolCall{ID: currentToolID, Name: currentToolName, Arguments: args})
					currentToolID = ""
					currentToolName = ""
				}
			case "message_delta":
				if len(ev.Delta) > 0 {
					var d struct {
						StopReason string `json:"stop_reason"`
					}
					if json.Unmarshal(ev.Delta, &d) == nil && d.StopReason != "" {
						finishReason = mapStopReason(d.StopReason)
					}
				}
				if ev.Usage.OutputTokens > 0 {
					totalOut = ev.Usage.OutputTokens
				}
			case "message_stop":
				usage := kyoci.TokenUsage{PromptTokens: totalIn, CompletionTokens: totalOut, TotalTokens: totalIn + totalOut}
				ch <- kyoci.FinalChunk("", usage, finishReason)
				return
			case "error":
				emitErr("stream error event")
				return
			}
		}
		if ctx.Err() != nil {
			ch <- kyoci.StreamError(fmt.Errorf("provider %s: stream canceled: %w", c.name, ctx.Err()))
			return
		}
		if err := scanner.Err(); err != nil {
			emitErr("streaming error: " + err.Error())
		}
	}()
	return ch, nil
}

func (c *AnthropicClient) recordFailure() {
	c.circuitBreaker.RecordFailure()
}

func truncateBody(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
