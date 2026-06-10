package agent

import (
	"testing"

	"github.com/nicholas/ai-agent/config"
	"github.com/nicholas/ai-agent/llm"
	"github.com/nicholas/ai-agent/tools"
)

// TestOrchestratorOptions tests configuration options
func TestOrchestratorOptions(t *testing.T) {
	cfg := &config.Config{
		LLM: config.LLMConfig{
			DefaultProvider: "openai",
		},
		Agent: config.AgentConfig{
			Template: "system",
			DataDir:  "/tmp/ai-agent-test",
		},
		Memory: config.MemoryConfig{
			MaxTokens:            5000,
			CompactionThreshold: 0.75,
			LongTermPath:         "/tmp/ai-agent-test/memory.json",
		},
	}

	router := llm.NewRouter(cfg)
	toolReg := tools.NewRegistry()
	master := NewV2(cfg, router, toolReg)

	// Test default values
	orch := NewOrchestrator(master)
	if orch.maxParallel != 3 {
		t.Errorf("Expected maxParallel=3, got %d", orch.maxParallel)
	}
	if orch.maxDepth != 1 {
		t.Errorf("Expected maxDepth=1, got %d", orch.maxDepth)
	}
	if orch.tokenBudget != 10000 {
		t.Errorf("Expected tokenBudget=10000, got %d", orch.tokenBudget)
	}

	// Test with options
	orch2 := NewOrchestrator(
		master,
		WithMaxParallel(5),
		WithMaxDepth(2),
		WithTokenBudget(20000),
	)
	if orch2.maxParallel != 5 {
		t.Errorf("Expected maxParallel=5, got %d", orch2.maxParallel)
	}
	if orch2.maxDepth != 2 {
		t.Errorf("Expected maxDepth=2, got %d", orch2.maxDepth)
	}
	if orch2.tokenBudget != 20000 {
		t.Errorf("Expected tokenBudget=20000, got %d", orch2.tokenBudget)
	}
}

// TestShouldDelegate tests the delegation logic (no LLM needed)
func TestShouldDelegate(t *testing.T) {
	cfg := &config.Config{
		LLM: config.LLMConfig{
			DefaultProvider: "openai",
		},
		Agent: config.AgentConfig{
			Template: "system",
			DataDir:  "/tmp/ai-agent-test",
		},
		Memory: config.MemoryConfig{
			MaxTokens:            5000,
			CompactionThreshold: 0.75,
			LongTermPath:         "/tmp/ai-agent-test/memory.json",
		},
	}

	router := llm.NewRouter(cfg)
	toolReg := tools.NewRegistry()
	master := NewV2(cfg, router, toolReg)
	orch := NewOrchestrator(master)

	tests := []struct {
		name     string
		task     string
		level    int
		expected bool
	}{
		{"Simple question", "What is the capital of France?", 0, false},
		{"Multi-step with 'and'", "Research and analyze market trends", 0, true},
		{"Multiple tasks with comma", "Task A, Task B, Task C", 0, true},
		{"Parallel keywords", "Process these files simultaneously", 0, true},
		{"Multiple questions", "What is X? What is Y?", 0, true},
		{"Deep nesting", "Complex task", 5, false}, // Exceeds maxDepth=1
		{"Various tasks", "Handle various tasks", 0, true},
		{"Several items", "Process several items", 0, true},
		{"Each of pattern", "For each file, analyze it", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := orch.ShouldDelegate(tt.task, tt.level)
			if result != tt.expected {
				t.Errorf("Task '%s' at level %d: expected delegate=%v, got %v",
					tt.task, tt.level, tt.expected, result)
			}
		})
	}
}

// TestOrchestratorGetStats tests statistics gathering (no LLM needed)
func TestOrchestratorGetStats(t *testing.T) {
	cfg := &config.Config{
		LLM: config.LLMConfig{
			DefaultProvider: "openai",
		},
		Agent: config.AgentConfig{
			Template: "system",
			DataDir:  "/tmp/ai-agent-test",
		},
		Memory: config.MemoryConfig{
			MaxTokens:            5000,
			CompactionThreshold: 0.75,
			LongTermPath:         "/tmp/ai-agent-test/memory.json",
		},
	}

	router := llm.NewRouter(cfg)
	toolReg := tools.NewRegistry()
	master := NewV2(cfg, router, toolReg)
	orch := NewOrchestrator(master, WithMaxParallel(5), WithMaxDepth(2), WithTokenBudget(15000))

	stats := orch.GetStats()

	// Verify stats
	if stats["max_parallel"] != 5 {
		t.Errorf("Expected max_parallel=5, got %v", stats["max_parallel"])
	}
	if stats["max_depth"] != 2 {
		t.Errorf("Expected max_depth=2, got %v", stats["max_depth"])
	}
	if stats["token_budget"] != 15000 {
		t.Errorf("Expected token_budget=15000, got %v", stats["token_budget"])
	}

	// Status counts should be present
	statusCounts, ok := stats["status_counts"].(map[string]int)
	if !ok {
		t.Error("Expected status_counts to be a map[string]int")
	} else {
		t.Logf("Status counts: %v", statusCounts)
	}
}

// TestOrchestratorSynthesize tests result synthesis (no LLM needed)
func TestOrchestratorSynthesize(t *testing.T) {
	cfg := &config.Config{
		LLM: config.LLMConfig{
			DefaultProvider: "openai",
		},
		Agent: config.AgentConfig{
			Template: "system",
			DataDir:  "/tmp/ai-agent-test",
		},
		Memory: config.MemoryConfig{
			MaxTokens:            5000,
			CompactionThreshold: 0.75,
			LongTermPath:         "/tmp/ai-agent-test/memory.json",
		},
	}

	router := llm.NewRouter(cfg)
	toolReg := tools.NewRegistry()
	master := NewV2(cfg, router, toolReg)
	orch := NewOrchestrator(master)

	// Create mock results
	results := []SubAgentResult{
		{
			ID:         "sa-1",
			Goal:       "Task A",
			Status:     "done",
			Result:     "Result from Task A",
			TokensUsed: 100,
			Duration:   100000000, // 100ms
		},
		{
			ID:         "sa-2",
			Goal:       "Task B",
			Status:     "done",
			Result:     "Result from Task B",
			TokensUsed: 150,
			Duration:   150000000, // 150ms
		},
		{
			ID:         "sa-3",
			Goal:       "Task C",
			Status:     "failed",
			Error:      "timeout",
			TokensUsed: 50,
			Duration:   200000000, // 200ms
		},
	}

	summary := orch.Synthesize(results)

	// Verify summary contains expected content
	if !contains(summary, "Sub-Agent Results Summary") {
		t.Error("Summary should contain 'Sub-Agent Results Summary'")
	}
	if !contains(summary, "2 successful") {
		t.Error("Summary should show 2 successful")
	}
	if !contains(summary, "1 failed") {
		t.Error("Summary should show 1 failed")
	}
	if !contains(summary, "300") { // Total tokens (100+150+50)
		t.Error("Summary should show total tokens")
	}
}

// contains is a helper to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}