package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"sync"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// todoItem represents a single task in the TODO list.
type todoItem struct {
	ID      int
	Content string
	Status  string
}

// todoStore is the in-memory storage for TODO items with thread-safe access.
var todoStore = struct {
	sync.RWMutex
	items []todoItem
}{
	items: make([]todoItem, 0),
}

// nextID is the next ID to assign to a new TODO item.
var nextID = 1

// TodoTool implements the kyoci.Tool interface for managing task lists.
type TodoTool struct {
	logger *slog.Logger
}

// NewTodoTool creates a new TODO tool instance.
func NewTodoTool() *TodoTool {
	return &TodoTool{
		logger: slog.Default(),
	}
}

// Name returns the tool name.
func (t *TodoTool) Name() string {
	return "todo"
}

// Description returns the tool description.
func (t *TodoTool) Description() string {
	return "Manage task lists: add, list, complete, clear tasks. Use to track multi-step work."
}

// Parameters returns the tool parameter definition.
func (t *TodoTool) Parameters() []kyoci.ToolParameter {
	return []kyoci.ToolParameter{
		{
			Name:        "action",
			Type:        "string",
			Description: "Action to perform: 'add' to add a task, 'list' to show all tasks, 'snapshot' for machine-readable state, 'complete' to mark a task as done, 'remove' to delete a task, 'clear' to remove all tasks",
			Required:    true,
			EnumValues:  []string{"add", "list", "snapshot", "complete", "remove", "clear"},
		},
		{
			Name:        "task",
			Type:        "string",
			Description: "For 'add': the task text. For 'complete' or 'remove': the task ID (number)",
			Required:    false,
		},
	}
}

// Execute performs the requested TODO action.
//
// Parameters:
//   - ctx: Context for cancellation
//   - params: Map containing "action" (required) and "task" (optional)
//
// Returns:
//   - string: JSON result with action taken and current task list
//   - error: Error if action is invalid or execution fails
func (t *TodoTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	// Extract action
	action, ok := params["action"].(string)
	if !ok || action == "" {
		return "", fmt.Errorf("action parameter is required and must be a string")
	}

	t.logger.Info("executing todo action", "action", action)

	// Handle different actions
	switch action {
	case "add":
		return t.addTask(params)
	case "list":
		return t.listTasks()
	case "snapshot":
		return t.snapshotTasks()
	case "complete":
		return t.completeTask(params)
	case "remove":
		return t.removeTask(params)
	case "clear":
		return t.clearTasks()
	default:
		return "", fmt.Errorf("invalid action: %s (must be 'add', 'list', 'snapshot', 'complete', 'remove', or 'clear')", action)
	}
}

// addTask adds a new task to the list.
func (t *TodoTool) addTask(params map[string]interface{}) (string, error) {
	task, ok := params["task"].(string)
	if !ok || task == "" {
		return "", fmt.Errorf("task parameter is required for 'add' action")
	}

	todoStore.Lock()
	item := todoItem{
		ID:      nextID,
		Content: task,
		Status:  "pending",
	}
	todoStore.items = append(todoStore.items, item)
	nextID++
	todoStore.Unlock()

	t.logger.Info("task added", "id", item.ID, "content", item.Content)

	return t.formatResult("added", fmt.Sprintf("Task added: %s", item.Content))
}

// listTasks returns all tasks with their IDs and statuses.
func (t *TodoTool) listTasks() (string, error) {
	todoStore.RLock()
	count := len(todoStore.items)
	todoStore.RUnlock()

	if count == 0 {
		return t.formatResult("listed", "No tasks in list")
	}

	t.logger.Info("tasks listed", "count", count)

	return t.formatResult("listed", fmt.Sprintf("Found %d task(s)", count))
}

