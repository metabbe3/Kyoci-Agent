package agent

import (
	"fmt"
	"strings"
	"sync"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// Context manages the conversation state and message history.
// It maintains the sequence of messages exchanged between the user, assistant, and tools.
// Goroutine-safe: All methods are safe for concurrent use.
// Uses internal synchronization (RWMutex) for thread-safe operations.
type Context struct {
	messages []kyoci.Message
	mu       sync.RWMutex
}

// NewContext creates a new empty conversation context.
func NewContext() *Context {
	return &Context{
		messages: make([]kyoci.Message, 0),
	}
}

// AddMessage appends a message to the conversation.
//
// Parameters:
//   - role: The message role (system, user, assistant, tool)
//   - content: The message content
func (c *Context) AddMessage(role kyoci.MessageRole, content string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.messages = append(c.messages, kyoci.Message{
		Role:    role,
		Content: content,
	})
}

// AddToolResult adds a tool result message to the conversation.
//
// Parameters:
//   - toolCallID: The ID of the tool call this result is for
//   - result: The tool execution result
func (c *Context) AddToolResult(toolCallID, result string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.messages = append(c.messages, kyoci.Message{
		Role:       kyoci.RoleTool,
		Content:    result,
		ToolCallID: toolCallID,
	})
}

// AddAssistantMessage adds an assistant message with optional tool calls.
//
// Parameters:
//   - content: The assistant's response content
//   - toolCalls: Any tool calls made by the assistant
func (c *Context) AddAssistantMessage(content string, toolCalls []kyoci.ToolCall) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.messages = append(c.messages, kyoci.Message{
		Role:       kyoci.RoleAssistant,
		Content:    content,
		ToolCalls:  toolCalls,
	})
}

// GetMessages returns a copy of all messages in the context.
//
// Returns:
//   - []kyoci.Message: Copy of all messages ready for LLM API
func (c *Context) GetMessages() []kyoci.Message {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Return a copy to prevent external modifications
	messages := make([]kyoci.Message, len(c.messages))
	copy(messages, c.messages)
	return messages
}

// TokenCount estimates the total token count in the context.
// This is a rough approximation based on character count (~4 chars per token).
//
// Returns:
//   - int: Estimated total tokens
func (c *Context) TokenCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	totalChars := 0
	for _, msg := range c.messages {
		totalChars += len(msg.Content)
	}
	// Approximate: 4 characters per token
	return totalChars / 4
}

// Compact triggers memory compaction when context is too large.
// This removes the oldest messages (except system messages) until under limit.
//
// Parameters:
//   - maxTokens: Target maximum token count
func (c *Context) Compact(maxTokens int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check token count WITHOUT calling TokenCount() (which would deadlock on RLock)
	totalChars := 0
	for _, msg := range c.messages {
		totalChars += len(msg.Content)
	}
	if totalChars/4 <= maxTokens {
		return
	}

	// Keep system messages, remove oldest until under limit
	compacted := make([]kyoci.Message, 0)
	remainingTokens := maxTokens * 4 // Convert to chars

	// Add system messages first (always keep)
	for _, msg := range c.messages {
		if msg.Role == kyoci.RoleSystem {
			compacted = append(compacted, msg)
			remainingTokens -= len(msg.Content)
		}
	}

	// Add newest messages until we hit the limit
	for i := len(c.messages) - 1; i >= 0; i-- {
		msg := c.messages[i]
		if msg.Role == kyoci.RoleSystem {
			continue // Already added
		}

		msgChars := len(msg.Content)
		if remainingTokens >= msgChars {
			compacted = append([]kyoci.Message{msg}, compacted...)
			remainingTokens -= msgChars
		}
	}

	c.messages = compacted
}

