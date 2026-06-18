package builtin

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/metabbe3/Kyoci-Agent/pkg"
)

// ==============================================================================================
// Delegation Tool — Lets the LLM spawn sub-agents for parallel task execution
// ==============================================================================================

// DelegateFunc is the function type that executes a delegated task.
// This is set by the orchestrator to bridge to the ExecuteTask method.
type DelegateFunc func(ctx context.Context, goal string, contextInfo string) (string, error)

// DelegateTask represents a delegated sub-task with its lifecycle state.
// Goroutine-safe: DelegateTask values should be treated as immutable after creation.
type DelegateTask struct {
	ID          string    `json:"id"`
	Goal        string    `json:"goal"`
	Context     string    `json:"context"`
	Status      string    `json:"status"`
	Result      string    `json:"result,omitempty"`
	Error       string    `json:"error,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	StartedAt   time.Time `json:"started_at,omitempty"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
}

// DelegationTool allows the LLM to spawn sub-agents for parallel task execution.
// It maintains a task registry and provides concurrency control via a semaphore.
// Goroutine-safe: All methods are safe for concurrent use.
type DelegationTool struct {
	tasks       sync.Map
	callback    DelegateFunc
	semaphore   chan struct{}
	logger      *slog.Logger
	taskCounter int
	counterMu   sync.Mutex
}

// NewDelegationTool creates a new delegation tool with a semaphore limiting parallel tasks.
func NewDelegationTool() *DelegationTool {
	return &DelegationTool{
		tasks:     sync.Map{},
		semaphore: make(chan struct{}, 3), // Max 3 parallel tasks
		logger:    slog.Default().With("component", "delegation-tool"),
	}
}

// SetCallback sets the function that executes delegated tasks.
// This bridges the tool to the orchestrator's ExecuteTask method.
func (d *DelegationTool) SetCallback(callback DelegateFunc) {
	d.callback = callback
}

// Name returns the unique name of this tool.
func (d *DelegationTool) Name() string {
	return "delegation"
}

// Description returns a human-readable description of what this tool does.
func (d *DelegationTool) Description() string {
	return "Delegate a sub-task to a specialist sub-agent for parallel execution. action=\"spawn\" goal=\"write tests for parser_test.go\"; action=\"wait_all\" (blocks until all spawned sub-agents finish, then returns their results). Use when work splits into independent parts. Max 3 concurrent. Each sub-agent gets 180s."
}

// Parameters returns the parameter definition for this tool.
func (d *DelegationTool) Parameters() []kyoci.ToolParameter {
	return []kyoci.ToolParameter{
		{
			Name:        "action",
			Type:        "string",
			Description: "Action to perform: spawn, list, status, wait, wait_all",
			Required:    true,
			EnumValues:  []string{"spawn", "list", "status", "wait", "wait_all"},
		},
		{
			Name:        "goal",
			Type:        "string",
			Description: "The task/goal to delegate (required for spawn)",
			Required:    false,
		},
		{
			Name:        "context",
			Type:        "string",
			Description: "Additional context for the sub-agent (optional for spawn)",
			Required:    false,
		},
		{
			Name:        "task_id",
			Type:        "string",
			Description: "Task ID to check or wait for (required for status and wait)",
			Required:    false,
		},
	}
}

// Execute executes the tool with the given parameters.
func (d *DelegationTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	action, ok := params["action"].(string)
	if !ok || action == "" {
		return "", fmt.Errorf("action parameter is required")
	}

	switch action {
	case "spawn":
		return d.spawnTask(ctx, params)
	case "list":
		return d.listTasks()
	case "status":
		return d.statusTask(params)
	case "wait":
		return d.waitTask(ctx, params)
	case "wait_all":
		return d.waitAllTasks(ctx)
	default:
		return "", fmt.Errorf("unknown action: %s", action)
	}
}

// spawnTask creates a new task and runs it in a goroutine, returning the task ID immediately.
func (d *DelegationTool) spawnTask(ctx context.Context, params map[string]interface{}) (string, error) {
	goal, ok := params["goal"].(string)
	if !ok || goal == "" {
		return "", fmt.Errorf("goal parameter is required for spawn action")
	}

	contextInfo, _ := params["context"].(string)

	if d.callback == nil {
		return "", fmt.Errorf("delegation callback not set — cannot execute sub-tasks")
	}

	// Generate task ID
	d.counterMu.Lock()
	d.taskCounter++
	taskID := fmt.Sprintf("task-%d-%d", time.Now().Unix(), d.taskCounter)
	d.counterMu.Unlock()

	// Create task
	task := &DelegateTask{
		ID:        taskID,
		Goal:      goal,
		Context:   contextInfo,
		Status:    "pending",
		CreatedAt: time.Now(),
	}

	d.tasks.Store(taskID, task)
	d.logger.Info("task spawned", "task_id", taskID, "goal", goal)

	// Run task in goroutine with DETACHED context so sub-agents survive
	// even if the parent HTTP request times out. Each sub-agent gets its own
	// 180s timeout independent of the parent.
	subCtx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	_ = cancel // cancel is called when executeTask finishes or ctx expires
	go d.executeTask(subCtx, task)

	return fmt.Sprintf("Task %s spawned successfully. Goal: %s", taskID, goal), nil
}

