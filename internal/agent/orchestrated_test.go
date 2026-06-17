package agent

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// =============================================================================
// Orchestrator-Worker Pipeline Tests
//
// These tests verify the 4-phase pipeline:
//   1. Planner    — decomposes task into []OrchStep via one LLM call (no tools)
//   2. Dispatcher — pure Go, runs independent steps in parallel goroutines
//   3. Worker     — each step runs as a focused legacy-ReAct loop with tools
//   4. Synthesizer — composes final answer from worker results (no tools)
//
// The pipeline replaces the monolithic JSON-scratchpad thinking mode which
// kept exhausting its budget on multi-step tasks (see plan: "thinking budget
// exhaustion"). Each unit here has ONE job, which is what 14B models handle
// reliably.
// =============================================================================

// -----------------------------------------------------------------------------
// Planner
// -----------------------------------------------------------------------------

// TestPlanner_DecompilesTask_IntoSteps verifies that planTask() calls the LLM
// once and parses the returned JSON array into a []OrchStep with the expected
// fields. The planner prompt has NO tools — the model can only emit the plan.
func TestPlanner_DecompilesTask_IntoSteps(t *testing.T) {
	plannerOutput := `[{"id":1,"description":"find the teknikalid project","depends_on":[],"tool_hint":"file"},{"id":2,"description":"list cron configs","depends_on":[1],"tool_hint":"file"},{"id":3,"description":"grep for tz","depends_on":[1],"tool_hint":"file"}]`

	provider := &mockProvider{
		name: "mock",
		response: &kyoci.CompletionResponse{
			Content:      plannerOutput,
			FinishReason: kyoci.FinishStop,
		},
	}
	cfg := DefaultAgentConfig()
	cfg.Orchestration.Enabled = true
	a := createTestAgent(cfg, provider)

	steps, err := a.planTask(context.Background(), "check teknikalid timezones")
	if err != nil {
		t.Fatalf("planTask failed: %v", err)
	}
	if len(steps) != 3 {
		t.Fatalf("expected 3 steps; got %d", len(steps))
	}
	if steps[0].ID != 1 || steps[0].Description != "find the teknikalid project" {
		t.Errorf("step 0 mismatch: %+v", steps[0])
	}
	if len(steps[1].DependsOn) != 1 || steps[1].DependsOn[0] != 1 {
		t.Errorf("step 1 depends_on mismatch: %+v", steps[1])
	}
	if steps[2].ToolHint != "file" {
		t.Errorf("step 2 tool_hint mismatch: %+v", steps[2])
	}
}

// TestPlanner_SimpleTask_ReturnsOneStep verifies that a trivial single-step
// plan parses correctly. The pipeline uses this as the fast-path signal: when
// len(steps)==1, orchestration overhead can be skipped.
func TestPlanner_SimpleTask_ReturnsOneStep(t *testing.T) {
	plannerOutput := `[{"id":1,"description":"answer the question directly","depends_on":[],"tool_hint":""}]`

	provider := &mockProvider{
		name: "mock",
		response: &kyoci.CompletionResponse{
			Content:      plannerOutput,
			FinishReason: kyoci.FinishStop,
		},
	}
	cfg := DefaultAgentConfig()
	cfg.Orchestration.Enabled = true
	a := createTestAgent(cfg, provider)

	steps, err := a.planTask(context.Background(), "what is 2+2")
	if err != nil {
		t.Fatalf("planTask failed: %v", err)
	}
	if len(steps) != 1 {
		t.Fatalf("expected 1 step for simple task; got %d", len(steps))
	}
	if steps[0].ID != 1 {
		t.Errorf("expected step ID 1; got %d", steps[0].ID)
	}
}

// TestPlanner_StripsCodeFencesAndProse verifies resilience to models that wrap
// output in markdown fences or prepend commentary. This is the #1 observed
// failure mode in qwen2.5-coder:14b — it often emits "Here is the plan:"
// before the JSON. The planner must extract the array regardless.
func TestPlanner_StripsCodeFencesAndProse(t *testing.T) {
	plannerOutput := "Here is the plan:\n\n```json\n[{\"id\":1,\"description\":\"do the thing\",\"depends_on\":[],\"tool_hint\":\"file\"}]\n```\n"

	provider := &mockProvider{
		name: "mock",
		response: &kyoci.CompletionResponse{
			Content:      plannerOutput,
			FinishReason: kyoci.FinishStop,
		},
	}
	cfg := DefaultAgentConfig()
	cfg.Orchestration.Enabled = true
	a := createTestAgent(cfg, provider)

	steps, err := a.planTask(context.Background(), "do the thing")
	if err != nil {
		t.Fatalf("planTask failed on fenced+prose output: %v", err)
	}
	if len(steps) != 1 {
		t.Fatalf("expected 1 step; got %d", len(steps))
	}
	if steps[0].Description != "do the thing" {
		t.Errorf("description mismatch: %q", steps[0].Description)
	}
}

