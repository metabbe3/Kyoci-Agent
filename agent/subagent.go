package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/nicholas/ai-agent/llm"
	"github.com/nicholas/ai-agent/memory"
	"github.com/nicholas/ai-agent/tools"
)

// Message represents a conversation message (local alias for isolation)
type Message struct {
	Role    string
	Content string
}

// SubAgent is an isolated agent that can run independently
type SubAgent struct {
	ID           string
	Goal         string
	Status       string    // pending, running, done, failed
	Result       string
	Conversation []Message
	TokenBudget  int
	TokensUsed   int

	// Components
	provider   llm.Provider
	memory     memory.Memory
	tools      []tools.Tool

	// Execution control
	ctx        context.Context
	cancel     context.CancelFunc
	done       chan struct{}
	startTime  time.Time
	mu         sync.RWMutex
	duration   time.Duration
	err        error

	// System prompt
	systemPrompt string
}

// SubAgentResult is the final result from a completed sub-agent
type SubAgentResult struct {
	ID         string
	Goal       string
	Status     string
	Result     string
	TokensUsed int
	Duration   time.Duration
	Error      string
}

// NewSubAgent creates a new isolated sub-agent
func NewSubAgent(id, goal string, provider llm.Provider, mem memory.Memory, toolList []tools.Tool, tokenBudget int) *SubAgent {
	ctx, cancel := context.WithCancel(context.Background())

	return &SubAgent{
		ID:           id,
		Goal:         goal,
		Status:       "pending",
		TokenBudget:  tokenBudget,
		TokensUsed:   0,
		Conversation: make([]Message, 0),
		provider:     provider,
		memory:       mem,
		tools:        toolList,
		ctx:          ctx,
		cancel:       cancel,
		done:         make(chan struct{}),
		systemPrompt: "You are a sub-agent assistant. Focus on completing the specific task you've been assigned. Be efficient and concise.",
	}
}

// SetSystemPrompt sets the system prompt for the sub-agent
func (sa *SubAgent) SetSystemPrompt(prompt string) {
	sa.mu.Lock()
	defer sa.mu.Unlock()
	sa.systemPrompt = prompt
}

// Start launches the sub-agent's execution in a goroutine
func (sa *SubAgent) Start(ctx context.Context) error {
	sa.mu.Lock()
	defer sa.mu.Unlock()

	if sa.Status != "pending" {
		return fmt.Errorf("sub-agent %s already started or completed (status: %s)", sa.ID, sa.Status)
	}

	sa.Status = "running"
	sa.startTime = time.Now()

	// Initialize memory with system prompt
	sa.memory.Add("system", sa.systemPrompt)

	go sa.run(ctx)

	return nil
}

// run executes the sub-agent's main loop
func (sa *SubAgent) run(ctx context.Context) {
	defer close(sa.done)

	// Create combined context
	saCtx, saCancel := context.WithCancel(ctx)
	defer saCancel()

	// Run the agent
	result, err := sa.executeAgent(saCtx)

	sa.mu.Lock()
	defer sa.mu.Unlock()

	if err != nil {
		sa.Status = "failed"
		sa.err = err
		sa.Result = fmt.Sprintf("Error: %v", err)
	} else {
		sa.Status = "done"
		sa.Result = result
	}
	sa.duration = time.Since(sa.startTime)
}

// executeAgent runs the isolated agent with token budget tracking
func (sa *SubAgent) executeAgent(ctx context.Context) (string, error) {
	// Add initial user message
	sa.memory.Add("user", sa.Goal)
	sa.Conversation = append(sa.Conversation, Message{
		Role:    "user",
		Content: sa.Goal,
	})

	// Build tool schemas
	toolSchemas := sa.buildToolSchemas()

	// ReAct loop
	maxIterations := 10

	for i := 0; i < maxIterations; i++ {
		// Check token budget before each iteration
		if sa.TokensUsed >= sa.TokenBudget {
			return fmt.Sprintf("Stopped after using %d tokens (budget: %d). Partial result: %s",
				sa.TokensUsed, sa.TokenBudget, sa.Result), fmt.Errorf("token budget exceeded")
		}

		// Get messages from memory
		messages := sa.convertMessages()

		// Call LLM
		resp, err := sa.provider.Chat(ctx, messages, toolSchemas)
		if err != nil {
			return "", fmt.Errorf("LLM call failed (iter %d): %w", i+1, err)
		}

		// Track tokens
		sa.TokensUsed += resp.Usage.InputTokens + resp.Usage.OutputTokens

		// Add assistant response to memory
		sa.memory.Add("assistant", resp.Content)
		sa.Conversation = append(sa.Conversation, Message{
			Role:    "assistant",
			Content: resp.Content,
		})

		// Check if done
		if resp.StopReason == "stop" || resp.StopReason == "max_tokens" {
			return resp.Content, nil
		}

		// Handle tool calls
		if resp.StopReason == "tool_use" && len(resp.ToolCalls) > 0 {
			for _, tc := range resp.ToolCalls {
				toolResult, err := sa.executeTool(ctx, tc.Name, tc.Arguments)
				if err != nil {
					sa.memory.Add("tool", fmt.Sprintf("[Tool: %s] Error: %v", tc.Name, err))
					sa.Conversation = append(sa.Conversation, Message{
						Role:    "tool",
						Content: fmt.Sprintf("[Tool: %s] Error: %v", tc.Name, err),
					})
				} else {
					sa.memory.Add("tool", fmt.Sprintf("[Tool: %s] %s", tc.Name, toolResult))
					sa.Conversation = append(sa.Conversation, Message{
						Role:    "tool",
						Content: fmt.Sprintf("[Tool: %s] %s", tc.Name, toolResult),
					})
				}
			}
			continue
		}

		// Unexpected stop reason - return what we have
		return resp.Content, nil
	}

	return sa.Result, fmt.Errorf("max iterations (%d) reached", maxIterations)
}

