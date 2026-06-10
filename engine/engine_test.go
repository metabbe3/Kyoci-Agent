package engine

import (
	"testing"
)

func TestNewEngineTask(t *testing.T) {
	task := NewEngineTask(SourceHTTP, "test message")
	if task.ID == "" {
		t.Error("Expected task ID to be generated")
	}
	if task.Source != SourceHTTP {
		t.Errorf("Expected source %v, got %v", SourceHTTP, task.Source)
	}
	if task.Message != "test message" {
		t.Errorf("Expected message 'test message', got '%s'", task.Message)
	}
}

func TestEngineTaskBuilder(t *testing.T) {
	task := NewEngineTask(SourceGRPC, "hello").
		WithSession("session-123").
		WithMetadata("user_id", "user-456").
		WithPriority(PriorityHigh).
		WithTokenBudget(1024)

	if task.SessionID != "session-123" {
		t.Errorf("Expected session ID 'session-123', got '%s'", task.SessionID)
	}
	if task.Priority != PriorityHigh {
		t.Errorf("Expected priority %v, got %v", PriorityHigh, task.Priority)
	}
	if task.MaxTokens != 1024 {
		t.Errorf("Expected max tokens 1024, got %d", task.MaxTokens)
	}
	if task.Metadata["user_id"] != "user-456" {
		t.Errorf("Expected user_id 'user-456', got '%s'", task.Metadata["user_id"])
	}
}

func TestNewTaskResult(t *testing.T) {
	result := NewTaskResult("task-123")
	if result.TaskID != "task-123" {
		t.Errorf("Expected task ID 'task-123', got '%s'", result.TaskID)
	}
	if !result.Success {
		t.Error("Expected default success to be true")
	}
}

func TestTaskResultWithError(t *testing.T) {
	result := NewTaskResult("task-456").WithError("test error")
	if result.Success {
		t.Error("Expected success to be false after error")
	}
	if result.Error != "test error" {
		t.Errorf("Expected error 'test error', got '%s'", result.Error)
	}
}