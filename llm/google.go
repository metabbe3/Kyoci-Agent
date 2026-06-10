package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/nicholas/ai-agent/config"
)

// GoogleProvider implements the Provider interface for Google Gemini API
type GoogleProvider struct {
	config     config.ProviderConfig
	httpClient *http.Client
}

// NewGoogleProvider creates a new Google Gemini provider
func NewGoogleProvider(cfg config.ProviderConfig) *GoogleProvider {
	return &GoogleProvider{
		config: cfg,
		httpClient: &http.Client{
			Timeout: 300 * time.Second, // 5 min
		},
	}
}

// Name returns the provider name
func (p *GoogleProvider) Name() string {
	return "google"
}

// Chat sends messages and returns a response (non-streaming)
func (p *GoogleProvider) Chat(ctx context.Context, messages []Message, tools []ToolSchema) (*Response, error) {
	// Convert to Gemini request format
	reqBody := p.buildRequest(messages, tools)

	// Build URL
	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s", p.config.BaseURL, p.config.Model, p.config.APIKey)

	// Marshal request body
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Send request
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	// Parse response
	return p.parseResponse(body)
}

// Stream sends messages and returns a channel of chunks
func (p *GoogleProvider) Stream(ctx context.Context, messages []Message, tools []ToolSchema) (<-chan Chunk, error) {
	// Convert to Gemini request format
	reqBody := p.buildRequest(messages, tools)

	// Build URL for streaming
	url := fmt.Sprintf("%s/models/%s:streamGenerateContent?key=%s&alt=sse", p.config.BaseURL, p.config.Model, p.config.APIKey)

	// Marshal request body
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Send request
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	// Create output channel
	chunkChan := make(chan Chunk, 10)

	// Start goroutine to read SSE stream
	go p.readSSEStream(ctx, resp.Body, chunkChan)

	return chunkChan, nil
}

// buildRequest constructs the Gemini API request from messages and tools
func (p *GoogleProvider) buildRequest(messages []Message, tools []ToolSchema) map[string]interface{} {
	req := map[string]interface{}{
		"generationConfig": map[string]interface{}{
			"temperature":      p.config.Temperature,
			"maxOutputTokens":  p.config.MaxTokens,
		},
	}

	var contents []map[string]interface{}
	var systemInstruction string

	for _, msg := range messages {
		if msg.Role == "system" {
			systemInstruction = msg.Content
			continue
		}

		role := "user"
		if msg.Role == "assistant" {
			role = "model"
		}

		parts := []map[string]interface{}{}

		// Handle text content
		if msg.Content != "" {
			parts = append(parts, map[string]interface{}{
				"text": msg.Content,
			})
		}

		// Handle tool calls (from assistant)
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			for _, tc := range msg.ToolCalls {
				var args map[string]interface{}
				if err := json.Unmarshal([]byte(tc.Arguments), &args); err != nil {
					args = map[string]interface{}{}
				}
				parts = append(parts, map[string]interface{}{
					"functionCall": map[string]interface{}{
						"name": tc.Name,
						"args": args,
					},
				})
			}
		}

		// Handle tool results (from tool role)
		if msg.Role == "tool" && msg.ToolCallID != "" {
			var result map[string]interface{}
			if err := json.Unmarshal([]byte(msg.Content), &result); err != nil {
				result = map[string]interface{}{
					"result": msg.Content,
				}
			}
			parts = append(parts, map[string]interface{}{
				"functionResponse": map[string]interface{}{
					"name":     msg.Name,
					"response": result,
				},
			})
		}

		contents = append(contents, map[string]interface{}{
			"role":  role,
			"parts": parts,
		})
	}

	req["contents"] = contents

	// Add system instruction if present
	if systemInstruction != "" {
		req["systemInstruction"] = map[string]interface{}{
			"parts": []map[string]interface{}{
				{"text": systemInstruction},
			},
		}
	}

	// Add tools if present
	if len(tools) > 0 {
		functionDeclarations := []map[string]interface{}{}
		for _, tool := range tools {
			functionDeclarations = append(functionDeclarations, map[string]interface{}{
				"name":        tool.Name,
				"description": tool.Description,
				"parameters":  tool.Parameters,
			})
		}
		req["tools"] = []map[string]interface{}{
			{"function_declarations": functionDeclarations},
		}
	}

	return req
}

