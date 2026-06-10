package engine

import "time"

// TaskResult represents the outcome of processing an EngineTask.
type TaskResult struct {
	TaskID    string
	Success   bool
	Message   string
	ModelUsed string
	TokensIn  int
	TokensOut int
	Tier      int // 0=code (zero-AI), 1=cheap AI, 2=complex AI
	Duration  time.Duration
	Error     string
	SubTasks  []TaskResult
}

// NewTaskResult initializes a TaskResult with a task ID and default success state.
func NewTaskResult(taskID string) *TaskResult {
	return &TaskResult{
		TaskID:  taskID,
		Success: true,
		SubTasks: []TaskResult{},
	}
}

// WithError marks the result as failed and records the error message.
func (r *TaskResult) WithError(err string) *TaskResult {
	r.Success = false
	r.Error = err
	return r
}