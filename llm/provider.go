package llm

import (
	"context"
)

// Message represents a single message in a conversation
type Message struct {
	Role       string     `json:"role"`        // "system", "user", "assistant", "tool"
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

// ToolCall represents a function call requested by the LLM
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON string
}

// ToolSchema defines a tool that the LLM can use
type ToolSchema struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"` // JSON Schema
}

// Response is the result from an LLM call
type Response struct {
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	Usage      Usage      `json:"usage"`
	StopReason string     `json:"stop_reason"` // "stop", "tool_use", "max_tokens"
	Model      string     `json:"model"`
}

// Usage tracks token consumption
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// Chunk is a streaming response piece
type Chunk struct {
	Content  string    `json:"content,omitempty"`
	ToolCall *ToolCall `json:"tool_call,omitempty"`
	Done     bool      `json:"done"`
}

// Provider is the unified interface for all LLM providers
type Provider interface {
	// Chat sends messages and returns a response (with tool calling support)
	Chat(ctx context.Context, messages []Message, tools []ToolSchema) (*Response, error)

	// Stream sends messages and returns a channel of chunks
	Stream(ctx context.Context, messages []Message, tools []ToolSchema) (<-chan Chunk, error)

	// Name returns the provider name
	Name() string
}
