package agent

import (
	"context"
	"log/slog"

	"github.com/metabbe3/Kyoci-Agent/internal/llm"
	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// Ports are the narrow interfaces the Agent depends on, defined in the agent
// package (not pkg/) to avoid an llm->pkg import cycle and to make the Agent
// unit-testable with stubs. Each is satisfied structurally by the existing
// concrete type, so call sites that pass the concrete types need no change
// (interface widening + variadic options are source-compatible).

// Router is the subset of *llm.Router the Agent calls.
type Router interface {
	Route(ctx context.Context, req kyoci.CompletionRequest, preferredProvider string) (*kyoci.CompletionResponse, error)
	RouteStream(ctx context.Context, req kyoci.CompletionRequest, preferredProvider string) (<-chan kyoci.StreamChunk, error)
}

// ToolProvider is the subset of *kyoci.ToolRegistry the Agent calls.
type ToolProvider interface {
	Register(tool kyoci.Tool) error
	List() []kyoci.ToolDefinition
	Execute(ctx context.Context, name string, params map[string]interface{}) (string, error)
}

// SkillProvider is the subset of *kyoci.SkillRegistry the Agent calls.
type SkillProvider interface {
	Register(skill kyoci.Skill) error
	Match(query string) (kyoci.Skill, bool)
	Execute(ctx context.Context, name, query string) (string, error)
}

// Compile-time assertions that the concrete types satisfy the ports.
var (
	_ Router        = (*llm.Router)(nil)
	_ ToolProvider  = (*kyoci.ToolRegistry)(nil)
	_ SkillProvider = (*kyoci.SkillRegistry)(nil)
)

// Option configures an Agent at construction. Passed to NewAgent as trailing
// variadic options; defaults (slog.Default logger, noop injector/recorder)
// apply first and are overridden by any matching option.
type Option func(*Agent)

// WithLogger overrides the default logger.
func WithLogger(l *slog.Logger) Option {
	return func(a *Agent) { if l != nil { a.logger = l } }
}

// WithRouter overrides the LLM router.
func WithRouter(r Router) Option { return func(a *Agent) { a.router = r } }

// WithTools overrides the tool provider.
func WithTools(t ToolProvider) Option { return func(a *Agent) { a.tools = t } }

// WithSkills overrides the skill provider.
func WithSkills(s SkillProvider) Option { return func(a *Agent) { a.skills = s } }

// WithInjector overrides the L3 context injector.
func WithInjector(i ContextInjector) Option {
	return func(a *Agent) {
		if i != nil { a.injector = i }
	}
}

// WithRecorder overrides the task recorder.
func WithRecorder(r TaskRecorder) Option {
	return func(a *Agent) {
		if r != nil { a.recorder = r }
	}
}

// WithActivitySink wires a channel that receives structured activity events
// for the live activity tree UI. The orchestrator calls this when constructing
// per-worker agents, passing the per-request SSE stream channel. Events flow
// through to the chat client AND (via the dashboard's broker) to the global
// Live Activity panel.
//
// The sink is SEND-ONLY (chan<-). Emitters use emitActivity which is a no-op
// when the sink is nil, so call sites don't need to nil-check.
func WithActivitySink(ch chan<- kyoci.StreamChunk) Option {
	return func(a *Agent) { a.activitySink = ch }
}

// WithActivityTaskID stamps every activity event this agent emits with the
// given TaskID + TaskName + Role. The orchestrator sets these per-worker so
// each step's events group into its own tree row.
func WithActivityTaskID(taskID, taskName, role string) Option {
	return func(a *Agent) {
		a.activityTaskID = taskID
		a.activityTaskName = taskName
		a.activityRole = role
	}
}
