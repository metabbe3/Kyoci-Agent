package kyoci

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// ==============================================================================
// Memory Interface and Types
// ==============================================================================

// MemoryType represents the type of memory storage.
// Goroutine-safe: This is a simple integer type and safe for concurrent use.
type MemoryType int

const (
	// MemoryShortTerm represents short-term memory for current conversation context
	MemoryShortTerm MemoryType = iota
	// MemoryLongTerm represents long-term memory for persistent knowledge
	MemoryLongTerm
	// MemorySkill represents skill-specific memory (e.g., for skill learning)
	MemorySkill
)

// String returns a string representation of the MemoryType.
func (mt MemoryType) String() string {
	switch mt {
	case MemoryShortTerm:
		return "short_term"
	case MemoryLongTerm:
		return "long_term"
	case MemorySkill:
		return "skill"
	default:
		return "unknown"
	}
}

// MemoryEntry represents a single entry in memory.
// Goroutine-safe: MemoryEntry values should be treated as immutable after creation.
type MemoryEntry struct {
	// ID is the unique identifier for this memory entry
	ID string
	// Content is the stored content
	Content string
	// Type is the memory type
	Type MemoryType
	// Metadata contains additional metadata
	Metadata map[string]string
	// CreatedAt is when this entry was created
	CreatedAt time.Time
	// RelevanceScore is the relevance score for this entry (from recall)
	RelevanceScore float64
	// Tags are optional tags for categorization
	Tags []string
	// TTL is the time-to-live for this entry (zero for no expiration)
	TTL time.Duration
}

// IsExpired checks if this memory entry has expired based on its TTL.
//
// Returns:
//   - bool: true if expired, false otherwise
func (e *MemoryEntry) IsExpired() bool {
	if e.TTL == 0 {
		return false
	}
	return time.Since(e.CreatedAt) > e.TTL
}

// HasTag checks if this entry has a specific tag.
//
// Parameters:
//   - tag: The tag to check
//
// Returns:
//   - bool: true if the tag exists
func (e *MemoryEntry) HasTag(tag string) bool {
	for _, t := range e.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

// GetMetadata retrieves a metadata value by key.
//
// Parameters:
//   - key: The metadata key
//
// Returns:
//   - string: The metadata value, empty if not found
func (e *MemoryEntry) GetMetadata(key string) string {
	if e.Metadata == nil {
		return ""
	}
	return e.Metadata[key]
}

// ==============================================================================
// Memory Store Interface
// ==============================================================================

// MemoryStore is the interface that all memory implementations must implement.
// It provides persistent storage and retrieval of conversation context and knowledge.
// Goroutine-safe: Implementations MUST be safe for concurrent use from multiple goroutines.
// The interface methods are called concurrently and must be properly synchronized.
type MemoryStore interface {
	// Store stores content in memory with the given type and metadata.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout control
	//   - content: The content to store
	//   - memType: The memory type (short-term, long-term, skill)
	//   - metadata: Optional metadata to attach to the entry
	//
	// Returns:
	//   - string: The ID of the stored entry
	//   - error: Any error that occurred during storage
	//
	// Errors:
	//   - context.Canceled if ctx was canceled
	//   - context.DeadlineExceeded if timeout exceeded
	//   - ErrMemoryStorage if storage failed
	Store(ctx context.Context, content string, memType MemoryType, metadata map[string]string) (string, error)

	// Recall retrieves memory entries relevant to a query.
	// Implementations should rank results by relevance.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout control
	//   - query: The query to search for
	//   - limit: Maximum number of entries to return (0 for no limit)
	//   - memType: The memory type to search (searches all if zero)
	//
	// Returns:
	//   - []MemoryEntry: Relevant memory entries, ranked by relevance
	//   - error: Any error that occurred during recall
	//
	// Errors:
	//   - context.Canceled if ctx was canceled
	//   - context.DeadlineExceeded if timeout exceeded
	//   - ErrMemoryRecall if recall failed
	Recall(ctx context.Context, query string, limit int, memType MemoryType) ([]MemoryEntry, error)

	// Delete removes a memory entry by ID.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout control
	//   - id: The ID of the entry to delete
	//
	// Returns:
	//   - error: nil on success, error if not found or deletion failed
	//
	// Errors:
	//   - context.Canceled if ctx was canceled
	//   - ErrMemoryNotFound if entry not found
	//   - ErrMemoryDelete if deletion failed
	Delete(ctx context.Context, id string) error

	// Compact reduces memory usage by removing old or less relevant entries.
	// This is typically called when memory exceeds a threshold.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout control
	//   - maxTokens: Target maximum token count (implementation-dependent)
	//
	// Returns:
	//   - error: Any error that occurred during compaction
	//
	// Errors:
	//   - context.Canceled if ctx was canceled
	//   - ErrMemoryCompact if compaction failed
	Compact(ctx context.Context, maxTokens int) error
}

// ==============================================================================
// In-Memory Store Implementation
// ==============================================================================

// InMemoryStore is a simple in-memory implementation of MemoryStore.
// It stores entries in a map and provides basic relevance scoring.
// Goroutine-safe: All methods are safe for concurrent use.
type InMemoryStore struct {
	mu        sync.RWMutex
	entries   map[string]MemoryEntry
	logger    *slog.Logger
	nextID    int64
}

// NewInMemoryStore creates a new in-memory store.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		entries: make(map[string]MemoryEntry),
		logger:  slog.Default(),
		nextID:  1,
	}
}

