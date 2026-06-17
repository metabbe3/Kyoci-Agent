package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/metabbe3/Kyoci-Agent/internal/llm"
	"github.com/metabbe3/Kyoci-Agent/internal/tool/builtin"
	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// =============================================================================
// Step 1: Prompts module tests
// =============================================================================

// TestBaseSystemPrompt_UnderTokenBudget verifies the base prompt stays compact.
// Small models (8B) drift on long prompts — hard cap at ~400 tokens (~1600 chars).
func TestBaseSystemPrompt_UnderTokenBudget(t *testing.T) {
	if len(BaseSystemPrompt) == 0 {
		t.Fatal("BaseSystemPrompt must not be empty")
	}
	// Heuristic: 1 token ~= 4 chars. 400 tokens ~= 1600 chars.
	const maxChars = 1700
	if len(BaseSystemPrompt) > maxChars {
		t.Errorf("BaseSystemPrompt is %d chars, exceeds %d-char budget (~400 tokens); "+
			"small models drift on long prompts",
			len(BaseSystemPrompt), maxChars)
	}
}

// TestBaseSystemPrompt_ContainsRequiredSections verifies the prompt encodes
// the output protocol + NEVER rules + ALWAYS rules.
func TestBaseSystemPrompt_ContainsRequiredSections(t *testing.T) {
	required := []string{
		"OUTPUT PROTOCOL", // structured-JSON contract
		"NEVER",           // anti-pattern forbidding
		"ALWAYS",          // positive rules
		"task_understanding",
		"next_action",
		"verification_evidence",
		"confidence",
	}
	for _, token := range required {
		if !strings.Contains(BaseSystemPrompt, token) {
			t.Errorf("BaseSystemPrompt missing required token %q", token)
		}
	}
}

// TestAssessPrompt_ContainsTask verifies per-task prompt builders embed the task.
func TestAssessPrompt_ContainsTask(t *testing.T) {
	got := AssessPrompt("create hello.txt")
	if !strings.Contains(got, "create hello.txt") {
		t.Errorf("AssessPrompt must embed the task text; got: %s", got)
	}
}

// TestPlanPrompt_ContainsTask verifies PlanPrompt embeds the task.
func TestPlanPrompt_ContainsTask(t *testing.T) {
	got := PlanPrompt("refactor auth module across 3 files")
	if !strings.Contains(got, "refactor auth module across 3 files") {
		t.Errorf("PlanPrompt must embed the task text; got: %s", got)
	}
}

// TestReflectPrompt_ContainsFailureInfo verifies ReflectPrompt surfaces the
// concrete failure history to the model.
func TestReflectPrompt_ContainsFailureInfo(t *testing.T) {
	failures := []failureEntry{
		{Iteration: 3, Tool: "terminal", Args: `{"command":"ls"}`, Error: "exit 1"},
	}
	got := ReflectPrompt(failures)
	if !strings.Contains(got, "terminal") {
		t.Errorf("ReflectPrompt must mention the failed tool; got: %s", got)
	}
	if !strings.Contains(got, "exit 1") {
		t.Errorf("ReflectPrompt must include the error message; got: %s", got)
	}
}

// TestFewShotExample_ContainsJSON verifies the few-shot anchor has valid JSON
// the model can pattern-match on.
func TestFewShotExample_ContainsJSON(t *testing.T) {
	if !strings.Contains(FewShotExample, `"task_understanding"`) {
		t.Error("FewShotExample must contain a task_understanding JSON field")
	}
	if !strings.Contains(FewShotExample, `"next_action"`) {
		t.Error("FewShotExample must contain a next_action JSON field")
	}
	if !strings.Contains(FewShotExample, "verification_evidence") {
		t.Error("FewShotExample must demonstrate verification_evidence")
	}
}

// =============================================================================
// Step 2: Thought parser tests
// =============================================================================
//
// parseThought extracts a Thought from the model's text output using a 3-tier
// strategy: (1) strict JSON, (2) brace-extraction, (3) regex salvage of
// next_action. Small models frequently emit fenced JSON, JSON with surrounding
// prose, or partial JSON — the parser must handle all of these gracefully.

func TestParseThought_ValidJSON(t *testing.T) {
	input := `{"task_understanding":"create file","ambiguity":[],"plan":[{"description":"write","tool":"file","status":"pending"}],"next_action":{"type":"tool_call","tool_call":{"id":"1","name":"file","arguments":"{}"}},"expected_result":"ok","confidence":0.9,"tool_rationale":"only way"}`
	th, err := parseThought(input)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if th.TaskUnderstanding != "create file" {
		t.Errorf("TaskUnderstanding = %q, want %q", th.TaskUnderstanding, "create file")
	}
	if th.NextAction.Type != "tool_call" {
		t.Errorf("NextAction.Type = %q, want %q", th.NextAction.Type, "tool_call")
	}
	if th.Confidence != 0.9 {
		t.Errorf("Confidence = %f, want 0.9", th.Confidence)
	}
	if th.ToolRationale != "only way" {
		t.Errorf("ToolRationale = %q, want %q", th.ToolRationale, "only way")
	}
	if len(th.Plan) != 1 {
		t.Errorf("Plan length = %d, want 1", len(th.Plan))
	}
}

func TestParseThought_FencedJSON(t *testing.T) {
	input := "```json\n" +
		`{"task_understanding":"fenced","ambiguity":[],"plan":[],"next_action":{"type":"final_answer","final_answer":"ok"},"expected_result":"ok","confidence":0.8}` +
		"\n```"
	th, err := parseThought(input)
	if err != nil {
		t.Fatalf("expected no error on fenced JSON, got: %v", err)
	}
	if th.TaskUnderstanding != "fenced" {
		t.Errorf("TaskUnderstanding = %q, want %q", th.TaskUnderstanding, "fenced")
	}
}

func TestParseThought_JSONWithSurroundingProse(t *testing.T) {
	input := `Let me think about this.
{"task_understanding":"with prose","ambiguity":[],"plan":[],"next_action":{"type":"tool_call","tool_call":{"id":"1","name":"file","arguments":"{}"}},"expected_result":"ok","confidence":0.7,"tool_rationale":"ok"}
That's my plan.`
	th, err := parseThought(input)
	if err != nil {
		t.Fatalf("expected no error on JSON with prose, got: %v", err)
	}
	if th.TaskUnderstanding != "with prose" {
		t.Errorf("TaskUnderstanding = %q, want %q", th.TaskUnderstanding, "with prose")
	}
	if th.NextAction.Type != "tool_call" {
		t.Errorf("NextAction.Type = %q, want tool_call", th.NextAction.Type)
	}
}

func TestParseThought_PartialJSONSalvagesNextAction(t *testing.T) {
	// Model emitted malformed JSON but the next_action block is intact.
	// The parser should still extract the tool call so we don't lose the turn.
	input := `Some prose.
{"next_action":{"type":"tool_call","tool_call":{"id":"1","name":"terminal","arguments":"{\"command\":\"ls\"}"}}}
More prose after.`
	th, err := parseThought(input)
	if err != nil {
		t.Fatalf("expected salvage to succeed, got: %v", err)
	}
	if th.NextAction.Type != "tool_call" {
		t.Errorf("NextAction.Type = %q, want tool_call", th.NextAction.Type)
	}
	if th.NextAction.ToolCall == nil {
		t.Fatal("NextAction.ToolCall must be populated on salvage")
	}
	if th.NextAction.ToolCall.Name != "terminal" {
		t.Errorf("ToolCall.Name = %q, want terminal", th.NextAction.ToolCall.Name)
	}
}

func TestParseThought_GarbageReturnsError(t *testing.T) {
	_, err := parseThought("hello world this is not json or a tool call")
	if err == nil {
		t.Fatal("expected error on garbage input, got nil")
	}
}

func TestParseThought_EmptyReturnsError(t *testing.T) {
	_, err := parseThought("")
	if err == nil {
		t.Fatal("expected error on empty input, got nil")
	}
}

// =============================================================================
// Step 3: Complexity heuristics tests
// =============================================================================
//
// assessComplexity classifies a task using deterministic regex heuristics. The
// Assess state uses this to decide between the fast path (simple → Execute) and
// the full multi-pass loop (complex → Plan). False positives are OK (Plan still
// works for simple tasks); false negatives are the real risk (skipping Plan on
// a task that needs it).

func TestAssessComplexity_Simple(t *testing.T) {
	r := assessComplexity("create hello.txt with content hi", nil)
	if r.Complex {
		t.Errorf("simple task flagged as complex: %+v", r)
	}
	if r.MultiFile || r.Vague || r.ExplicitPlan || r.AnyFailure {
		t.Errorf("simple task should have no complexity signals: %+v", r)
	}
}

func TestAssessComplexity_MultiFile(t *testing.T) {
	r := assessComplexity("refactor auth.go and user.go to share a session helper", nil)
	if !r.MultiFile {
		t.Errorf("multi-file task should set MultiFile: %+v", r)
	}
	if !r.Complex {
		t.Error("multi-file task should be Complex")
	}
}

func TestAssessComplexity_MultiFileAcrossKeyword(t *testing.T) {
	r := assessComplexity("update tests across multiple packages", nil)
	if !r.MultiFile {
		t.Errorf("'across multiple' should set MultiFile: %+v", r)
	}
}

func TestAssessComplexity_VagueShortTask(t *testing.T) {
	r := assessComplexity("fix it", nil)
	if !r.Vague {
		t.Errorf("very short task should set Vague: %+v", r)
	}
	if !r.Complex {
		t.Error("vague task should be Complex")
	}
}

func TestAssessComplexity_VagueQuestionWithoutContext(t *testing.T) {
	r := assessComplexity("why does it fail?", nil)
	if !r.Vague {
		t.Errorf("question without file/command/code reference should set Vague: %+v", r)
	}
}

func TestAssessComplexity_ExplicitPlan(t *testing.T) {
	r := assessComplexity("plan the migration steps for the database", nil)
	if !r.ExplicitPlan {
		t.Errorf("task with 'plan' keyword should set ExplicitPlan: %+v", r)
	}
	if !r.Complex {
		t.Error("explicit-plan task should be Complex")
	}
}

func TestAssessComplexity_AnyFailure(t *testing.T) {
	history := []failureEntry{{Iteration: 1, Tool: "terminal", Args: "{}", Error: "exit 1"}}
	r := assessComplexity("run the tests", history)
	if !r.AnyFailure {
		t.Errorf("non-empty failure history should set AnyFailure: %+v", r)
	}
	if !r.Complex {
		t.Error("task with prior failures should be Complex")
	}
}

// =============================================================================
// Step 4: loopState + budget logic tests
// =============================================================================
//
// loopState tracks everything between iterations of the state machine:
// tool-call budget, unique-call hashes for loop detection, failure history,
// reflection/replan caps. These are deterministic — no LLM calls involved.

func newTestLoopState() *loopState {
	return &loopState{
		toolBudget:       15,
		maxReflections:   3,
		maxReplans:       2,
		uniqueCallHashes: make(map[string]int),
	}
}

func TestLoopState_RecordToolCall_IncrementsCount(t *testing.T) {
	ls := newTestLoopState()
	ls.recordToolCall("terminal", `{"command":"ls"}`)
	ls.recordToolCall("file", `{"operation":"read","path":"a.txt"}`)
	if ls.toolCallsUsed != 2 {
		t.Errorf("toolCallsUsed = %d, want 2", ls.toolCallsUsed)
	}
}

