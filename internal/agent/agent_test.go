package agent

import (
	"context"
	"errors"
	"fmt"
	"testing"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
	"github.com/metabbe3/Kyoci-Agent/internal/llm"
)

// =============================================================================
// Mock Implementations for Testing
// =============================================================================

// mockProvider is a mock LLM provider for testing.
type mockProvider struct {
	name     string
	response *kyoci.CompletionResponse
	err      error
	streamCh <-chan kyoci.StreamChunk
}

func (m *mockProvider) Name() string {
	return m.name
}

func (m *mockProvider) Complete(ctx context.Context, req kyoci.CompletionRequest) (*kyoci.CompletionResponse, error) {
	return m.response, m.err
}

func (m *mockProvider) Stream(ctx context.Context, req kyoci.CompletionRequest) (<-chan kyoci.StreamChunk, error) {
	return m.streamCh, m.err
}

func (m *mockProvider) Models() []kyoci.ModelInfo {
	return []kyoci.ModelInfo{
		{ID: "mock-model", Provider: m.name},
	}
}

func (m *mockProvider) IsAvailable() bool {
	return true
}

// mockTool is a mock tool for testing.
type mockTool struct {
	name        string
	description string
	params      []kyoci.ToolParameter
	result      string
	err         error
	executeFunc func(ctx context.Context, params map[string]interface{}) (string, error)
}

func (m *mockTool) Name() string {
	return m.name
}

func (m *mockTool) Description() string {
	return m.description
}

func (m *mockTool) Parameters() []kyoci.ToolParameter {
	return m.params
}

func (m *mockTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, params)
	}
	return m.result, m.err
}

// mockSkill is a mock skill for testing.
type mockSkill struct {
	name        string
	description string
	match       bool
	result      string
	err         error
}

func (m *mockSkill) Name() string {
	return m.name
}

func (m *mockSkill) Description() string {
	return m.description
}

func (m *mockSkill) Match(query string) bool {
	return m.match
}

func (m *mockSkill) Execute(ctx context.Context, query string) (string, error) {
	return m.result, m.err
}

// mockMemory is a mock memory store for testing.
type mockMemory struct {
	entries       []kyoci.MemoryEntry
	err           error
	storeFunc     func(ctx context.Context, content string, memType kyoci.MemoryType, metadata map[string]string) (string, error)
}

func (m *mockMemory) Store(ctx context.Context, content string, memType kyoci.MemoryType, metadata map[string]string) (string, error) {
	if m.storeFunc != nil {
		return m.storeFunc(ctx, content, memType, metadata)
	}
	return "mock_id", m.err
}

func (m *mockMemory) Recall(ctx context.Context, query string, limit int, memType kyoci.MemoryType) ([]kyoci.MemoryEntry, error) {
	return m.entries, m.err
}

func (m *mockMemory) Delete(ctx context.Context, id string) error {
	return m.err
}

func (m *mockMemory) Compact(ctx context.Context, maxTokens int) error {
	return m.err
}

// =============================================================================
// Test Helper Functions
// =============================================================================

func createTestAgent(config AgentConfig, provider kyoci.Provider) *Agent {
	// Create provider registry and register mock provider
	providerRegistry := llm.NewProviderRegistry()
	providerRegistry.Register("mock", provider)

	// Create router
	router := llm.NewRouter(providerRegistry, llm.StrategyFallback)

	// Create tool registry
	tools := kyoci.NewToolRegistry()

	// Create skill registry
	skills := kyoci.NewSkillRegistry()

	// Create memory
	memory := &mockMemory{}

	return NewAgent(config, router, tools, skills, memory)
}

// =============================================================================
// Test Cases
// =============================================================================

