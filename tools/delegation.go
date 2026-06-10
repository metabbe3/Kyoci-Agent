package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// DelegateFunc is the callback type for spawning sub-agents
type DelegateFunc func(ctx context.Context, goal string, context string, toolsets []string) (string, error)

// TaskStatus represents the status of a delegated task
type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCancelled TaskStatus = "cancelled"
)

// Task represents a delegated sub-agent task
type Task struct {
	ID          string
	Goal        string
	Context     string
	Toolsets    []string
	Status      TaskStatus
	Result      string
	Error       string
	CreatedAt   time.Time
	StartedAt   time.Time
	CompletedAt time.Time
	MaxParallel int
}

// DelegationTool manages spawning and tracking sub-agents for parallel work
type DelegationTool struct {
	tasks     sync.Map
	callback  DelegateFunc
	semaphore chan struct{} // For limiting parallel execution
}

// NewDelegationTool creates a new delegation tool
func NewDelegationTool() *DelegationTool {
	return &DelegationTool{}
}

func (t *DelegationTool) Name() string {
	return "delegation"
}

func (t *DelegationTool) Description() string {
	return "Spawn and manage sub-agents for parallel task execution. Supports spawning tasks, listing status, and cancelling running tasks."
}

func (t *DelegationTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"description": "Action to perform: spawn, list, status, cancel",
				"enum":        []string{"spawn", "list", "status", "cancel"},
			},
			"task_id": map[string]interface{}{
				"type":        "string",
				"description": "Task identifier (required for status and cancel)",
			},
			"goal": map[string]interface{}{
				"type":        "string",
				"description": "The goal/objective for the sub-agent (required for spawn)",
			},
			"context": map[string]interface{}{
				"type":        "string",
				"description": "Additional context information for the sub-agent (optional for spawn)",
			},
			"toolsets": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "List of toolsets available to the sub-agent (optional for spawn)",
			},
			"max_parallel": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum number of parallel tasks (optional for spawn, defaults to 3)",
			},
		},
	}
}

func (t *DelegationTool) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var params struct {
		Action     string   `json:"action"`
		TaskID     string   `json:"task_id"`
		Goal       string   `json:"goal"`
		Context    string   `json:"context"`
		Toolsets   []string `json:"toolsets"`
		MaxParallel int     `json:"max_parallel"`
	}

	if err := json.Unmarshal(raw, &params); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}

	switch params.Action {
	case "spawn":
		if params.Goal == "" {
			return "", fmt.Errorf("goal is required for spawn action")
		}
		return t.spawnTask(ctx, params.Goal, params.Context, params.Toolsets, params.MaxParallel)

	case "list":
		return t.listTasks()

	case "status":
		if params.TaskID == "" {
			return "", fmt.Errorf("task_id is required for status action")
		}
		return t.getTaskStatus(params.TaskID)

	case "cancel":
		if params.TaskID == "" {
			return "", fmt.Errorf("task_id is required for cancel action")
		}
		return t.cancelTask(params.TaskID)

	default:
		return "", fmt.Errorf("unknown action: %s. Valid actions: spawn, list, status, cancel", params.Action)
	}
}

// SetCallback registers the delegate callback function
func (t *DelegationTool) SetCallback(callback DelegateFunc) {
	t.callback = callback
}

// SetMaxParallel sets the maximum number of parallel tasks
func (t *DelegationTool) SetMaxParallel(max int) {
	t.semaphore = make(chan struct{}, max)
}

// spawnTask creates and starts a new delegated task
func (t *DelegationTool) spawnTask(ctx context.Context, goal, context string, toolsets []string, maxParallel int) (string, error) {
	// Check if callback is configured
	if t.callback == nil {
		return "", fmt.Errorf("delegation not configured: no callback function set")
	}

	// Generate unique task ID
	taskID := fmt.Sprintf("task_%d", time.Now().UnixNano())

	// Set default max parallel if not specified
	if maxParallel <= 0 {
		maxParallel = 3
	}

	// Initialize semaphore if not set
	if t.semaphore == nil {
		t.SetMaxParallel(maxParallel)
	}

	task := &Task{
		ID:          taskID,
		Goal:        goal,
		Context:     context,
		Toolsets:    toolsets,
		Status:      TaskStatusPending,
		CreatedAt:   time.Now(),
		MaxParallel: maxParallel,
	}

	t.tasks.Store(taskID, task)

	// Execute the task in a goroutine
	go t.executeTask(task, ctx)

	return fmt.Sprintf("Task '%s' spawned successfully. Status: pending. Goal: %s", taskID, goal), nil
}

// executeTask runs the delegated task using the callback
func (t *DelegationTool) executeTask(task *Task, ctx context.Context) {
	// Wait for semaphore slot
	t.semaphore <- struct{}{}
	defer func() { <-t.semaphore }()

	// Update status to running
	task.Status = TaskStatusRunning
	task.StartedAt = time.Now()

	// Execute the callback
	result, err := t.callback(ctx, task.Goal, task.Context, task.Toolsets)

	task.CompletedAt = time.Now()

	if err != nil {
		task.Status = TaskStatusFailed
		task.Error = err.Error()
	} else {
		task.Status = TaskStatusCompleted
		task.Result = result
	}
}