// -----------------------------------------------------------------------------
// Dispatcher
// -----------------------------------------------------------------------------

// TestDispatcher_RunsIndependentStepsInParallel verifies that three steps with
// no mutual dependencies execute concurrently, not sequentially. We measure
// wall-clock time: each worker sleeps 100ms; parallel execution finishes in
// ~100ms, sequential would take ~300ms.
func TestDispatcher_RunsIndependentStepsInParallel(t *testing.T) {
	cfg := DefaultAgentConfig()
	cfg.Orchestration.Enabled = true
	cfg.Orchestration.MaxParallel = 3
	// We bypass planTask — inject steps directly.
	a := createTestAgent(cfg, &mockProvider{name: "mock"})

	steps := []OrchStep{
		{ID: 1, Description: "s1"},
		{ID: 2, Description: "s2"},
		{ID: 3, Description: "s3"},
	}

	// Inject a worker function that sleeps 100ms to prove concurrency.
	a.setOrchWorkerForTest(func(ctx context.Context, task string, step OrchStep, prior map[int]string) (string, error) {
		time.Sleep(100 * time.Millisecond)
		return step.Description + ":done", nil
	})

	start := time.Now()
	results, err := a.executeWorkers(context.Background(), "task", steps)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("executeWorkers failed: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results; got %d", len(results))
	}
	// Parallel ~100ms; sequential would be ~300ms. 250ms upper bound is
	// generous but still proves concurrency beyond doubt.
	if elapsed > 250*time.Millisecond {
		t.Errorf("steps did not run in parallel: elapsed=%v (expected ~100ms)", elapsed)
	}
}

// TestDispatcher_RespectsDependencies verifies that a step's depends_on are
// honored — step 2 must not start until step 1 has produced its result, and
// the prior result must be injected into step 2's context.
func TestDispatcher_RespectsDependencies(t *testing.T) {
	cfg := DefaultAgentConfig()
	cfg.Orchestration.Enabled = true
	cfg.Orchestration.MaxParallel = 3
	a := createTestAgent(cfg, &mockProvider{name: "mock"})

	steps := []OrchStep{
		{ID: 1, Description: "first"},
		{ID: 2, Description: "second", DependsOn: []int{1}},
	}

	var mu sync.Mutex
	var observedPrior map[int]string
	a.setOrchWorkerForTest(func(ctx context.Context, task string, step OrchStep, prior map[int]string) (string, error) {
		if step.ID == 2 {
			mu.Lock()
			observedPrior = prior
			mu.Unlock()
		}
		return step.Description + ":done", nil
	})

	results, err := a.executeWorkers(context.Background(), "task", steps)
	if err != nil {
		t.Fatalf("executeWorkers failed: %v", err)
	}
	if results[2] != "second:done" {
		t.Errorf("step 2 result mismatch: %q", results[2])
	}
	mu.Lock()
	defer mu.Unlock()
	if observedPrior == nil || observedPrior[1] != "first:done" {
		t.Errorf("step 2 did not receive step 1 result in prior context; got: %v", observedPrior)
	}
}

// -----------------------------------------------------------------------------
// Worker spawner
// -----------------------------------------------------------------------------

// TestWorker_UsesLegacyLoopAndProducesResult verifies that runWorker executes
// the legacy ReAct loop with native function-calling (no JSON scratchpad) and
// returns a usable result string. The worker is a focused call: one step, one
// job, a tight tool budget.
func TestWorker_UsesLegacyLoopAndProducesResult(t *testing.T) {
	// Worker LLM returns a plain-text final answer (no tool_calls) — this is
	// the simplest path through the legacy ReAct loop.
	provider := &mockProvider{
		name: "mock",
		response: &kyoci.CompletionResponse{
			Content:      "The project uses GMT+7 (Asia/Jakarta) across all cron jobs.",
			FinishReason: kyoci.FinishStop,
		},
	}
	cfg := DefaultAgentConfig()
	cfg.Orchestration.Enabled = true
	cfg.Orchestration.WorkerMaxIterations = 4
	a := createTestAgent(cfg, provider)

	step := OrchStep{ID: 1, Description: "examine the cron configs"}
	out, err := a.runWorker(context.Background(), "check tz", step, map[int]string{})
	if err != nil {
		t.Fatalf("runWorker failed: %v", err)
	}
	if !strings.Contains(out, "GMT+7") {
		t.Errorf("worker output missing expected content; got: %q", out)
	}
}