// SmartCompact compacts context by keeping system messages, the original user
// task, recent messages, and replacing older tool results with a summary.
// This is smarter than Compact() — it preserves context structure instead of
// blindly dropping messages, so the model doesn't lose track of what it did.
//
// Parameters:
//   - maxTokens: Target maximum token count
func (c *Context) SmartCompact(maxTokens int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	totalChars := 0
	for _, msg := range c.messages {
		totalChars += len(msg.Content)
	}
	if totalChars/4 <= maxTokens {
		return
	}

	// Separate messages by role
	var systemMsgs []kyoci.Message
	var otherMsgs []kyoci.Message
	for _, msg := range c.messages {
		if msg.Role == kyoci.RoleSystem {
			systemMsgs = append(systemMsgs, msg)
		} else {
			otherMsgs = append(otherMsgs, msg)
		}
	}

	if len(otherMsgs) <= 6 {
		// Not enough to compact
		return
	}

	// Keep the first user message (original task) + last 6 messages
	// Summarize everything in between into a single message
	var firstUser *kyoci.Message
	var toSummarize []kyoci.Message
	var toKeep []kyoci.Message

	for i := 0; i < len(otherMsgs); i++ {
		if otherMsgs[i].Role == kyoci.RoleUser && firstUser == nil {
			firstUser = &otherMsgs[i]
			continue
		}
		if i >= len(otherMsgs)-6 {
			toKeep = append(toKeep, otherMsgs[i])
		} else if firstUser != nil {
			toSummarize = append(toSummarize, otherMsgs[i])
		} else {
			toSummarize = append(toSummarize, otherMsgs[i])
		}
	}

	// Build a compact summary of older messages
	var summary strings.Builder
	summary.WriteString("[Context Compacted — summary of earlier actions]\n")
	toolCallCount := 0
	for _, msg := range toSummarize {
		switch msg.Role {
		case kyoci.RoleAssistant:
			if len(msg.ToolCalls) > 0 {
				for _, tc := range msg.ToolCalls {
					toolCallCount++
					summary.WriteString(fmt.Sprintf("- Called %s\n", tc.Name))
				}
			}
		case kyoci.RoleTool:
			// Just note that a tool result was received
			toolCallCount++
		case kyoci.RoleUser:
			summary.WriteString("- User: " + truncateForSummary(msg.Content, 100) + "\n")
		}
	}
	summary.WriteString(fmt.Sprintf("(%d tool calls summarized)\n", toolCallCount))

	// Rebuild messages: system + firstUser + summary + recent
	compacted := make([]kyoci.Message, 0, len(systemMsgs)+len(toKeep)+2)
	compacted = append(compacted, systemMsgs...)
	if firstUser != nil {
		compacted = append(compacted, *firstUser)
	}
	compacted = append(compacted, kyoci.Message{
		Role:    kyoci.RoleUser,
		Content: summary.String(),
	})
	compacted = append(compacted, toKeep...)

	c.messages = compacted
}

// truncateForSummary shortens a string for use in compaction summaries.
func truncateForSummary(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// Reset clears all messages from the context for a new conversation.
func (c *Context) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.messages = make([]kyoci.Message, 0)
}

// MessageCount returns the number of messages in the context.
//
// Returns:
//   - int: Number of messages
func (c *Context) MessageCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.messages)
}

// LastAssistantContent returns the content of the last assistant message.
//
// Returns:
//   - string: Last assistant content, empty if no assistant messages
func (c *Context) LastAssistantContent() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for i := len(c.messages) - 1; i >= 0; i-- {
		if c.messages[i].Role == kyoci.RoleAssistant {
			return c.messages[i].Content
		}
	}
	return ""
}

// HasToolCalls checks if there are any pending tool calls.
//
// Returns:
//   - bool: true if there are tool calls that haven't been processed
func (c *Context) HasToolCalls() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Check the last assistant message for tool calls
	if len(c.messages) == 0 {
		return false
	}

	lastMsg := c.messages[len(c.messages)-1]
	return lastMsg.Role == kyoci.RoleAssistant && len(lastMsg.ToolCalls) > 0
}