package builtin

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// resetTodoStoreForTest is a test helper that clears the store to isolate tests.
// It is safe to call before each test case.
func resetTodoStoreForTest() {
	todoStore.Lock()
	defer todoStore.Unlock()
	todoStore.items = make([]todoItem, 0)
	nextID = 1
}

// TestTodoTool_Snapshot_ReturnsAllTasksWithStatus verifies that the snapshot
// action returns a machine-readable JSON with all tasks, their IDs, content,
// and status — suitable for the thinking system's Verify state to check
// whether every plan step is done.
func TestTodoTool_Snapshot_ReturnsAllTasksWithStatus(t *testing.T) {
	resetTodoStoreForTest()
	tool := NewTodoTool()

	// Add two tasks, then complete one.
	if _, err := tool.Execute(context.Background(), map[string]interface{}{
		"action": "add",
		"task":   "first step",
	}); err != nil {
		t.Fatalf("add failed: %v", err)
	}
	if _, err := tool.Execute(context.Background(), map[string]interface{}{
		"action": "add",
		"task":   "second step",
	}); err != nil {
		t.Fatalf("add failed: %v", err)
	}
	if _, err := tool.Execute(context.Background(), map[string]interface{}{
		"action": "complete",
		"task":   "1",
	}); err != nil {
		t.Fatalf("complete failed: %v", err)
	}

	// Snapshot
	out, err := tool.Execute(context.Background(), map[string]interface{}{
		"action": "snapshot",
	})
	if err != nil {
		t.Fatalf("snapshot failed: %v", err)
	}

	var snap struct {
		Action  string `json:"action"`
		Summary struct {
			Total     int `json:"total"`
			Pending   int `json:"pending"`
			Completed int `json:"completed"`
		} `json:"summary"`
		Tasks []struct {
			ID      int    `json:"id"`
			Content string `json:"content"`
			Status  string `json:"status"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal([]byte(out), &snap); err != nil {
		t.Fatalf("snapshot output is not valid JSON: %v\nraw: %s", err, out)
	}
	if snap.Action != "snapshot" {
		t.Errorf("action = %q, want %q", snap.Action, "snapshot")
	}
	if snap.Summary.Total != 2 {
		t.Errorf("summary.total = %d, want 2", snap.Summary.Total)
	}
	if snap.Summary.Pending != 1 {
		t.Errorf("summary.pending = %d, want 1", snap.Summary.Pending)
	}
	if snap.Summary.Completed != 1 {
		t.Errorf("summary.completed = %d, want 1", snap.Summary.Completed)
	}
	if len(snap.Tasks) != 2 {
		t.Fatalf("tasks length = %d, want 2", len(snap.Tasks))
	}
	// Find the completed one and verify its status.
	var completedFound, pendingFound bool
	for _, tk := range snap.Tasks {
		if tk.ID == 1 && tk.Status == "completed" {
			completedFound = true
		}
		if tk.ID == 2 && tk.Status == "pending" {
			pendingFound = true
		}
	}
	if !completedFound {
		t.Error("expected task 1 to be completed in snapshot")
	}
	if !pendingFound {
		t.Error("expected task 2 to be pending in snapshot")
	}
}

// TestTodoTool_Snapshot_EmptyStore verifies the snapshot action returns a
// valid (but empty) result when the store has no tasks.
func TestTodoTool_Snapshot_EmptyStore(t *testing.T) {
	resetTodoStoreForTest()
	tool := NewTodoTool()

	out, err := tool.Execute(context.Background(), map[string]interface{}{
		"action": "snapshot",
	})
	if err != nil {
		t.Fatalf("snapshot failed: %v", err)
	}
	var snap struct {
		Summary struct {
			Total int `json:"total"`
		} `json:"summary"`
		Tasks []struct{} `json:"tasks"`
	}
	if err := json.Unmarshal([]byte(out), &snap); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if snap.Summary.Total != 0 {
		t.Errorf("total = %d, want 0 on empty store", snap.Summary.Total)
	}
	if len(snap.Tasks) != 0 {
		t.Errorf("tasks length = %d, want 0", len(snap.Tasks))
	}
}

// TestResetTodoStore_ClearsItems verifies that ResetTodoStore removes all items
// and resets the ID counter so the next add starts from 1 again.
func TestResetTodoStore_ClearsItems(t *testing.T) {
	resetTodoStoreForTest()
	tool := NewTodoTool()

	// Populate
	tool.Execute(context.Background(), map[string]interface{}{"action": "add", "task": "a"})
	tool.Execute(context.Background(), map[string]interface{}{"action": "add", "task": "b"})

	// Reset
	ResetTodoStore()

	// Verify: next add should get ID 1 (counter reset), and old items are gone.
	out, err := tool.Execute(context.Background(), map[string]interface{}{"action": "add", "task": "c"})
	if err != nil {
		t.Fatalf("add after reset failed: %v", err)
	}
	if !strings.Contains(out, `"id": 1`) {
		t.Errorf("expected next add to get ID 1 after reset; got: %s", out)
	}

	// Snapshot to confirm only one item remains.
	snap, _ := tool.Execute(context.Background(), map[string]interface{}{"action": "snapshot"})
	var snapStruct struct {
		Summary struct {
			Total int `json:"total"`
		} `json:"summary"`
	}
	json.Unmarshal([]byte(snap), &snapStruct)
	if snapStruct.Summary.Total != 1 {
		t.Errorf("total after reset + add = %d, want 1", snapStruct.Summary.Total)
	}
}

// TestResetTodoStore_IsIdempotent verifies that calling ResetTodoStore on an
// already-empty store does not panic or error.
func TestResetTodoStore_IsIdempotent(t *testing.T) {
	resetTodoStoreForTest()
	ResetTodoStore()
	ResetTodoStore() // should not panic
	tool := NewTodoTool()
	out, err := tool.Execute(context.Background(), map[string]interface{}{"action": "snapshot"})
	if err != nil {
		t.Fatalf("snapshot after double-reset failed: %v", err)
	}
	var snap struct {
		Summary struct {
			Total int `json:"total"`
		} `json:"summary"`
	}
	json.Unmarshal([]byte(out), &snap)
	if snap.Summary.Total != 0 {
		t.Errorf("total = %d, want 0 after double reset", snap.Summary.Total)
	}
}
