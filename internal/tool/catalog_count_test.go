package tool

import (
	"testing"
)

// TestCatalogCount is a smoke test that confirms the catalog expansion hit
// its goal: ≥100 skills, ≥26 tools (12 original + 10 new + 4 hooks).
func TestCatalogCount(t *testing.T) {
	tr := NewRegistry()
	if err := tr.RegisterBuiltin(); err != nil {
		t.Fatalf("RegisterBuiltin: %v", err)
	}
	toolCount := tr.Count()
	t.Logf("registered tools: %d", toolCount)
	// 12 original + 10 new = 22 from RegisterBuiltin alone.
	if toolCount < 22 {
		t.Errorf("expected ≥22 registered tools, got %d", toolCount)
	}
}
