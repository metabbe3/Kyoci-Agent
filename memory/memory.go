package memory

import (
	"fmt"
)

// Memory is the interface for conversation memory systems
type Memory interface {
	// Add stores a message in memory
	Add(role, content string)

	// GetMessages returns all stored messages
	GetMessages() []Message

	// Clear removes all messages
	Clear()

	// TokenCount estimates the total tokens used
	TokenCount() int
}

// ShortTerm is the interface for short-term (conversation) memory
type ShortTerm interface {
	Memory

	// SetMaxTokens sets the maximum token limit
	SetMaxTokens(max int)

	// GetMaxTokens returns the maximum token limit
	GetMaxTokens() int

	// SetTokenEstimator sets the custom token estimator function
	SetTokenEstimator(estimator TokenEstimator)

	// AddSystemMessage adds a system message that is always preserved
	AddSystemMessage(content string)

	// GetSystemMessage returns the system message, if any
	GetSystemMessage() *Message

	// TrimOldest removes the oldest non-system message
	TrimOldest()

	// Compact performs compaction if needed
	Compact() error
}

// LongTerm is the interface for long-term (persistent) memory
type LongTerm interface {
	// AddEntry adds a new memory entry
	AddEntry(entry *MemoryEntry) error

	// GetEntry retrieves an entry by ID
	GetEntry(id string) (*MemoryEntry, error)

	// GetByCategory retrieves all entries of a specific category
	GetByCategory(category Category) ([]*MemoryEntry, error)

	// Search searches for entries containing the query string
	Search(query string) ([]*MemoryEntry, error)

	// UpdateEntry updates an existing entry
	UpdateEntry(entry *MemoryEntry) error

	// DeleteEntry removes an entry by ID
	DeleteEntry(id string) error

	// GetAll retrieves all entries
	GetAll() ([]*MemoryEntry, error)

	// ExtractFromConversation analyzes conversation messages and extracts important facts
	ExtractFromConversation(messages []Message) ([]*MemoryEntry, error)

	// IncrementAccess increments the access count for an entry
	IncrementAccess(id string) error

	// Save persists all entries to storage
	Save() error

	// Load loads entries from storage
	Load() error
}

// AutoCompactor is the interface for automatic memory compaction
type AutoCompactor interface {
	// SetThreshold sets the token usage threshold (0.0-1.0) for triggering compaction
	SetThreshold(threshold float64)

	// GetThreshold returns the current threshold
	GetThreshold() float64

	// SetSummarizeFunc sets the function used to summarize messages
	SetSummarizeFunc(fn SummarizeFunc)

	// ShouldCompact determines if compaction is needed
	ShouldCompact() bool

	// Compact performs the compaction
	Compact() error

	// GetStats returns compaction statistics
	GetStats() CompactionStats

	// ResetStats resets compaction statistics
	ResetStats()
}

// CompactionError represents an error during compaction
type CompactionError struct {
	Op  string
	Err error
}

func (e *CompactionError) Error() string {
	return fmt.Sprintf("compaction error during %s: %v", e.Op, e.Err)
}

func (e *CompactionError) Unwrap() error {
	return e.Err
}