// Store stores content in memory.
func (s *InMemoryStore) Store(ctx context.Context, content string, memType MemoryType, metadata map[string]string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := fmt.Sprintf("mem_%d", s.nextID)
	s.nextID++

	entry := MemoryEntry{
		ID:        id,
		Content:   content,
		Type:      memType,
		Metadata:  metadata,
		CreatedAt: time.Now(),
		Tags:      []string{},
	}

	s.entries[id] = entry
	s.logger.Debug("memory stored", "id", id, "type", memType.String(), "size", len(content))

	return id, nil
}

// Recall retrieves memory entries relevant to a query.
// Uses simple keyword matching for relevance scoring.
func (s *InMemoryStore) Recall(ctx context.Context, query string, limit int, memType MemoryType) ([]MemoryEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	queryWords := tokenize(query)
	results := make([]MemoryEntry, 0)

	for _, entry := range s.entries {
		// Check expiration
		if entry.IsExpired() {
			continue
		}

		// Filter by type if specified
		if memType != 0 && entry.Type != memType {
			continue
		}

		// Calculate relevance score
		score := calculateRelevance(entry.Content, queryWords)
		if score > 0 {
			entry.RelevanceScore = score
			results = append(results, entry)
		}
	}

	// Sort by relevance score (descending)
	sortByRelevance(results)

	// Apply limit
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	s.logger.Debug("memory recalled", "query", query, "results", len(results), "limit", limit)
	return results, nil
}

// Delete removes a memory entry by ID.
func (s *InMemoryStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.entries[id]; !ok {
		return ErrMemoryNotFound
	}

	delete(s.entries, id)
	s.logger.Debug("memory deleted", "id", id)
	return nil
}

// Compact reduces memory usage by removing old entries.
// Removes expired entries first, then oldest entries until under limit.
func (s *InMemoryStore) Compact(ctx context.Context, maxTokens int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove expired entries
	for id, entry := range s.entries {
		if entry.IsExpired() {
			delete(s.entries, id)
		}
	}

	// Estimate current token count (rough approximation: 4 chars per token)
	currentTokens := 0
	for _, entry := range s.entries {
		currentTokens += len(entry.Content) / 4
	}

	// If under limit, we're done
	if currentTokens <= maxTokens {
		return nil
	}

	// Remove oldest entries until under limit
	entriesByAge := make([]MemoryEntry, 0, len(s.entries))
	for _, entry := range s.entries {
		entriesByAge = append(entriesByAge, entry)
	}

	// Sort by creation time (oldest first)
	sortByCreatedAt(entriesByAge)

	removed := 0
	for _, entry := range entriesByAge {
		if currentTokens <= maxTokens {
			break
		}
		delete(s.entries, entry.ID)
		currentTokens -= len(entry.Content) / 4
		removed++
	}

	s.logger.Debug("memory compacted", "removed", removed, "tokens", currentTokens)
	return nil
}

