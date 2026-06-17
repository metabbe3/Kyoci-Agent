package memory

import (
	"log/slog"
	"strings"
	"sync"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// ==============================================================================
// Short-Term Memory
// ==============================================================================

// ShortTermMemory manages a circular buffer of recent conversation messages
// with token budget management. It is thread-safe.
type ShortTermMemory struct {
	mu            sync.RWMutex
	entries       []*kyoci.MemoryEntry
	maxTokens     int
	currentTokens int
	logger        *slog.Logger
	maxCapacity   int // Maximum number of entries regardless of token limit
}

// NewShortTermMemory creates a new short-term memory instance.
func NewShortTermMemory(maxTokens int, logger *slog.Logger) *ShortTermMemory {
	return &ShortTermMemory{
		entries:       make([]*kyoci.MemoryEntry, 0),
		maxTokens:     maxTokens,
		currentTokens: 0,
		logger:        logger,
		maxCapacity:   1000, // Reasonable limit to prevent unbounded growth
	}
}

// Add adds an entry to short-term memory, auto-trimming oldest entries
// when exceeding the max tokens or max capacity.
func (stm *ShortTermMemory) Add(entry *kyoci.MemoryEntry) {
	stm.mu.Lock()
	defer stm.mu.Unlock()

	entryTokens := estimateTokens(entry.Content)

	// Add the entry
	stm.entries = append(stm.entries, entry)
	stm.currentTokens += entryTokens

	// Trim entries if we exceed token limit or capacity
	stm.trim()
	stm.logger.Debug("entry added to short-term memory", "id", entry.ID, "tokens", entryTokens, "total_tokens", stm.currentTokens)
}

// GetRecent returns the last n entries from short-term memory.
func (stm *ShortTermMemory) GetRecent(n int) []kyoci.MemoryEntry {
	stm.mu.RLock()
	defer stm.mu.RUnlock()

	if n <= 0 {
		return []kyoci.MemoryEntry{}
	}

	start := len(stm.entries) - n
	if start < 0 {
		start = 0
	}

	results := make([]kyoci.MemoryEntry, 0, len(stm.entries)-start)
	for i := start; i < len(stm.entries); i++ {
		results = append(results, *stm.entries[i])
	}

	return results
}

// Search performs a simple keyword search on short-term memory entries.
func (stm *ShortTermMemory) Search(query string, limit int) []kyoci.MemoryEntry {
	stm.mu.RLock()
	defer stm.mu.RUnlock()

	queryWords := tokenize(strings.ToLower(query))
	results := make([]kyoci.MemoryEntry, 0)

	for _, entry := range stm.entries {
		// Check expiration
		if entry.IsExpired() {
			continue
		}

		// Calculate relevance score
		score := calculateRelevance(strings.ToLower(entry.Content), queryWords)
		if score > 0 {
			entry.RelevanceScore = score
			results = append(results, *entry)
		}
	}

	// Sort by relevance score (descending)
	sortByRelevance(results)

	// Apply limit
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results
}

// Clear resets the short-term memory buffer.
func (stm *ShortTermMemory) Clear() {
	stm.mu.Lock()
	defer stm.mu.Unlock()

	stm.entries = make([]*kyoci.MemoryEntry, 0)
	stm.currentTokens = 0
	stm.logger.Debug("short-term memory cleared")
}

// TokenCount returns the current estimated token count.
func (stm *ShortTermMemory) TokenCount() int {
	stm.mu.RLock()
	defer stm.mu.RUnlock()

	return stm.currentTokens
}

// GetMaxTokens returns the maximum token capacity.
func (stm *ShortTermMemory) GetMaxTokens() int {
	stm.mu.RLock()
	defer stm.mu.RUnlock()
	return stm.maxTokens
}

// GetAllEntries returns all entries from short-term memory.
func (stm *ShortTermMemory) GetAllEntries() []*kyoci.MemoryEntry {
	stm.mu.RLock()
	defer stm.mu.RUnlock()

	// Return a copy to avoid external modification
	result := make([]*kyoci.MemoryEntry, len(stm.entries))
	for i, entry := range stm.entries {
		// Create a copy of the entry
		entryCopy := *entry
		result[i] = &entryCopy
	}
	return result
}

// Delete removes an entry by ID from short-term memory.
func (stm *ShortTermMemory) Delete(id string) bool {
	stm.mu.Lock()
	defer stm.mu.Unlock()

	for i, entry := range stm.entries {
		if entry.ID == id {
			stm.currentTokens -= estimateTokens(entry.Content)
			if stm.currentTokens < 0 {
				stm.currentTokens = 0
			}
			stm.entries = append(stm.entries[:i], stm.entries[i+1:]...)
			stm.logger.Debug("entry deleted from short-term memory", "id", id)
			return true
		}
	}
	return false
}

// Stats returns statistics about the short-term memory.
func (stm *ShortTermMemory) Stats() MemoryStats {
	stm.mu.RLock()
	defer stm.mu.RUnlock()

	return MemoryStats{
		TotalEntries:     len(stm.entries),
		ShortTermEntries: len(stm.entries),
		EstimatedTokens:  stm.currentTokens,
	}
}

// trim removes oldest entries until we're under token limit or capacity.
func (stm *ShortTermMemory) trim() {
	for len(stm.entries) > 0 && (stm.currentTokens > stm.maxTokens || len(stm.entries) > stm.maxCapacity) {
		oldest := stm.entries[0]
		stm.currentTokens -= estimateTokens(oldest.Content)
		if stm.currentTokens < 0 {
			stm.currentTokens = 0
		}
		stm.entries = stm.entries[1:]
		stm.logger.Debug("trimmed oldest entry from short-term memory", "id", oldest.ID, "remaining_tokens", stm.currentTokens)
	}
}

// estimateTokens estimates the number of tokens in a string (rough: chars/4).
func estimateTokens(s string) int {
	return len(s) / 4
}

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

	score := 0.0
	wordSet := make(map[string]bool)

	for _, word := range queryWords {
		if wordSet[word] {
			continue
		}
		wordSet[word] = true

		// Count occurrences
		count := countOccurrences(content, word)
		if count > 0 {
			score += float64(count)
		}
	}

	// Normalize by query length
	return score / float64(len(queryWords))
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