func TestLoopState_DetectRepeatedCall(t *testing.T) {
	ls := newTestLoopState()
	// Three identical calls in last 4 → loop detected.
	ls.recordToolCall("terminal", `{"command":"ls"}`)
	ls.recordToolCall("terminal", `{"command":"ls"}`)
	ls.recordToolCall("file", `{"operation":"read","path":"x"}`)
	ls.recordToolCall("terminal", `{"command":"ls"}`)
	if !ls.isLooping() {
		t.Error("expected isLooping=true after 3 identical calls in last 4")
	}
}

func TestLoopState_DifferentCalls_NotLooping(t *testing.T) {
	ls := newTestLoopState()
	ls.recordToolCall("terminal", `{"command":"ls"}`)
	ls.recordToolCall("file", `{"operation":"read","path":"a"}`)
	ls.recordToolCall("terminal", `{"command":"pwd"}`)
	ls.recordToolCall("file", `{"operation":"read","path":"b"}`)
	if ls.isLooping() {
		t.Error("expected isLooping=false when all recent calls are distinct")
	}
}

func TestLoopState_BudgetExhausted(t *testing.T) {
	ls := newTestLoopState()
	ls.toolBudget = 3
	ls.recordToolCall("terminal", `{"command":"ls"}`)
	ls.recordToolCall("terminal", `{"command":"ls"}`)
	ls.recordToolCall("terminal", `{"command":"ls"}`)
	if !ls.budgetExhausted() {
		t.Error("expected budgetExhausted=true at 3/3 calls")
	}
}

func TestLoopState_BudgetRemaining(t *testing.T) {
	ls := newTestLoopState()
	ls.toolBudget = 15
	ls.recordToolCall("terminal", `{"command":"ls"}`)
	if ls.budgetExhausted() {
		t.Error("expected budgetExhausted=false at 1/15 calls")
	}
}

func TestLoopState_BudgetNearExhaustedNoProgress(t *testing.T) {
	ls := newTestLoopState()
	ls.toolBudget = 4
	// 3 calls, only 1 unique (lots of repetition) → should escalate
	ls.recordToolCall("terminal", `{"command":"ls"}`)
	ls.recordToolCall("terminal", `{"command":"ls"}`)
	ls.recordToolCall("terminal", `{"command":"ls"}`)
	if !ls.budgetNearExhaustedNoProgress() {
		t.Error("expected budgetNearExhaustedNoProgress=true (3/4 calls, 1/3 unique)")
	}
}

func TestLoopState_CanReflect(t *testing.T) {
	ls := newTestLoopState()
	ls.maxReflections = 3
	if !ls.canReflect() {
		t.Error("expected canReflect=true at 0/3 reflections")
	}
	ls.reflectionsUsed = 3
	if ls.canReflect() {
		t.Error("expected canReflect=false at 3/3 reflections")
	}
}

func TestLoopState_CanReplan(t *testing.T) {
	ls := newTestLoopState()
	ls.maxReplans = 2
	if !ls.canReplan() {
		t.Error("expected canReplan=true at 0/2 replans")
	}
	ls.replansUsed = 2
	if ls.canReplan() {
		t.Error("expected canReplan=false at 2/2 replans")
	}
}

func TestLoopState_RecordFailure(t *testing.T) {
	ls := newTestLoopState()
	ls.recordFailure(failureEntry{Iteration: 1, Tool: "terminal", Args: "{}", Error: "exit 1"})
	if len(ls.failureHistory) != 1 {
		t.Errorf("failureHistory length = %d, want 1", len(ls.failureHistory))
	}
}

// =============================================================================
// Step 4b: search-budget tracking tests (Change 2 — exploration tasks)
// =============================================================================
//
// 8B-class models frequently vary a file-search pattern slightly each turn
// ("timezone|TZ|GMT" → "timezone|TZ|GMT|Asia.*Jakarta" → ...) instead of
// reading the files they already found. The hash-based loop detector misses
// this because each variant produces a different hash. searchesUsed +
// searchBudgetExceeded give executeStep a deterministic signal to inject a
// "stop searching, start reading" nudge.

func TestLoopState_RecordToolCall_TracksSearches(t *testing.T) {
	ls := newTestLoopState()
	ls.recordToolCall("file", `{"operation":"search","path":"/x","pattern":"tz"}`)
	ls.recordToolCall("file", `{"operation":"search","path":"/x","pattern":"cron"}`)
	// Other file operations don't count toward the search budget.
	ls.recordToolCall("file", `{"operation":"read","path":"/x/y"}`)
	ls.recordToolCall("file", `{"operation":"list","path":"/x"}`)
	// Other tools don't count either.
	ls.recordToolCall("terminal", `{"command":"ls"}`)
	if ls.searchesUsed != 2 {
		t.Errorf("searchesUsed = %d, want 2 (only file operation:search counts)", ls.searchesUsed)
	}
}

func TestLoopState_SearchBudgetExceeded_AtThreshold(t *testing.T) {
	ls := newTestLoopState()
	for i := 0; i < maxSearchesPerTask; i++ {
		ls.recordToolCall("file", fmt.Sprintf(`{"operation":"search","pattern":"p%d"}`, i))
	}
	if !ls.searchBudgetExceeded() {
		t.Errorf("expected searchBudgetExceeded=true at %d searches", maxSearchesPerTask)
	}
}

func TestLoopState_SearchBudgetExceeded_BelowThreshold(t *testing.T) {
	ls := newTestLoopState()
	for i := 0; i < maxSearchesPerTask-1; i++ {
		ls.recordToolCall("file", fmt.Sprintf(`{"operation":"search","pattern":"p%d"}`, i))
	}
	if ls.searchBudgetExceeded() {
		t.Errorf("expected searchBudgetExceeded=false below %d searches", maxSearchesPerTask)
	}
}

func TestLoopState_RecordToolCall_MalformedArgsDoesNotPanic(t *testing.T) {
	// Garbage JSON or empty args must not crash the search-tracker; they
	// just don't increment searchesUsed (can't tell what op was requested).
	ls := newTestLoopState()
	ls.recordToolCall("file", `not-json`)
	ls.recordToolCall("file", ``)
	ls.recordToolCall("file", `{"operation":}`) // malformed value
	if ls.searchesUsed != 0 {
		t.Errorf("malformed args should not increment searchesUsed; got %d", ls.searchesUsed)
	}
	if ls.toolCallsUsed != 3 {
		t.Errorf("toolCallsUsed should still count every call; got %d want 3", ls.toolCallsUsed)
	}
}

// =============================================================================
// Step 5: Assess state tests
// =============================================================================
//
// assessStep is the entry state of the loop. It calls the LLM once with
// AssessPrompt, parses the response as a Thought, and decides between the
// fast path (→ Execute) and the full multi-pass loop (→ Plan).

// thoughtJSON builds a valid JSON Thought response for the mock provider.
func thoughtJSON(overrides ...func(*Thought)) string {
	th := Thought{
		TaskUnderstanding: "test task",
		Ambiguity:         []string{},
		Plan:              []PlanStep{{Description: "step", Tool: "file", Status: "pending"}},
		NextAction: NextAction{
			Type: "tool_call",
			ToolCall: &kyoci.ToolCall{
				ID:        "1",
				Name:      "file",
				Arguments: `{"operation":"write"}`,
			},
		},
		ExpectedResult: "ok",
		Confidence:     0.9,
		ToolRationale:  "test",
	}
	for _, o := range overrides {
		o(&th)
	}
	b, _ := json.Marshal(th)
	return string(b)
}

func TestAssess_FastPath_SimpleTask(t *testing.T) {
	// Mock returns a high-confidence single-step plan with a tool_call — fast path.
	provider := &mockProvider{
		name: "mock",
		response: &kyoci.CompletionResponse{
			Content:      thoughtJSON(),
			FinishReason: kyoci.FinishStop,
		},
	}
	cfg := DefaultAgentConfig()
	cfg.EnableThinking = true
	agent := createTestAgent(cfg, provider)

	ls := newTestLoopState()
	convo := NewContext()
	convo.AddMessage(kyoci.RoleSystem, "test")
	convo.AddMessage(kyoci.RoleUser, "create hello.txt")

	_, next, err := agent.assessStep(context.Background(), convo, ls, "create hello.txt")
	if err != nil {
		t.Fatalf("assessStep error: %v", err)
	}
	if next != StateExecute {
		t.Errorf("simple high-confidence task should fast-path to Execute; got %v", next)
	}
}

func TestAssess_Escalates_MultiFile(t *testing.T) {
	// Even if the model is confident, multi-file signal forces Plan.
	provider := &mockProvider{
		name: "mock",
		response: &kyoci.CompletionResponse{
			Content:      thoughtJSON(),
			FinishReason: kyoci.FinishStop,
		},
	}
	cfg := DefaultAgentConfig()
	cfg.EnableThinking = true
	agent := createTestAgent(cfg, provider)

	ls := newTestLoopState()
	convo := NewContext()
	convo.AddMessage(kyoci.RoleSystem, "test")
	convo.AddMessage(kyoci.RoleUser, "refactor auth.go and user.go")

	_, next, err := agent.assessStep(context.Background(), convo, ls, "refactor auth.go and user.go")
	if err != nil {
		t.Fatalf("assessStep error: %v", err)
	}
	if next != StatePlan {
		t.Errorf("multi-file task should escalate to Plan; got %v", next)
	}
}

func TestAssess_Escalates_LowConfidence(t *testing.T) {
	provider := &mockProvider{
		name: "mock",
		response: &kyoci.CompletionResponse{
			Content: thoughtJSON(func(th *Thought) {
				th.Confidence = 0.4
			}),
			FinishReason: kyoci.FinishStop,
		},
	}
	cfg := DefaultAgentConfig()
	cfg.EnableThinking = true
	agent := createTestAgent(cfg, provider)

	ls := newTestLoopState()
	convo := NewContext()
	convo.AddMessage(kyoci.RoleSystem, "test")
	convo.AddMessage(kyoci.RoleUser, "create hello.txt")

	_, next, err := agent.assessStep(context.Background(), convo, ls, "create hello.txt")
	if err != nil {
		t.Fatalf("assessStep error: %v", err)
	}
	if next != StatePlan {
		t.Errorf("low-confidence task should escalate to Plan; got %v", next)
	}
}

func TestAssess_ParseFailure_ReturnsError(t *testing.T) {
	provider := &mockProvider{
		name: "mock",
		response: &kyoci.CompletionResponse{
			Content:      "this is not valid JSON or a tool call",
			FinishReason: kyoci.FinishStop,
		},
	}
	cfg := DefaultAgentConfig()
	cfg.EnableThinking = true
	agent := createTestAgent(cfg, provider)

	ls := newTestLoopState()
	convo := NewContext()
	convo.AddMessage(kyoci.RoleSystem, "test")
	convo.AddMessage(kyoci.RoleUser, "create hello.txt")

	_, _, err := agent.assessStep(context.Background(), convo, ls, "create hello.txt")
	if err == nil {
		t.Fatal("expected error on parse failure, got nil")
	}
}

// =============================================================================
// Step 6: Plan state tests
// =============================================================================
//
// planStep decomposes a complex task into a multi-step plan. It records the
// plan in loopState.currentPlan and transitions to Execute. Plans are capped
// at 8 items to keep the model focused.