// -----------------------------------------------------------------------------
// Synthesizer
// -----------------------------------------------------------------------------

// TestSynthesizer_ComposesFromStepResults verifies that synthesize() calls the
// LLM once with all step results in scope and returns a coherent final answer.
// The synthesizer has NO tools — it can only write prose from evidence.
func TestSynthesizer_ComposesFromStepResults(t *testing.T) {
	provider := &mockProvider{
		name: "mock",
		response: &kyoci.CompletionResponse{
			Content:      "All 8 LaunchAgents run on GMT+7. The source code hard-codes Asia/Jakarta.",
			FinishReason: kyoci.FinishStop,
		},
	}
	cfg := DefaultAgentConfig()
	cfg.Orchestration.Enabled = true
	a := createTestAgent(cfg, provider)

	steps := []OrchStep{
		{ID: 1, Description: "list cron configs"},
		{ID: 2, Description: "read each plist"},
	}
	results := map[int]string{
		1: "found 8 .plist files",
		2: "all plists reference Asia/Jakarta",
	}

	out, err := a.synthesize(context.Background(), "check tz", steps, results)
	if err != nil {
		t.Fatalf("synthesize failed: %v", err)
	}
	if !strings.Contains(out, "GMT+7") {
		t.Errorf("synthesizer output missing expected content; got: %q", out)
	}
}

// -----------------------------------------------------------------------------
// Pipeline integration
// -----------------------------------------------------------------------------

// sequencerProvider returns scripted responses in order, one per LLM call.
// This lets an end-to-end test drive the full plan→workers→synthesize pipeline
// without a real model. It also captures every request so tests can assert on
// the exact messages / tool_choice sent to the model.
type sequencerProvider struct {
	name      string
	responses []*kyoci.CompletionResponse
	calls     int
	captured  []kyoci.CompletionRequest
	mu        sync.Mutex
}

func (s *sequencerProvider) Name() string              { return s.name }
func (s *sequencerProvider) Models() []kyoci.ModelInfo { return nil }
func (s *sequencerProvider) IsAvailable() bool         { return true }
func (s *sequencerProvider) Stream(ctx context.Context, req kyoci.CompletionRequest) (<-chan kyoci.StreamChunk, error) {
	return nil, nil
}
func (s *sequencerProvider) Complete(ctx context.Context, req kyoci.CompletionRequest) (*kyoci.CompletionResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.captured = append(s.captured, req)
	if s.calls >= len(s.responses) {
		return &kyoci.CompletionResponse{Content: "fallback", FinishReason: kyoci.FinishStop}, nil
	}
	r := s.responses[s.calls]
	s.calls++
	return r, nil
}