// parseResponse parses the Gemini API response into our Response format
func (p *GoogleProvider) parseResponse(body []byte) (*Response, error) {
	var geminiResp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text          string `json:"text,omitempty"`
					FunctionCall  *struct {
						Name string                 `json:"name"`
						Args map[string]interface{} `json:"args"`
					} `json:"functionCall,omitempty"`
				} `json:"parts"`
				Role string `json:"role"`
			} `json:"content"`
			FinishReason string `json:"finishReason"`
		} `json:"candidates"`
		UsageMetadata struct {
			PromptTokenCount     int `json:"promptTokenCount"`
			CandidatesTokenCount int `json:"candidatesTokenCount"`
			TotalTokenCount      int `json:"totalTokenCount"`
		} `json:"usageMetadata"`
	}

	if err := json.Unmarshal(body, &geminiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(geminiResp.Candidates) == 0 {
		return nil, fmt.Errorf("no candidates in response")
	}

	candidate := geminiResp.Candidates[0]
	resp := &Response{
		Model: p.config.Model,
		Usage: Usage{
			InputTokens:  geminiResp.UsageMetadata.PromptTokenCount,
			OutputTokens: geminiResp.UsageMetadata.CandidatesTokenCount,
		},
	}

	// Map finish reason
	switch candidate.FinishReason {
	case "STOP":
		resp.StopReason = "stop"
	case "MAX_TOKENS":
		resp.StopReason = "max_tokens"
	case "SAFETY", "RECITATION", "OTHER":
		resp.StopReason = "stop"
	default:
		resp.StopReason = "stop"
	}

	// Extract content and tool calls
	var contentBuilder strings.Builder
	var toolCalls []ToolCall

	for _, part := range candidate.Content.Parts {
		if part.Text != "" {
			contentBuilder.WriteString(part.Text)
		}
		if part.FunctionCall != nil {
			argsJSON, err := json.Marshal(part.FunctionCall.Args)
			if err != nil {
				argsJSON = []byte("{}")
			}
			toolCalls = append(toolCalls, ToolCall{
				ID:       fmt.Sprintf("call_%d", len(toolCalls)),
				Name:     part.FunctionCall.Name,
				Arguments: string(argsJSON),
			})
		}
	}

	resp.Content = contentBuilder.String()
	resp.ToolCalls = toolCalls

	// If there are tool calls, set stop reason to tool_use
	if len(toolCalls) > 0 && resp.StopReason == "stop" {
		resp.StopReason = "tool_use"
	}

	return resp, nil
}

// readSSEStream reads SSE events from the response body and sends chunks to the channel
func (p *GoogleProvider) readSSEStream(ctx context.Context, body io.ReadCloser, chunkChan chan<- Chunk) {
	defer close(chunkChan)
	defer body.Close()

	scanner := bufio.NewScanner(body)
	var accumulatedText strings.Builder
	var accumulatedToolCall *ToolCall
	var toolCallIndex int

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}

		line := scanner.Text()

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}

		// Parse SSE format: "data: {...}"
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		jsonStr := strings.TrimPrefix(line, "data: ")

		// Handle done signal
		if jsonStr == "[DONE]" {
			break
		}

		// Parse JSON data
		var geminiResp struct {
			Candidates []struct {
				Content struct {
					Parts []struct {
						Text          string `json:"text,omitempty"`
						FunctionCall  *struct {
							Name string                 `json:"name"`
							Args map[string]interface{} `json:"args"`
						} `json:"functionCall,omitempty"`
					} `json:"parts"`
					Role string `json:"role"`
				} `json:"content"`
				FinishReason string `json:"finishReason,omitempty"`
			} `json:"candidates"`
			UsageMetadata *struct {
				PromptTokenCount     int `json:"promptTokenCount"`
				CandidatesTokenCount int `json:"candidatesTokenCount"`
			} `json:"usageMetadata,omitempty"`
		}

		if err := json.Unmarshal([]byte(jsonStr), &geminiResp); err != nil {
			continue
		}

		if len(geminiResp.Candidates) == 0 {
			continue
		}

		candidate := geminiResp.Candidates[0]

		// Process parts
		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				// Send text chunk
				chunkChan <- Chunk{
					Content: part.Text,
					Done:    false,
				}
				accumulatedText.WriteString(part.Text)
			}

			if part.FunctionCall != nil {
				argsJSON, err := json.Marshal(part.FunctionCall.Args)
				if err != nil {
					argsJSON = []byte("{}")
				}
				toolCall := ToolCall{
					ID:        fmt.Sprintf("call_%d", toolCallIndex),
					Name:      part.FunctionCall.Name,
					Arguments: string(argsJSON),
				}
				accumulatedToolCall = &toolCall
				toolCallIndex++

				// Send tool call chunk
				chunkChan <- Chunk{
					ToolCall: &toolCall,
					Done:     false,
				}
			}
		}

		// Check if this is the final chunk
		if candidate.FinishReason != "" {
			var stopReason string
			switch candidate.FinishReason {
			case "STOP":
				stopReason = "stop"
			case "MAX_TOKENS":
				stopReason = "max_tokens"
			default:
				stopReason = "stop"
			}

			if accumulatedToolCall != nil && stopReason == "stop" {
				stopReason = "tool_use"
			}

			chunkChan <- Chunk{
				Done: true,
			}
			break
		}
	}

	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		// Log error if not from context cancellation
		fmt.Printf("Error reading SSE stream: %v\n", err)
	}
}