func TestPlan_TransitionsToExecute(t *testing.T) {
	provider := &mockProvider{
		name: "mock",
		response: &kyoci.CompletionResponse{
			Content: thoughtJSON(func(th *Thought) {
				th.Plan = []PlanStep{
					{Description: "step 1", Tool: "file", Status: "pending"},
					{Description: "step 2", Tool: "terminal", Status: "pending"},
					{Description: "step 3", Tool: "file", Status: "pending"},
				}
			}),
			FinishReason: kyoci.FinishStop,
		},
	}
	cfg := DefaultAgentConfig()
	cfg.EnableThinking = true
	agent := createTestAgent(cfg, provider)

	ls := newTestLoopState()
	convo := NewContext()
	convo.AddMessage(kyoci.RoleSystem, "test")
	convo.AddMessage(kyoci.RoleUser, "refactor auth.go and user.go")

	_, next, err := agent.planStep(context.Background(), convo, ls, "refactor auth.go and user.go")
	if err != nil {
		t.Fatalf("planStep error: %v", err)
	}
	if next != StateExecute {
		t.Errorf("Plan should transition to Execute; got %v", next)
	}
	if len(ls.currentPlan) != 3 {
		t.Errorf("currentPlan length = %d, want 3", len(ls.currentPlan))
	}
}

func TestPlan_CapsAtEight(t *testing.T) {
	// Model emits 12 plan items — we should cap at 8.
	bigPlan := make([]PlanStep, 12)
	for i := range bigPlan {
		bigPlan[i] = PlanStep{Description: "step", Tool: "file", Status: "pending"}
	}
	provider := &mockProvider{
		name: "mock",
		response: &kyoci.CompletionResponse{
			Content: thoughtJSON(func(th *Thought) {
				th.Plan = bigPlan
			}),
			FinishReason: kyoci.FinishStop,
		},
	}
	cfg := DefaultAgentConfig()
	cfg.EnableThinking = true
	agent := createTestAgent(cfg, provider)

	ls := newTestLoopState()
	convo := NewContext()
	convo.AddMessage(kyoci.RoleSystem, "test")

	_, _, err := agent.planStep(context.Background(), convo, ls, "complex task")
	if err != nil {
		t.Fatalf("planStep error: %v", err)
	}
	if len(ls.currentPlan) > 8 {
		t.Errorf("currentPlan length = %d, should be capped at 8", len(ls.currentPlan))
	}
}

func TestPlan_ParseFailure_ReturnsError(t *testing.T) {
	provider := &mockProvider{
		name: "mock",
		response: &kyoci.CompletionResponse{
			Content:      "garbage response",
			FinishReason: kyoci.FinishStop,
		},
	}
	cfg := DefaultAgentConfig()
	cfg.EnableThinking = true
	agent := createTestAgent(cfg, provider)

	ls := newTestLoopState()
	convo := NewContext()
	convo.AddMessage(kyoci.RoleSystem, "test")

	_, _, err := agent.planStep(context.Background(), convo, ls, "complex task")
	if err == nil {
		t.Fatal("expected error on parse failure, got nil")
	}
}

// =============================================================================
// Step 7: Execute state tests
// =============================================================================
//
// executeStep processes ONE turn: calls the LLM with ExecutePrompt, parses the
// Thought, and either runs the tool call (staying in Execute or escalating to
// Reflect on failure) or transitions out (Verify on final_answer, Reflect on
// replan). The driver calls executeStep repeatedly until it transitions out.

func TestExecute_RunsToolCall_StaysInExecute(t *testing.T) {
	tool := &mockTool{
		name:        "file",
		description: "test file tool",
		params:      []kyoci.ToolParameter{{Name: "operation", Type: "string", Required: true}},
		result:      "ok",
	}
	provider := &mockProvider{
		name: "mock",
		response: &kyoci.CompletionResponse{
			Content:      thoughtJSON(),
			FinishReason: kyoci.FinishStop,
		},
	}
	cfg := DefaultAgentConfig()
	cfg.EnableThinking = true
	agent := createTestAgent(cfg, provider)
	agent.tools.Register(tool)

	ls := newTestLoopState()
	convo := NewContext()
	convo.AddMessage(kyoci.RoleSystem, "test")

	_, next, err := agent.executeStep(context.Background(), convo, ls)
	if err != nil {
		t.Fatalf("executeStep error: %v", err)
	}
	if next != StateExecute {
		t.Errorf("successful tool call should stay in Execute; got %v", next)
	}
	if ls.toolCallsUsed != 1 {
		t.Errorf("toolCallsUsed = %d, want 1", ls.toolCallsUsed)
	}
}

func TestExecute_TransitionsToVerify_OnFinalAnswer(t *testing.T) {
	provider := &mockProvider{
		name: "mock",
		response: &kyoci.CompletionResponse{
			Content: thoughtJSON(func(th *Thought) {
				th.NextAction = NextAction{
					Type:        "final_answer",
					FinalAnswer: "Done. Task completed.",
				}
				th.VerificationEvidence = []string{"file exists"}
			}),
			FinishReason: kyoci.FinishStop,
		},
	}
	cfg := DefaultAgentConfig()
	cfg.EnableThinking = true
	agent := createTestAgent(cfg, provider)

	ls := newTestLoopState()
	convo := NewContext()
	convo.AddMessage(kyoci.RoleSystem, "test")

	_, next, err := agent.executeStep(context.Background(), convo, ls)
	if err != nil {
		t.Fatalf("executeStep error: %v", err)
	}
	if next != StateVerify {
		t.Errorf("final_answer should transition to Verify; got %v", next)
	}
}

func TestExecute_TransitionsToReflect_OnToolError(t *testing.T) {
	tool := &mockTool{
		name:        "file",
		description: "test file tool",
		params:      []kyoci.ToolParameter{{Name: "operation", Type: "string", Required: true}},
		err:         errors.New("permission denied"),
	}
	provider := &mockProvider{
		name: "mock",
		response: &kyoci.CompletionResponse{
			Content:      thoughtJSON(),
			FinishReason: kyoci.FinishStop,
		},
	}
	cfg := DefaultAgentConfig()
	cfg.EnableThinking = true
	agent := createTestAgent(cfg, provider)
	agent.tools.Register(tool)

	ls := newTestLoopState()
	convo := NewContext()
	convo.AddMessage(kyoci.RoleSystem, "test")

	_, next, err := agent.executeStep(context.Background(), convo, ls)
	if err != nil {
		t.Fatalf("executeStep error: %v", err)
	}
	if next != StateReflect {
		t.Errorf("tool error should transition to Reflect; got %v", next)
	}
	if len(ls.failureHistory) != 1 {
		t.Errorf("failureHistory length = %d, want 1", len(ls.failureHistory))
	}
}

func TestExecute_TransitionsToReflect_OnReplan(t *testing.T) {
	provider := &mockProvider{
		name: "mock",
		response: &kyoci.CompletionResponse{
			Content: thoughtJSON(func(th *Thought) {
				th.NextAction = NextAction{Type: "replan"}
				th.RootCause = "wrong tool selected"
			}),
			FinishReason: kyoci.FinishStop,
		},
	}
	cfg := DefaultAgentConfig()
	cfg.EnableThinking = true
	agent := createTestAgent(cfg, provider)

	ls := newTestLoopState()
	convo := NewContext()
	convo.AddMessage(kyoci.RoleSystem, "test")

	_, next, err := agent.executeStep(context.Background(), convo, ls)
	if err != nil {
		t.Fatalf("executeStep error: %v", err)
	}
	if next != StateReflect {
		t.Errorf("replan action should transition to Reflect; got %v", next)
	}
}

func TestExecute_TransitionsToReflect_OnLoopingDetected(t *testing.T) {
	// loopState already shows the model is stuck AND we've already used the
	// one-shot loop-break nudge (loopBreakAttempted=1). executeStep should
	// not even call the LLM; it should transition straight to Reflect.
	provider := &mockProvider{
		name: "mock",
		response: &kyoci.CompletionResponse{
			Content:      thoughtJSON(),
			FinishReason: kyoci.FinishStop,
		},
	}
	cfg := DefaultAgentConfig()
	cfg.EnableThinking = true
	agent := createTestAgent(cfg, provider)

	ls := newTestLoopState()
	// Three identical prior calls in recent window → isLooping() returns true.
	ls.recordToolCall("terminal", `{"command":"ls"}`)
	ls.recordToolCall("terminal", `{"command":"ls"}`)
	ls.recordToolCall("terminal", `{"command":"ls"}`)
	// The loop-break nudge was already attempted on a prior turn — the model
	// kept looping anyway, so this turn must escalate to Reflect.
	ls.loopBreakAttempted = 1

	convo := NewContext()
	convo.AddMessage(kyoci.RoleSystem, "test")

	_, next, err := agent.executeStep(context.Background(), convo, ls)
	if err != nil {
		t.Fatalf("executeStep error: %v", err)
	}
	if next != StateReflect {
		t.Errorf("looping should transition to Reflect without executing; got %v", next)
	}
	// The escalation must record a failure so that reflectExhaustionMessage
	// shows a real cause instead of "Last failure: none".
	if len(ls.failureHistory) == 0 {
		t.Error("loop detection should record a failure so the exhaustion message shows a real cause, not 'none'")
	}
}

// TestExecute_LoopBreakNudge_FirstDetection_StaysInExecute covers the recovery
// path: the 9B model got a tool result but fixates on re-calling the same tool.
// Rather than burning a reflection on the first detection, we inject a
// LoopBreakNudge directive and give the model one more turn to emit a
// final_answer from the results it already has.
func TestExecute_LoopBreakNudge_FirstDetection_StaysInExecute(t *testing.T) {
	// Mock returns a final_answer — the LoopBreakNudge should make the model
	// stop calling tools and present the result.
	finalAnswerJSON := thoughtJSON(func(th *Thought) {
		th.NextAction = NextAction{
			Type:        "final_answer",
			FinalAnswer: "The answer based on tool results: 4",
		}
		th.VerificationEvidence = []string{"calculator returned 4"}
		th.Confidence = 0.9
	})
	provider := &mockProvider{
		name: "mock",
		response: &kyoci.CompletionResponse{
			Content:      finalAnswerJSON,
			FinishReason: kyoci.FinishStop,
		},
	}
	cfg := DefaultAgentConfig()
	cfg.EnableThinking = true
	agent := createTestAgent(cfg, provider)

	ls := newTestLoopState()
	// Three identical prior calls → isLooping() returns true. This is exactly
	// the death-spiral pattern we see on 8B models: the tool succeeded but the
	// model re-calls it instead of transitioning to final_answer.
	ls.recordToolCall("calculator", `{"expression":"2 + 2"}`)
	ls.recordToolCall("calculator", `{"expression":"2 + 2"}`)
	ls.recordToolCall("calculator", `{"expression":"2 + 2"}`)

	convo := NewContext()
	convo.AddMessage(kyoci.RoleSystem, "test")

	_, next, err := agent.executeStep(context.Background(), convo, ls)
	if err != nil {
		t.Fatalf("executeStep error: %v", err)
	}
	if next == StateReflect {
		t.Errorf("first loop detection with no failures should stay in Execute via nudge, not escalate to Reflect; " +
			"burning a reflection here is what causes 3/3 exhaustion on simple tasks")
	}
	if ls.loopBreakAttempted != 1 {
		t.Errorf("loopBreakAttempted = %d, want 1 (nudge should fire exactly once)", ls.loopBreakAttempted)
	}
}