// TestOrchestrated_TeknikalidStyleTask_Succeeds is the end-to-end test:
// plan(2 steps) → workers(tool_call then finding) → synthesize. Asserts:
//   - no "budget exhausted" in the output
//   - real answer mentions the key fact
//   - the planner + synthesizer each fire exactly once
//   - each worker fires exactly twice (one tool_call, one finding) — this is
//     the healthy evidence-gathering path the Layer 2 guard now expects.
func TestOrchestrated_TeknikalidStyleTask_Succeeds(t *testing.T) {
	plannerJSON := `[{"id":1,"description":"find project + list plists","depends_on":[],"tool_hint":"file"},{"id":2,"description":"read plists, extract tz","depends_on":[1],"tool_hint":"file"}]`
	worker1Out := "found teknikalidnew with 8 .plist cron files"
	worker2Out := "all plists set StartCalendarInterval with TZ=Asia/Jakarta"
	synthOut := "Yes — teknikalid uses GMT+7 (Asia/Jakarta) consistently across all 8 LaunchAgent cron jobs."

	seq := &sequencerProvider{
		name: "mock",
		responses: []*kyoci.CompletionResponse{
			{Content: plannerJSON, FinishReason: kyoci.FinishStop}, // planner
			// Worker 1: tool_call on iter 0 (guard does NOT fire), then finding.
			{ToolCalls: []kyoci.ToolCall{{ID: "c1", Name: "file", Arguments: `{"operation":"list","path":"~/Documents/teknikalidnew"}`}}, FinishReason: kyoci.FinishToolCall},
			{Content: worker1Out, FinishReason: kyoci.FinishStop},
			// Worker 2 runs after worker 1 (depends_on:[1]): same tool→finding shape.
			{ToolCalls: []kyoci.ToolCall{{ID: "c2", Name: "file", Arguments: `{"operation":"read","path":"~/Documents/teknikalidnew/run.plist"}`}}, FinishReason: kyoci.FinishToolCall},
			{Content: worker2Out, FinishReason: kyoci.FinishStop},
			{Content: synthOut, FinishReason: kyoci.FinishStop}, // synthesizer
		},
	}
	cfg := DefaultAgentConfig()
	cfg.Orchestration.Enabled = true
	cfg.Orchestration.MaxParallel = 2
	cfg.Orchestration.WorkerMaxIterations = 4
	a := createTestAgent(cfg, seq)

	result, err := a.executeOrchestrated(context.Background(), "check teknikalid tz")
	if err != nil {
		t.Fatalf("executeOrchestrated failed: %v", err)
	}
	if strings.Contains(result.Content, "budget exhausted") {
		t.Errorf("pipeline produced budget-exhaustion: %q", result.Content)
	}
	if !strings.Contains(result.Content, "GMT+7") {
		t.Errorf("pipeline answer missing expected fact; got: %q", result.Content)
	}
	// 6 LLM calls: 1 planner + (2 worker-1 + 2 worker-2) + 1 synthesizer.
	if seq.calls != 6 {
		t.Errorf("expected 6 LLM calls (plan + 2×2 workers + synth); got %d", seq.calls)
	}
}

// =============================================================================
// Worker evidence-guard tests
//
// The first end-to-end run showed every worker completing with tool_calls=0:
// qwen2.5-coder:14b answered from parametric memory instead of reading files.
// These tests pin the three-layer fix:
//   - Layer 1: directive WorkerSystemPrompt + step.ToolHint injected into the
//     worker user message.
//   - Layer 2: a Go-side guard that re-prompts (once) when iter-0 has no tool
//     calls and a non-empty tool_hint, tagging refused output with
//     "[no tool evidence ...]" so the synthesizer can honestly report the gap.
// =============================================================================

// TestWorker_PromptIncludesToolHint verifies Layer 1: the worker's first LLM
// request carries (a) a directive system prompt that demands a tool call and
// (b) the assigned step.ToolHint in the user message. The provider returns a
// tool_call on call 1 so the evidence guard never fires — we are asserting on
// the PROMPT, not the guard.
func TestWorker_PromptIncludesToolHint(t *testing.T) {
	seq := &sequencerProvider{
		name: "mock",
		responses: []*kyoci.CompletionResponse{
			{
				// Call 1: a file tool call. The guard won't fire because tools were called.
				ToolCalls: []kyoci.ToolCall{
					{ID: "call_1", Name: "file", Arguments: `{"operation":"list","path":"/tmp"}`},
				},
				FinishReason: kyoci.FinishToolCall,
			},
			{
				// Call 2: plain-text finding → worker returns.
				Content:      "listed /tmp successfully",
				FinishReason: kyoci.FinishStop,
			},
		},
	}
	cfg := DefaultAgentConfig()
	cfg.Orchestration.Enabled = true
	cfg.Orchestration.WorkerMaxIterations = 4
	a := createTestAgent(cfg, seq)

	step := OrchStep{ID: 1, Description: "list files in the project", ToolHint: "file"}
	if _, err := a.runWorker(context.Background(), "find the project", step, map[int]string{}); err != nil {
		t.Fatalf("runWorker failed: %v", err)
	}

	if len(seq.captured) < 1 {
		t.Fatalf("no LLM requests captured")
	}
	first := seq.captured[0]
	if len(first.Messages) < 2 {
		t.Fatalf("first request has too few messages: %d", len(first.Messages))
	}
	sysMsg := first.Messages[0].Content
	userMsg := first.Messages[1].Content

	// Layer 1a: system prompt must demand a tool call as the first action.
	if !strings.Contains(sysMsg, "MUST be a tool call") {
		t.Errorf("system prompt missing directive; got:\n%s", sysMsg)
	}
	// Layer 1b: the assigned tool family must appear in the user message so the
	// model has a concrete default rather than having to choose.
	if !strings.Contains(userMsg, "file") {
		t.Errorf("user message missing tool hint 'file'; got:\n%s", userMsg)
	}
	// Layer 3c: iter-0 request must ask the provider to force a tool call.
	if first.ToolChoice != "required" {
		t.Errorf("iter-0 request ToolChoice = %q, want %q", first.ToolChoice, "required")
	}
	// Iter 1+ must fall back to "" (auto) so the model can decide when it's done.
	if len(seq.captured) >= 2 && seq.captured[1].ToolChoice != "" {
		t.Errorf("iter-1 request ToolChoice = %q, want %q (auto)", seq.captured[1].ToolChoice, "")
	}
}