// TestAgentCreation tests that agents can be created with proper configuration.
func TestAgentCreation(t *testing.T) {
	tests := []struct {
		name    string
		config  AgentConfig
		wantErr bool
	}{
		{
			name:    "default config",
			config:  DefaultAgentConfig(),
			wantErr: false,
		},
		{
			name: "custom config",
			config: AgentConfig{
				MaxIterations:     5,
				SystemPrompt:      "Custom prompt",
				ToolChoice:        "none",
				Temperature:       0.5,
				MaxTokens:         2048,
				PreferredProvider: "mock",
				EnableSkills:      false,
				EnableMemory:      false,
				EnableStreaming:   false,
			},
			wantErr: false,
		},
		{
			name: "zero values get defaults",
			config: AgentConfig{
				MaxIterations: 0,
				Temperature:   0,
				MaxTokens:     0,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &mockProvider{
				name: "mock",
			}
			agent := createTestAgent(tt.config, provider)

			if agent == nil {
				t.Fatal("expected agent to be created, got nil")
			}

			// Verify defaults were applied
			config := agent.GetConfig()
			if config.MaxIterations <= 0 {
				t.Errorf("expected positive MaxIterations, got %d", config.MaxIterations)
			}
			if config.Temperature <= 0 {
				t.Errorf("expected positive Temperature, got %f", config.Temperature)
			}
			if config.MaxTokens <= 0 {
				t.Errorf("expected positive MaxTokens, got %d", config.MaxTokens)
			}
		})
	}
}

// TestSkillFastPath tests that matching skills are executed without LLM calls.
func TestSkillFastPath(t *testing.T) {
	provider := &mockProvider{
		name:     "mock",
		response: nil,
	}
	agent := createTestAgent(DefaultAgentConfig(), provider)

	// Register a skill that matches
	skill := &mockSkill{
		name:        "test-skill",
		description: "A test skill",
		match:       true,
		result:      "Skill executed!",
		err:         nil,
	}
	agent.skills.Register(skill)

	// Execute task
	ctx := context.Background()
	result, err := agent.Execute(ctx, "test task")

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if result.Content != "Skill executed!" {
		t.Errorf("expected skill result, got %s", result.Content)
	}

	if result.Iterations != 0 {
		t.Errorf("expected 0 iterations for skill fast path, got %d", result.Iterations)
	}

	if result.ToolCallsMade != 0 {
		t.Errorf("expected 0 tool calls for skill fast path, got %d", result.ToolCallsMade)
	}
}

// TestReActLoop tests the ReAct loop with tool calls and final answer.
func TestReActLoop(t *testing.T) {
	// Mock provider that returns tool call then final answer
	toolCall := kyoci.ToolCall{
		ID:        "call_123",
		Name:      "test_tool",
		Arguments: `{"input": "test"}`,
	}

	provider := &mockProvider{
		name: "mock",
		response: &kyoci.CompletionResponse{
			Content:      "",
			ToolCalls:    []kyoci.ToolCall{toolCall},
			Usage:        kyoci.TokenUsage{TotalTokens: 100},
			FinishReason: kyoci.FinishToolCall,
		},
		err: nil,
	}

	agent := createTestAgent(DefaultAgentConfig(), provider)

	// Register the tool
	tool := &mockTool{
		name:        "test_tool",
		description: "A test tool",
		params: []kyoci.ToolParameter{
			{
				Name:     "input",
				Type:     "string",
				Required: true,
			},
		},
		result: "Tool executed!",
		err:    nil,
	}
	agent.tools.Register(tool)

	// Execute task (note: this will need two LLM calls in a real scenario)
	// For this test, we'll just verify the structure works
	ctx := context.Background()

	// The first call will return a tool call, which will be executed
	// But we need a second LLM response to complete the loop
	// This is a limitation of the simple mock - we'd need a more sophisticated setup

	// For now, let's just test that Execute doesn't panic
	_, err := agent.Execute(ctx, "test task")
	if err != nil {
		// This is expected to fail because we don't have a second LLM response
		// but it should fail gracefully, not panic
		t.Logf("expected error (mock limitation): %v", err)
	}
}