// Stats returns statistics about the memory store.
// Goroutine-safe: Safe for concurrent use.
func (s *InMemoryStore) Stats() MemoryStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := MemoryStats{
		TotalEntries: len(s.entries),
	}

	for _, entry := range s.entries {
		switch entry.Type {
		case MemoryShortTerm:
			stats.ShortTermEntries++
		case MemoryLongTerm:
			stats.LongTermEntries++
		case MemorySkill:
			stats.SkillEntries++
		}

		stats.EstimatedTokens += len(entry.Content) / 4
	}

	return stats
}

// MemoryStats represents statistics about a memory store.
type MemoryStats struct {
	TotalEntries      int
	ShortTermEntries  int
	LongTermEntries   int
	SkillEntries      int
	EstimatedTokens   int
}

// ==============================================================================
// Utility Functions
// ==============================================================================

// tokenize splits a string into words.
func tokenize(s string) []string {
	words := make([]string, 0)
	current := ""
	for _, r := range s {
		if isWordChar(r) {
			current += string(r)
		} else if current != "" {
			words = append(words, current)
			current = ""
		}
	}
	if current != "" {
		words = append(words, current)
	}
	return words
}

// isWordChar checks if a rune is a word character.
func isWordChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_'
}

// calculateRelevance calculates a simple relevance score based on keyword matching.
func calculateRelevance(content string, queryWords []string) float64 {
	if len(queryWords) == 0 {
		return 0
	}

	contentLower := toLower(content)
	score := 0.0
	wordSet := make(map[string]bool)

	for _, word := range queryWords {
		wordLower := toLower(word)
		if wordSet[wordLower] {
			continue
		}
		wordSet[wordLower] = true

		// Count occurrences
		count := countOccurrences(contentLower, wordLower)
		if count > 0 {
			score += float64(count)
		}
	}

	// Normalize by query length
	return score / float64(len(queryWords))
}

// toLower converts a string to lowercase.
func toLower(s string) string {
	result := make([]byte, len(s))
	for i, b := range []byte(s) {
		if b >= 'A' && b <= 'Z' {
			result[i] = b + 32
		} else {
			result[i] = b
		}
	}
	return string(result)
}

// countOccurrences counts how many times a substring appears in a string.
func countOccurrences(s, substr string) int {
	count := 0
	idx := 0
	for {
		i := findSubstring(s[idx:], substr)
		if i == -1 {
			break
		}
		count++
		idx += i + len(substr)
	}
	return count
}

// findSubstring finds the index of a substring.
func findSubstring(s, substr string) int {
	if len(substr) > len(s) {
		return -1
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// sortByRelevance sorts entries by relevance score (descending).
func sortByRelevance(entries []MemoryEntry) {
	for i := 0; i < len(entries); i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[j].RelevanceScore > entries[i].RelevanceScore {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}
}

// sortByCreatedAt sorts entries by creation time (oldest first).
func sortByCreatedAt(entries []MemoryEntry) {
	for i := 0; i < len(entries); i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[j].CreatedAt.Before(entries[i].CreatedAt) {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}
}

// ==============================================================================
// Error Types
// ==============================================================================

// ErrMemoryStorage indicates that memory storage failed.
var ErrMemoryStorage = NewValidationError("memory", "memory storage failed", nil)

// ErrMemoryRecall indicates that memory recall failed.
var ErrMemoryRecall = NewValidationError("memory", "memory recall failed", nil)

// ErrMemoryNotFound indicates that a memory entry was not found.
var ErrMemoryNotFound = NewValidationError("memory_id", "memory entry not found", nil)

// ErrMemoryDelete indicates that memory deletion failed.
var ErrMemoryDelete = NewValidationError("memory", "memory deletion failed", nil)