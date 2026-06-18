package agent

import (
	"context"
	"fmt"
	"strings"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// =====================================================================================
// Explore sub-agent — a read-only worker variant that mirrors Claude Code's
// context-isolation pattern. The parent agent dispatches an Explore task via
// delegation with tool_hint="explore"; the explore worker runs with a
// restricted toolset (glob, grep, file:read, git, codesearch, lsp) and returns
// ONLY a structured Markdown summary. The parent never sees the raw file dumps
// or intermediate tool calls.
//
// Key properties:
//  1. Restricted toolset enforced at the ToolProvider layer (airtight — no
//     prompt-injection can escape into a write/terminal tool).
//  2. System prompt directs the model to investigate and summarize, never
//     modify.
//  3. Dispatched via the existing DelegationTool — no new tool surface.
//
// Use cases: "explore: find all uses of context.Background()", "explore: map
// the auth flow", "explore: what does the frontend render for /dashboard".
// =====================================================================================

// ExploreSystemPrompt is the system prompt for the explore worker. It directs
// the model to investigate read-only and return a structured summary.
const ExploreSystemPrompt = `You are an Explore agent. Investigate the codebase using ONLY read-only tools (glob, grep, file:read, git, codesearch, lsp). You CANNOT edit, write, patch, or execute shell commands — those tools are unavailable.

BEHAVIOR:
1. Start with the goal. Decide what files / patterns / symbols would answer it.
2. Use glob to find files, grep to find content, file:read to inspect specific files.
3. Read enough to answer the question confidently — but no more. Do not dump entire files into your response.
4. Cite every claim with file:line references so the caller can verify.

OUTPUT FORMAT (Markdown only — no prose preamble, no closing remarks):
- Start with a one-sentence direct answer to the question.
- Then a "## Findings" section with bullet points, each citing file:line.
- Then a "## Files Examined" section listing the paths you actually opened.
- If you cannot answer confidently, say so explicitly and explain what's missing.

Do NOT propose changes. Do NOT write code. Do NOT run shell commands. You are read-only.`

// ExploreToolAllowlist is the set of tools the explore worker may use. Anything
// else is filtered out by ReadOnlyToolFilter. Keep this list tight — every
// addition widens what a prompt-injection could exploit.
var ExploreToolAllowlist = map[string]bool{
	"glob":       true,
	"grep":       true,
	"file":       true, // file tool supports action=read AND action=write; filter rejects writes at execute time
	"git":        true,
	"codesearch": true,
	"lsp":        true,
	"todo":       true, // planning aid, no side effects
}

// exploreFileWriteActions are file-tool action values that the filter rejects.
// "read" is allowed; "write", "append", "delete", "mkdir", "move", "copy" are not.
var exploreFileWriteActions = map[string]bool{
	"write": true, "append": true, "delete": true,
	"mkdir": true, "move": true, "copy": true, "touch": true,
}

// ReadOnlyToolFilter wraps a ToolProvider and restricts both the visible tools
// and the actions that can be taken. Used by ExploreWorker to enforce
// read-only behavior at the tool layer.
//
// The filter is airtight: List() omits disallowed tools entirely (so the model
// never sees them to attempt them), AND Execute() rejects any tool that's not
// in the allowlist (defense in depth in case a tool name is passed by index or
// guess). For the "file" tool — which has both read and write actions under a
// single name — Execute() inspects the "action" parameter and rejects writes.
type ReadOnlyToolFilter struct {
	inner     ToolProvider
	allowlist map[string]bool
}

// NewReadOnlyToolFilter wraps inner so only tools in allowlist are visible and
// executable. Nil allowlist defaults to ExploreToolAllowlist.
func NewReadOnlyToolFilter(inner ToolProvider, allowlist map[string]bool) *ReadOnlyToolFilter {
	if allowlist == nil {
		allowlist = ExploreToolAllowlist
	}
	return &ReadOnlyToolFilter{inner: inner, allowlist: allowlist}
}

// Register passes through to inner. The explore worker doesn't register tools
// itself, so this is effectively a no-op in practice but satisfies the
// ToolProvider interface.
func (f *ReadOnlyToolFilter) Register(tool kyoci.Tool) error {
	return f.inner.Register(tool)
}

// List returns only the ToolDefinitions whose names are in the allowlist. The
// model literally cannot see blocked tools.
func (f *ReadOnlyToolFilter) List() []kyoci.ToolDefinition {
	all := f.inner.List()
	out := make([]kyoci.ToolDefinition, 0, len(all))
	for _, td := range all {
		if f.allowlist[td.Name] {
			out = append(out, td)
		}
	}
	return out
}

// Execute runs the named tool after two checks:
//  1. Tool name must be in the allowlist.
//  2. If the tool is "file", its "action" parameter must not be a write action.
//
// Returns ErrExploreToolNotAllowed for any rejection so the model gets a
// specific error message it can incorporate into its plan.
func (f *ReadOnlyToolFilter) Execute(ctx context.Context, name string, params map[string]interface{}) (string, error) {
	if !f.allowlist[name] {
		return "", fmt.Errorf("%w: %q is not in the explore allowlist", ErrExploreToolNotAllowed, name)
	}
	if name == "file" {
		action, _ := params["action"].(string)
		if action == "" {
			action = "read" // default
		}
		if exploreFileWriteActions[action] {
			return "", fmt.Errorf("%w: file action %q is a write; explore worker is read-only", ErrExploreToolNotAllowed, action)
		}
	}
	return f.inner.Execute(ctx, name, params)
}

// ErrExploreToolNotAllowed signals that a tool call was rejected by the
// explore worker's read-only filter. Distinct type so callers can branch on it.
var ErrExploreToolNotAllowed = fmt.Errorf("explore: tool not allowed")

// ExploreWorker runs a read-only investigation using the parent agent's
// infrastructure but with tools filtered and the explore system prompt.
//
// Usage from wireDelegation:
//
//	if strings.HasPrefix(goal, "explore:") {
//	    return exploreWorker.Run(ctx, strings.TrimPrefix(goal, "explore:"))
//	}
//
// The function constructs a new Agent that shares the parent's router, skills,
// memory, and logger — only the tool provider is wrapped. This keeps the
// explore worker fast to spawn (no new LLM client, no new memory store) while
// guaranteeing read-only behavior.
func (a *Agent) ExploreWorker(ctx context.Context, goal string) (string, error) {
	exploreAgent := NewAgent(
		a.config,
		a.router,
		NewReadOnlyToolFilter(a.tools, nil),
		a.skills,
		a.memory,
		WithLogger(a.logger),
		WithInjector(a.injector),
		WithRecorder(a.recorder),
	)
	// Override the system prompt to the explore variant. The agent's
	// Execute() uses a.config.SystemPrompt for the legacy ReAct loop; for the
	// orchestrated path the worker system prompt is hardcoded in prompts.go.
	// The simplest hook is to prefix the goal with a directive.
	exploreAgent.config.SystemPrompt = ExploreSystemPrompt

	// Force the legacy ReAct loop (not the orchestrator) so the explore
	// prompt is the actual system prompt — orchestrator would override.
	exploreAgent.config.Orchestration.Enabled = false
	exploreAgent.config.EnableThinking = false
	exploreAgent.config.EnableSkills = false // skills don't apply to exploration
	exploreAgent.config.MaxIterations = 15   // bound exploration depth

	// Run the explore agent and return its content directly (no metrics
	// appended — caller wants just the summary).
	result, err := exploreAgent.Execute(ctx, goal)
	if err != nil {
		return "", err
	}
	return result.Content, nil
}

// HasExplorePrefix reports whether goal is a request to dispatch an explore
// sub-agent. The prefix is "explore:" or "explore " (case-insensitive). Used
// by wireDelegation to route to ExploreWorker instead of the regular recursive
// orchestrator.
func HasExplorePrefix(goal string) bool {
	low := strings.ToLower(strings.TrimSpace(goal))
	return strings.HasPrefix(low, "explore:") || strings.HasPrefix(low, "explore ")
}

// StripExplorePrefix removes the "explore:" / "explore " prefix from goal and
// returns the underlying investigation question.
func StripExplorePrefix(goal string) string {
	out := strings.TrimSpace(goal)
	low := strings.ToLower(out)
	if strings.HasPrefix(low, "explore:") {
		return strings.TrimSpace(out[len("explore:"):])
	}
	if strings.HasPrefix(low, "explore ") {
		return strings.TrimSpace(out[len("explore "):])
	}
	return out
}
