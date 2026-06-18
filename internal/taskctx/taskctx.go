// Package taskctx carries per-task request-scoped values through the agent
// pipeline via context.Context. The orchestrator sets these at the start of
// each task; downstream consumers (tools, loggers, hooks) read them to scope
// their behavior to the current task without mutating shared state.
//
// This package is intentionally tiny and depends only on the standard library
// so it can be imported from both internal/agent and internal/tool/builtin
// without creating cycles.
package taskctx

import "context"

// ctxKey is unexported so callers cannot construct the key outside this
// package — WithWorkspace / WorkspaceFromCtx are the only API surface.
type ctxKey int

const (
	keyWorkspace ctxKey = iota
)

// WithWorkspace returns a copy of ctx carrying the per-task workspace dir.
// An empty dir clears any previously-set workspace (treated as "no workspace").
//
// The orchestrator calls this once per task at dispatch time; tools read it
// via WorkspaceFromCtx in their Execute(ctx, ...) handlers.
func WithWorkspace(ctx context.Context, dir string) context.Context {
	if dir == "" {
		return context.WithValue(ctx, keyWorkspace, "")
	}
	return context.WithValue(ctx, keyWorkspace, dir)
}

// WorkspaceFromCtx returns the per-task workspace dir set by WithWorkspace,
// or "" if none is set. Callers should treat "" as "no workspace" — fall
// back to their default behavior (no path rewriting, no extra allowed dir).
func WorkspaceFromCtx(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(keyWorkspace).(string); ok {
		return v
	}
	return ""
}

// WithTaskID is reserved for future use — currently the orchestrator threads
// taskID through function arguments and log fields. If a tool ever needs the
// active task_id (e.g., a manifest-writing tool), add it here.
type TaskIDKey struct{}

// TaskIDFromCtx returns the task_id carried in ctx, or "" if unset.
// Reads the public TaskIDKey so other packages can set it without importing
// this package — currently unused, but reserved.
func TaskIDFromCtx(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(TaskIDKey{}).(string); ok {
		return v
	}
	return ""
}
