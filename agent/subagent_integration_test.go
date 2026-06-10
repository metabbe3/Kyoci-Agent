package agent

import (
	"context"
	"testing"

	"github.com/nicholas/ai-agent/config"
	"github.com/nicholas/ai-agent/llm"
	"github.com/nicholas/ai-agent/tools"
)

// TestOrchestratorCreation tests orchestrator creation and configuration
func TestOrchestratorCreation(t *testing.T) {
	cfg, router := getConfigForTest()
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

// TestOrchestratorShouldDelegateLogic tests delegation decision logic
func TestOrchestratorShouldDelegateLogic(t *testing.T) {
	cfg, router := getConfigForTest()
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

// TestOrchestratorSpawn tests spawning a single sub-agent
func TestOrchestratorSpawn(t *testing.T) {
	cfg, router := getConfigForTest()
	toolReg := tools.NewRegistry()
	master := NewV2(cfg, router, toolReg)
	orch := NewOrchestrator(master)

	ctx := context.Background()
	sa, err := orch.Spawn(ctx, "Analyze data")
	if err != nil {
		t.Fatalf("Failed to spawn sub-agent: %v", err)
	}

	if sa == nil {
		t.Fatal("Expected non-nil sub-agent")
	}

	if sa.Goal != "Analyze data" {
		t.Errorf("Expected goal 'Analyze data', got '%s'", sa.Goal)
	}

	// Cleanup
	orch.Shutdown()
}

// TestOrchestratorSpawnBatch tests spawning multiple sub-agents
func TestOrchestratorSpawnBatch(t *testing.T) {
	cfg, router := getConfigForTest()
	toolReg := tools.NewRegistry()
	master := NewV2(cfg, router, toolReg)
	orch := NewOrchestrator(master, WithMaxParallel(3))

	goals := []string{
		"Analyze data A",
		"Analyze data B",
		"Analyze data C",
	}

	ctx := context.Background()
	subAgents, err := orch.SpawnBatch(ctx, goals)
	if err != nil {
		t.Fatalf("Failed to spawn batch: %v", err)
	}

	if len(subAgents) != 3 {
		t.Errorf("Expected 3 sub-agents, got %d", len(subAgents))
	}

	// Verify each sub-agent
	for i, sa := range subAgents {
		if sa.Goal != goals[i] {
			t.Errorf("Sub-agent %d: expected goal '%s', got '%s'", i, goals[i], sa.Goal)
		}
		if sa.GetStatus() != "running" && sa.GetStatus() != "done" {
			t.Logf("Sub-agent %d status: %s", i, sa.GetStatus())
		}
	}

	// Cleanup
	orch.Shutdown()
}

// TestOrchestratorParallelLimit tests that parallel limit is enforced
func TestOrchestratorParallelLimit(t *testing.T) {
	cfg, router := getConfigForTest()
	toolReg := tools.NewRegistry()
	master := NewV2(cfg, router, toolReg)
	orch := NewOrchestrator(master, WithMaxParallel(2)) // Limit to 2

	ctx := context.Background()

	// Spawn 2 successfully
	_, err1 := orch.Spawn(ctx, "Task 1")
	_, err2 := orch.Spawn(ctx, "Task 2")

	if err1 != nil || err2 != nil {
		t.Fatalf("Failed to spawn initial sub-agents: err1=%v, err2=%v", err1, err2)
	}

	// Try to spawn 3rd - should fail
	sa3, err := orch.Spawn(ctx, "Task 3")
	if err == nil {
		t.Error("Expected error when spawning beyond parallel limit")
	}
	if sa3 != nil {
		t.Error("Expected nil sub-agent when exceeding limit")
	}

	// Verify active count
	active := orch.GetActiveCount()
	t.Logf("Active sub-agents: %d", active)

	// Cleanup
	orch.Shutdown()
}

// TestOrchestratorCollectResults tests result collection
func TestOrchestratorCollectResults(t *testing.T) {
	cfg, router := getConfigForTest()
	toolReg := tools.NewRegistry()
	master := NewV2(cfg, router, toolReg)
	orch := NewOrchestrator(master)

	ctx := context.Background()
	_, err := orch.Spawn(ctx, "Test task")
	if err != nil {
		t.Fatalf("Failed to spawn: %v", err)
	}

	// Collect all results
	allResults := orch.CollectResults()
	if len(allResults) == 0 {
		t.Error("Expected at least one result")
	}

	// Collect filtered results
	runningResults := orch.CollectResultsFiltered("running")
	t.Logf("Running results: %d", len(runningResults))

	// Cleanup
	orch.Shutdown()
}

// TestOrchestratorGetSubAgent tests retrieving individual sub-agents
func TestOrchestratorGetSubAgent(t *testing.T) {
	cfg, router := getConfigForTest()
	toolReg := tools.NewRegistry()
	master := NewV2(cfg, router, toolReg)
	orch := NewOrchestrator(master)

	ctx := context.Background()
	sa, err := orch.Spawn(ctx, "Test task")
	if err != nil {
		t.Fatalf("Failed to spawn: %v", err)
	}

	// Get sub-agent by ID
	retrieved, ok := orch.GetSubAgent(sa.ID)
	if !ok {
		t.Error("Failed to retrieve sub-agent by ID")
	}
	if retrieved == nil {
		t.Error("Expected non-nil retrieved sub-agent")
	}
	if retrieved.ID != sa.ID {
		t.Errorf("Expected ID %s, got %s", sa.ID, retrieved.ID)
	}

	// Try to get non-existent sub-agent
	_, ok = orch.GetSubAgent("non-existent")
	if ok {
		t.Error("Expected false for non-existent sub-agent")
	}

	// Cleanup
	orch.Shutdown()
}

// getConfigForTest loads a minimal config for testing
func getConfigForTest() (*config.Config, *llm.Router) {
	cfg := &config.Config{
		LLM: config.LLMConfig{
			DefaultProvider: "mock",
			Providers: map[string]config.ProviderConfig{
				"mock": {
					Model: "mock-model",
				},
			},
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
	// Register mock provider
	router.RegisterProvider("mock", llm.NewMockProvider("Mock response for testing"))
	return cfg, router
}