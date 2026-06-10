package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/nicholas/ai-agent/config"
)

// TestCircuitBreaker_Basic tests basic circuit breaker functionality
func TestCircuitBreaker_Basic(t *testing.T) {
	cb := NewCircuitBreaker(
		"test",
		WithFailureThreshold(3),
		WithSuccessThreshold(2),
		WithTimeout(100*time.Millisecond),
	)

	// Should start closed
	if state, _, _ := cb.Stats(); state != StateClosed {
		t.Errorf("expected StateClosed, got %v", state)
	}

	// Simulate failures
	for i := 0; i < 3; i++ {
		_, err := cb.Execute(func() (interface{}, error) {
			return nil, errors.New("test error")
		})
		if err == nil {
			t.Error("expected error")
		}
	}

	// Should be open now
	if state, _, _ := cb.Stats(); state != StateOpen {
		t.Errorf("expected StateOpen, got %v", state)
	}

	// Next call should fail with ErrCircuitOpen
	_, err := cb.Execute(func() (interface{}, error) {
		return nil, nil
	})
	if err != ErrCircuitOpen {
		t.Errorf("expected ErrCircuitOpen, got %v", err)
	}
}

// TestCircuitBreaker_HalfOpen tests half-open state recovery
func TestCircuitBreaker_HalfOpen(t *testing.T) {
	cb := NewCircuitBreaker(
		"test-recovery",
		WithFailureThreshold(2),
		WithSuccessThreshold(2),
		WithTimeout(50*time.Millisecond),
	)

	// Trip the circuit
	for i := 0; i < 2; i++ {
		cb.Execute(func() (interface{}, error) {
			return nil, errors.New("test error")
		})
	}

	// Wait for timeout
	time.Sleep(60 * time.Millisecond)

	// First success in half-open
	_, err := cb.Execute(func() (interface{}, error) {
		return "success", nil
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Second success should close circuit
	_, err = cb.Execute(func() (interface{}, error) {
		return "success", nil
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Should be closed now
	if state, _, _ := cb.Stats(); state != StateClosed {
		t.Errorf("expected StateClosed, got %v", state)
	}
}

// TestTieredRouter_Route tests routing functionality
func TestTieredRouter_Route(t *testing.T) {
	cfg := config.DefaultConfig()
	tr := NewTieredRouter(cfg)

	// Try to route to Tier2
	provider, err := tr.Route(int(Tier2))
	if err != nil {
		t.Errorf("expected provider, got error: %v", err)
	}
	if provider == nil {
		t.Fatal("expected non-nil provider")
	}

	t.Logf("Routed to provider: %s (model: %s, tier: %d)", provider.Name, provider.Model, provider.Tier)

	// Get available tiers
	tiers := tr.AvailableTiers()
	t.Logf("Available tiers: %v", tiers)

	if len(tiers) == 0 {
		t.Error("expected at least one available tier")
	}
}

// TestDAGExecutor_Parallel tests parallel execution
func TestDAGExecutor_Parallel(t *testing.T) {
	ctx := context.Background()
	plan := DAGPlan{
		PlanID:        "test-parallel",
		ExecutionMode: "PARALLEL",
		Tasks: []DAGTask{
			{Step: 1, ServiceTarget: "service-a", RPCMethod: "method-a", TierFallback: 1, Dependencies: []int{}},
			{Step: 2, ServiceTarget: "service-b", RPCMethod: "method-b", TierFallback: 1, Dependencies: []int{}},
			{Step: 3, ServiceTarget: "service-c", RPCMethod: "method-c", TierFallback: 1, Dependencies: []int{}},
		},
	}

	exec := NewDAGExecutor(3, 5*time.Second, nil)
	defer exec.Shutdown()

	results := exec.Execute(ctx, plan)

	// Count non-zero results (result.Step indicates it was set)
	actualCount := 0
	for _, r := range results {
		if r.Step != 0 {
			actualCount++
		}
	}
	if actualCount != 3 {
		t.Errorf("expected 3 results, got %d", actualCount)
	}

	for _, result := range results {
		if result.Step != 0 { // Only log actual results
			t.Logf("Step %d: Success=%v, Error=%v", result.Step, result.Success, result.Error)
		}
	}
}

// TestDAGExecutor_SequentialWithDeps tests sequential execution with dependencies
func TestDAGExecutor_SequentialWithDeps(t *testing.T) {
	ctx := context.Background()
	plan := DAGPlan{
		PlanID:        "test-deps",
		ExecutionMode: "SEQUENTIAL",
		Tasks: []DAGTask{
			{Step: 1, ServiceTarget: "service-a", RPCMethod: "method-a", TierFallback: 1, Dependencies: []int{}},
			{Step: 2, ServiceTarget: "service-b", RPCMethod: "method-b", TierFallback: 1, Dependencies: []int{1}},
			{Step: 3, ServiceTarget: "service-c", RPCMethod: "method-c", TierFallback: 1, Dependencies: []int{1, 2}},
		},
	}

	exec := NewDAGExecutor(1, 5*time.Second, nil)
	defer exec.Shutdown()

	results := exec.Execute(ctx, plan)

	// Count non-zero results (result.Step indicates it was set)
	actualCount := 0
	for _, r := range results {
		if r.Step != 0 {
			actualCount++
		}
	}
	if actualCount != 3 {
		t.Errorf("expected 3 results, got %d", actualCount)
	}

	for _, result := range results {
		if result.Step != 0 { // Only log actual results
			t.Logf("Step %d: Success=%v, Error=%v", result.Step, result.Success, result.Error)
		}
	}
}

// TestParseDAGPlan tests JSON parsing of DAG plans
func TestParseDAGPlan(t *testing.T) {
	jsonData := []byte(`{
		"plan_id": "test-plan",
		"execution_mode": "PARALLEL",
		"tasks": [
			{
				"step": 1,
				"service_target": "test-service",
				"rpc_method": "test-method",
				"payload": "{\"key\":\"value\"}",
				"tier_fallback": 1,
				"dependencies": []
			}
		]
	}`)

	plan, err := ParseDAGPlan(jsonData)
	if err != nil {
		t.Fatalf("failed to parse plan: %v", err)
	}

	if plan.PlanID != "test-plan" {
		t.Errorf("expected plan_id 'test-plan', got '%s'", plan.PlanID)
	}

	if plan.ExecutionMode != "PARALLEL" {
		t.Errorf("expected mode 'PARALLEL', got '%s'", plan.ExecutionMode)
	}

	if len(plan.Tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(plan.Tasks))
	}
}

// TestTierTimeouts tests tier timeout configuration
func TestTierTimeouts(t *testing.T) {
	expected := map[Tier]time.Duration{
		Tier0: 5 * time.Second,
		Tier1: 150 * time.Second,
		Tier2: 300 * time.Second,
	}

	for tier, expectedTimeout := range expected {
		if TierTimeouts[tier] != expectedTimeout {
			t.Errorf("Tier %d: expected timeout %v, got %v", tier, expectedTimeout, TierTimeouts[tier])
		}
	}
}

// TestTaskResult_Marshaling tests JSON marshaling of task results with errors
func TestTaskResult_Marshaling(t *testing.T) {
	result := TaskResult{
		Step:     1,
		Success:  false,
		Error:    errors.New("test error"),
		Duration: 100 * time.Millisecond,
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	t.Logf("Marshaled result: %s", string(data))

	var unmarshaled TaskResult
	err = json.Unmarshal(data, &unmarshaled)
	if err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if unmarshaled.Step != 1 {
		t.Errorf("expected step 1, got %d", unmarshaled.Step)
	}
}

// BenchmarkCircuitBreaker_Execute benchmarks circuit breaker execution
func BenchmarkCircuitBreaker_Execute(b *testing.B) {
	cb := NewCircuitBreaker("bench", WithFailureThreshold(1000))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = cb.Execute(func() (interface{}, error) {
			return "result", nil
		})
	}
}