// TestExecute_LoopBreakNudge_SecondDetection_EscalatesToReflect covers the
// case where the nudge was tried but the model kept looping. We must not
// nudge forever — escalate to Reflect for a real replan.
func TestExecute_LoopBreakNudge_SecondDetection_EscalatesToReflect(t *testing.T) {
	provider := &mockProvider{
		name: "mock",
		response: &kyoci.CompletionResponse{
			Content:      thoughtJSON(),
			FinishReason: kyoci.FinishStop,
		},
	}
	cfg := DefaultAgentConfig()
	cfg.EnableThinking = true
	agent := createTestAgent(cfg, provider)

	ls := newTestLoopState()
	ls.recordToolCall("terminal", `{"command":"ls"}`)
	ls.recordToolCall("terminal", `{"command":"ls"}`)
	ls.recordToolCall("terminal", `{"command":"ls"}`)
	ls.loopBreakAttempted = 1 // nudge already tried on prior turn

	convo := NewContext()
	convo.AddMessage(kyoci.RoleSystem, "test")

	_, next, err := agent.executeStep(context.Background(), convo, ls)
	if err != nil {
		t.Fatalf("executeStep error: %v", err)
	}
	if next != StateReflect {
		t.Errorf("second loop detection (nudge already used) should escalate to Reflect; got %v", next)
	}
	if len(ls.failureHistory) == 0 {
		t.Error("escalation after failed nudge must record a failure so the exhaustion message shows a real cause")
	}
}

// TestExecute_LoopBreakNudge_SkippedWhenRealToolFailure covers the case where
// a tool genuinely failed AND the model is now looping. There's already
// something concrete to reflect on; don't waste a turn on the nudge.
func TestExecute_LoopBreakNudge_SkippedWhenRealToolFailure(t *testing.T) {
	provider := &mockProvider{
		name: "mock",
		response: &kyoci.CompletionResponse{
			Content:      thoughtJSON(),
			FinishReason: kyoci.FinishStop,
		},
	}
	cfg := DefaultAgentConfig()
	cfg.EnableThinking = true
	agent := createTestAgent(cfg, provider)

	ls := newTestLoopState()
	ls.recordToolCall("terminal", `{"command":"ls /nonexistent"}`)
	ls.recordToolCall("terminal", `{"command":"ls /nonexistent"}`)
	ls.recordToolCall("terminal", `{"command":"ls /nonexistent"}`)
	// A real tool failed — the nudge would be counterproductive.
	ls.recordFailure(failureEntry{Tool: "terminal", Error: "exit 1: no such file"})

	convo := NewContext()
	convo.AddMessage(kyoci.RoleSystem, "test")

	_, next, err := agent.executeStep(context.Background(), convo, ls)
	if err != nil {
		t.Fatalf("executeStep error: %v", err)
	}
	if next != StateReflect {
		t.Errorf("loop with a real tool failure should escalate to Reflect immediately; got %v", next)
	}
	if ls.loopBreakAttempted != 0 {
		t.Errorf("nudge should not fire when there's a real tool failure; loopBreakAttempted = %d", ls.loopBreakAttempted)
	}
}

// TestExecute_LoopBreakNudge_SkippedWhenNearExhausted covers the case where
// budget is critical (≥75% used with low unique-call ratio). Don't waste a
// turn on the nudge — the model is far gone and needs replanning.
func TestExecute_LoopBreakNudge_SkippedWhenNearExhausted(t *testing.T) {
	provider := &mockProvider{
		name: "mock",
		response: &kyoci.CompletionResponse{
			Content:      thoughtJSON(),
			FinishReason: kyoci.FinishStop,
		},
	}
	cfg := DefaultAgentConfig()
	cfg.EnableThinking = true
	agent := createTestAgent(cfg, provider)

	ls := newTestLoopState()
	ls.toolBudget = 4
	// 3/4 calls, all identical → near-exhausted + looping.
	ls.recordToolCall("terminal", `{"command":"ls"}`)
	ls.recordToolCall("terminal", `{"command":"ls"}`)
	ls.recordToolCall("terminal", `{"command":"ls"}`)

	convo := NewContext()
	convo.AddMessage(kyoci.RoleSystem, "test")

	_, next, err := agent.executeStep(context.Background(), convo, ls)
	if err != nil {
		t.Fatalf("executeStep error: %v", err)
	}
	if next != StateReflect {
		t.Errorf("near-exhausted budget should escalate to Reflect, not nudge; got %v", next)
	}
}

// TestExecute_SearchBudgetNudge_FiresAtThreshold covers the recovery path
// for exploration tasks: the model has done 3 file searches but isn't
// actually looping (different patterns → different hashes). Inject
// SearchBudgetNudge and give it another turn to switch to reading.
func TestExecute_SearchBudgetNudge_FiresAtThreshold(t *testing.T) {
	// Mock returns a final_answer — the nudge should make the model stop
	// searching and present an answer (or, as here, at least not loop).
	finalAnswerJSON := thoughtJSON(func(th *Thought) {
		th.NextAction = NextAction{
			Type:        "final_answer",
			FinalAnswer: "Based on the files I read, the project uses Asia/Jakarta.",
		}
		th.VerificationEvidence = []string{"prisma/schema.prisma sets timezone"}
		th.Confidence = 0.85
	})
	provider := &mockProvider{
		name: "mock",
		response: &kyoci.CompletionResponse{
			Content:      finalAnswerJSON,
			FinishReason: kyoci.FinishStop,
		},
	}
	cfg := DefaultAgentConfig()
	cfg.EnableThinking = true
	agent := createTestAgent(cfg, provider)

	ls := newTestLoopState()
	// 3 distinct file searches — different patterns, so isLooping() stays
	// false. But searchBudgetExceeded() should fire.
	ls.recordToolCall("file", `{"operation":"search","pattern":"timezone"}`)
	ls.recordToolCall("file", `{"operation":"search","pattern":"cron"}`)
	ls.recordToolCall("file", `{"operation":"search","pattern":"schedule"}`)

	if ls.isLooping() {
		t.Fatalf("test setup invalid: distinct searches should not trigger isLooping")
	}
	if !ls.searchBudgetExceeded() {
		t.Fatalf("test setup invalid: 3 searches should trigger searchBudgetExceeded")
	}

	convo := NewContext()
	convo.AddMessage(kyoci.RoleSystem, "test")

	_, next, err := agent.executeStep(context.Background(), convo, ls)
	if err != nil {
		t.Fatalf("executeStep error: %v", err)
	}
	// Must NOT escalate to Reflect — the model isn't looping, it's
	// over-searching. Nudge + LLM call should produce a normal transition.
	if next == StateReflect {
		t.Errorf("search-budget nudge should keep model in Execute/Verify, not escalate to Reflect; " +
			"the model isn't stuck — it's using the wrong tool. Burning a reflection here " +
			"causes budget exhaustion on legitimate exploration tasks.")
	}
	// Verify the nudge was actually injected into the conversation. Without
	// this assertion the test would pass even if executeStep silently
	// fell through to the default ExecutePrompt path.
	msgs := convo.GetMessages()
	foundNudge := false
	for _, m := range msgs {
		if m.Role == kyoci.RoleUser && strings.Contains(m.Content, "STOP calling file search") {
			foundNudge = true
			break
		}
	}
	if !foundNudge {
		t.Errorf("SearchBudgetNudge not injected after %d searches. messages seen: %d",
			maxSearchesPerTask, len(msgs))
	}
}

func TestExecute_SearchBudgetNudge_DoesNotFireBelowThreshold(t *testing.T) {
	// 2 searches (below threshold) → ExecutePrompt path, NOT SearchBudgetNudge.
	provider := &mockProvider{
		name: "mock",
		response: &kyoci.CompletionResponse{
			Content:      thoughtJSON(),
			FinishReason: kyoci.FinishStop,
		},
	}
	cfg := DefaultAgentConfig()
	cfg.EnableThinking = true
	agent := createTestAgent(cfg, provider)

	ls := newTestLoopState()
	ls.recordToolCall("file", `{"operation":"search","pattern":"timezone"}`)
	ls.recordToolCall("file", `{"operation":"search","pattern":"cron"}`)
	if ls.searchBudgetExceeded() {
		t.Fatalf("test setup invalid: 2 searches should not trigger searchBudgetExceeded")
	}

	convo := NewContext()
	convo.AddMessage(kyoci.RoleSystem, "test")

	_, _, err := agent.executeStep(context.Background(), convo, ls)
	if err != nil {
		t.Fatalf("executeStep error below threshold: %v", err)
	}
	// Nudge must NOT have been injected.
	msgs := convo.GetMessages()
	for _, m := range msgs {
		if m.Role == kyoci.RoleUser && strings.Contains(m.Content, "STOP calling file search") {
			t.Errorf("SearchBudgetNudge must not fire below threshold; found it in messages")
		}
	}
}

func TestExecute_TransitionsToDone_OnBudgetExhausted(t *testing.T) {
	// Budget already exhausted — executeStep should go to Done (honest termination).
	provider := &mockProvider{
		name: "mock",
		response: &kyoci.CompletionResponse{
			Content:      thoughtJSON(),
			FinishReason: kyoci.FinishStop,
		},
	}
	cfg := DefaultAgentConfig()
	cfg.EnableThinking = true
	agent := createTestAgent(cfg, provider)

	ls := newTestLoopState()
	ls.toolBudget = 1
	ls.toolCallsUsed = 1 // already exhausted

	convo := NewContext()
	convo.AddMessage(kyoci.RoleSystem, "test")

	_, next, err := agent.executeStep(context.Background(), convo, ls)
	if err != nil {
		t.Fatalf("executeStep error: %v", err)
	}
	if next != StateDone {
		t.Errorf("exhausted budget should transition to Done; got %v", next)
	}
}

// =============================================================================
// Step 8: Verify state tests
// =============================================================================
//
// verifyStep is the gate before DONE. It accepts the model's claim only if
// concrete verification_evidence is present OR no tools were called (pure
// knowledge question). Otherwise it sends the model to Reflect to produce
// real evidence before claiming success.

func TestVerify_AcceptsEvidence(t *testing.T) {
	cfg := DefaultAgentConfig()
	cfg.EnableThinking = true
	agent := createTestAgent(cfg, &mockProvider{name: "mock"})

	ls := newTestLoopState()
	ls.thoughts = append(ls.thoughts, Thought{
		NextAction:           NextAction{Type: "final_answer", FinalAnswer: "Done."},
		VerificationEvidence: []string{"file read returned: hi"},
	})
	ls.toolCallsUsed = 2 // tools WERE called, but evidence is present

	convo := NewContext()
	_, next, err := agent.verifyStep(context.Background(), convo, ls)
	if err != nil {
		t.Fatalf("verifyStep error: %v", err)
	}
	if next != StateDone {
		t.Errorf("evidence present should accept to Done; got %v", next)
	}
}

func TestVerify_RejectsWithoutEvidenceAfterActualFailure(t *testing.T) {
	// A real tool failed and the model claims done with no evidence — this
	// is the scenario that genuinely warrants reflection.
	cfg := DefaultAgentConfig()
	cfg.EnableThinking = true
	agent := createTestAgent(cfg, &mockProvider{name: "mock"})

	ls := newTestLoopState()
	ls.thoughts = append(ls.thoughts, Thought{
		NextAction:           NextAction{Type: "final_answer", FinalAnswer: "Done."},
		VerificationEvidence: nil, // missing!
	})
	ls.toolCallsUsed = 2 // tools were called
	ls.recordFailure(failureEntry{Tool: "file", Error: "permission denied"})

	convo := NewContext()
	_, next, err := agent.verifyStep(context.Background(), convo, ls)
	if err != nil {
		t.Fatalf("verifyStep error: %v", err)
	}
	if next != StateReflect {
		t.Errorf("missing evidence after a real tool failure should go to Reflect; got %v", next)
	}
	if len(ls.failureHistory) == 0 {
		t.Error("expected a failure entry explaining the verification rejection")
	}
}