// snapshotTasks returns a machine-readable JSON view of the todo state with
// per-item fields and aggregate counts. Unlike list (which is human-oriented),
// snapshot is designed for programmatic consumers — e.g., the thinking
// system's Verify state checking whether every plan step is done.
func (t *TodoTool) snapshotTasks() (string, error) {
	todoStore.RLock()
	defer todoStore.RUnlock()

	tasks := make([]map[string]interface{}, 0, len(todoStore.items))
	pending, completed := 0, 0
	for _, item := range todoStore.items {
		tasks = append(tasks, map[string]interface{}{
			"id":      item.ID,
			"content": item.Content,
			"status":  item.Status,
		})
		switch item.Status {
		case "pending":
			pending++
		case "completed":
			completed++
		}
	}

	result := map[string]interface{}{
		"action": "snapshot",
		"summary": map[string]int{
			"total":     len(todoStore.items),
			"pending":   pending,
			"completed": completed,
		},
		"tasks": tasks,
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to format snapshot: %v", err)
	}
	return string(jsonBytes), nil
}

// completeTask marks a task as completed by its ID.
func (t *TodoTool) completeTask(params map[string]interface{}) (string, error) {
	taskIDStr, ok := params["task"].(string)
	if !ok || taskIDStr == "" {
		return "", fmt.Errorf("task parameter is required for 'complete' action (must be task ID)")
	}

	taskID, err := strconv.Atoi(taskIDStr)
	if err != nil {
		return "", fmt.Errorf("task ID must be a valid number: %v", err)
	}

	todoStore.Lock()
	found := false
	for i := range todoStore.items {
		if todoStore.items[i].ID == taskID {
			todoStore.items[i].Status = "completed"
			found = true
			t.logger.Info("task completed", "id", taskID, "content", todoStore.items[i].Content)
			break
		}
	}
	todoStore.Unlock()

	if !found {
		return "", fmt.Errorf("task with ID %d not found", taskID)
	}

	return t.formatResult("completed", fmt.Sprintf("Task %d marked as completed", taskID))
}

// removeTask deletes a task by its ID.
func (t *TodoTool) removeTask(params map[string]interface{}) (string, error) {
	taskIDStr, ok := params["task"].(string)
	if !ok || taskIDStr == "" {
		return "", fmt.Errorf("task parameter is required for 'remove' action (must be task ID)")
	}

	taskID, err := strconv.Atoi(taskIDStr)
	if err != nil {
		return "", fmt.Errorf("task ID must be a valid number: %v", err)
	}

	todoStore.Lock()
	found := false
	newItems := make([]todoItem, 0, len(todoStore.items)-1)
	for _, item := range todoStore.items {
		if item.ID == taskID {
			found = true
			t.logger.Info("task removed", "id", taskID, "content", item.Content)
		} else {
			newItems = append(newItems, item)
		}
	}

	if !found {
		todoStore.Unlock()
		return "", fmt.Errorf("task with ID %d not found", taskID)
	}

	todoStore.items = newItems
	todoStore.Unlock()

	return t.formatResult("removed", fmt.Sprintf("Task %d removed", taskID))
}

// clearTasks removes all tasks from the list.
func (t *TodoTool) clearTasks() (string, error) {
	todoStore.Lock()
	count := len(todoStore.items)
	todoStore.items = make([]todoItem, 0)
	nextID = 1
	todoStore.Unlock()

	t.logger.Info("all tasks cleared", "count", count)

	return t.formatResult("cleared", fmt.Sprintf("Cleared %d task(s)", count))
}

// ResetTodoStore clears all todo items and resets the ID counter. Call this at
// the start of each task Execute to prevent state leakage between unrelated
// tasks — without it, a previous task's todos persist into the next task's
// context and confuse small models.
//
// This is exported as a package-level function (rather than a tool action) so
// callers that set up the agent (cmd/server, role registry, tests) can reset
// the store without routing through the LLM/tool-registry path.
func ResetTodoStore() {
	todoStore.Lock()
	defer todoStore.Unlock()
	todoStore.items = make([]todoItem, 0)
	nextID = 1
}

// formatResult formats the result as JSON with action taken and current task list.
func (t *TodoTool) formatResult(action, message string) (string, error) {
	todoStore.RLock()
	defer todoStore.RUnlock()

	// Build task list for output
	tasks := make([]map[string]interface{}, 0, len(todoStore.items))
	for _, item := range todoStore.items {
		tasks = append(tasks, map[string]interface{}{
			"id":      item.ID,
			"content": item.Content,
			"status":  item.Status,
		})
	}

	result := map[string]interface{}{
		"action":  action,
		"message": message,
		"tasks":   tasks,
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to format result: %v", err)
	}

	return string(jsonBytes), nil
}