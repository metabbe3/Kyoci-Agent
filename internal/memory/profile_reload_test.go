package memory

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// ==============================================================================================
// ProfileStore.Reload — cross-session cache freshness
//
// The `remember` tool writes profile entries via the generic MemoryStore
// (MemoryManager → LongTermMemory), NOT through ProfileStore.Set(). This means
// the ProfileStore's in-memory cache — loaded once at startup — goes stale
// when new entries arrive mid-process. Reload() refreshes the cache from
// SQLite so the ContextInjector sees entries written by prior sessions.
// ==============================================================================================

// newTestProfileStore creates a ProfileStore backed by a fresh temp SQLite DB.
func newTestProfileStore(t *testing.T) (*ProfileStore, *LongTermMemory) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test_profile.db")
	ltm, err := NewLongTermMemory(dbPath, silentLogger())
	if err != nil {
		t.Fatalf("NewLongTermMemory: %v", err)
	}
	t.Cleanup(func() { _ = ltm.Close() })
	ps := NewProfileStore(ltm, silentLogger())
	return ps, ltm
}

// storeProfileViaLongTerm simulates what the `remember` tool does: writes a
// profile-format JSON blob to LongTermMemory with the appropriate metadata.
// This is the write path the ProfileSetTool uses in production.
func storeProfileViaLongTerm(t *testing.T, ltm *LongTermMemory, key, value, category string) {
	t.Helper()
	data, _ := json.Marshal(ProfileEntry{
		Key:      key,
		Value:    value,
		Category: category,
	})
	metadata := map[string]string{
		"category": category,
		"key":      key,
	}
	if _, err := ltm.Store(context.Background(), string(data), kyoci.MemoryLongTerm, metadata); err != nil {
		t.Fatalf("ltm.Store: %v", err)
	}
}

// TestProfileStore_Reload_SeesNewEntries verifies that after writing to
// LongTermMemory (bypassing the ProfileStore cache), Reload() makes the
// new entry visible in FormatForPrompt().
func TestProfileStore_Reload_SeesNewEntries(t *testing.T) {
	ps, ltm := newTestProfileStore(t)

	// Initially the cache is empty (fresh DB).
	if got := ps.FormatForPrompt(); got != "" {
		t.Fatalf("expected empty prompt on fresh store, got %q", got)
	}

	// Simulate what the `remember` tool does: write directly to LongTermMemory.
	storeProfileViaLongTerm(t, ltm, "lang", "Go", "preference")

	// BUG: without Reload, the cache is stale — FormatForPrompt returns "".
	if got := ps.FormatForPrompt(); got != "" {
		t.Fatalf("expected stale cache to still be empty before Reload, got %q", got)
	}

	// FIX: Reload refreshes the cache from SQLite.
	if err := ps.Reload(context.Background()); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	got := ps.FormatForPrompt()
	if got == "" {
		t.Fatalf("expected non-empty prompt after Reload, got empty string")
	}
	if want := "lang"; !contains(got, want) {
		t.Errorf("expected prompt to contain key %q, got %q", want, got)
	}
	if want := "Go"; !contains(got, want) {
		t.Errorf("expected prompt to contain value %q, got %q", want, got)
	}
}

// TestProfileStore_Reload_ClearsStaleOnEmpty verifies that Reload on a DB
// that has been emptied produces an empty cache (no zombie entries).
func TestProfileStore_Reload_NoErrorOnEmptyDB(t *testing.T) {
	ps, _ := newTestProfileStore(t)

	if err := ps.Reload(context.Background()); err != nil {
		t.Fatalf("Reload on empty DB should not error: %v", err)
	}
	if got := ps.FormatForPrompt(); got != "" {
		t.Errorf("expected empty prompt after reload of empty DB, got %q", got)
	}
}

// TestContextInjector_Inject_ReloadsProfile verifies that the ContextInjector
// calls Reload() on the ProfileStore before formatting, so entries written by
// a prior session (via the `remember` tool) are visible to the next session's
// system prompt injection.
func TestContextInjector_Inject_ReloadsProfile(t *testing.T) {
	ps, ltm := newTestProfileStore(t)

	exp := NewExperienceEngine(ltm, silentLogger())
	refl := NewReflectionEngine(ltm, ps, silentLogger())
	ci := NewContextInjector(exp, ps, refl, silentLogger())

	// Initially nothing to inject (fresh store).
	if got := ci.Inject("write a Go API"); got != "" {
		t.Fatalf("expected empty injection on fresh store, got %q", got)
	}

	// Session A writes a preference via the `remember` tool path.
	storeProfileViaLongTerm(t, ltm, "go_backend", "Use net/http only", "preference")

	// Session B: Inject() should reload and see the new preference.
	got := ci.Inject("write a Go API")
	if got == "" {
		t.Fatalf("expected non-empty injection after preference stored, got empty")
	}
	if want := "net/http"; !contains(got, want) {
		t.Errorf("expected injected context to contain %q, got %q", want, got)
	}
}

// contains is a minimal substring check (avoids importing strings just for this).
func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
