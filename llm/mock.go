package llm

import (
	"context"
)

// MockProvider is a mock LLM provider for testing
type MockProvider struct {
	ResponseContent string
	FailAfter       int
	callCount       int
}

// NewMockProvider creates a new mock provider
func NewMockProvider(response string) *MockProvider {
	return &MockProvider{
		ResponseContent: response,
	}
}

// Chat returns a mock response
func (m *MockProvider) Chat(ctx context.Context, messages []Message, tools []ToolSchema) (*Response, error) {
	m.callCount++
	if m.FailAfter > 0 && m.callCount >= m.FailAfter {
		return nil, ctx.Err()
	}
	return &Response{
		Content: m.ResponseContent,
		Usage: Usage{
			InputTokens:  100,
			OutputTokens: 50,
		},
		StopReason: "stop",
	}, nil
}

// Stream returns a mock streaming response
func (m *MockProvider) Stream(ctx context.Context, messages []Message, tools []ToolSchema) (<-chan Chunk, error) {
	ch := make(chan Chunk, 1)
	ch <- Chunk{Content: m.ResponseContent, Done: true}
	close(ch)
	return ch, nil
}

// Name returns the provider name
func (m *MockProvider) Name() string {
	return "mock"
}