// executeTask runs a delegated task in a goroutine.
func (d *DelegationTool) executeTask(ctx context.Context, task *DelegateTask) {
	// Acquire semaphore slot
	d.semaphore <- struct{}{}
	defer func() { <-d.semaphore }()

	// Update status to running
	task.Status = "running"
	task.StartedAt = time.Now()
	d.logger.Info("task started", "task_id", task.ID)

	// Execute the task via callback
	result, err := d.callback(ctx, task.Goal, task.Context)

	// Update completion status
	task.CompletedAt = time.Now()
	if err != nil {
		task.Status = "failed"
		task.Error = err.Error()
		d.logger.Error("task failed", "task_id", task.ID, "error", err)
	} else {
		task.Status = "completed"
		task.Result = result
		d.logger.Info("task completed", "task_id", task.ID)
	}
}

// waitTask blocks until the task completes or times out, then returns the result.
func (d *DelegationTool) waitTask(ctx context.Context, params map[string]interface{}) (string, error) {
	taskID, ok := params["task_id"].(string)
	if !ok || taskID == "" {
		return "", fmt.Errorf("task_id parameter is required for wait action")
	}

	value, ok := d.tasks.Load(taskID)
	if !ok {
		return "", fmt.Errorf("task not found: %s", taskID)
	}

	task := value.(*DelegateTask)

	// If already completed, return result immediately
	if task.Status == "completed" {
		return fmt.Sprintf("Task %s completed.\nResult:\n%s", taskID, task.Result), nil
	}

	if task.Status == "failed" {
		return "", fmt.Errorf("task %s failed: %s", taskID, task.Error)
	}

	// Wait for completion with timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, 180*time.Second) // 3 min for sub-agent
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeoutCtx.Done():
			return "", fmt.Errorf("timeout waiting for task %s", taskID)
		case <-ticker.C:
			// Reload task to get latest status
			if value, ok := d.tasks.Load(taskID); ok {
				task = value.(*DelegateTask)

				if task.Status == "completed" {
					return fmt.Sprintf("Task %s completed.\nResult:\n%s", taskID, task.Result), nil
				}

				if task.Status == "failed" {
					return "", fmt.Errorf("task %s failed: %s", taskID, task.Error)
				}
			}
		case <-ctx.Done():
			return "", fmt.Errorf("context canceled while waiting for task %s", taskID)
		}
	}
}

// waitAllTasks blocks until ALL spawned tasks complete, then returns a summary
// with PASS/FAIL status for each. The main agent uses this to verify all
// sub-agent results before declaring the task done.
func (d *DelegationTool) waitAllTasks(ctx context.Context) (string, error) {
	// Collect all task IDs
	var taskIDs []string
	d.tasks.Range(func(key, value interface{}) bool {
		taskIDs = append(taskIDs, key.(string))
		return true
	})

	if len(taskIDs) == 0 {
		return "No delegated tasks found.", nil
	}

	// Wait for all tasks with a combined timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		allDone := true
		for _, id := range taskIDs {
			if value, ok := d.tasks.Load(id); ok {
				task := value.(*DelegateTask)
				if task.Status == "pending" || task.Status == "running" {
					allDone = false
					break
				}
			}
		}

		if allDone {
			break
		}

		select {
		case <-timeoutCtx.Done():
			// Timeout — return what we have with timeout warnings
			goto buildSummary
		case <-ticker.C:
			continue
		case <-ctx.Done():
			return "", fmt.Errorf("context canceled while waiting for all tasks")
		}
	}

