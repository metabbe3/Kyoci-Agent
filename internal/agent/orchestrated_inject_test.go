package agent

import (
	"context"
	"strings"
	"testing"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// =============================================================================
// L3 Context Injection into the Orchestrated Pipeline
//
// The orchestrated path (planTask + runWorker) is the DEFAULT execution pipeline.
// Unlike the legacy ReAct loop (loop.go) and the thinking state machine
// (thinking.go), the orchestrated path did NOT call a.injector.Inject(task),
// meaning multi-step tasks got NO L3 memory context (user profile, past
// experiences, lessons). These tests pin the fix: L3 context must appear in
// the planner user prompt and the worker system prompt.
// =============================================================================

// fakeInjector is a test double for ContextInjector that returns a fixed string.
type fakeInjector struct {
	content string
}

func (f fakeInjector) Inject(task string) string { return f.content }

const l3Sentinel = "SENTINEL_L3_CONTEXT_USER_PREFERS_GO_NETHTTP_SNAKE_CASE"

// --- Planner injection ---

// TestPlanTask_InjectsL3ContextIntoUserPrompt verifies that when an injector is
// installed, planTask prepends its output to the user prompt (not the system
// prompt, which must stay a strict "output JSON only" directive).
func TestPlanTask_InjectsL3ContextIntoUserPrompt(t *testing.T) {
	plannerOutput := `[{"id":1,"description":"do it","depends_on":[],"tool_hint":"file"}]`

	seq := &sequencerProvider{
		name: "mock",
		responses: []*kyoci.CompletionResponse{
			{Content: plannerOutput, FinishReason: kyoci.FinishStop},
		},
	}
	cfg := DefaultAgentConfig()
	cfg.Orchestration.Enabled = true
	a := createTestAgent(cfg, seq)
	a.SetContextInjector(fakeInjector{content: l3Sentinel})

	if _, err := a.planTask(context.Background(), "write a Go API"); err != nil {
		t.Fatalf("planTask failed: %v", err)
	}
	if len(seq.captured) < 1 {
		t.Fatalf("no LLM requests captured")
	}
	req := seq.captured[0]
	if len(req.Messages) < 2 {
		t.Fatalf("expected >=2 messages; got %d", len(req.Messages))
	}
	sysMsg := req.Messages[0].Content
	userMsg := req.Messages[1].Content

	// The L3 sentinel must appear in the USER message, not the system message.
	if !strings.Contains(userMsg, l3Sentinel) {
		t.Errorf("L3 sentinel missing from user prompt;\nuserMsg:\n%s", userMsg)
	}
	if strings.Contains(sysMsg, l3Sentinel) {
		t.Errorf("L3 sentinel leaked into system prompt (must stay strict JSON directive);\nsysMsg:\n%s", sysMsg)
	}
	// The original task text must still be present (injection prepends, not replaces).
	if !strings.Contains(userMsg, "write a Go API") {
		t.Errorf("original task text missing from user prompt after injection;\nuserMsg:\n%s", userMsg)
	}
}

// TestPlanTask_EmptyInjectionLeavesPromptUnchanged verifies that when Inject
// returns "" (no relevant memories), the user prompt is exactly PlannerPrompt
// with no leftover injection artifacts (no stray separators, no empty blocks).
func TestPlanTask_EmptyInjectionLeavesPromptUnchanged(t *testing.T) {
	plannerOutput := `[{"id":1,"description":"x","depends_on":[],"tool_hint":""}]`

	seq := &sequencerProvider{
		name: "mock",
		responses: []*kyoci.CompletionResponse{
			{Content: plannerOutput, FinishReason: kyoci.FinishStop},
		},
	}
	cfg := DefaultAgentConfig()
	cfg.Orchestration.Enabled = true
	a := createTestAgent(cfg, seq)
	a.SetContextInjector(fakeInjector{content: ""})

	if _, err := a.planTask(context.Background(), "simple task"); err != nil {
		t.Fatalf("planTask failed: %v", err)
	}
	userMsg := seq.captured[0].Messages[1].Content
	want := PlannerPrompt("simple task")
	if userMsg != want {
		t.Errorf("empty injection altered the prompt;\n got: %q\nwant: %q", userMsg, want)
	}
}

// --- Worker injection ---

// TestRunWorker_InjectsL3ContextIntoSystemPrompt verifies that runWorker
// appends L3 context to the worker system prompt (mirrors loop.go:210-214).
func TestRunWorker_InjectsL3ContextIntoSystemPrompt(t *testing.T) {
	seq := &sequencerProvider{
		name: "mock",
		responses: []*kyoci.CompletionResponse{
			{
				ToolCalls: []kyoci.ToolCall{
					{ID: "c1", Name: "file", Arguments: `{"operation":"list","path":"/tmp"}`},
				},
				FinishReason: kyoci.FinishToolCall,
			},
			{Content: "done", FinishReason: kyoci.FinishStop},
		},
	}
	cfg := DefaultAgentConfig()
	cfg.Orchestration.Enabled = true
	cfg.Orchestration.WorkerMaxIterations = 4
	a := createTestAgent(cfg, seq)
	a.SetContextInjector(fakeInjector{content: l3Sentinel})

	step := OrchStep{ID: 1, Description: "write the file", ToolHint: "file"}
	if _, err := a.runWorker(context.Background(), "write a Go API", step, map[int]string{}); err != nil {
		t.Fatalf("runWorker failed: %v", err)
	}
	if len(seq.captured) < 1 {
		t.Fatalf("no LLM requests captured")
	}
	sysMsg := seq.captured[0].Messages[0].Content

	// The L3 sentinel must appear in the worker SYSTEM message.
	if !strings.Contains(sysMsg, l3Sentinel) {
		t.Errorf("L3 sentinel missing from worker system prompt;\nsysMsg:\n%s", sysMsg)
	}
	// The original WorkerSystemPrompt content must still be present.
	if !strings.Contains(sysMsg, "MUST be a tool call") {
		t.Errorf("original WorkerSystemPrompt directive missing after injection;\nsysMsg:\n%s", sysMsg)
	}
}

// TestRunWorker_EmptyInjectionLeavesSystemPromptUnchanged verifies that an empty
// injection does not alter the worker system prompt.
func TestRunWorker_EmptyInjectionLeavesSystemPromptUnchanged(t *testing.T) {
	seq := &sequencerProvider{
		name: "mock",
		responses: []*kyoci.CompletionResponse{
			{
				ToolCalls: []kyoci.ToolCall{
					{ID: "c1", Name: "file", Arguments: `{"operation":"list","path":"/tmp"}`},
				},
				FinishReason: kyoci.FinishToolCall,
			},
			{Content: "done", FinishReason: kyoci.FinishStop},
		},
	}
	cfg := DefaultAgentConfig()
	cfg.Orchestration.Enabled = true
	cfg.Orchestration.WorkerMaxIterations = 4
	a := createTestAgent(cfg, seq)
	a.SetContextInjector(fakeInjector{content: ""})

	step := OrchStep{ID: 1, Description: "write the file", ToolHint: "file"}
	if _, err := a.runWorker(context.Background(), "write a Go API", step, map[int]string{}); err != nil {
		t.Fatalf("runWorker failed: %v", err)
	}
	sysMsg := seq.captured[0].Messages[0].Content
	if sysMsg != WorkerSystemPrompt {
		t.Errorf("empty injection altered the worker system prompt;\n got: %q\nwant: %q", sysMsg, WorkerSystemPrompt)
	}
}
