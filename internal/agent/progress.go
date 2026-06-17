package agent

import "context"

// ProgressEvent describes what the agent is doing right now.
// The gateway uses this to send real-time progress updates.
type ProgressEvent struct {
	Type      string // "think", "act", "observe", "done", "delegate", "approval"
	Tool      string // tool name (for "act")
	Iteration int    // ReAct iteration number
	Message   string // human-readable detail

	// Rich detail for "act" and "observe" events
	ToolParams string // short summary of what the tool is doing
	Result     string // short result summary (for "observe")
	Success    bool   // whether the tool call succeeded (for "observe")
	DurationMs int64  // how long the tool took (for "observe")

	// For "approval" events — the command that needs approval
	ApprovalCommand string
	ApprovalRisk    string // "low", "medium", "high"
}

// ProgressFunc is called by the agent loop to report progress.
type ProgressFunc func(ProgressEvent)

type progressCtxKey struct{}

// WithProgress attaches a progress callback to the context.
// The agent loop reads it and fires events as it works.
func WithProgress(ctx context.Context, fn ProgressFunc) context.Context {
	return context.WithValue(ctx, progressCtxKey{}, fn)
}

// getProgress extracts the progress callback from context (nil if not set).
func getProgress(ctx context.Context) ProgressFunc {
	if fn, ok := ctx.Value(progressCtxKey{}).(ProgressFunc); ok {
		return fn
	}
	return nil
}