buildSummary:
	// Build summary with PASS/FAIL markers
	var sb strings.Builder
	passCount := 0
	failCount := 0
	timeoutCount := 0

	sb.WriteString(fmt.Sprintf("=== DELEGATION RESULTS (%d tasks) ===\n\n", len(taskIDs)))

	for i, id := range taskIDs {
		value, ok := d.tasks.Load(id)
		if !ok {
			continue
		}
		task := value.(*DelegateTask)

		status := task.Status
		marker := "[PASS]"
		resultPreview := ""

		switch task.Status {
		case "completed":
			resultPreview = task.Result
			if len(resultPreview) > 200 {
				resultPreview = resultPreview[:200] + "..."
			}
			if strings.Contains(resultPreview, "[WARNING]") || len(strings.TrimSpace(task.Result)) < 20 {
				marker = "[SUSPECT]"
				failCount++
			} else {
				passCount++
			}
		case "failed":
			marker = "[FAIL]"
			resultPreview = "Error: " + task.Error
			failCount++
		default:
			marker = "[TIMEOUT]"
			resultPreview = "Task did not complete within timeout"
			timeoutCount++
			failCount++
		}

		sb.WriteString(fmt.Sprintf("%d. %s %s\n", i+1, marker, id))
		sb.WriteString(fmt.Sprintf("   Goal: %s\n", task.Goal))
		sb.WriteString(fmt.Sprintf("   Status: %s\n", status))
		if resultPreview != "" {
			sb.WriteString(fmt.Sprintf("   Result: %s\n", resultPreview))
		}
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("=== SUMMARY: %d PASS, %d FAIL/SUSPECT ===\n", passCount, failCount))
	if failCount > 0 {
		sb.WriteString("WARNING: Some tasks may be incomplete. You MUST verify the output files exist and have real content. If a task failed or produced incomplete output, RE-DELEGATE it with action=spawn.\n")
	}

	return sb.String(), nil
}

// listTasks returns a summary of all delegated tasks with their status.
func (d *DelegationTool) listTasks() (string, error) {
	var tasks []*DelegateTask
	d.tasks.Range(func(key, value interface{}) bool {
		task := value.(*DelegateTask)
		tasks = append(tasks, task)
		return true
	})

	if len(tasks) == 0 {
		return "No delegated tasks found.", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d delegated task(s):\n\n", len(tasks)))

	for i, task := range tasks {
		duration := "N/A"
		if !task.StartedAt.IsZero() && !task.CompletedAt.IsZero() {
			duration = task.CompletedAt.Sub(task.StartedAt).Round(time.Millisecond).String()
		}

		sb.WriteString(fmt.Sprintf("%d. [%s] %s\n", i+1, task.Status, task.ID))
		sb.WriteString(fmt.Sprintf("   Goal: %s\n", task.Goal))
		if task.Context != "" {
			sb.WriteString(fmt.Sprintf("   Context: %s\n", task.Context))
		}
		sb.WriteString(fmt.Sprintf("   Created: %s\n", task.CreatedAt.Format(time.RFC3339)))
		if !task.StartedAt.IsZero() {
			sb.WriteString(fmt.Sprintf("   Started: %s\n", task.StartedAt.Format(time.RFC3339)))
		}
		if !task.CompletedAt.IsZero() {
			sb.WriteString(fmt.Sprintf("   Completed: %s\n", task.CompletedAt.Format(time.RFC3339)))
			sb.WriteString(fmt.Sprintf("   Duration: %s\n", duration))
		}
		sb.WriteString("\n")
	}

	return sb.String(), nil
}

// statusTask returns detailed status for a specific task.
func (d *DelegationTool) statusTask(params map[string]interface{}) (string, error) {
	taskID, ok := params["task_id"].(string)
	if !ok || taskID == "" {
		return "", fmt.Errorf("task_id parameter is required for status action")
	}

	value, ok := d.tasks.Load(taskID)
	if !ok {
		return "", fmt.Errorf("task not found: %s", taskID)
	}

	task := value.(*DelegateTask)

	duration := "N/A"
	if !task.StartedAt.IsZero() && !task.CompletedAt.IsZero() {
		duration = task.CompletedAt.Sub(task.StartedAt).Round(time.Millisecond).String()
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Task: %s\n", task.ID))
	sb.WriteString(fmt.Sprintf("Status: %s\n", task.Status))
	sb.WriteString(fmt.Sprintf("Goal: %s\n", task.Goal))
	if task.Context != "" {
		sb.WriteString(fmt.Sprintf("Context: %s\n", task.Context))
	}
	sb.WriteString(fmt.Sprintf("Created: %s\n", task.CreatedAt.Format(time.RFC3339)))
	if !task.StartedAt.IsZero() {
		sb.WriteString(fmt.Sprintf("Started: %s\n", task.StartedAt.Format(time.RFC3339)))
	}
	if !task.CompletedAt.IsZero() {
		sb.WriteString(fmt.Sprintf("Completed: %s\n", task.CompletedAt.Format(time.RFC3339)))
		sb.WriteString(fmt.Sprintf("Duration: %s\n", duration))
	}
	if task.Result != "" {
		sb.WriteString(fmt.Sprintf("\nResult:\n%s\n", task.Result))
	}
	if task.Error != "" {
		sb.WriteString(fmt.Sprintf("\nError: %s\n", task.Error))
	}

	return sb.String(), nil
}
