package memory

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// =============================================================================
// Compactor + SummarizerFunc integration (L3 auto-compaction)
//
// These tests pin the contract between the Compactor and an optional
// SummarizerFunc: when a summarizer is installed, the compactor must delegate
// to it instead of the legacy concatenation logic. An empty summarizer result
// means "nothing worth remembering" and must skip long-term storage entirely.
// A summarizer error must fall back to the legacy per-entry storage path.
// =============================================================================

// newTestCompactor creates a Compactor backed by a fresh temp SQLite DB and a
// short-term buffer large enough that trim() won't interfere with tests.
func newTestCompactor(t *testing.T) *Compactor {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test_compactor.db")
	ltm, err := NewLongTermMemory(dbPath, silentLogger())
	if err != nil {
		t.Fatalf("NewLongTermMemory: %v", err)
	}
	t.Cleanup(func() { _ = ltm.Close() })
	stm := NewShortTermMemory(100000, silentLogger())
	return NewCompactor(stm, ltm, silentLogger())
}

func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

// addEntries fills short-term memory with n entries, each large enough that a
// handful exceed a small maxTokens budget (forcing compaction to trigger and
// to have entries to compact).
func addEntries(c *Compactor, n int) {
	for i := 0; i < n; i++ {
		c.shortTerm.Add(&kyoci.MemoryEntry{
			ID:        fmt.Sprintf("entry-%d", i),
			Content:   fmt.Sprintf("user prefers golang and snake_case json tags (entry %d)", i),
			Type:      kyoci.MemoryShortTerm,
			CreatedAt: time.Now(),
			Tags:      []string{},
		})
	}
}

// TestCompact_WithLLMSummarizer_Success verifies that when a summarizer is
// installed and returns prose, that prose is stored verbatim in long-term memory.
func TestCompact_WithLLMSummarizer_Success(t *testing.T) {
	c := newTestCompactor(t)
	addEntries(c, 8)

	const wantSummary = "User prefers Go with net/http and snake_case JSON tags."
	called := false
	c.SetSummarizer(func(ctx context.Context, entries []*kyoci.MemoryEntry) (string, error) {
		called = true
		if len(entries) == 0 {
			t.Errorf("summarizer received 0 entries")
		}
		return wantSummary, nil
	})

	stats, err := c.Compact(context.Background(), 10) // small maxTokens → forces compaction
	if err != nil {
		t.Fatalf("Compact: %v", err)
	}
	if !called {
		t.Errorf("summarizer was never called")
	}
	if stats.EntriesCompacted == 0 {
		t.Errorf("expected entries to be compacted, got 0")
	}

	// Verify the summary was stored verbatim in long-term memory.
	results, err := c.longTerm.Recall(context.Background(),"prefers", 10, kyoci.MemoryLongTerm)
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	found := false
	for _, e := range results {
		if strings.Contains(e.Content, wantSummary) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("summary %q not found in long-term memory; got %d entries", wantSummary, len(results))
	}
}

// TestCompact_WithLLMSummarizer_Error_Fallback verifies that when the summarizer
// returns an error, the compactor falls back to storing entries individually
// (the legacy error path). No data should be lost.
func TestCompact_WithLLMSummarizer_Error_Fallback(t *testing.T) {
	c := newTestCompactor(t)
	addEntries(c, 8)

	c.SetSummarizer(func(ctx context.Context, entries []*kyoci.MemoryEntry) (string, error) {
		return "", fmt.Errorf("simulated LLM outage")
	})

	stats, err := c.Compact(context.Background(), 10)
	if err != nil {
		t.Fatalf("Compact: %v", err)
	}
	if stats.EntriesCompacted == 0 {
		t.Errorf("expected fallback to store entries individually, got 0 compacted")
	}

	// Entries should be present individually in long-term memory.
	results, err := c.longTerm.Recall(context.Background(),"prefers", 20, kyoci.MemoryLongTerm)
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(results) == 0 {
		t.Errorf("expected individual entries in long-term memory after fallback, got 0")
	}
}

// TestCompact_WithLLMSummarizer_EmptyResult verifies that when the summarizer
// returns ("", nil) — meaning "nothing worth remembering" — the compactor does
// NOT store anything in long-term memory. Entries are still removed from
// short-term (they were processed, just not memorable).
func TestCompact_WithLLMSummarizer_EmptyResult(t *testing.T) {
	c := newTestCompactor(t)
	addEntries(c, 8)

	c.SetSummarizer(func(ctx context.Context, entries []*kyoci.MemoryEntry) (string, error) {
		return "", nil // nothing to remember
	})

	before := c.longTerm.Stats().TotalEntries

	_, err := c.Compact(context.Background(), 10)
	if err != nil {
		t.Fatalf("Compact: %v", err)
	}

	after := c.longTerm.Stats().TotalEntries
	if after != before {
		t.Errorf("expected no entries stored for empty summary; before=%d after=%d", before, after)
	}
}

// TestCompact_NoSummarizer_LegacyConcatenation verifies that without a
// summarizer installed, the existing concatenation-based behavior is unchanged.
// This pins backward compatibility for callers that never call SetSummarizer.
func TestCompact_NoSummarizer_LegacyConcatenation(t *testing.T) {
	c := newTestCompactor(t)
	addEntries(c, 8)

	// No SetSummarizer call — legacy path must be used.
	stats, err := c.Compact(context.Background(), 10)
	if err != nil {
		t.Fatalf("Compact: %v", err)
	}
	if stats.EntriesCompacted == 0 {
		t.Errorf("expected entries compacted via legacy path, got 0")
	}

	// Legacy summary begins with "Summary of N compacted messages:".
	results, err := c.longTerm.Recall(context.Background(),"summary", 10, kyoci.MemoryLongTerm)
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	found := false
	for _, e := range results {
		if strings.Contains(e.Content, "Summary of") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("legacy summary not found in long-term memory; got %d entries", len(results))
	}
}
