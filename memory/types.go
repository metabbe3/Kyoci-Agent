package memory

import (
	"sync"
	"time"
)

// Category represents the type of memory entry
type Category string

const (
	CategoryPreference Category = "preference"
	CategoryFact       Category = "fact"
	CategorySkill      Category = "skill"
	CategoryCorrection Category = "correction"
)

// MemoryEntry represents a single entry in long-term memory
type MemoryEntry struct {
	ID          string    `json:"id"`
	Category    Category  `json:"category"`
	Content     string    `json:"content"`
	CreatedAt   time.Time `json:"created_at"`
	AccessCount int       `json:"access_count"`
	Importance  int       `json:"importance"` // 1-10 scale
}

// memoryEntryMutex is used for thread-safe access to MemoryEntry
type memoryEntryMutex struct {
	entry MemoryEntry
	mu    sync.RWMutex
}

// Message represents a single conversation message
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// CompactionStats tracks statistics about memory compaction
type CompactionStats struct {
	TotalCompactions  int       `json:"total_compactions"`
	MessagesSummarized int      `json:"messages_summarized"`
	LastCompaction    time.Time `json:"last_compaction"`
	TokensSaved       int       `json:"tokens_saved"`
	mu                sync.RWMutex
}

// TokenEstimator is a function type for estimating token count
type TokenEstimator func(text string) int

// SummarizeFunc is a callback function that uses an LLM to summarize messages
type SummarizeFunc func(messages []Message) (string, error)

// DefaultTokenEstimator provides a rough estimate of token count
// Approximately 4 characters per token for English text
func DefaultTokenEstimator(text string) int {
	return len(text) / 4
}

// MemoryStats represents statistics about memory storage
type MemoryStats struct {
	TotalEntries int64
	Categories   map[string]int64
	DBSize       int64 // bytes
}