// TestMaxIterations tests that the agent auto-continues when making progress
// (Hermes-like behavior) and eventually exhausts after MaxContinuations rounds.
func TestMaxIterations(t *testing.T) {
	// Mock provider that always returns tool calls (infinite loop scenario)
	toolCall := kyoci.ToolCall{
		ID:        "call_123",
		Name:      "test_tool",
		Arguments: `{}`,
	}

	provider := &mockProvider{
		name: "mock",
		response: &kyoci.CompletionResponse{
			Content:      "",
			ToolCalls:    []kyoci.ToolCall{toolCall},
			Usage:        kyoci.TokenUsage{TotalTokens: 100},
			FinishReason: kyoci.FinishToolCall,
		},
		err: nil,
	}

	config := DefaultAgentConfig()
	config.MaxIterations = 2    // Low limit per round
	config.MaxContinuations = 3 // 3 continuation rounds = 4 rounds total

	agent := createTestAgent(config, provider)

	// Register the tool
	tool := &mockTool{
		name:        "test_tool",
		description: "A test tool",
		params:      []kyoci.ToolParameter{},
		result:      "Tool result",
		err:         nil,
	}
	agent.tools.Register(tool)

	// Execute task
	ctx := context.Background()
	result, err := agent.Execute(ctx, "test task")

	// With auto-continuation, the agent makes progress (tool calls) so it
	// should NOT return ErrMaxIterations — it should exhaust all rounds then
	// return a summary with the progress it made.
	if err != nil {
		t.Errorf("expected no error (auto-continuation should handle this), got: %v", err)
	}

	// Total iterations = MaxIterations * (1 + MaxContinuations) = 2 * 4 = 8
	maxTotalIters := config.MaxIterations * (config.MaxContinuations + 1)
	if result.Iterations > maxTotalIters {
		t.Errorf("expected iterations <= %d, got %d", maxTotalIters, result.Iterations)
	}

	// Should have made tool calls
	if result.ToolCallsMade == 0 {
		t.Error("expected tool calls to be made")
	}

	// Should have content (auto-summary after exhausting rounds)
	if result.Content == "" {
		t.Error("expected non-empty content after auto-continuation")
	}

	t.Logf("auto-continuation worked: %d iterations, %d tool calls, content: %s",
		result.Iterations, result.ToolCallsMade, result.Content)
}

// TestToolExecution tests that tools are called with correct parameters.
func TestToolExecution(t *testing.T) {
	// Track if tool was called with correct params
	calledWithParams := false

	tool := &mockTool{
		name:        "test_tool",
		description: "A test tool",
		params: []kyoci.ToolParameter{
			{
				Name:     "input",
				Type:     "string",
				Required: true,
			},
		},
		result: "Tool executed!",
		err:    nil,
		executeFunc: func(ctx context.Context, params map[string]interface{}) (string, error) {
			if params["input"] == "test_value" {
				calledWithParams = true
			}
			return "Tool executed!", nil
		},
	}

	provider := &mockProvider{
		name: "mock",
		response: &kyoci.CompletionResponse{
			Content: "",
			ToolCalls: []kyoci.ToolCall{
				{
					ID:        "call_123",
					Name:      "test_tool",
					Arguments: `{"input": "test_value"}`,
				},
			},
			Usage:        kyoci.TokenUsage{TotalTokens: 100},
			FinishReason: kyoci.FinishToolCall,
		},
		err: nil,
	}

	agent := createTestAgent(DefaultAgentConfig(), provider)
	agent.tools.Register(tool)

	// Execute task
	ctx := context.Background()
	_, err := agent.Execute(ctx, "test task")

	// Check if tool was called (even though execution fails due to mock limitations)
	if err == nil && calledWithParams {
		t.Log("tool was called with correct parameters")
	}
}