// TestWorker_RePromptsWhenFirstAnswerHasNoToolCalls verifies Layer 2: when the
// model answers from memory on turn 1 (no tool_calls) despite a non-empty
// tool_hint, the worker must NOT accept it. It appends the WorkerEvidenceNudge
// and re-runs the LLM. The model then calls the tool, sees real results, and
// reports a finding grounded in those results.
func TestWorker_RePromptsWhenFirstAnswerHasNoToolCalls(t *testing.T) {
	seq := &sequencerProvider{
		name: "mock",
		responses: []*kyoci.CompletionResponse{
			{Content: "From memory, the project is at ~/Documents/teknikalidnew.", FinishReason: kyoci.FinishStop},
			{ToolCalls: []kyoci.ToolCall{
				{ID: "call_1", Name: "file", Arguments: `{"operation":"list","path":"~/Documents/teknikalidnew"}`},
			}, FinishReason: kyoci.FinishToolCall},
			{Content: "Confirmed by listing ~/Documents/teknikalidnew: found 8 .plist files.", FinishReason: kyoci.FinishStop},
		},
	}
	cfg := DefaultAgentConfig()
	cfg.Orchestration.Enabled = true
	cfg.Orchestration.WorkerMaxIterations = 6
	a := createTestAgent(cfg, seq)

	step := OrchStep{ID: 1, Description: "find the project files", ToolHint: "file"}
	out, err := a.runWorker(context.Background(), "find the project", step, map[int]string{})
	if err != nil {
		t.Fatalf("runWorker failed: %v", err)
	}
	// The worker must have re-prompted at least once after the memory-only answer.
	if seq.calls < 2 {
		t.Errorf("expected >=2 LLM calls after re-prompt; got %d", seq.calls)
	}
	// The second request's messages must include the nudge.
	if len(seq.captured) >= 2 {
		var joined strings.Builder
		for _, m := range seq.captured[1].Messages {
			joined.WriteString(m.Content)
			joined.WriteByte('\n')
		}
		if !strings.Contains(joined.String(), "NOT acceptable evidence") {
			t.Errorf("nudge not injected between call 1 and call 2; messages:\n%s", joined.String())
		}
	}
	// Output must NOT carry the no-evidence tag (the model did call a tool after the nudge).
	if strings.HasPrefix(strings.TrimSpace(out), "[no tool evidence") {
		t.Errorf("output was tagged despite eventual tool call; got: %q", out)
	}
}