// listTasks returns information about all tasks
func (t *DelegationTool) listTasks() (string, error) {
	var tasks []string

	t.tasks.Range(func(key, value interface{}) bool {
		task := value.(*Task)

		var toolsetList string
		if len(task.Toolsets) > 0 {
			toolsetList = strings.Join(task.Toolsets, ", ")
		} else {
			toolsetList = "all"
		}

		tasks = append(tasks, fmt.Sprintf(
			"- ID: %s\n  Status: %s\n  Goal: %s\n  Context: %s\n  Toolsets: %s\n  Created: %s\n  Started: %s\n  Completed: %s",
			task.ID,
			task.Status,
			task.Goal,
			task.Context,
			toolsetList,
			task.CreatedAt.Format(time.RFC3339),
			formatTimeOrNil(task.StartedAt),
			formatTimeOrNil(task.CompletedAt),
		))

		return true
	})

	if len(tasks) == 0 {
		return "No delegated tasks found", nil
	}

	return fmt.Sprintf("Delegated Tasks (%d):\n%s", len(tasks), strings.Join(tasks, "\n\n")), nil
}

// getTaskStatus returns detailed status of a specific task
func (t *DelegationTool) getTaskStatus(taskID string) (string, error) {
	value, exists := t.tasks.Load(taskID)
	if !exists {
		return "", fmt.Errorf("task not found: %s", taskID)
	}

	task := value.(*Task)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Task ID: %s\n", task.ID))
	sb.WriteString(fmt.Sprintf("Status: %s\n", task.Status))
	sb.WriteString(fmt.Sprintf("Goal: %s\n", task.Goal))

	if task.Context != "" {
		sb.WriteString(fmt.Sprintf("Context: %s\n", task.Context))
	}

	if len(task.Toolsets) > 0 {
		sb.WriteString(fmt.Sprintf("Toolsets: %s\n", strings.Join(task.Toolsets, ", ")))
	}

	sb.WriteString(fmt.Sprintf("Created: %s\n", task.CreatedAt.Format(time.RFC3339)))

	if !task.StartedAt.IsZero() {
		sb.WriteString(fmt.Sprintf("Started: %s\n", task.StartedAt.Format(time.RFC3339)))
		duration := task.CompletedAt.Sub(task.StartedAt)
		if !task.CompletedAt.IsZero() {
			sb.WriteString(fmt.Sprintf("Duration: %v\n", duration))
		}
	}

	if !task.CompletedAt.IsZero() {
		sb.WriteString(fmt.Sprintf("Completed: %s\n", task.CompletedAt.Format(time.RFC3339)))
	}

	if task.Result != "" {
		sb.WriteString(fmt.Sprintf("\nResult:\n%s\n", task.Result))
	}

	if task.Error != "" {
		sb.WriteString(fmt.Sprintf("\nError: %s\n", task.Error))
	}

	return sb.String(), nil
}

// cancelTask cancels a running task
func (t *DelegationTool) cancelTask(taskID string) (string, error) {
	value, exists := t.tasks.Load(taskID)
	if !exists {
		return "", fmt.Errorf("task not found: %s", taskID)
	}

	task := value.(*Task)

	if task.Status == TaskStatusCompleted || task.Status == TaskStatusFailed || task.Status == TaskStatusCancelled {
		return fmt.Sprintf("Task '%s' is already %s and cannot be cancelled", taskID, task.Status), nil
	}

	task.Status = TaskStatusCancelled
	task.CompletedAt = time.Now()

	return fmt.Sprintf("Task '%s' cancelled successfully", taskID), nil
}

// formatTimeOrNil formats a time or returns "not yet" if zero
func formatTimeOrNil(t time.Time) string {
	if t.IsZero() {
		return "not yet"
	}
	return t.Format(time.RFC3339)
}

// GetActiveTaskCount returns the number of currently running tasks
func (t *DelegationTool) GetActiveTaskCount() int {
	count := 0
	t.tasks.Range(func(key, value interface{}) bool {
		task := value.(*Task)
		if task.Status == TaskStatusRunning || task.Status == TaskStatusPending {
			count++
		}
		return true
	})
	return count
}

// ClearCompleted removes completed, failed, or cancelled tasks older than the specified duration
func (t *DelegationTool) ClearCompleted(olderThan time.Duration) int {
	cleared := 0
	now := time.Now()

	t.tasks.Range(func(key, value interface{}) bool {
		task := value.(*Task)
		if (task.Status == TaskStatusCompleted || task.Status == TaskStatusFailed || task.Status == TaskStatusCancelled) &&
			!task.CompletedAt.IsZero() && now.Sub(task.CompletedAt) > olderThan {
			t.tasks.Delete(key)
			cleared++
		}
		return true
	})

	return cleared
}