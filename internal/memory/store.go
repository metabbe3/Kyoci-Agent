package memory

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/metabbe3/Kyoci-Agent/internal/config"
	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// ==============================================================================
// Memory Manager
// ==============================================================================

// MemoryManager manages both short-term and long-term memory storage.
// It implements the kyoci.MemoryStore interface and is thread-safe.
type MemoryManager struct {
	mu             sync.RWMutex
	shortTerm      *ShortTermMemory
	longTerm       *LongTermMemory
	compactor      *Compactor
	logger         *slog.Logger
	config         config.MemoryConfig
	closed         bool
}

// NewStore creates a new memory manager with the given configuration.
func NewStore(cfg config.MemoryConfig) (*MemoryManager, error) {
	logger := slog.Default()

	// Initialize long-term memory first (may fail due to database)
	longTerm, err := NewLongTermMemory(cfg.GetDBPath(), logger)
	if err != nil {
		return nil, kyoci.NewValidationError("memory", "failed to initialize long-term memory", err)
	}

	// Initialize short-term memory
	shortTerm := NewShortTermMemory(cfg.GetMaxShortTermTokens(), logger)

	// Initialize compactor
	compactor := NewCompactor(shortTerm, longTerm, logger)

	return &MemoryManager{
		shortTerm: shortTerm,
		longTerm:  longTerm,
		compactor: compactor,
		logger:    logger,
		config:    cfg,
		closed:    false,
	}, nil
}

// Store stores content in memory with the given type and metadata.
// Implements kyoci.MemoryStore interface.
func (m *MemoryManager) Store(ctx context.Context, content string, memType kyoci.MemoryType, metadata map[string]string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return "", kyoci.ErrMemoryStorage
	}

	entry := &kyoci.MemoryEntry{
		ID:        generateID(),
		Content:   content,
		Type:      memType,
		Metadata:  metadata,
		CreatedAt: time.Now(),
		Tags:      []string{},
	}

	switch memType {
	case kyoci.MemoryShortTerm:
		m.shortTerm.Add(entry)
		// Check if compaction is needed
		if m.compactor.ShouldTrigger() {
			m.logger.Info("compaction threshold reached, triggering auto-compaction")
			stats, err := m.compactor.Compact(ctx, m.config.GetMaxShortTermTokens())
			if err != nil {
				m.logger.Warn("auto-compaction failed", "error", err)
			} else {
				m.logger.Info("auto-compaction completed", "entries_compacted", stats.EntriesCompacted, "tokens_saved", stats.TokensSaved)
				}
				}
				case kyoci.MemoryLongTerm, kyoci.MemorySkill:
				id, err := m.longTerm.Store(entry.Content, entry.Type, entry.Metadata)
				if err != nil {
				return "", kyoci.ErrMemoryStorage
				}
				entry.ID = id
				default:
				return "", kyoci.ErrMemoryStorage
				}

	m.logger.Debug("memory stored", "id", entry.ID, "type", memType.String(), "size", len(content))
	return entry.ID, nil
}

// Recall retrieves memory entries relevant to a query.
// Implements kyoci.MemoryStore interface.
func (m *MemoryManager) Recall(ctx context.Context, query string, limit int, memType kyoci.MemoryType) ([]kyoci.MemoryEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return nil, kyoci.ErrMemoryRecall
	}

	var results []kyoci.MemoryEntry

	// Search short-term memory if requested
	if memType == kyoci.MemoryShortTerm || memType == 0 {
		shortTermResults := m.shortTerm.Search(query, limit)
		results = append(results, shortTermResults...)
	}

	// Search long-term memory if requested
	if memType == kyoci.MemoryLongTerm || memType == kyoci.MemorySkill || memType == 0 {
		longTermResults, err := m.longTerm.Recall(query, limit, memType)
		if err != nil {
			m.logger.Warn("long-term recall failed", "error", err)
		} else {
			results = append(results, longTermResults...)
		}
	}

	// Sort results by relevance score (descending)
	sortByRelevance(results)

	// Apply limit
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	m.logger.Debug("memory recalled", "query", query, "results", len(results), "limit", limit)
	return results, nil
}