func TestVerify_AcceptsNoToolKnowledgeQuestion(t *testing.T) {
	// Pure knowledge question — no tools called, no evidence needed.
	cfg := DefaultAgentConfig()
	cfg.EnableThinking = true
	agent := createTestAgent(cfg, &mockProvider{name: "mock"})

	ls := newTestLoopState()
	ls.thoughts = append(ls.thoughts, Thought{
		NextAction:           NextAction{Type: "final_answer", FinalAnswer: "Paris is the capital of France."},
		VerificationEvidence: nil,
	})
	ls.toolCallsUsed = 0 // no tools called

	convo := NewContext()
	_, next, err := agent.verifyStep(context.Background(), convo, ls)
	if err != nil {
		t.Fatalf("verifyStep error: %v", err)
	}
	if next != StateDone {
		t.Errorf("pure-knowledge answer with no tools should accept to Done; got %v", next)
	}
}

func TestVerify_HandlesNoThoughtsGracefully(t *testing.T) {
	// Defensive: shouldn't happen, but verifyStep must not panic.
	cfg := DefaultAgentConfig()
	cfg.EnableThinking = true
	agent := createTestAgent(cfg, &mockProvider{name: "mock"})

	ls := newTestLoopState() // no thoughts
	convo := NewContext()
	_, next, err := agent.verifyStep(context.Background(), convo, ls)
	if err != nil {
		t.Fatalf("verifyStep error: %v", err)
	}
	if next != StateDone {
		t.Errorf("empty thoughts should default to Done; got %v", next)
	}
}

// =============================================================================
// Verify leniency for small models (Change 3)
// =============================================================================
//
// Small models (8B) routinely skip the optional verification_evidence JSON
// field even when tools succeeded. Forcing Reflect in that case burns the
// reflection budget without diagnosing a real problem — the tool results in
// the conversation ARE implicit evidence. verifyStep should only force Reflect
// when a tool ACTUALLY failed (a real tool, not verify/replan/unknown).

func TestHasToolExecutionFailure_OnlyVerifyFailures(t *testing.T) {
	ls := newTestLoopState()
	ls.recordFailure(failureEntry{Tool: "verify", Error: "no evidence"})
	ls.recordFailure(failureEntry{Tool: "replan", Error: "wrong approach"})
	ls.recordFailure(failureEntry{Tool: "unknown", Error: "weird action"})
	if ls.hasToolExecutionFailure() {
		t.Error("verify/replan/unknown failures are not tool-execution failures; expected false")
	}
}

func TestHasToolExecutionFailure_RealToolFailure(t *testing.T) {
	ls := newTestLoopState()
	ls.recordFailure(failureEntry{Tool: "verify", Error: "no evidence"})
	ls.recordFailure(failureEntry{Tool: "terminal", Error: "exit 1"})
	if !ls.hasToolExecutionFailure() {
		t.Error("a terminal failure is a real tool-execution failure; expected true")
	}
}

func TestHasToolExecutionFailure_NoFailures(t *testing.T) {
	ls := newTestLoopState()
	if ls.hasToolExecutionFailure() {
		t.Error("empty failureHistory should return false")
	}
}

func TestVerifyStep_AcceptsDoneWhenToolsSucceededNoEvidence(t *testing.T) {
	// Tools called and succeeded (no failures), but the small model skipped
	// the optional verification_evidence field. The tool results in the
	// conversation are implicit evidence — accept Done instead of burning
	// the reflection budget.
	cfg := DefaultAgentConfig()
	cfg.EnableThinking = true
	agent := createTestAgent(cfg, &mockProvider{name: "mock"})

	ls := newTestLoopState()
	ls.thoughts = append(ls.thoughts, Thought{
		NextAction:           NextAction{Type: "final_answer", FinalAnswer: "Done."},
		VerificationEvidence: nil, // small model skipped this field
	})
	ls.toolCallsUsed = 2 // tools were called and succeeded

	convo := NewContext()
	_, next, err := agent.verifyStep(context.Background(), convo, ls)
	if err != nil {
		t.Fatalf("verifyStep error: %v", err)
	}
	if next != StateDone {
		t.Errorf("tools succeeded without evidence should accept Done (small models skip the field); got %v", next)
	}
}

func TestVerifyStep_RejectsDoneWhenToolActuallyFailed(t *testing.T) {
	// A real tool failed and the model claims done with no evidence — this
	// genuinely warrants reflection to diagnose the failure.
	cfg := DefaultAgentConfig()
	cfg.EnableThinking = true
	agent := createTestAgent(cfg, &mockProvider{name: "mock"})

	ls := newTestLoopState()
	ls.thoughts = append(ls.thoughts, Thought{
		NextAction:           NextAction{Type: "final_answer", FinalAnswer: "Done."},
		VerificationEvidence: nil,
	})
	ls.toolCallsUsed = 2
	ls.recordFailure(failureEntry{Tool: "terminal", Error: "command not found"})

	convo := NewContext()
	_, next, err := agent.verifyStep(context.Background(), convo, ls)
	if err != nil {
		t.Fatalf("verifyStep error: %v", err)
	}
	if next != StateReflect {
		t.Errorf("actual tool failure + no evidence should go to Reflect; got %v", next)
	}
}

// =============================================================================
// Step 9: Reflect state tests
// =============================================================================
//
// reflectStep forces a structured root-cause analysis after failure. It either
// produces a new plan (→ Plan, counting against the replan cap), retries with
// a different approach (→ Execute), or terminates honestly when caps are hit.

func TestReflect_Replans_TransitionsToPlan(t *testing.T) {
	provider := &mockProvider{
		name: "mock",
		response: &kyoci.CompletionResponse{
			Content: thoughtJSON(func(th *Thought) {
				th.RootCause = "wrong file path"
				th.NextAction = NextAction{Type: "replan"}
				th.Plan = []PlanStep{{Description: "try different path", Tool: "file", Status: "pending"}}
			}),
			FinishReason: kyoci.FinishStop,
		},
	}
	cfg := DefaultAgentConfig()
	cfg.EnableThinking = true
	agent := createTestAgent(cfg, provider)

	ls := newTestLoopState()
	ls.recordFailure(failureEntry{Iteration: 1, Tool: "file", Error: "not found"})

	convo := NewContext()
	convo.AddMessage(kyoci.RoleSystem, "test")

	_, next, err := agent.reflectStep(context.Background(), convo, ls)
	if err != nil {
		t.Fatalf("reflectStep error: %v", err)
	}
	if next != StatePlan {
		t.Errorf("replan action should transition to Plan; got %v", next)
	}
	if ls.replansUsed != 1 {
		t.Errorf("replansUsed = %d, want 1", ls.replansUsed)
	}
	if ls.reflectionsUsed != 1 {
		t.Errorf("reflectionsUsed = %d, want 1", ls.reflectionsUsed)
	}
}

func TestReflect_RetriesDifferentApproach_TransitionsToExecute(t *testing.T) {
	// The mock terminal tool succeeds, so reflectStep executes it via runToolCall
	// and transitions to Execute.
	tool := &mockTool{
		name:   "terminal",
		result: "total 0",
	}
	provider := &mockProvider{
		name: "mock",
		response: &kyoci.CompletionResponse{
			Content: thoughtJSON(func(th *Thought) {
				th.RootCause = "command failed due to permissions"
				th.NextAction = NextAction{
					Type: "tool_call",
					ToolCall: &kyoci.ToolCall{
						ID:        "1",
						Name:      "terminal",
						Arguments: `{"command":"sudo ls"}`,
					},
				}
			}),
			FinishReason: kyoci.FinishStop,
		},
	}
	cfg := DefaultAgentConfig()
	cfg.EnableThinking = true
	agent := createTestAgent(cfg, provider)
	agent.tools.Register(tool)

	ls := newTestLoopState()
	ls.recordFailure(failureEntry{Iteration: 1, Tool: "terminal", Error: "permission denied"})

	convo := NewContext()
	convo.AddMessage(kyoci.RoleSystem, "test")

	_, next, err := agent.reflectStep(context.Background(), convo, ls)
	if err != nil {
		t.Fatalf("reflectStep error: %v", err)
	}
	if next != StateExecute {
		t.Errorf("different-approach tool_call should transition to Execute; got %v", next)
	}
	if ls.reflectionsUsed != 1 {
		t.Errorf("reflectionsUsed = %d, want 1", ls.reflectionsUsed)
	}
	// Note: replansUsed should NOT be incremented for tool_call retry.
	if ls.replansUsed != 0 {
		t.Errorf("replansUsed = %d, want 0 (tool_call is not a replan)", ls.replansUsed)
	}
	// The retry tool call must be executed (not just proposed), so it counts
	// against the tool-call budget.
	if ls.toolCallsUsed != 1 {
		t.Errorf("toolCallsUsed = %d, want 1 (reflect should execute the retry)", ls.toolCallsUsed)
	}
}

func TestReflect_CapsAtMaxReflections_TerminatesHonestly(t *testing.T) {
	// Already used all reflections — reflectStep should go straight to Done
	// without calling the LLM.
	provider := &mockProvider{
		name: "mock",
		response: &kyoci.CompletionResponse{
			Content: thoughtJSON(),
		},
	}
	cfg := DefaultAgentConfig()
	cfg.EnableThinking = true
	agent := createTestAgent(cfg, provider)

	ls := newTestLoopState()
	ls.reflectionsUsed = ls.maxReflections // at cap
	ls.recordFailure(failureEntry{Iteration: 1, Tool: "file", Error: "fail"})

	convo := NewContext()
	convo.AddMessage(kyoci.RoleSystem, "test")

	_, next, err := agent.reflectStep(context.Background(), convo, ls)
	if err != nil {
		t.Fatalf("reflectStep error: %v", err)
	}
	if next != StateDone {
		t.Errorf("at reflection cap should terminate to Done; got %v", next)
	}
	if ls.finalContent == "" {
		t.Error("expected honest termination message in finalContent")
	}
}

func TestReflect_ReplanCapExceeded_TerminatesHonestly(t *testing.T) {
	// Model wants to replan but replan cap is hit — should terminate.
	provider := &mockProvider{
		name: "mock",
		response: &kyoci.CompletionResponse{
			Content: thoughtJSON(func(th *Thought) {
				th.RootCause = "still wrong"
				th.NextAction = NextAction{Type: "replan"}
			}),
		},
	}
	cfg := DefaultAgentConfig()
	cfg.EnableThinking = true
	agent := createTestAgent(cfg, provider)

	ls := newTestLoopState()
	ls.replansUsed = ls.maxReplans // at replan cap
	ls.recordFailure(failureEntry{Iteration: 1, Tool: "file", Error: "fail"})

	convo := NewContext()
	convo.AddMessage(kyoci.RoleSystem, "test")

	_, next, err := agent.reflectStep(context.Background(), convo, ls)
	if err != nil {
		t.Fatalf("reflectStep error: %v", err)
	}
	if next != StateDone {
		t.Errorf("replan cap exceeded should terminate to Done; got %v", next)
	}
}

