package agent

import "context"

// =============================================================================
// Intelligence Hooks — Decoupled Interface Between Agent and Memory Systems
// =============================================================================
//
// These interfaces allow the agent to use intelligence features (context
// injection, experience recording, reflection) without importing the memory
// package directly. The orchestrator provides concrete implementations.
//

// ContextInjector injects relevant memory context (user profile, past
// experiences, lessons learned) into the system prompt BEFORE the LLM is called.
// This is what makes the agent "remember" across sessions.
type ContextInjector interface {
	// Inject returns additional context text for the system prompt,
	// or empty string if nothing relevant was found.
	Inject(task string) string
}

// TaskRecord captures the outcome of a completed task for learning.
type TaskRecord struct {
	Task       string   `json:"task"`
	Role       string   `json:"role"`
	ToolsUsed  []string `json:"tools_used"`
	Iterations int      `json:"iterations"`
	ToolCalls  int      `json:"tool_calls"`
	Success    bool     `json:"success"`
	DurationMs int64    `json:"duration_ms"`
	ErrorMsg   string   `json:"error,omitempty"`
}

// TaskRecorder records task outcomes and performs post-task reflection.
// Called AFTER the task completes — never blocks the response path.
type TaskRecorder interface {
	// Record stores the experience and triggers reflection.
	// Must be non-blocking — implementation should handle async.
	Record(ctx context.Context, rec TaskRecord)
}

// noopRecorder is a no-op implementation used when intelligence is disabled.
type noopRecorder struct{}

func (noopRecorder) Record(context.Context, TaskRecord) {}

// noopInjector is a no-op implementation used when intelligence is disabled.
type noopInjector struct{}

func (noopInjector) Inject(string) string { return "" }

// SetContextInjector sets the context injector for this agent.
// Pass nil to disable context injection.
func (a *Agent) SetContextInjector(injector ContextInjector) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if injector == nil {
		a.injector = noopInjector{}
	} else {
		a.injector = injector
	}
}

// SetTaskRecorder sets the task recorder for this agent.
// Pass nil to disable experience recording and reflection.
func (a *Agent) SetTaskRecorder(recorder TaskRecorder) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if recorder == nil {
		a.recorder = noopRecorder{}
	} else {
		a.recorder = recorder
	}
}
