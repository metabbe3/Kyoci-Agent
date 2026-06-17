package memory

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/metabbe3/Kyoci-Agent/internal/config"
	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// =============================================================================
// MemoryManager — CompactIfNeeded + GetCompactor accessors
//
// These tests pin the post-task compaction trigger path. CompactIfNeeded is a
// no-op when short-term memory is below the threshold, and triggers compaction
// (moving entries to long-term) when the threshold is exceeded.
// =============================================================================

// newTestStore creates a MemoryManager backed by a fresh temp SQLite DB with
// a small short-term budget so compaction triggers after a handful of entries.
func newTestStore(t *testing.T) *MemoryManager {
	t.Helper()
	cfg := config.MemoryConfig{
		DBPath:             filepath.Join(t.TempDir(), "test_store.db"),
		MaxShortTermTokens: 20, // tiny budget; 2-token entries → ~10 fit before trim
	}
	mgr, err := NewStore(cfg)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Close() })
	return mgr
}

// TestGetCompactor_ReturnsCompactor verifies the accessor returns a non-nil
// compactor that callers can use to install a summarizer.
func TestGetCompactor_ReturnsCompactor(t *testing.T) {
	mgr := newTestStore(t)
	c := mgr.GetCompactor()
	if c == nil {
		t.Fatalf("GetCompactor returned nil")
	}
}

// TestCompactIfNeeded_NoTrigger_NoOp verifies that when short-term memory is
// below the threshold, CompactIfNeeded is a no-op and returns nil.
func TestCompactIfNeeded_NoTrigger_NoOp(t *testing.T) {
	mgr := newTestStore(t)
	// No entries added → short-term is empty → well below threshold.
	if mgr.GetCompactor().ShouldTrigger() {
		t.Fatalf("expected ShouldTrigger=false on empty memory")
	}
	err := mgr.CompactIfNeeded(context.Background())
	if err != nil {
		t.Errorf("CompactIfNeeded returned error when below threshold: %v", err)
	}
}

// TestCompactIfNeeded_TriggersCompact verifies that when short-term memory
// exceeds the threshold, CompactIfNeeded triggers compaction. Entries are
// added directly to short-term (bypassing Store's inline compaction) to
// isolate the CompactIfNeeded code path.
func TestCompactIfNeeded_TriggersCompact(t *testing.T) {
	mgr := newTestStore(t)
	stm := mgr.GetShortTermMemory()

	// Add entries directly to short-term, bypassing Store's inline compaction.
	// MaxShortTermTokens=20, threshold=0.75 → trigger at >15 tokens.
	// Each entry is 10 chars → 2 tokens. trim() caps at 10 entries (20 tokens).
	// ShouldTrigger: 20 > 15 → true. Compact target=10: 20 > 10 → proceeds.
	// GetRecent(10/2)=GetRecent(5) → keeps last 5, compacts oldest 5.
	for i := 0; i < 12; i++ {
		stm.Add(&kyoci.MemoryEntry{
			ID:        fmt.Sprintf("msg-%02d", i),
			Content:   fmt.Sprintf("msg-%02d-ok", i), // 10 chars → 2 tokens
			Type:      kyoci.MemoryShortTerm,
			CreatedAt: time.Now(),
			Tags:      []string{},
		})
	}

	if !mgr.GetCompactor().ShouldTrigger() {
		t.Fatalf("expected ShouldTrigger=true after filling short-term")
	}

	err := mgr.CompactIfNeeded(context.Background())
	if err != nil {
		t.Errorf("CompactIfNeeded returned error: %v", err)
	}

	// After compaction, short-term should be under threshold.
	if mgr.GetCompactor().ShouldTrigger() {
		t.Errorf("expected ShouldTrigger=false after compaction")
	}

	// Long-term memory should now have at least one entry (the summary).
	ltmStats := mgr.GetLongTermMemory().Stats()
	if ltmStats.TotalEntries == 0 {
		t.Errorf("expected at least 1 entry in long-term memory after compaction, got 0")
	}
}