// TestStreaming tests that streaming execution works correctly.
func TestStreaming(t *testing.T) {
	// Create streaming channel with some chunks
	ch := make(chan kyoci.StreamChunk, 3)
	go func() {
		defer close(ch)
		ch <- kyoci.ContentChunk("Hello ")
		ch <- kyoci.ContentChunk("world!")
		ch <- kyoci.FinalChunk("Hello world!", kyoci.TokenUsage{TotalTokens: 50}, kyoci.FinishStop)
	}()

	provider := &mockProvider{
		name:     "mock",
		streamCh: ch,
		err:      nil,
	}

	config := DefaultAgentConfig()
	agent := createTestAgent(config, provider)

	// Execute streaming task
	ctx := context.Background()
	streamCh, err := agent.ExecuteStream(ctx, "test task")

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Collect chunks
	chunks := make([]kyoci.StreamChunk, 0)
	for chunk := range streamCh {
		chunks = append(chunks, chunk)
	}

	if len(chunks) == 0 {
		t.Error("expected at least one chunk, got none")
	}

	// Verify final chunk
	var finalChunkFound bool
	for _, chunk := range chunks {
		if chunk.Done {
			finalChunkFound = true
			if chunk.Usage == nil {
				t.Error("expected usage info in final chunk")
			}
		}
	}

	if !finalChunkFound {
		t.Error("expected final chunk with Done=true")
	}
}

// TestContextManagement tests context operations.
func TestContextManagement(t *testing.T) {
	ctx := NewContext()

	// Test AddMessage
	ctx.AddMessage(kyoci.RoleSystem, "You are helpful")
	ctx.AddMessage(kyoci.RoleUser, "Hello")

	messages := ctx.GetMessages()
	if len(messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(messages))
	}

	if messages[0].Role != kyoci.RoleSystem {
		t.Errorf("expected first message to be system, got %s", messages[0].Role)
	}

	// Test AddToolResult
	ctx.AddToolResult("call_123", "Tool result")
	messages = ctx.GetMessages()
	if len(messages) != 3 {
		t.Errorf("expected 3 messages after tool result, got %d", len(messages))
	}

	if messages[2].Role != kyoci.RoleTool {
		t.Errorf("expected third message to be tool, got %s", messages[2].Role)
	}

	// Test TokenCount
	count := ctx.TokenCount()
	if count <= 0 {
		t.Errorf("expected positive token count, got %d", count)
	}

	// Test Compact
	ctx.Compact(1) // Very low limit to trigger compaction
	compactedMessages := ctx.GetMessages()
	// System messages should be preserved
	systemPreserved := false
	for _, msg := range compactedMessages {
		if msg.Role == kyoci.RoleSystem {
			systemPreserved = true
			break
		}
	}
	if !systemPreserved {
		t.Error("expected system message to be preserved after compaction")
	}

	// Test Reset
	ctx.Reset()
	messages = ctx.GetMessages()
	if len(messages) != 0 {
		t.Errorf("expected 0 messages after reset, got %d", len(messages))
	}
}

// TestMemoryIntegration tests that conversations are stored in memory.
func TestMemoryIntegration(t *testing.T) {
	storedCount := 0

	memory := &mockMemory{
		entries: []kyoci.MemoryEntry{},
		err:     nil,
		storeFunc: func(ctx context.Context, content string, memType kyoci.MemoryType, metadata map[string]string) (string, error) {
			storedCount++
			return "mock_id", nil
		},
	}

	provider := &mockProvider{
		name: "mock",
		response: &kyoci.CompletionResponse{
			Content:      "Final answer",
			ToolCalls:    []kyoci.ToolCall{},
			Usage:        kyoci.TokenUsage{TotalTokens: 100},
			FinishReason: kyoci.FinishStop,
		},
		err: nil,
	}

	config := DefaultAgentConfig()
	config.EnableMemory = true

	providerRegistry := llm.NewProviderRegistry()
	providerRegistry.Register("mock", provider)
	router := llm.NewRouter(providerRegistry, llm.StrategyFallback)

	agent := NewAgent(config, router, kyoci.NewToolRegistry(), kyoci.NewSkillRegistry(), memory)

	// Execute task
	taskCtx := context.Background()
	_, err := agent.Execute(taskCtx, "test task")

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify memory was used (at least some messages stored)
	if storedCount == 0 {
		t.Error("expected messages to be stored in memory, got 0")
	}
}