func TestReflect_ParseFailure_ReturnsError(t *testing.T) {
	provider := &mockProvider{
		name: "mock",
		response: &kyoci.CompletionResponse{
			Content:      "garbage",
			FinishReason: kyoci.FinishStop,
		},
	}
	cfg := DefaultAgentConfig()
	cfg.EnableThinking = true
	agent := createTestAgent(cfg, provider)

	ls := newTestLoopState()
	ls.recordFailure(failureEntry{Iteration: 1, Tool: "file", Error: "fail"})

	convo := NewContext()
	convo.AddMessage(kyoci.RoleSystem, "test")

	_, _, err := agent.reflectStep(context.Background(), convo, ls)
	if err == nil {
		t.Fatal("expected error on parse failure, got nil")
	}
}

// =============================================================================
// Step 10: Execute() wiring + sanitizeContent tests
// =============================================================================
//
// Step 10 wires the per-state step methods into a driver (executeWithThinking)
// and adds a dispatch branch in Execute(). These tests verify:
//   - sanitizeContent strips leaked Thought JSON lines (defense in depth)
//   - Execute() with EnableThinking=true routes through the thinking loop and
//     returns only the parsed FinalAnswer (not raw JSON)

// TestSanitizeContent_StripsThoughtJSON verifies that a single-line Thought
// JSON object is stripped from final content. The thinking driver parses the
// Thought and surfaces only FinalAnswer, but this is defense in depth for any
// code path that might leak raw response.Content.
func TestSanitizeContent_StripsThoughtJSON(t *testing.T) {
	input := `Here is some intro prose.
{"task_understanding":"test","next_action":{"type":"final_answer","final_answer":"done"},"confidence":0.9}
Here is some closing prose.`
	out := sanitizeContent(input)
	if strings.Contains(out, "task_understanding") {
		t.Errorf("sanitize should strip thought JSON line; got: %s", out)
	}
	if !strings.Contains(out, "Here is some intro prose") {
		t.Errorf("sanitize should preserve non-JSON prose; got: %s", out)
	}
	if !strings.Contains(out, "closing prose") {
		t.Errorf("sanitize should preserve trailing prose; got: %s", out)
	}
}

// TestSanitizeContent_PreservesOrdinaryJSON verifies that JSON without the
// Thought schema markers is NOT stripped — we only catch Thought-shaped JSON.
func TestSanitizeContent_PreservesOrdinaryJSON(t *testing.T) {
	input := `Result: {"items": [1, 2, 3], "count": 3}`
	out := sanitizeContent(input)
	if !strings.Contains(out, `"items"`) {
		t.Errorf("sanitize should preserve ordinary JSON; got: %s", out)
	}
}

// TestExecute_EnableThinking_DispatchesToThinkingLoop is the wiring smoke
// test. The thinking loop parses the Thought and returns only FinalAnswer. The
// legacy loop would have surfaced response.Content (the raw JSON) directly. So
// if the result contains the clean FinalAnswer and NOT the raw JSON marker, we
// know the thinking path ran.
func TestExecute_EnableThinking_DispatchesToThinkingLoop(t *testing.T) {
	finalAnswer := "The capital of France is Paris."
	provider := &mockProvider{
		name: "mock",
		response: &kyoci.CompletionResponse{
			Content: thoughtJSON(func(th *Thought) {
				th.Confidence = 0.9
				th.NextAction = NextAction{Type: "final_answer", FinalAnswer: finalAnswer}
			}),
			FinishReason: kyoci.FinishStop,
		},
	}
	cfg := DefaultAgentConfig()
	cfg.EnableThinking = true
	cfg.EnableSkills = false // bypass skill fast-path so we exercise the thinking loop
	agent := createTestAgent(cfg, provider)

	result, err := agent.Execute(context.Background(), "What is the capital of France?")
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result == nil {
		t.Fatal("Execute returned nil result")
	}
	if !strings.Contains(result.Content, finalAnswer) {
		t.Errorf("expected final answer %q in content; got %q", finalAnswer, result.Content)
	}
	// The raw Thought JSON must NOT leak — that proves the thinking path ran
	// rather than the legacy loop surfacing response.Content verbatim.
	if strings.Contains(result.Content, "task_understanding") {
		t.Errorf("content should not contain raw thought JSON; got %q", result.Content)
	}
}