// buildToolSchemas converts tools to LLM-compatible schemas
func (sa *SubAgent) buildToolSchemas() []llm.ToolSchema {
	schemas := make([]llm.ToolSchema, 0, len(sa.tools))
	for _, t := range sa.tools {
		schemas = append(schemas, llm.ToolSchema{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.Parameters(),
		})
	}
	return schemas
}

// convertMessages converts memory messages to LLM messages
func (sa *SubAgent) convertMessages() []llm.Message {
	memMessages := sa.memory.GetMessages()
	result := make([]llm.Message, len(memMessages))
	for i, m := range memMessages {
		result[i] = llm.Message{Role: m.Role, Content: m.Content}
	}
	return result
}

// executeTool executes a tool by name
func (sa *SubAgent) executeTool(ctx context.Context, name, args string) (string, error) {
	for _, t := range sa.tools {
		if t.Name() == name {
			return t.Execute(ctx, []byte(args))
		}
	}
	return "", fmt.Errorf("tool not found: %s", name)
}

// Wait blocks until the sub-agent completes or context is cancelled
func (sa *SubAgent) Wait() (*SubAgentResult, error) {
	<-sa.done
	return sa.GetResult(), nil
}

// WaitWithTimeout blocks until completion or timeout
func (sa *SubAgent) WaitWithTimeout(timeout time.Duration) (*SubAgentResult, error) {
	select {
	case <-sa.done:
		return sa.GetResult(), nil
	case <-time.After(timeout):
		sa.mu.RLock()
		defer sa.mu.RUnlock()
		return &SubAgentResult{
			ID:         sa.ID,
			Goal:       sa.Goal,
			Status:     sa.Status,
			Result:     "",
			TokensUsed: sa.TokensUsed,
			Duration:   time.Since(sa.startTime),
			Error:      "timeout",
		}, fmt.Errorf("wait timeout after %v", timeout)
	}
}

// Cancel stops the sub-agent
func (sa *SubAgent) Cancel() {
	sa.mu.Lock()
	defer sa.mu.Unlock()
	if sa.Status == "running" {
		sa.cancel()
	}
}

// IsDone returns true if the sub-agent has completed
func (sa *SubAgent) IsDone() bool {
	sa.mu.RLock()
	defer sa.mu.RUnlock()
	return sa.Status == "done" || sa.Status == "failed"
}

// GetResult returns the current result (nil if not done)
func (sa *SubAgent) GetResult() *SubAgentResult {
	sa.mu.RLock()
	defer sa.mu.RUnlock()

	var errMsg string
	if sa.err != nil {
		errMsg = sa.err.Error()
	}

	return &SubAgentResult{
		ID:         sa.ID,
		Goal:       sa.Goal,
		Status:     sa.Status,
		Result:     sa.Result,
		TokensUsed: sa.TokensUsed,
		Duration:   sa.duration,
		Error:      errMsg,
	}
}

// GetStatus returns the current status
func (sa *SubAgent) GetStatus() string {
	sa.mu.RLock()
	defer sa.mu.RUnlock()
	return sa.Status
}

// GetConversation returns the conversation history
func (sa *SubAgent) GetConversation() []Message {
	sa.mu.RLock()
	defer sa.mu.RUnlock()
	// Return a copy
	result := make([]Message, len(sa.Conversation))
	copy(result, sa.Conversation)
	return result
}

// AddMessage adds a message to the conversation
func (sa *SubAgent) AddMessage(role, content string) {
	sa.mu.Lock()
	defer sa.mu.Unlock()
	sa.Conversation = append(sa.Conversation, Message{
		Role:    role,
		Content: content,
	})
}

// ResetConversation clears the conversation history
func (sa *SubAgent) ResetConversation() {
	sa.mu.Lock()
	defer sa.mu.Unlock()
	sa.Conversation = make([]Message, 0)
}

// GetTokenUsage returns token usage information
func (sa *SubAgent) GetTokenUsage() (used, budget int, percentage float64) {
	sa.mu.RLock()
	defer sa.mu.RUnlock()
	return sa.TokensUsed, sa.TokenBudget, float64(sa.TokensUsed) / float64(sa.TokenBudget)
}

// String returns a string representation
func (sa *SubAgent) String() string {
	sa.mu.RLock()
	defer sa.mu.RUnlock()
	return fmt.Sprintf("SubAgent[%s]: status=%s, goal=%q, tokens=%d/%d",
		sa.ID, sa.Status, truncateString(sa.Goal, 40), sa.TokensUsed, sa.TokenBudget)
}

// truncateString truncates a string to max length
func truncateString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}