// TestWorker_TagsNoEvidenceOutputAfterRefusal verifies the refusal path: the
// model ignores the nudge and answers from memory a second time. The worker
// must accept the answer (no infinite loop) but tag it with
// "[no tool evidence ...]" so the synthesizer can honestly report the gap to
// the user instead of presenting memory as if it were observed evidence.
func TestWorker_TagsNoEvidenceOutputAfterRefusal(t *testing.T) {
	seq := &sequencerProvider{
		name: "mock",
		responses: []*kyoci.CompletionResponse{
			{Content: "From memory, it uses GMT+7.", FinishReason: kyoci.FinishStop},
			{Content: "I'm still confident it's GMT+7 from memory.", FinishReason: kyoci.FinishStop},
		},
	}
	cfg := DefaultAgentConfig()
	cfg.Orchestration.Enabled = true
	cfg.Orchestration.WorkerMaxIterations = 6
	a := createTestAgent(cfg, seq)

	step := OrchStep{ID: 1, Description: "check timezone", ToolHint: "file"}
	out, err := a.runWorker(context.Background(), "check tz", step, map[int]string{})
	if err != nil {
		t.Fatalf("runWorker failed: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(out), "[no tool evidence") {
		t.Errorf("expected output tagged with [no tool evidence ...]; got: %q", out)
	}
}

// TestWorker_GuardTerminatesAfterOneNudge ensures the guard does not loop
// forever: exactly one nudge, then accept (with tag if still no tools). Same
// scenario as the refusal test, but asserts the precise call count so a future
// regression that double-nudges is caught.
func TestWorker_GuardTerminatesAfterOneNudge(t *testing.T) {
	seq := &sequencerProvider{
		name: "mock",
		responses: []*kyoci.CompletionResponse{
			{Content: "answer from memory", FinishReason: kyoci.FinishStop},
			{Content: "still from memory", FinishReason: kyoci.FinishStop},
			{Content: "should never reach here", FinishReason: kyoci.FinishStop},
		},
	}
	cfg := DefaultAgentConfig()
	cfg.Orchestration.Enabled = true
	cfg.Orchestration.WorkerMaxIterations = 6
	a := createTestAgent(cfg, seq)

	step := OrchStep{ID: 1, Description: "check timezone", ToolHint: "file"}
	out, err := a.runWorker(context.Background(), "check tz", step, map[int]string{})
	if err != nil {
		t.Fatalf("runWorker failed: %v", err)
	}
	// Exactly 2 LLM calls: the refused answer + the post-nudge refusal.
	if seq.calls != 2 {
		t.Errorf("expected exactly 2 LLM calls (1 nudge only); got %d", seq.calls)
	}
	if !strings.HasPrefix(strings.TrimSpace(out), "[no tool evidence") {
		t.Errorf("expected tagged output; got: %q", out)
	}
}

// TestWorker_AcceptsFastPathWhenToolsCalled is a regression guard: when the
// model does the right thing (calls a tool on turn 1, then reports findings),
// the evidence guard must NOT fire and no nudge is injected. Exactly 2 LLM
// calls — a third "just in case" call would needlessly burn latency/tokens.
func TestWorker_AcceptsFastPathWhenToolsCalled(t *testing.T) {
	seq := &sequencerProvider{
		name: "mock",
		responses: []*kyoci.CompletionResponse{
			{ToolCalls: []kyoci.ToolCall{
				{ID: "call_1", Name: "file", Arguments: `{"operation":"read","path":"/etc/timezone"}`},
			}, FinishReason: kyoci.FinishToolCall},
			{Content: "The file says Asia/Jakarta (GMT+7).", FinishReason: kyoci.FinishStop},
		},
	}
	cfg := DefaultAgentConfig()
	cfg.Orchestration.Enabled = true
	cfg.Orchestration.WorkerMaxIterations = 6
	a := createTestAgent(cfg, seq)

	step := OrchStep{ID: 1, Description: "read timezone config", ToolHint: "file"}
	out, err := a.runWorker(context.Background(), "check tz", step, map[int]string{})
	if err != nil {
		t.Fatalf("runWorker failed: %v", err)
	}
	if seq.calls != 2 {
		t.Errorf("expected exactly 2 LLM calls on the fast path; got %d", seq.calls)
	}
	if strings.HasPrefix(strings.TrimSpace(out), "[no tool evidence") {
		t.Errorf("fast-path output was wrongly tagged; got: %q", out)
	}
}

// TestWorker_SkipsGuardWhenToolHintEmpty verifies the pure-reasoning exception:
// a step with no tool_hint (arithmetic, summarization) must be allowed to
// answer directly on turn 1 without the guard firing. Forcing a tool call on
// such steps would be wrong — there's nothing to investigate.
func TestWorker_SkipsGuardWhenToolHintEmpty(t *testing.T) {
	seq := &sequencerProvider{
		name: "mock",
		responses: []*kyoci.CompletionResponse{
			{Content: "The sum is 4.", FinishReason: kyoci.FinishStop},
		},
	}
	cfg := DefaultAgentConfig()
	cfg.Orchestration.Enabled = true
	cfg.Orchestration.WorkerMaxIterations = 6
	a := createTestAgent(cfg, seq)

	step := OrchStep{ID: 1, Description: "add 2 and 2", ToolHint: ""}
	out, err := a.runWorker(context.Background(), "compute the sum", step, map[int]string{})
	if err != nil {
		t.Fatalf("runWorker failed: %v", err)
	}
	if seq.calls != 1 {
		t.Errorf("expected exactly 1 LLM call for pure-reasoning step; got %d", seq.calls)
	}
	if strings.HasPrefix(strings.TrimSpace(out), "[no tool evidence") {
		t.Errorf("pure-reasoning output was wrongly tagged; got: %q", out)
	}
}
