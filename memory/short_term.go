package memory

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// ConversationBuffer implements ShortTerm memory with a sliding window
type ConversationBuffer struct {
	messages          []Message
	systemMessage     *Message
	maxTokens         int
	tokenEstimator    TokenEstimator
	compactor         AutoCompactor
	mu                sync.RWMutex
	currentTokenCount int
}

// NewConversationBuffer creates a new conversation buffer
func NewConversationBuffer(maxTokens int) *ConversationBuffer {
	return &ConversationBuffer{
		messages:       make([]Message, 0),
		maxTokens:      maxTokens,
		tokenEstimator: DefaultTokenEstimator,
	}
}

// Add stores a message in memory
func (cb *ConversationBuffer) Add(role, content string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	msg := Message{
		Role:    role,
		Content: content,
	}

	cb.messages = append(cb.messages, msg)
	cb.recalculateTokenCount()
}

// GetMessages returns all stored messages
func (cb *ConversationBuffer) GetMessages() []Message {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	// Return a copy to prevent external modifications
	result := make([]Message, len(cb.messages))
	copy(result, cb.messages)
	return result
}

// Clear removes all messages
func (cb *ConversationBuffer) Clear() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.messages = make([]Message, 0)
	cb.currentTokenCount = 0
}

// TokenCount estimates the total tokens used
func (cb *ConversationBuffer) TokenCount() int {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return cb.currentTokenCount
}

// SetMaxTokens sets the maximum token limit
func (cb *ConversationBuffer) SetMaxTokens(max int) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.maxTokens = max
}

// GetMaxTokens returns the maximum token limit
func (cb *ConversationBuffer) GetMaxTokens() int {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return cb.maxTokens
}

// SetTokenEstimator sets the custom token estimator function
func (cb *ConversationBuffer) SetTokenEstimator(estimator TokenEstimator) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.tokenEstimator = estimator
	cb.recalculateTokenCount()
}

// AddSystemMessage adds a system message that is always preserved
func (cb *ConversationBuffer) AddSystemMessage(content string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	msg := Message{
		Role:    "system",
		Content: content,
	}

	cb.systemMessage = &msg
	cb.recalculateTokenCount()
}

// GetSystemMessage returns the system message, if any
func (cb *ConversationBuffer) GetSystemMessage() *Message {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	if cb.systemMessage == nil {
		return nil
	}

	// Return a copy
	copy := *cb.systemMessage
	return &copy
}

// TrimOldest removes the oldest non-system message
func (cb *ConversationBuffer) TrimOldest() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if len(cb.messages) == 0 {
		return
	}

	// Remove the oldest message
	cb.messages = cb.messages[1:]
	cb.recalculateTokenCount()
}

// SetCompactor sets the auto-compactor for this buffer
func (cb *ConversationBuffer) SetCompactor(compactor AutoCompactor) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.compactor = compactor
}

// Compact performs compaction if needed
func (cb *ConversationBuffer) Compact() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.compactor == nil {
		return fmt.Errorf("no compactor configured")
	}

	if !cb.compactor.ShouldCompact() {
		return nil
	}

	// Need to release lock during compaction to avoid deadlock
	cb.mu.Unlock()
	err := cb.compactor.Compact()
	cb.mu.Lock()

	return err
}

// recalculateTokenCount recalculates the total token count
func (cb *ConversationBuffer) recalculateTokenCount() {
	total := 0

	// Count system message tokens
	if cb.systemMessage != nil {
		total += cb.tokenEstimator(cb.systemMessage.Content)
	}

	// Count message tokens
	for _, msg := range cb.messages {
		total += cb.tokenEstimator(msg.Content)
		// Add some overhead for role
		total += cb.tokenEstimator(msg.Role)
	}

	cb.currentTokenCount = total
}

// GetFullContext returns all messages including system message
func (cb *ConversationBuffer) GetFullContext() []Message {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	var result []Message

	// Add system message first
	if cb.systemMessage != nil {
		result = append(result, *cb.systemMessage)
	}

	// Add conversation messages
	result = append(result, cb.messages...)

	return result
}

// GetTokenUsage returns the current token usage as a percentage
func (cb *ConversationBuffer) GetTokenUsage() float64 {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	if cb.maxTokens <= 0 {
		return 0.0
	}

	return float64(cb.currentTokenCount) / float64(cb.maxTokens)
}

// GetLastNMessages returns the last N messages
func (cb *ConversationBuffer) GetLastNMessages(n int) []Message {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	if n <= 0 {
		return []Message{}
	}

	if n > len(cb.messages) {
		n = len(cb.messages)
	}

	start := len(cb.messages) - n
	result := make([]Message, n)
	copy(result, cb.messages[start:])
	return result
}

// GetMessagesAfter returns all messages after a given timestamp
func (cb *ConversationBuffer) GetMessagesAfter(timestamp time.Time) []Message {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	// Note: ConversationBuffer doesn't track timestamps per message
	// This method returns empty as a placeholder implementation
	// For timestamped messages, use a different implementation
	return []Message{}
}

// EstimateMessageSize estimates the token size of a potential message
func (cb *ConversationBuffer) EstimateMessageSize(role, content string) int {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return cb.tokenEstimator(role) + cb.tokenEstimator(content)
}

// String returns a string representation of the conversation
func (cb *ConversationBuffer) String() string {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("ConversationBuffer (tokens: %d/%d)\n", cb.currentTokenCount, cb.maxTokens))

	if cb.systemMessage != nil {
		builder.WriteString(fmt.Sprintf("[system] %s\n", cb.systemMessage.Content))
	}

	for i, msg := range cb.messages {
		builder.WriteString(fmt.Sprintf("[%d][%s] %s\n", i+1, msg.Role, msg.Content))
	}

	return builder.String()
}