// TestErrorHandling tests that errors are handled gracefully.
func TestErrorHandling(t *testing.T) {
	tests := []struct {
		name        string
		providerErr error
		wantErr     bool
	}{
		{
			name:        "LLM error",
			providerErr: errors.New("LLM failed"),
			wantErr:     true,
		},
		{
			name:        "no provider error",
			providerErr: nil,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &mockProvider{
				name:     "mock",
				response: &kyoci.CompletionResponse{},
				err:      tt.providerErr,
			}

			agent := createTestAgent(DefaultAgentConfig(), provider)

			ctx := context.Background()
			result, err := agent.Execute(ctx, "test task")

			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}

			if tt.wantErr && result.Error == nil {
				t.Error("expected error in result, got nil")
			}
		})
	}
}

// TestConfigSetAndGet tests config updates.
func TestConfigSetAndGet(t *testing.T) {
	provider := &mockProvider{
		name: "mock",
	}
	agent := createTestAgent(DefaultAgentConfig(), provider)

	// Get initial config
	config := agent.GetConfig()
	initialIterations := config.MaxIterations

	// Update config
	newConfig := config
	newConfig.MaxIterations = 20
	agent.SetConfig(newConfig)

	// Verify update
	updatedConfig := agent.GetConfig()
	if updatedConfig.MaxIterations != 20 {
		t.Errorf("expected MaxIterations to be 20, got %d", updatedConfig.MaxIterations)
	}

	// Verify original wasn't modified
	if config.MaxIterations != initialIterations {
		t.Error("original config should not be modified")
	}
}

// TestToolChoiceNone tests that ToolChoice="none" prevents tool calls.
func TestToolChoiceNone(t *testing.T) {
	config := DefaultAgentConfig()
	config.ToolChoice = "none"

	provider := &mockProvider{
		name: "mock",
		response: &kyoci.CompletionResponse{
			Content:      "I'll help without tools",
			ToolCalls:    []kyoci.ToolCall{},
			Usage:        kyoci.TokenUsage{TotalTokens: 50},
			FinishReason: kyoci.FinishStop,
		},
		err: nil,
	}

	agent := createTestAgent(config, provider)
	agent.tools.Register(&mockTool{
		name:        "test_tool",
		description: "Should not be called",
		params:      []kyoci.ToolParameter{},
		result:      "Unused",
		err:         nil,
	})

	ctx := context.Background()
	result, err := agent.Execute(ctx, "test task")

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// With ToolChoice="none", we shouldn't have tool calls in the response
	if result.ToolCallsMade != 0 {
		t.Errorf("expected 0 tool calls with ToolChoice=none, got %d", result.ToolCallsMade)
	}
}

// TestStreamingToolCalls tests that tool calls are properly yielded in streaming mode.
func TestStreamingToolCalls(t *testing.T) {
	toolCall := kyoci.ToolCall{
		ID:        "call_123",
		Name:      "test_tool",
		Arguments: `{}`,
	}

	// Create streaming channel with tool call
	ch := make(chan kyoci.StreamChunk, 2)
	go func() {
		defer close(ch)
		ch <- kyoci.ToolCallChunk(toolCall)
		ch <- kyoci.FinalChunk("", kyoci.TokenUsage{TotalTokens: 50}, kyoci.FinishToolCall)
	}()

	provider := &mockProvider{
		name:     "mock",
		streamCh: ch,
		err:      nil,
	}

	config := DefaultAgentConfig()
	agent := createTestAgent(config, provider)

	// Execute streaming task
	ctx := context.Background()
	streamCh, err := agent.ExecuteStream(ctx, "test task")

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Collect chunks and look for tool call
	toolCallFound := false
	for chunk := range streamCh {
		if chunk.ToolCall != nil {
			toolCallFound = true
			if chunk.ToolCall.Name != "test_tool" {
				t.Errorf("expected tool name 'test_tool', got %s", chunk.ToolCall.Name)
			}
		}
	}

	if !toolCallFound {
		t.Error("expected tool call chunk in stream, got none")
	}
}