// Delete removes a memory entry by ID.
// Implements kyoci.MemoryStore interface.
func (m *MemoryManager) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return kyoci.ErrMemoryDelete
	}

	// Try short-term first
	if m.shortTerm.Delete(id) {
		m.logger.Debug("memory deleted from short-term", "id", id)
		return nil
	}

	// Try long-term
	err := m.longTerm.Delete(id)
	if err == nil {
		m.logger.Debug("memory deleted from long-term", "id", id)
		return nil
	}

	m.logger.Warn("memory not found for deletion", "id", id)
	return kyoci.ErrMemoryNotFound
}

// Compact reduces memory usage by removing old or less relevant entries.
// Implements kyoci.MemoryStore interface.
func (m *MemoryManager) Compact(ctx context.Context, maxTokens int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return kyoci.ErrMemoryDelete
	}

	stats, err := m.compactor.Compact(ctx, maxTokens)
	if err != nil {
		m.logger.Error("compaction failed", "error", err)
		return kyoci.ErrMemoryDelete
	}

	m.logger.Info("compaction completed", "entries_compacted", stats.EntriesCompacted, "tokens_saved", stats.TokensSaved)
	return nil
}

// CompactIfNeeded triggers compaction only when the short-term token budget
// exceeds the configured threshold. This is the post-task backup trigger —
// it complements the inline trigger in Store() which fires on every
// MemoryShortTerm write. A no-op (returns nil) when below threshold.
//
// The compaction target is half the short-term budget: this compacts entries
// down to a comfortable margin below the trigger threshold, since ShouldTrigger
// fires at 75% of budget and we compact toward 50%.
func (m *MemoryManager) CompactIfNeeded(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil
	}
	if !m.compactor.ShouldTrigger() {
		return nil
	}

	m.logger.Info("post-task compaction threshold reached, triggering compaction")
	target := m.config.GetMaxShortTermTokens() / 2
	stats, err := m.compactor.Compact(ctx, target)
	if err != nil {
		m.logger.Warn("post-task compaction failed", "error", err)
		return err
	}
	m.logger.Info("post-task compaction completed",
		"entries_compacted", stats.EntriesCompacted,
		"tokens_saved", stats.TokensSaved)
	return nil
}

// GetCompactor returns the compactor instance. Callers use this to install an
// LLM-backed summarizer via SetSummarizer.
func (m *MemoryManager) GetCompactor() *Compactor {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.compactor
}

// Close closes the memory manager and releases resources.
func (m *MemoryManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil
	}

	m.closed = true

	if m.longTerm != nil {
		if err := m.longTerm.Close(); err != nil {
			m.logger.Error("failed to close long-term memory", "error", err)
			return err
		}
	}

	m.logger.Info("memory manager closed")
	return nil
}

// GetShortTermMemory returns the short-term memory instance.
func (m *MemoryManager) GetShortTermMemory() *ShortTermMemory {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.shortTerm
}

// GetLongTermMemory returns the long-term memory instance.
func (m *MemoryManager) GetLongTermMemory() *LongTermMemory {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.longTerm
}

// GetStats returns statistics about the memory manager.
func (m *MemoryManager) GetStats() MemoryStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	shortTermStats := m.shortTerm.Stats()
	longTermStats := m.longTerm.Stats()

	return MemoryStats{
		TotalEntries:      shortTermStats.TotalEntries + longTermStats.TotalEntries,
		ShortTermEntries:  shortTermStats.TotalEntries,
		LongTermEntries:   longTermStats.TotalEntries,
		SkillEntries:      longTermStats.SkillEntries,
		EstimatedTokens:   shortTermStats.EstimatedTokens + longTermStats.EstimatedTokens,
	}
}

// MemoryStats represents statistics about the memory store.
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

// generateID generates a unique memory entry ID.
func generateID() string {
	return fmt.Sprintf("mem_%d", time.Now().UnixNano())
}

// sortByRelevance sorts entries by relevance score (descending).
func sortByRelevance(entries []kyoci.MemoryEntry) {
	for i := 0; i < len(entries); i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[j].RelevanceScore > entries[i].RelevanceScore {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}
}