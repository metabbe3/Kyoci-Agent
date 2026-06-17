package memory

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// ==============================================================================
// Compactor
// ==============================================================================

// CompactorStats represents statistics about a compaction operation.
type CompactorStats struct {
	EntriesCompacted int
	TokensSaved      int
}

// SummarizerFunc generates a long-term summary from a batch of short-term
// entries. Returning ("", nil) means "nothing worth remembering" — the
// compactor will skip long-term storage entirely. Returning an error triggers
// the legacy per-entry fallback. The function receives the entries to be
// compacted (i.e., the ones being evicted from short-term).
type SummarizerFunc func(ctx context.Context, entries []*kyoci.MemoryEntry) (string, error)

// Compactor manages automatic compaction of short-term memory to long-term storage.
type Compactor struct {
	shortTerm   *ShortTermMemory
	longTerm    *LongTermMemory
	logger      *slog.Logger
	threshold   float64
	summarizer  SummarizerFunc
	mu          sync.RWMutex
}

// NewCompactor creates a new compactor instance.
func NewCompactor(shortTerm *ShortTermMemory, longTerm *LongTermMemory, logger *slog.Logger) *Compactor {
	return &Compactor{
		shortTerm: shortTerm,
		longTerm:  longTerm,
		logger:    logger,
		threshold: 0.75, // Default threshold
	}
}

// SetThreshold sets the compaction threshold (0.0-1.0).
func (c *Compactor) SetThreshold(threshold float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if threshold < 0 {
		c.threshold = 0
	} else if threshold > 1 {
		c.threshold = 1
	} else {
		c.threshold = threshold
	}
}

// GetThreshold returns the current compaction threshold.
func (c *Compactor) GetThreshold() float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.threshold
}

// SetSummarizer installs an optional LLM-backed summarizer. When set, the
// compactor delegates summary generation to fn instead of using the legacy
// concatenation logic. Pass nil to revert to the legacy behavior.
func (c *Compactor) SetSummarizer(fn SummarizerFunc) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.summarizer = fn
}

// ShouldTrigger checks if compaction should be triggered based on current usage.
func (c *Compactor) ShouldTrigger() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	currentTokens := c.shortTerm.TokenCount()
	maxTokens := c.shortTerm.GetMaxTokens()

	if maxTokens == 0 {
		return false
	}

	return float64(currentTokens) > (c.threshold * float64(maxTokens))
}

// Compact performs compaction by summarizing old entries and moving to long-term.
// Returns statistics about the compaction operation.
func (c *Compactor) Compact(ctx context.Context, maxTokens int) (*CompactorStats, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	stats := &CompactorStats{
		EntriesCompacted: 0,
		TokensSaved:      0,
	}

	currentTokens := c.shortTerm.TokenCount()
	if currentTokens <= maxTokens {
		c.logger.Debug("compaction not needed", "current_tokens", currentTokens, "max_tokens", maxTokens)
		return stats, nil
	}

	// Get recent entries to keep in short-term (keep newest 50%)
	recentEntries := c.shortTerm.GetRecent(maxTokens / 2)

	// Get all entries from short-term
	allEntries := c.shortTerm.GetAllEntries()

	// Determine which entries to compact (keep only recent entries)
	entriesToCompact := make([]*kyoci.MemoryEntry, 0)
	keepIDs := make(map[string]bool)

	for _, entry := range recentEntries {
		keepIDs[entry.ID] = true
	}

	for _, entry := range allEntries {
		if !keepIDs[entry.ID] {
			entriesToCompact = append(entriesToCompact, entry)
		}
	}

	if len(entriesToCompact) == 0 {
		c.logger.Debug("no entries to compact")
		return stats, nil
	}

	// Generate summary and store in long-term
	summary, err := c.generateSummary(ctx, entriesToCompact)
	switch {
	case err != nil:
		c.logger.Warn("failed to generate summary, storing entries individually", "error", err)
		// Fallback: store entries individually
		for _, entry := range entriesToCompact {
			_, err := c.longTerm.Store(entry.Content, kyoci.MemoryLongTerm, entry.Metadata)
			if err != nil {
				c.logger.Error("failed to store entry in long-term", "id", entry.ID, "error", err)
				continue
			}
			stats.EntriesCompacted++
			stats.TokensSaved += estimateTokens(entry.Content)
		}
	case summary == "":
		// Summarizer (or legacy path) determined nothing is worth remembering.
		// Skip long-term storage but still remove entries from short-term below.
		c.logger.Info("compaction produced empty summary, skipping long-term storage", "entries", len(entriesToCompact))
	default:
		// Store summary
		metadata := map[string]string{
			"compacted": "true",
			"entry_count": fmt.Sprintf("%d", len(entriesToCompact)),
		}
		_, err := c.longTerm.Store(summary, kyoci.MemoryLongTerm, metadata)
		if err != nil {
			c.logger.Error("failed to store summary in long-term", "error", err)
		} else {
			stats.EntriesCompacted = len(entriesToCompact)
			for _, entry := range entriesToCompact {
				stats.TokensSaved += estimateTokens(entry.Content)
			}
		}
	}

	// Remove compacted entries from short-term
	for _, entry := range entriesToCompact {
		c.shortTerm.Delete(entry.ID)
	}

	c.logger.Info("compaction completed",
		"entries_compacted", stats.EntriesCompacted,
		"tokens_saved", stats.TokensSaved,
		"summary_used", summary != "")

	return stats, nil
}

// generateSummary generates a summary of entries. If an LLM-backed summarizer
// is installed (via SetSummarizer), it is used instead of the legacy
// concatenation logic. The lock is already held by the caller (Compact).
func (c *Compactor) generateSummary(ctx context.Context, entries []*kyoci.MemoryEntry) (string, error) {
	if len(entries) == 0 {
		return "", nil
	}

	// If a summarizer is installed, delegate to it.
	if c.summarizer != nil {
		return c.summarizer(ctx, entries)
	}

	// Legacy concatenation-based summary (preserved for callers without a summarizer).
	var summary strings.Builder
	summary.WriteString(fmt.Sprintf("Summary of %d compacted messages:\n", len(entries)))

	for i, entry := range entries {
		if i >= 5 { // Limit to first 5 entries in summary
			summary.WriteString(fmt.Sprintf("... and %d more messages\n", len(entries)-5))
			break
		}
		summary.WriteString(fmt.Sprintf("[%d] %s\n", i+1, truncateString(entry.Content, 200)))
	}

	// Add metadata hints
	summary.WriteString("\nMetadata:")
	if entryTags := collectTags(entries); len(entryTags) > 0 {
		summary.WriteString(fmt.Sprintf(" Tags: %s", strings.Join(entryTags, ", ")))
	}

	return summary.String(), nil
}

// truncateString truncates a string to a maximum length.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// collectTags collects all unique tags from entries.
func collectTags(entries []*kyoci.MemoryEntry) []string {
	tagSet := make(map[string]bool)
	for _, entry := range entries {
		for _, tag := range entry.Tags {
			tagSet[tag] = true
		}
	}

	tags := make([]string, 0, len(tagSet))
	for tag := range tagSet {
		tags = append(tags, tag)
	}
	return tags
}