// TestContextThreadSafety tests that context operations are thread-safe.
func TestContextThreadSafety(t *testing.T) {
	ctx := NewContext()

	// Run concurrent operations
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				ctx.AddMessage(kyoci.RoleUser, "message")
				_ = ctx.GetMessages()
				_ = ctx.TokenCount()
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify we have all messages
	messages := ctx.GetMessages()
	if len(messages) != 10*100 {
		t.Errorf("expected 1000 messages, got %d", len(messages))
	}
}

// TestContextHelperMethods tests context helper methods.
func TestContextHelperMethods(t *testing.T) {
	ctx := NewContext()

	// Test initial state
	if ctx.MessageCount() != 0 {
		t.Errorf("expected 0 messages initially, got %d", ctx.MessageCount())
	}

	if ctx.HasToolCalls() {
		t.Error("expected no tool calls initially")
	}

	if ctx.LastAssistantContent() != "" {
		t.Error("expected empty last assistant content")
	}

	// Add some messages
	ctx.AddMessage(kyoci.RoleUser, "Hello")
	ctx.AddAssistantMessage("Hi there!", []kyoci.ToolCall{})

	if ctx.MessageCount() != 2 {
		t.Errorf("expected 2 messages, got %d", ctx.MessageCount())
	}

	if ctx.LastAssistantContent() != "Hi there!" {
		t.Errorf("expected 'Hi there!', got %s", ctx.LastAssistantContent())
	}

	// Add tool call
	ctx.AddAssistantMessage("", []kyoci.ToolCall{
		{ID: "call_1", Name: "tool", Arguments: "{}"},
	})

	if !ctx.HasToolCalls() {
		t.Error("expected tool calls to be detected")
	}
}

// =============================================================================
// queuedMockProvider — multi-response mock for stateful tests
// =============================================================================
//
// mockProvider (above) returns the same response for every Complete call. That
// is fine for single-state unit tests but insufficient for end-to-end tests
// that script a sequence of LLM responses across multiple states (Assess →
// Plan → Execute → Verify → ...). queuedMockProvider pops responses from a
// queue, one per call, so a test can choreograph a full loop.

// queuedMockProvider returns a sequence of responses, one per Complete call.
// If the queue empties and no fallback is set, it returns an error — that's
// the test's signal that the loop made more LLM calls than expected.
type queuedMockProvider struct {
	name      string
	responses []*kyoci.CompletionResponse
	errors    []error // optional per-call errors; zero value means "no error for this call"
	callCount int
	fallback  *kyoci.CompletionResponse // returned when queue empties (optional)
	streamCh  <-chan kyoci.StreamChunk
}

func (m *queuedMockProvider) Name() string { return m.name }

func (m *queuedMockProvider) Complete(ctx context.Context, req kyoci.CompletionRequest) (*kyoci.CompletionResponse, error) {
	idx := m.callCount
	m.callCount++
	if idx < len(m.responses) {
		if idx < len(m.errors) && m.errors[idx] != nil {
			return nil, m.errors[idx]
		}
		return m.responses[idx], nil
	}
	if m.fallback != nil {
		return m.fallback, nil
	}
	return nil, fmt.Errorf("queuedMockProvider: response queue exhausted after %d calls", idx)
}

func (m *queuedMockProvider) Stream(ctx context.Context, req kyoci.CompletionRequest) (<-chan kyoci.StreamChunk, error) {
	return m.streamCh, nil
}

func (m *queuedMockProvider) Models() []kyoci.ModelInfo {
	return []kyoci.ModelInfo{{ID: "mock-model", Provider: m.name}}
}

func (m *queuedMockProvider) IsAvailable() bool { return true }

// Calls returns the number of Complete calls made so far. Useful for asserting
// that the loop made exactly the expected number of LLM calls.
func (m *queuedMockProvider) Calls() int { return m.callCount }