// TestExecute_EnableThinkingFalse_RunsLegacyLoop confirms the dispatch branch
// doesn't break the existing ReAct loop when EnableThinking is false.
func TestExecute_EnableThinkingFalse_RunsLegacyLoop(t *testing.T) {
	// Legacy loop surfaces response.Content directly. A plain-text response
	// (no tool calls, no thought JSON) should pass through unchanged.
	provider := &mockProvider{
		name: "mock",
		response: &kyoci.CompletionResponse{
			Content:       "Legacy loop was here.",
			FinishReason:  kyoci.FinishStop,
		},
	}
	cfg := DefaultAgentConfig()
	cfg.EnableThinking = false
	cfg.EnableSkills = false
	agent := createTestAgent(cfg, provider)

	result, err := agent.Execute(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !strings.Contains(result.Content, "Legacy loop was here") {
		t.Errorf("legacy loop output lost; got %q", result.Content)
	}
}

// =============================================================================
// Step 11: Full end-to-end integration tests
// =============================================================================
//
// These tests drive the full Execute() → executeWithThinking path with a
// queuedMockProvider that scripts a sequence of LLM responses across multiple
// states. They verify that the per-state step methods compose correctly into
// the state machine documented in the plan.

// thoughtResponse wraps thoughtJSON in a CompletionResponse so the
// queuedMockProvider can serve it.
func thoughtResponse(overrides ...func(*Thought)) *kyoci.CompletionResponse {
	return &kyoci.CompletionResponse{
		Content:      thoughtJSON(overrides...),
		FinishReason: kyoci.FinishStop,
	}
}

// TestFullLoop_SimpleTask_EndToEnd drives the happy path:
// Assess (fast) → Execute (tool) → Execute (final_answer + evidence) → Verify → Done.
func TestFullLoop_SimpleTask_EndToEnd(t *testing.T) {
	// The file mock tool just returns "ok" for any invocation. It exists so
	// runToolCall has something to dispatch to and record in the budget.
	tool := &mockTool{
		name:        "file",
		description: "test file tool",
		params:      []kyoci.ToolParameter{{Name: "operation", Type: "string", Required: true}},
		result:      "ok",
	}

	provider := &queuedMockProvider{
		name: "mock",
		responses: []*kyoci.CompletionResponse{
			// Call 1 — Assess: simple task, confident tool_call → fast path → Execute.
			thoughtResponse(func(th *Thought) {
				th.TaskUnderstanding = "Create hello.txt with content hi"
				th.Confidence = 0.9
				th.NextAction = NextAction{
					Type: "tool_call",
					ToolCall: &kyoci.ToolCall{
						ID:        "1",
						Name:      "file",
						Arguments: `{"operation":"write","path":"hello.txt","content":"hi"}`,
					},
				}
			}),
			// Call 2 — Execute: same tool call (runs the tool).
			thoughtResponse(func(th *Thought) {
				th.NextAction = NextAction{
					Type: "tool_call",
					ToolCall: &kyoci.ToolCall{
						ID:        "2",
						Name:      "file",
						Arguments: `{"operation":"write","path":"hello.txt","content":"hi"}`,
					},
				}
			}),
			// Call 3 — Execute: final answer with verification evidence.
			thoughtResponse(func(th *Thought) {
				th.NextAction = NextAction{
					Type:        "final_answer",
					FinalAnswer: "Created hello.txt with content hi.",
				}
				th.VerificationEvidence = []string{"file write returned ok"}
			}),
		},
	}

	cfg := DefaultAgentConfig()
	cfg.EnableThinking = true
	cfg.EnableSkills = false
	agent := createTestAgent(cfg, provider)
	agent.tools.Register(tool)

	result, err := agent.Execute(context.Background(), "create hello.txt with content hi")
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result == nil {
		t.Fatal("nil result")
	}
	if !strings.Contains(result.Content, "Created hello.txt") {
		t.Errorf("expected final answer in content; got %q", result.Content)
	}
	if result.ToolCallsMade != 1 {
		t.Errorf("ToolCallsMade = %d, want 1", result.ToolCallsMade)
	}
	if provider.Calls() != 3 {
		t.Errorf("LLM calls = %d, want 3 (Assess + Execute + Execute)", provider.Calls())
	}
	if strings.Contains(result.Content, "task_understanding") {
		t.Errorf("raw thought JSON leaked into content: %q", result.Content)
	}
}

// TestFullLoop_FailingTask_RecoversViaReflect drives the recovery path:
// Assess → Execute (tool fails) → Reflect (root_cause + retry) → Execute (succeeds)
// → Execute (final_answer + evidence) → Verify → Done.
func TestFullLoop_FailingTask_RecoversViaReflect(t *testing.T) {
	// The tool fails on its first invocation and succeeds afterwards, so the
	// loop is forced through Reflect to recover.
	toolInvocations := 0
	tool := &mockTool{
		name:        "file",
		description: "test file tool that fails once",
		params:      []kyoci.ToolParameter{{Name: "operation", Type: "string", Required: true}},
		executeFunc: func(ctx context.Context, params map[string]interface{}) (string, error) {
			toolInvocations++
			if toolInvocations == 1 {
				return "", errors.New("permission denied")
			}
			return "ok", nil
		},
	}

	provider := &queuedMockProvider{
		name: "mock",
		responses: []*kyoci.CompletionResponse{
			// Call 1 — Assess: confident tool_call → fast path → Execute.
			thoughtResponse(func(th *Thought) {
				th.Confidence = 0.9
				th.NextAction = NextAction{
					Type: "tool_call",
					ToolCall: &kyoci.ToolCall{
						ID:        "1",
						Name:      "file",
						Arguments: `{"operation":"write","path":"hello.txt","content":"hi"}`,
					},
				}
			}),
			// Call 2 — Execute: same tool call → tool FAILS → Reflect.
			thoughtResponse(func(th *Thought) {
				th.NextAction = NextAction{
					Type: "tool_call",
					ToolCall: &kyoci.ToolCall{
						ID:        "2",
						Name:      "file",
						Arguments: `{"operation":"write","path":"hello.txt","content":"hi"}`,
					},
				}
			}),
			// Call 3 — Reflect: root_cause + different-approach tool_call → Execute.
			thoughtResponse(func(th *Thought) {
				th.RootCause = "permission denied on direct write"
				th.NextAction = NextAction{
					Type: "tool_call",
					ToolCall: &kyoci.ToolCall{
						ID:        "3",
						Name:      "file",
						Arguments: `{"operation":"sudo_write","path":"hello.txt","content":"hi"}`,
					},
				}
			}),
			// Call 4 — Execute: tool succeeds (2nd invocation) → stay in Execute.
			thoughtResponse(func(th *Thought) {
				th.NextAction = NextAction{
					Type: "final_answer",
					FinalAnswer: "Created hello.txt with content hi after recovering from a permission error.",
				}
				th.VerificationEvidence = []string{"file write returned ok on retry"}
			}),
		},
	}

	cfg := DefaultAgentConfig()
	cfg.EnableThinking = true
	cfg.EnableSkills = false
	agent := createTestAgent(cfg, provider)
	agent.tools.Register(tool)

	result, err := agent.Execute(context.Background(), "create hello.txt with content hi")
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result == nil {
		t.Fatal("nil result")
	}
	if !strings.Contains(result.Content, "Created hello.txt") {
		t.Errorf("expected final answer in content; got %q", result.Content)
	}
	if result.ToolCallsMade != 2 {
		t.Errorf("ToolCallsMade = %d, want 2 (failed write + retry)", result.ToolCallsMade)
	}
	// 4 LLM calls: Assess + Execute(fail) + Reflect + Execute(final_answer).
	if provider.Calls() != 4 {
		t.Errorf("LLM calls = %d, want 4", provider.Calls())
	}
}

// TestFullLoop_LoopDetection_EscalatesToReflect verifies the loop-detection
// pre-flight: when the model would repeat the same failing call, Execute
// escalates to Reflect without making the duplicate LLM call.
func TestFullLoop_LoopDetection_EscalatesToReflect(t *testing.T) {
	tool := &mockTool{
		name:        "terminal",
		description: "test terminal",
		params:      []kyoci.ToolParameter{{Name: "command", Type: "string", Required: true}},
		err:         errors.New("exit 1"),
	}

	provider := &queuedMockProvider{
		name: "mock",
		responses: []*kyoci.CompletionResponse{
			// Call 1 — Assess: confident tool_call → fast path → Execute.
			thoughtResponse(func(th *Thought) {
				th.Confidence = 0.9
				th.NextAction = NextAction{
					Type: "tool_call",
					ToolCall: &kyoci.ToolCall{
						ID:        "1",
						Name:      "terminal",
						Arguments: `{"command":"failing-cmd"}`,
					},
				}
			}),
			// Call 2 — Execute: same tool call → tool fails → Reflect.
			thoughtResponse(func(th *Thought) {
				th.NextAction = NextAction{
					Type: "tool_call",
					ToolCall: &kyoci.ToolCall{
						ID:        "2",
						Name:      "terminal",
						Arguments: `{"command":"failing-cmd"}`,
					},
				}
			}),
			// Call 3 — Reflect: model still wants the same call; but we terminate
			// via the reflection cap path. Provide a final_answer so the loop
			// completes gracefully on subsequent reflection escalations.
			thoughtResponse(func(th *Thought) {
				th.RootCause = "command not found"
				th.NextAction = NextAction{
					Type:        "final_answer",
					FinalAnswer: "Could not complete: the command is unavailable.",
				}
			}),
		},
	}

	cfg := DefaultAgentConfig()
	cfg.EnableThinking = true
	cfg.EnableSkills = false
	agent := createTestAgent(cfg, provider)
	agent.tools.Register(tool)

	result, err := agent.Execute(context.Background(), "run failing-cmd")
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	// The loop should terminate (not hang) even though the tool keeps failing.
	if result == nil {
		t.Fatal("nil result")
	}
	if result.Content == "" {
		t.Error("expected non-empty content from recovery path")
	}
}

// =============================================================================
// PR #2: Config wiring tests
// =============================================================================

// TestDefaultAgentConfig_ThinkingBudgetDefaults verifies that DefaultAgentConfig
// populates the thinking budget fields with the same values as config.ThinkingConfig
// defaults, so the thinking system works correctly even without YAML config.
func TestDefaultAgentConfig_ThinkingBudgetDefaults(t *testing.T) {
	cfg := DefaultAgentConfig()

	if cfg.ThinkingToolBudget != 15 {
		t.Errorf("ThinkingToolBudget = %d, want 15", cfg.ThinkingToolBudget)
	}
	if cfg.ThinkingMaxReflections != 3 {
		t.Errorf("ThinkingMaxReflections = %d, want 3", cfg.ThinkingMaxReflections)
	}
	if cfg.ThinkingMaxReplans != 2 {
		t.Errorf("ThinkingMaxReplans = %d, want 2", cfg.ThinkingMaxReplans)
	}
	if cfg.ThinkingConfidenceThreshold != 0.7 {
		t.Errorf("ThinkingConfidenceThreshold = %f, want 0.7", cfg.ThinkingConfidenceThreshold)
	}
	if !cfg.ThinkingFewShot {
		t.Error("ThinkingFewShot = false, want true")
	}
}

// TestNewLoopState_UsesAgentConfig verifies that newLoopState (as a method on
// *Agent) reads thinking budget values from AgentConfig rather than using
// hardcoded constants.
func TestNewLoopState_UsesAgentConfig(t *testing.T) {
	provider := &mockProvider{}
	cfg := DefaultAgentConfig()
	cfg.ThinkingToolBudget = 25
	cfg.ThinkingMaxReflections = 5
	cfg.ThinkingMaxReplans = 3
	a := createTestAgent(cfg, provider)

	ls := a.newLoopState()

	if ls.toolBudget != 25 {
		t.Errorf("toolBudget = %d, want 25 (from config)", ls.toolBudget)
	}
	if ls.maxReflections != 5 {
		t.Errorf("maxReflections = %d, want 5 (from config)", ls.maxReflections)
	}
	if ls.maxReplans != 3 {
		t.Errorf("maxReplans = %d, want 3 (from config)", ls.maxReplans)
	}
}

// TestNewLoopState_FallsBackToDefaultsOnZero verifies that when AgentConfig
// thinking fields are zero, newLoopState falls back to the hardcoded constants.
// This matters for callers that construct AgentConfig without using
// DefaultAgentConfig() (e.g. legacy code paths).
func TestNewLoopState_FallsBackToDefaultsOnZero(t *testing.T) {
	provider := &mockProvider{}
	cfg := AgentConfig{} // all zero
	a := createTestAgent(cfg, provider)

	ls := a.newLoopState()

	if ls.toolBudget != defaultThoughtToolBudget {
		t.Errorf("toolBudget = %d, want default %d", ls.toolBudget, defaultThoughtToolBudget)
	}
	if ls.maxReflections != defaultThoughtMaxReflections {
		t.Errorf("maxReflections = %d, want default %d", ls.maxReflections, defaultThoughtMaxReflections)
	}
	if ls.maxReplans != defaultThoughtMaxReplans {
		t.Errorf("maxReplans = %d, want default %d", ls.maxReplans, defaultThoughtMaxReplans)
	}
}

// TestAssess_CustomConfidenceThreshold verifies that assessStep uses the
// ThinkingConfidenceThreshold from AgentConfig, not the hardcoded constant.
// With confidence 0.9 and threshold 0.95, a normally-fast-path task should
// escalate to Plan.
func TestAssess_CustomConfidenceThreshold(t *testing.T) {
	provider := &mockProvider{
		name: "mock",
		response: &kyoci.CompletionResponse{
			Content:      thoughtJSON(), // confidence = 0.9
			FinishReason: kyoci.FinishStop,
		},
	}
	cfg := DefaultAgentConfig()
	cfg.EnableThinking = true
	cfg.ThinkingConfidenceThreshold = 0.95 // higher than the model's 0.9 confidence
	a := createTestAgent(cfg, provider)

	ls := newTestLoopState()
	convo := NewContext()
	convo.AddMessage(kyoci.RoleSystem, "test")
	convo.AddMessage(kyoci.RoleUser, "create hello.txt")

	_, next, err := a.assessStep(context.Background(), convo, ls, "create hello.txt")
	if err != nil {
		t.Fatalf("assessStep error: %v", err)
	}
	// With threshold 0.95 and confidence 0.9, should escalate to Plan.
	if next != StatePlan {
		t.Errorf("with confidence 0.9 < threshold 0.95, expected StatePlan; got %v", next)
	}
}

// capturingProvider wraps mockProvider and records the concatenated system
// prompt from the first request, so tests can verify prompt composition.
type capturingProvider struct {
	mockProvider
	capturedSystem string
	got            bool
}

func (c *capturingProvider) Complete(ctx context.Context, req kyoci.CompletionRequest) (*kyoci.CompletionResponse, error) {
	if !c.got {
		for _, msg := range req.Messages {
			if msg.Role == kyoci.RoleSystem {
				c.capturedSystem += msg.Content + "\n"
			}
		}
		c.got = true
	}
	return c.mockProvider.Complete(ctx, req)
}

// TestExecuteWithThinking_FewShotToggle verifies that executeWithThinking
// includes FewShotExample when ThinkingFewShot=true and omits it when false.
func TestExecuteWithThinking_FewShotToggle(t *testing.T) {
	// Response that gives a final answer immediately (Assess fast path).
	finalAnswerJSON := thoughtJSON(func(th *Thought) {
		th.NextAction = NextAction{
			Type:        "final_answer",
			FinalAnswer: "done",
		}
		th.VerificationEvidence = []string{"N/A — pure knowledge question"}
		th.Confidence = 0.95
	})

	t.Run("few_shot_disabled_omits_example", func(t *testing.T) {
		cap := &capturingProvider{
			mockProvider: mockProvider{
				name: "mock",
				response: &kyoci.CompletionResponse{
					Content:      finalAnswerJSON,
					FinishReason: kyoci.FinishStop,
				},
			},
		}
		cfg := DefaultAgentConfig()
		cfg.EnableThinking = true
		cfg.ThinkingFewShot = false
		a := createTestAgent(cfg, cap)

		_, _ = a.Execute(context.Background(), "what is 2+2")

		if strings.Contains(cap.capturedSystem, "EXAMPLE — simple task") {
			t.Error("system prompt contains FewShotExample even though ThinkingFewShot=false")
		}
	})

	t.Run("few_shot_enabled_includes_example", func(t *testing.T) {
		cap := &capturingProvider{
			mockProvider: mockProvider{
				name: "mock",
				response: &kyoci.CompletionResponse{
					Content:      finalAnswerJSON,
					FinishReason: kyoci.FinishStop,
				},
			},
		}
		cfg := DefaultAgentConfig()
		cfg.EnableThinking = true
		cfg.ThinkingFewShot = true
		a := createTestAgent(cfg, cap)

		_, _ = a.Execute(context.Background(), "what is 2+2")

		if !strings.Contains(cap.capturedSystem, "EXAMPLE — simple task") {
			t.Error("system prompt missing FewShotExample even though ThinkingFewShot=true")
		}
	})
}

// TestExecuteWithThinking_InjectsSkillsBeforeProtocol verifies that injected
// skill markdown appears BEFORE the JSON protocol (BaseSystemPrompt) in the
// system prompt. Small models (8B) rely on recency — the protocol must be the
// last strong signal before the task, otherwise the model drifts off-format
// and burns the reflection budget. See plan: "thinking budget exhaustion."
func TestExecuteWithThinking_InjectsSkillsBeforeProtocol(t *testing.T) {
	const skillMarker = "MARKER_SKILL_TEXT_HERE"

	finalAnswerJSON := thoughtJSON(func(th *Thought) {
		th.NextAction = NextAction{
			Type:        "final_answer",
			FinalAnswer: "done",
		}
		th.VerificationEvidence = []string{"N/A"}
		th.Confidence = 0.95
	})

	cap := &capturingProvider{
		mockProvider: mockProvider{
			name: "mock",
			response: &kyoci.CompletionResponse{
				Content:      finalAnswerJSON,
				FinishReason: kyoci.FinishStop,
			},
		},
	}
	cfg := DefaultAgentConfig()
	cfg.EnableThinking = true
	cfg.ThinkingFewShot = true
	a := createTestAgent(cfg, cap)
	// Inject a skill marker via the ContextInjector hook.
	a.SetContextInjector(staticInjector(skillMarker))

	_, _ = a.Execute(context.Background(), "do a thing that matches a skill")

	system := cap.capturedSystem
	skillIdx := strings.Index(system, skillMarker)
	// "OUTPUT PROTOCOL" is the load-bearing section of BaseSystemPrompt.
	protoIdx := strings.Index(system, "OUTPUT PROTOCOL")
	if skillIdx < 0 {
		t.Fatal("injected skill marker missing from system prompt")
	}
	if protoIdx < 0 {
		t.Fatal("OUTPUT PROTOCOL section missing from system prompt")
	}
	if skillIdx > protoIdx {
		t.Errorf("skill markdown (idx %d) must appear BEFORE OUTPUT PROTOCOL (idx %d); "+
			"injecting after the protocol dilutes it for small models and causes "+
			"reflection-loop exhaustion", skillIdx, protoIdx)
	}
}

// staticInjector returns a ContextInjector that always emits the given text.
type staticInjector string

func (s staticInjector) Inject(string) string { return string(s) }

// =============================================================================
// Step 14: Gated real-LLM smoke test
// =============================================================================
//
// This test runs ONLY when KYOCI_THINKING_SMOKE=1 is set AND a real Ollama
// server is reachable. It exercises the full thinking state machine against a
// real model (default: qwen3.5:9b) on a trivial file-creation task.
//
// Run manually:
//
//	KYOCI_THINKING_SMOKE=1 go test ./internal/agent/ -run RealLLM_Smoke -timeout 120s -v
//
// Provider URL and model can be overridden:
//
//	KYOCI_SMOKE_BASE_URL=http://localhost:11434/v1
//	KYOCI_SMOKE_MODEL=qwen3.5:9b
func TestRealLLM_Smoke_CreateFile(t *testing.T) {
	// ── Gate ──
	if v := os.Getenv("KYOCI_THINKING_SMOKE"); v != "1" && v != "true" {
		t.Skip("skipping real-LLM smoke test; set KYOCI_THINKING_SMOKE=1 to run")
	}

	// ── Configurable provider settings ──
	baseURL := os.Getenv("KYOCI_SMOKE_BASE_URL")
	if baseURL == "" {
		baseURL = "http://192.168.2.1:11434/v1"
	}
	model := os.Getenv("KYOCI_SMOKE_MODEL")
	if model == "" {
		model = "qwen3.5:9b"
	}

	// ── Create real Ollama provider ──
	providerCfg := kyoci.ProviderConfig{
		BaseURL:      baseURL,
		APIKey:       "ollama", // dummy; Ollama doesn't require a real key
		DefaultModel: model,
		MaxRetries:   2,
		Timeout:      60 * time.Second,
		Headers:      make(map[string]string),
		Logger:       slog.Default(),
	}
	provider, err := llm.NewProvider("ollama", providerCfg)
	if err != nil {
		t.Fatalf("failed to create Ollama provider: %v", err)
	}

	// Skip (don't fail) if the Ollama server isn't reachable.
	if !provider.IsAvailable() {
		t.Skipf("Ollama not available at %s; skipping smoke test", baseURL)
	}

	// ── Build agent with thinking enabled ──
	providerReg := llm.NewProviderRegistry()
	if err := providerReg.Register("ollama", provider); err != nil {
		t.Fatalf("failed to register provider: %v", err)
	}
	router := llm.NewRouter(providerReg, llm.StrategyFallback)

	tools := kyoci.NewToolRegistry()
	if err := tools.Register(builtin.NewFileTool()); err != nil {
		t.Fatalf("failed to register file tool: %v", err)
	}
	if err := tools.Register(builtin.NewTodoTool()); err != nil {
		t.Fatalf("failed to register todo tool: %v", err)
	}
	if err := tools.Register(builtin.NewTerminalTool()); err != nil {
		t.Fatalf("failed to register terminal tool: %v", err)
	}

	// Reset todo store so plan items from prior tests don't leak in.
	builtin.ResetTodoStore()

	skills := kyoci.NewSkillRegistry()

	agentCfg := DefaultAgentConfig()
	agentCfg.EnableThinking = true
	agentCfg.EnableSkills = false
	agentCfg.EnableMemory = false
	agentCfg.PreferredProvider = "ollama"
	agentCfg.Model = model
	agentCfg.MaxIterations = 10 // smoke test: don't let it run forever

	testAgent := NewAgent(agentCfg, router, tools, skills, &mockMemory{})

	// ── Compute test file path (in cwd, which FileTool allows) ──
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	testFile := filepath.Join(cwd, "kyoci_smoke_test.txt")
	os.Remove(testFile) // remove any stale file
	t.Cleanup(func() { os.Remove(testFile) })

	// ── Execute trivial task ──
	task := fmt.Sprintf(
		"Create a file at %s containing exactly the text 'hello from kyoci thinking' "+
			"(no quotes). Use the file tool with operation 'write'. "+
			"After writing, verify the file exists by reading it back.",
		testFile,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	result, err := testAgent.Execute(ctx, task)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result == nil {
		t.Fatal("nil result")
	}
	if result.Content == "" {
		t.Error("expected non-empty final content from the agent")
	}

	// ── Assert the file was actually created with correct content ──
	data, readErr := os.ReadFile(testFile)
	if readErr != nil {
		t.Fatalf("file was not created or cannot be read: %v\n"+
			"Agent output: %s", readErr, result.Content)
	}
	content := strings.ToLower(string(data))
	if !strings.Contains(content, "hello") {
		t.Errorf("file content does not contain 'hello'; got: %q", string(data))
	}

	t.Logf("smoke test passed — file created, content verified")
	t.Logf("agent output: %s", result.Content)
	t.Logf("file content: %s", string(data))
}

// =============================================================================
// Change C: Narrative fallback when JSON nudges are exhausted
// =============================================================================
//
// When the model can't emit valid JSON after 2 ForcedJSONReminder nudges, the
// loop used to terminate with "thinking budget exhausted". Change C adds a
// graceful fallback: one plain-prose turn whose output becomes the final answer.
// The user gets a real (if unstructured) answer instead of a termination error.
//
// The narrative fallback is one-shot per task: if the narrative call itself
// fails or returns empty, the task terminates (no infinite narrative loops).

// TestExecuteWithThinking_NarrativeFallback_AfterParseFailures verifies the
// happy path of the fallback: after 2 consecutive parse failures, the loop
// drops the JSON protocol and asks for plain prose. The prose becomes the
// final answer — no "budget exhausted" error.
func TestExecuteWithThinking_NarrativeFallback_AfterParseFailures(t *testing.T) {
	const proseAnswer = "The teknikalid project uses Asia/Jakarta (GMT+7) for its cron jobs."

	provider := &queuedMockProvider{
		name: "mock",
		responses: []*kyoci.CompletionResponse{
			// Call 1 — Assess: bad JSON → parse failure #1.
			{Content: "I think the task is about timezones.", FinishReason: kyoci.FinishStop},
			// Call 2 — Assess retry: bad JSON → parse failure #2.
			{Content: "Let me check the files first.", FinishReason: kyoci.FinishStop},
			// Call 3 — Assess retry: bad JSON → parse failure #3 (cap exceeded)
			// → narrative fallback triggers.
			{Content: "```json\n{broken", FinishReason: kyoci.FinishStop},
			// Call 4 — Narrative fallback: plain prose → becomes finalContent.
			{Content: proseAnswer, FinishReason: kyoci.FinishStop},
		},
	}
	cfg := DefaultAgentConfig()
	cfg.EnableThinking = true
	cfg.EnableSkills = false
	agent := createTestAgent(cfg, provider)

	result, err := agent.Execute(context.Background(), "check timezone in teknikalid")
	if err != nil {
		t.Fatalf("Execute error: %v — narrative fallback should complete without error", err)
	}
	if result == nil {
		t.Fatal("nil result")
	}
	if !strings.Contains(result.Content, proseAnswer) {
		t.Errorf("expected narrative prose %q in content; got %q", proseAnswer, result.Content)
	}
	if strings.Contains(result.Content, "budget exhausted") {
		t.Errorf("narrative fallback should prevent budget-exhausted termination; got %q", result.Content)
	}
	if provider.Calls() != 4 {
		t.Errorf("LLM calls = %d, want 4 (3 bad JSON + 1 narrative)", provider.Calls())
	}
}

// TestExecuteWithThinking_NarrativeFallback_NotUsedOnHealthyTask verifies the
// fallback never fires when the model emits valid JSON. The NarrativeFallbackPrompt
// string must never appear in the conversation on a healthy task.
func TestExecuteWithThinking_NarrativeFallback_NotUsedOnHealthyTask(t *testing.T) {
	// Single-call fast path: Assess emits a confident final_answer with evidence.
	healthyJSON := thoughtJSON(func(th *Thought) {
		th.Confidence = 0.95
		th.NextAction = NextAction{
			Type:        "final_answer",
			FinalAnswer: "Paris is the capital of France.",
		}
		th.VerificationEvidence = []string{"general knowledge"}
	})

	// capturingProvider records all user-role messages so we can verify
	// NarrativeFallbackPrompt was never injected.
	cap := &narrativeCapturingProvider{
		mockProvider: mockProvider{
			name: "mock",
			response: &kyoci.CompletionResponse{
				Content:      healthyJSON,
				FinishReason: kyoci.FinishStop,
			},
		},
	}
	cfg := DefaultAgentConfig()
	cfg.EnableThinking = true
	cfg.EnableSkills = false
	agent := createTestAgent(cfg, cap)

	_, err := agent.Execute(context.Background(), "what is the capital of France?")
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	for _, content := range cap.userMessages {
		if strings.Contains(content, "failed to emit valid JSON") {
			t.Errorf("NarrativeFallbackPrompt leaked into healthy task; saw: %q", content)
		}
	}
}

// TestExecuteWithThinking_NarrativeFallback_OneShot_TerminatesOnEmpty verifies
// the fallback is one-shot: if the narrative call itself returns empty content
// (or fails), the task terminates with the original error rather than retrying
// the narrative prompt.
func TestExecuteWithThinking_NarrativeFallback_OneShot_TerminatesOnEmpty(t *testing.T) {
	provider := &queuedMockProvider{
		name: "mock",
		responses: []*kyoci.CompletionResponse{
			// 3 bad-JSON calls exhaust the 2-nudge cap and trigger narrative.
			{Content: "bad json 1", FinishReason: kyoci.FinishStop},
			{Content: "bad json 2", FinishReason: kyoci.FinishStop},
			{Content: "bad json 3", FinishReason: kyoci.FinishStop},
			// Narrative call returns empty → fall through to terminate.
			{Content: "", FinishReason: kyoci.FinishStop},
		},
	}
	cfg := DefaultAgentConfig()
	cfg.EnableThinking = true
	cfg.EnableSkills = false
	agent := createTestAgent(cfg, provider)

	result, _ := agent.Execute(context.Background(), "check timezone")
	if result == nil {
		t.Fatal("nil result")
	}
	// Exactly 4 calls: 3 bad + 1 narrative. A 5th call would mean the loop
	// retried the narrative prompt (which must not happen).
	if provider.Calls() != 4 {
		t.Errorf("LLM calls = %d, want 4 (narrative must be one-shot, not retried)", provider.Calls())
	}
	// Content must NOT contain the narrative prose (it was empty) and must NOT
	// hang or retry indefinitely.
	if strings.Contains(result.Content, "bad json") {
		t.Errorf("empty narrative response should not become content; got %q", result.Content)
	}
}

// narrativeCapturingProvider records every user-role message seen across all
// Complete calls, so tests can assert that a given prompt was (or was not)
// injected into the conversation.
type narrativeCapturingProvider struct {
	mockProvider
	userMessages []string
}

func (c *narrativeCapturingProvider) Complete(ctx context.Context, req kyoci.CompletionRequest) (*kyoci.CompletionResponse, error) {
	for _, msg := range req.Messages {
		if msg.Role == kyoci.RoleUser {
			c.userMessages = append(c.userMessages, msg.Content)
		}
	}
	return c.mockProvider.Complete(ctx, req)
}
