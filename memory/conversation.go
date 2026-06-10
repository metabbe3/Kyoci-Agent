package memory

import "strings"

// ConversationMemory keeps a rolling buffer of recent messages
type ConversationMemory struct {
	messages  []Message
	maxSize   int  // Max messages to keep
	maxTokens int  // Rough token budget
}

// NewConversationMemory creates a new conversation buffer
func NewConversationMemory(maxSize, maxTokens int) *ConversationMemory {
	if maxSize <= 0 {
		maxSize = 20
	}
	if maxTokens <= 0 {
		maxTokens = 8192
	}
	return &ConversationMemory{
		messages:  make([]Message, 0, maxSize),
		maxSize:   maxSize,
		maxTokens: maxTokens,
	}
}

func (m *ConversationMemory) Add(role, content string) {
	m.messages = append(m.messages, Message{Role: role, Content: content})

	// Trim oldest messages if over size
	if len(m.messages) > m.maxSize {
		// Keep system message if it's first
		start := 1
		if m.messages[0].Role != "system" {
			start = 0
		}
		m.messages = append(m.messages[:start], m.messages[len(m.messages)-m.maxSize+start:]...)
	}

	// Trim if over token budget
	m.trimToTokenBudget()
}

func (m *ConversationMemory) GetMessages() []Message {
	return m.messages
}

func (m *ConversationMemory) Clear() {
	m.messages = m.messages[:0]
}

func (m *ConversationMemory) TokenCount() int {
	total := 0
	for _, msg := range m.messages {
		total += estimateTokens(msg.Content)
	}
	return total
}

// trimToTokenBudget removes oldest messages until under token limit
func (m *ConversationMemory) trimToTokenBudget() {
	for m.TokenCount() > m.maxTokens && len(m.messages) > 1 {
		// Don't remove system message
		if m.messages[0].Role == "system" && len(m.messages) > 1 {
			m.messages = append(m.messages[:1], m.messages[2:]...)
		} else {
			m.messages = m.messages[1:]
		}
	}
}

// estimateTokens provides a rough token estimate (1 token ≈ 4 chars)
func estimateTokens(text string) int {
	return len(text) / 4
}

// Summary returns a compact summary of the conversation
func (m *ConversationMemory) Summary() string {
	var parts []string
	for _, msg := range m.messages {
		prefix := msg.Role + ": "
		content := msg.Content
		if len(content) > 100 {
			content = content[:100] + "..."
		}
		parts = append(parts, prefix+content)
	}
	return strings.Join(parts, "\n")
}
