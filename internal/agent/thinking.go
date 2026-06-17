package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// =============================================================================
// Thinking system types
// =============================================================================

// failureEntry records a single tool-call failure for reflection and loop detection.
type failureEntry struct {
	Iteration int
	Tool      string
	Args      string
	Error     string
	Timestamp time.Time
}

// Thought is the structured scratchpad object that flows between thinking-system
// states. The model emits this as JSON before any tool call or final answer,
// externalizing the reasoning that frontier models hold internally.
type Thought struct {
	TaskUnderstanding    string     `json:"task_understanding"`
	Ambiguity            []string   `json:"ambiguity"`
	Plan                 []PlanStep `json:"plan"`
	NextAction           NextAction `json:"next_action"`
	ExpectedResult       string     `json:"expected_result"`
	Confidence           float64    `json:"confidence"`
	ToolRationale        string     `json:"tool_rationale,omitempty"`
	VerificationEvidence []string   `json:"verification_evidence,omitempty"`
	RootCause            string     `json:"root_cause,omitempty"`
}

// PlanStep is a single item in the model's decomposition of the task.
type PlanStep struct {
	Description string `json:"description"`
	Tool        string `json:"tool"`
	Status      string `json:"status"` // pending | in_progress | done
}

// NextAction is the single decision the model makes for the current turn.
type NextAction struct {
	Type                  string           `json:"type"` // tool_call | final_answer | need_info_from_user | replan
	ToolCall              *kyoci.ToolCall  `json:"tool_call,omitempty"`
	FinalAnswer           string           `json:"final_answer,omitempty"`
	ClarificationQuestion string           `json:"clarification_question,omitempty"`
}

// =============================================================================
// State machine
// =============================================================================

// LoopState represents a state in the hybrid thinking state machine.
type LoopState int

const (
	StateAssess  LoopState = iota // Pre-flight: parse task, check ambiguity, inventory context
	StatePlan                     // Decompose into todos / single-step decision
	StateExecute                  // One or more tool calls driven by LLM
	StateVerify                   // Evidence check before allowing DONE
	StateReflect                  // Structured root-cause on failure
	StateDone                     // Terminal — emit final answer
)

// loopState tracks everything between iterations of the state machine.
// All fields are per-task; one loopState is created at the start of each
// executeWithThinking call.
type loopState struct {
	state            LoopState
	thoughts         []Thought
	currentPlan      []PlanStep // active plan from latest Plan/Assess state
	toolCallsUsed    int
	toolBudget       int // default 15
	recentCallHashes []string
	uniqueCallHashes map[string]int
	failureHistory   []failureEntry
	reflectionsUsed  int
	replansUsed      int
	maxReflections   int // default 3
	maxReplans       int // default 2
	finalContent      string
	startedAt         time.Time
	// loopBreakAttempted counts how many times the LoopBreakNudge has been
	// injected on this task. Capped at 1: if the model keeps looping after
	// the nudge, we escalate to Reflect rather than nudging forever.
	loopBreakAttempted int
	// searchesUsed counts file-search tool calls (file tool with
	// operation: "search"). It powers the search-budget nudge: 8B-class
	// models often vary the search pattern slightly each turn instead of
	// reading the files they already found, defeating hash-based loop
	// detection. When this exceeds maxSearchesPerTask, executeStep injects
	// SearchBudgetNudge to force the model to switch from searching to reading.
	searchesUsed int
}

// maxSearchesPerTask caps how many file-search calls a task should make
// before executeStep injects SearchBudgetNudge. 3 is enough for an audit:
// one to scope the directory, one or two to find candidate files. More than
// that is the model varying patterns instead of reading what it found.
const maxSearchesPerTask = 3

// recordToolCall logs a tool invocation for budget tracking and loop detection.
// The hash is over (toolName, sortedArgs) so reordered JSON args collide.
func (ls *loopState) recordToolCall(toolName, args string) {
	ls.toolCallsUsed++
	h := hashToolCall(toolName, args)
	ls.uniqueCallHashes[h]++
	ls.recentCallHashes = append(ls.recentCallHashes, h)
	// Keep only the last 4 for short-window loop detection.
	if len(ls.recentCallHashes) > 4 {
		ls.recentCallHashes = ls.recentCallHashes[len(ls.recentCallHashes)-4:]
	}
	// Track file-search usage for the search-budget nudge. Malformed args
	// are tolerated — they simply don't increment the counter (we can't
	// tell what operation was requested).
	if isFileSearchCall(toolName, args) {
		ls.searchesUsed++
	}
}

// isFileSearchCall reports whether the call is a file tool invocation with
// operation: "search". Returns false on parse failure — recordToolCall must
// never panic on malformed model output.
func isFileSearchCall(toolName, args string) bool {
	if toolName != "file" {
		return false
	}
	var parsed struct {
		Operation string `json:"operation"`
	}
	if err := json.Unmarshal([]byte(args), &parsed); err != nil {
		return false
	}
	return strings.EqualFold(parsed.Operation, "search")
}

// searchBudgetExceeded returns true once the model has made
// maxSearchesPerTask file-search calls. executeStep uses this to inject
// SearchBudgetNudge, which tells the model to stop searching and start
// reading the files it already found.
func (ls *loopState) searchBudgetExceeded() bool {
	return ls.searchesUsed >= maxSearchesPerTask
}

// isLooping returns true if any single call hash appears ≥2 times in the last
// 4 calls. This is the primary signal for "model is stuck retrying the same
// failing command".
func (ls *loopState) isLooping() bool {
	counts := make(map[string]int, len(ls.recentCallHashes))
	for _, h := range ls.recentCallHashes {
		counts[h]++
		if counts[h] >= 2 {
			return true
		}
	}
	return false
}

// budgetExhausted returns true when the tool-call budget is fully consumed.
func (ls *loopState) budgetExhausted() bool {
	return ls.toolCallsUsed >= ls.toolBudget
}

// budgetNearExhaustedNoProgress returns true when ≥75% of the budget is used
// AND fewer than half of all calls were unique. This catches the degenerate
// case where the model is repeatedly trying variations that don't work.
func (ls *loopState) budgetNearExhaustedNoProgress() bool {
	if ls.toolCallsUsed == 0 {
		return false
	}
	if float64(ls.toolCallsUsed) < float64(ls.toolBudget)*0.75 {
		return false
	}
	uniqueCalls := len(ls.uniqueCallHashes)
	return float64(uniqueCalls)/float64(ls.toolCallsUsed) < 0.5
}

// canReflect returns true if the reflection cap has not been reached.
func (ls *loopState) canReflect() bool {
	return ls.reflectionsUsed < ls.maxReflections
}

// canReplan returns true if the replan cap has not been reached.
func (ls *loopState) canReplan() bool {
	return ls.replansUsed < ls.maxReplans
}

// recordFailure appends a failure entry with the timestamp filled in.
func (ls *loopState) recordFailure(entry failureEntry) {
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}
	ls.failureHistory = append(ls.failureHistory, entry)
}

// hasToolExecutionFailure returns true if any recorded failure came from a
// real tool (terminal, file, http_client, etc.) rather than from the loop's
// own control-flow entries (verify/replan/unknown). Small models routinely
// skip the optional verification_evidence JSON field even when tools succeeded;
// verifyStep uses this to distinguish "tools worked but model skipped a field"
// from "a tool genuinely failed and the model claims done without evidence".
func (ls *loopState) hasToolExecutionFailure() bool {
	for _, f := range ls.failureHistory {
		if f.Tool != "verify" && f.Tool != "replan" && f.Tool != "unknown" && f.Tool != "" {
			return true
		}
	}
	return false
}

// hashToolCall returns a deterministic hash of (toolName, args) where args is
// sorted by key first so reordering doesn't produce a different hash.
func hashToolCall(toolName, args string) string {
	// Try to parse + re-sort args; fall back to raw string on parse failure.
	var m map[string]interface{}
	sorted := args
	if err := json.Unmarshal([]byte(args), &m); err == nil {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var sb strings.Builder
		sb.WriteString("{")
		for i, k := range keys {
			if i > 0 {
				sb.WriteString(",")
			}
			v, _ := json.Marshal(m[k])
			sb.WriteString(fmt.Sprintf("%q:%s", k, string(v)))
		}
		sb.WriteString("}")
		sorted = sb.String()
	}
	h := sha256.Sum256([]byte(toolName + "|" + sorted))
	return hex.EncodeToString(h[:])[:16]
}

// =============================================================================
// State step methods (one per LoopState)
// =============================================================================
//
// Each step method is responsible for: calling the LLM if needed, parsing the
// response into a Thought, recording it in loopState, and returning the next
// LoopState. The main loop driver (added in Step 10) dispatches on ls.state.

// assessConfidenceThreshold is the minimum confidence for the fast path.
// Below this, even simple tasks get routed through Plan.
const assessConfidenceThreshold = 0.7

// assessStep is the entry state. It analyzes the task and decides between the
// fast path (simple + confident → Execute) and the full multi-pass loop (→ Plan).
func (a *Agent) assessStep(ctx context.Context, convo *Context, ls *loopState, task string) (*Thought, LoopState, error) {
	// Prompt the model for a structured assessment.
	convo.AddMessage(kyoci.RoleUser, AssessPrompt(task))

	_, resp, err := a.think(ctx, convo)
	if err != nil {
		return nil, StateAssess, fmt.Errorf("assess: LLM call failed: %w", err)
	}

	th, err := parseThought(resp.Content)
	if err != nil {
		// The model emitted something we couldn't parse. Caller (the loop driver)
		// will inject ForcedJSONReminder and retry, up to the nudge cap.
		return nil, StateAssess, fmt.Errorf("assess: parse failed: %w", err)
	}

	ls.thoughts = append(ls.thoughts, *th)

	// Deterministic complexity check — even a confident model can't skip Plan
	// when the task text itself signals complexity.
	complexity := assessComplexity(task, ls.failureHistory)

	threshold := a.config.ThinkingConfidenceThreshold
	if threshold <= 0 {
		threshold = assessConfidenceThreshold
	}

	if !complexity.Complex &&
		th.Confidence >= threshold &&
		(th.NextAction.Type == "tool_call" || th.NextAction.Type == "final_answer") {
		// Fast path: simple task with a confident decision — record plan and
		// go straight to Execute.
		if len(th.Plan) > 0 {
			ls.currentPlan = capPlan(th.Plan)
		}
		return th, StateExecute, nil
	}

	return th, StatePlan, nil
}

// maxPlanItems caps plan length to keep the model focused. Long plans hurt
// small-model attention and rarely complete in a single task turn.
const maxPlanItems = 8

// planStep is the second state when Assess escalates. It asks the model for a
// structured decomposition, records it in loopState, and transitions to Execute.
func (a *Agent) planStep(ctx context.Context, convo *Context, ls *loopState, task string) (*Thought, LoopState, error) {
	convo.AddMessage(kyoci.RoleUser, PlanPrompt(task))

	_, resp, err := a.think(ctx, convo)
	if err != nil {
		return nil, StatePlan, fmt.Errorf("plan: LLM call failed: %w", err)
	}

	th, err := parseThought(resp.Content)
	if err != nil {
		return nil, StatePlan, fmt.Errorf("plan: parse failed: %w", err)
	}

	ls.thoughts = append(ls.thoughts, *th)
	ls.currentPlan = capPlan(th.Plan)

	return th, StateExecute, nil
}

// capPlan truncates a plan to maxPlanItems. Truncation is logged by the caller.
func capPlan(plan []PlanStep) []PlanStep {
	if len(plan) <= maxPlanItems {
		return plan
	}
	return plan[:maxPlanItems]
}

// executeStep processes ONE turn of the Execute state. It calls the LLM with
// ExecutePrompt, parses the Thought, and either runs the tool call (staying in
// Execute on success or escalating to Reflect on failure/loop) or transitions
// out (Verify on final_answer, Reflect on replan, Done on budget exhaustion).
//
// Pre-flight deterministic checks (run before the LLM call):
//   - ls.budgetExhausted()    → StateDone (honest termination)
//   - ls.isLooping() (first time, no real failure) → LoopBreakNudge (recovery)
//   - ls.isLooping() / ls.budgetNearExhaustedNoProgress() otherwise → StateReflect
func (a *Agent) executeStep(ctx context.Context, convo *Context, ls *loopState) (*Thought, LoopState, error) {
	// Deterministic pre-flight: terminate honestly if budget is gone.
	if ls.budgetExhausted() {
		ls.finalContent = "Task terminated: tool-call budget exhausted without completion."
		return nil, StateDone, nil
	}
	// Compute loop signals once — both helpers look at the same state.
	looping := ls.isLooping()
	nearExhausted := ls.budgetNearExhaustedNoProgress()

	// Recovery path (one-shot nudge): the model got a tool result but keeps
	// re-calling the same tool. This is a common 8B-model failure mode — the
	// model has the answer but can't transition to final_answer on its own.
	// Inject a directive nudge and give it one more turn. We try this at most
	// once per task; if the model loops again, escalate to Reflect.
	//
	// Skip the nudge when there's a real tool failure (Reflect has something
	// concrete to diagnose) or when the budget is near exhausted (the model
	// is far gone and a nudge would waste a turn it may not have).
	if looping && ls.loopBreakAttempted == 0 && !ls.hasToolExecutionFailure() && !nearExhausted {
		ls.loopBreakAttempted++
		a.logger.Warn("execute pre-flight: injecting loop-break nudge",
			"tool_calls_used", ls.toolCallsUsed,
			"unique_calls", len(ls.uniqueCallHashes))
		convo.AddMessage(kyoci.RoleUser, LoopBreakNudge)
	} else if looping || nearExhausted {
		// Escalate to Reflect. Record a failure so that
		// reflectExhaustionMessage shows a real cause instead of "none".
		reason := "loop detected: repeating same tool call"
		switch {
		case nearExhausted:
			reason = fmt.Sprintf("budget %d/%d used with low unique-call ratio (%d unique)",
				ls.toolCallsUsed, ls.toolBudget, len(ls.uniqueCallHashes))
		case ls.loopBreakAttempted > 0:
			reason = "loop detected after loop-break nudge (model did not transition to final_answer)"
		case ls.hasToolExecutionFailure():
			reason = "loop detected with prior tool failure"
		}
		ls.recordFailure(failureEntry{
			Iteration: len(ls.thoughts),
			Tool:      "execute",
			Error:     reason,
		})
		a.logger.Warn("execute pre-flight: escalating to Reflect",
			"reason", reason,
			"tool_calls_used", ls.toolCallsUsed,
			"unique_calls", len(ls.uniqueCallHashes))
		return nil, StateReflect, nil
	} else if ls.searchBudgetExceeded() {
		// Search-budget nudge: the model isn't looping (different patterns
		// produce different hashes) but it has done maxSearchesPerTask
		// file searches without switching to reading. This is the typical
		// failure mode on exploration/audit tasks — 8B models vary the
		// pattern slightly each turn and "find" overlapping matches,
		// genuinely believing they're making progress. Force them to read.
		a.logger.Warn("execute pre-flight: injecting search-budget nudge",
			"searches_used", ls.searchesUsed,
			"tool_calls_used", ls.toolCallsUsed)
		convo.AddMessage(kyoci.RoleUser, SearchBudgetNudge)
	} else {
		convo.AddMessage(kyoci.RoleUser, ExecutePrompt)
	}

	_, resp, err := a.think(ctx, convo)
	if err != nil {
		return nil, StateExecute, fmt.Errorf("execute: LLM call failed: %w", err)
	}

	th, err := parseThought(resp.Content)
	if err != nil {
		return nil, StateExecute, fmt.Errorf("execute: parse failed: %w", err)
	}
	ls.thoughts = append(ls.thoughts, *th)

	switch th.NextAction.Type {
	case "final_answer":
		ls.finalContent = th.NextAction.FinalAnswer
		return th, StateVerify, nil
	case "replan":
		// The model itself decided the plan is wrong. Treat like a failure
		// requiring reflection.
		if th.RootCause != "" {
			ls.recordFailure(failureEntry{
				Tool:  "replan",
				Error: th.RootCause,
			})
		}
		return th, StateReflect, nil
	case "need_info_from_user":
		// Treat as terminal — the loop can't make progress without the user.
		ls.finalContent = "Need clarification from user: " + th.NextAction.ClarificationQuestion
		return th, StateDone, nil
	case "tool_call":
		if th.NextAction.ToolCall == nil {
			ls.recordFailure(failureEntry{Error: "tool_call action missing tool_call field"})
			return th, StateReflect, nil
		}
		return a.runToolCall(ctx, convo, ls, th)
	default:
		// Unknown action type — escalate to Reflect for diagnosis.
		ls.recordFailure(failureEntry{
			Tool:  "unknown",
			Error: fmt.Sprintf("unknown next_action type %q", th.NextAction.Type),
		})
		return th, StateReflect, nil
	}
}

// runToolCall executes a single tool call from a Thought's NextAction and
// returns the next LoopState (Execute on success, Reflect on failure).
func (a *Agent) runToolCall(ctx context.Context, convo *Context, ls *loopState, th *Thought) (*Thought, LoopState, error) {
	tc := *th.NextAction.ToolCall
	ls.recordToolCall(tc.Name, tc.Arguments)

	// Execute via the same path used by the legacy ReAct loop.
	result, err := a.act(ctx, tc)
	if err != nil {
		ls.recordFailure(failureEntry{
			Iteration: len(ls.thoughts),
			Tool:      tc.Name,
			Args:      tc.Arguments,
			Error:     err.Error(),
		})
		// Surface the failure to the model so the next Reflect turn has context.
		result = fmt.Sprintf("Error: %v", err)
		convo.AddToolResult(tc.ID, result)
		return th, StateReflect, nil
	}

	// Truncate large outputs to prevent context bloat, matching the legacy loop.
	result = truncateToolResult(result, 4000)
	convo.AddToolResult(tc.ID, result)

	// Success — stay in Execute so the driver calls us again for the next turn.
	return th, StateExecute, nil
}

// verifyStep is the gate before DONE. It is deterministic: no LLM call in the
// common case. It accepts the model's "done" claim if any of these hold:
//   - the model produced concrete verification_evidence, OR
//   - no tools were called (pure-knowledge question that can't be verified), OR
//   - tools were called and none actually failed (the tool results in the
//     conversation are implicit evidence — small models routinely skip the
//     optional verification_evidence field and forcing Reflect here burns the
//     reflection budget without diagnosing a real problem).
//
// It only forces Reflect when a tool genuinely failed and the model claims done
// without addressing that failure. This is the primary defense against premature
// success claims from small models — but it's scoped to actual failures so it
// doesn't kill well-behaved tasks that just skipped an optional JSON field.
func (a *Agent) verifyStep(ctx context.Context, convo *Context, ls *loopState) (*Thought, LoopState, error) {
	if len(ls.thoughts) == 0 {
		// Nothing to verify — accept Done to avoid an infinite loop.
		return nil, StateDone, nil
	}
	last := ls.thoughts[len(ls.thoughts)-1]

	if len(last.VerificationEvidence) > 0 {
		return &last, StateDone, nil
	}
	if ls.toolCallsUsed == 0 {
		// Pure-knowledge answer; no verification possible.
		return &last, StateDone, nil
	}

	// Tools were called but no evidence. If none of them actually failed,
	// accept Done — the tool results in the conversation are implicit
	// evidence, and small models routinely skip the optional
	// verification_evidence field. Forcing Reflect here was burning the
	// reflection budget on well-behaved tasks.
	if !ls.hasToolExecutionFailure() {
		return &last, StateDone, nil
	}

	// A tool genuinely failed and the model claims done without addressing it
	// — demand verification.
	ls.recordFailure(failureEntry{
		Iteration: len(ls.thoughts),
		Tool:      "verify",
		Error:     "claimed done without verification_evidence after tool failure",
	})
	return &last, StateReflect, nil
}

// =============================================================================
// Thought parser (3-tier defensive strategy)
// =============================================================================
//
// Small models frequently emit:
//   1. Clean JSON (tier 1 — strict unmarshal)
//   2. JSON wrapped in markdown fences or surrounded by prose (tier 2 — extract)
//   3. Truncated/malformed JSON with an intact next_action block (tier 3 — salvage)
//
// The parser never panics; on hard failure it returns an error so the loop can
// inject ForcedJSONReminder on the next turn.

// parseThought extracts a Thought from the model's text output. Returns an
// error only if no JSON could be recovered at all.
func parseThought(input string) (*Thought, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, errors.New("parseThought: empty input")
	}

	// Strip markdown code fences if present (```json ... ```)
	input = stripCodeFences(input)

	// Tier 1: strict JSON parse
	var th Thought
	if err := json.Unmarshal([]byte(input), &th); err == nil {
		return &th, nil
	}

	// Tier 2: extract outermost { ... } and retry
	if extracted := extractOutermostJSON(input); extracted != "" {
		var th2 Thought
		if err := json.Unmarshal([]byte(extracted), &th2); err == nil {
			return &th2, nil
		}
	}

	// Tier 3: regex/scan salvage of next_action block
	if salvaged := salvageNextAction(input); salvaged != nil {
		return salvaged, nil
	}

	return nil, fmt.Errorf("parseThought: no valid thought JSON found in %d-char input", len(input))
}

// stripCodeFences removes ```lang ... ``` wrappers so fenced JSON parses cleanly.
func stripCodeFences(input string) string {
	if !strings.HasPrefix(input, "```") {
		return input
	}
	lines := strings.Split(input, "\n")
	var inner []string
	inBlock := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			if inBlock {
				break
			}
			inBlock = true
			continue
		}
		if inBlock {
			inner = append(inner, line)
		}
	}
	if len(inner) > 0 {
		return strings.Join(inner, "\n")
	}
	return input
}

// extractOutermostJSON returns the substring from the first '{' to the last '}'.
// This recovers JSON that is wrapped in leading/trailing prose.
func extractOutermostJSON(input string) string {
	start := strings.Index(input, "{")
	end := strings.LastIndex(input, "}")
	if start == -1 || end == -1 || end <= start {
		return ""
	}
	return input[start : end+1]
}

// salvageNextAction scans for a `"next_action"` key and extracts the balanced
// `{...}` block that follows, ignoring string-internal braces. Used when the
// overall JSON is malformed but the per-turn decision is still recoverable.
func salvageNextAction(input string) *Thought {
	idx := strings.Index(input, `"next_action"`)
	if idx == -1 {
		return nil
	}
	// Find the first '{' after "next_action"
	relStart := strings.Index(input[idx:], "{")
	if relStart == -1 {
		return nil
	}
	braceStart := idx + relStart

	// Scan with brace counting, respecting string literals and escapes.
	depth := 0
	end := -1
	inString := false
	escape := false
	for i := braceStart; i < len(input); i++ {
		c := input[i]
		if escape {
			escape = false
			continue
		}
		if c == '\\' {
			escape = true
			continue
		}
		if c == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		if c == '{' {
			depth++
		} else if c == '}' {
			depth--
			if depth == 0 {
				end = i
				break
			}
		}
	}
	if end == -1 {
		return nil
	}
	block := input[braceStart : end+1]
	var na NextAction
	if err := json.Unmarshal([]byte(block), &na); err != nil {
		return nil
	}
	return &Thought{NextAction: na}
}

// =============================================================================
// Reflect state
// =============================================================================
//
// reflectStep is the recovery state after failure (tool error, looping, or
// missing verification evidence). It forces the model to produce a structured
// root_cause and choose a DIFFERENT approach. The "different approach" rule is
// enforced by the ReflectPrompt which shows the model the concrete failing
// command and asks for an alternative.
//
// Transitions:
//   - at reflection cap            → StateDone (honest termination, no LLM call)
//   - next_action.type=replan      → StatePlan (counts against replan cap)
//   - next_action.type=tool_call   → StateExecute (different-approach retry)
//   - next_action.type=final_answer→ StateDone (model gives up honestly)
//   - replan cap hit + wants replan→ StateDone (honest termination)
//
// The hard caps (maxReflections, maxReplans) guarantee the loop terminates
// even on an intractable problem.

// reflectStep is the recovery state after failure.
func (a *Agent) reflectStep(ctx context.Context, convo *Context, ls *loopState) (*Thought, LoopState, error) {
	// Pre-flight: if we've hit the reflection cap, terminate honestly without
	// spending another LLM call. This is the hard stop that prevents infinite
	// reflection loops.
	if !ls.canReflect() {
		ls.finalContent = reflectExhaustionMessage(ls)
		return nil, StateDone, nil
	}

	convo.AddMessage(kyoci.RoleUser, ReflectPrompt(ls.failureHistory))

	_, resp, err := a.think(ctx, convo)
	if err != nil {
		return nil, StateReflect, fmt.Errorf("reflect: LLM call failed: %w", err)
	}

	th, err := parseThought(resp.Content)
	if err != nil {
		return nil, StateReflect, fmt.Errorf("reflect: parse failed: %w", err)
	}

	ls.thoughts = append(ls.thoughts, *th)
	ls.reflectionsUsed++

	switch th.NextAction.Type {
	case "replan":
		if !ls.canReplan() {
			// Model wants to replan but we're out of replans — terminate honestly.
			ls.finalContent = reflectExhaustionMessage(ls)
			return th, StateDone, nil
		}
		ls.replansUsed++
		// If the model already proposed a new plan in its reflect response, use
		// it directly so the driver can skip a redundant Plan LLM call when
		// practical. planStep will overwrite this if it runs.
		if len(th.Plan) > 0 {
			ls.currentPlan = capPlan(th.Plan)
		}
		return th, StatePlan, nil
	case "tool_call":
		// The model proposed a different approach — execute it immediately
		// rather than deferring to the next executeStep call (which would
		// discard this Thought's tool_call and waste the LLM response).
		// runToolCall dispatches to act(), records the call against the budget,
		// and transitions to Execute on success or Reflect on failure.
		if th.NextAction.ToolCall == nil {
			ls.recordFailure(failureEntry{Error: "reflect: tool_call action missing tool_call field"})
			return th, StateDone, nil
		}
		return a.runToolCall(ctx, convo, ls, th)
	case "final_answer":
		// The model decided it can't recover and is giving an honest final answer.
		ls.finalContent = th.NextAction.FinalAnswer
		return th, StateDone, nil
	default:
		// Unknown action — treat as a forced replan if budget remains, else
		// terminate honestly. This keeps the loop moving forward.
		if ls.canReplan() {
			ls.replansUsed++
			return th, StatePlan, nil
		}
		ls.finalContent = reflectExhaustionMessage(ls)
		return th, StateDone, nil
	}
}

// reflectExhaustionMessage builds an honest termination message when the
// reflection or replan cap is hit. Surfacing the last concrete failure gives
// the user enough context to intervene without re-running the whole task.
func reflectExhaustionMessage(ls *loopState) string {
	return fmt.Sprintf(
		"Task terminated: thinking budget exhausted (%d/%d reflections, %d/%d replans). "+
			"Last failure: %s",
		ls.reflectionsUsed, ls.maxReflections,
		ls.replansUsed, ls.maxReplans,
		lastFailureSummary(ls.failureHistory),
	)
}

// lastFailureSummary returns a short description of the most recent failure,
// or "none" if there are no recorded failures.
func lastFailureSummary(history []failureEntry) string {
	if len(history) == 0 {
		return "none"
	}
	last := history[len(history)-1]
	if last.Args != "" {
		return fmt.Sprintf("%s(%s) → %s", last.Tool, last.Args, last.Error)
	}
	return fmt.Sprintf("%s → %s", last.Tool, last.Error)
}

// =============================================================================
// Thinking loop driver
// =============================================================================
//
// executeWithThinking is the state-machine driver invoked by Execute() when
// AgentConfig.EnableThinking is true. It replaces the free-ReAct loop with the
// explicit Assess → Plan → Execute → Verify → Reflect → Done transitions.
//
// Responsibilities of the driver:
//   - seed the conversation with BaseSystemPrompt + FewShotExample (+ L3)
//   - dispatch to the per-state step methods based on ls.state
//   - recover from Thought-parse failures with ForcedJSONReminder (max 2 in a row)
//   - enforce a hard iteration ceiling as a secondary safety net
//   - compose a TaskResult from loopState on Done or termination
//
// Token-usage accumulation is intentionally deferred: the per-state step
// methods call a.think() and currently discard the TokenUsage return. This
// keeps Step 10 small; Usage is left zero on the thinking path.

// defaultThoughtToolBudget caps the number of tool calls per task. The
// fallback is 25 (config can override): 15 was too tight for legitimate
// exploration/audit tasks — see the search-budget nudge for the related
// guard against redundant file-search patterns.
const defaultThoughtToolBudget = 25

// defaultThoughtMaxReflections caps recovery attempts per task.
const defaultThoughtMaxReflections = 3

// defaultThoughtMaxReplans caps plan-revisions per task.
const defaultThoughtMaxReplans = 2

// maxParseNudgesInARow caps how many ForcedJSONReminder injections we allow
// on consecutive parse failures before giving up on the current state.
const maxParseNudgesInARow = 2

// newLoopState constructs a loopState with caps from AgentConfig, falling back
// to hardcoded constants when config fields are zero. One instance is created
// per Execute call; it is not shared across tasks.
func (a *Agent) newLoopState() *loopState {
	tb := a.config.ThinkingToolBudget
	if tb <= 0 {
		tb = defaultThoughtToolBudget
	}
	mr := a.config.ThinkingMaxReflections
	if mr <= 0 {
		mr = defaultThoughtMaxReflections
	}
	rp := a.config.ThinkingMaxReplans
	if rp <= 0 {
		rp = defaultThoughtMaxReplans
	}
	return &loopState{
		state:            StateAssess,
		toolBudget:       tb,
		maxReflections:   mr,
		maxReplans:       rp,
		uniqueCallHashes: make(map[string]int),
		startedAt:        time.Now(),
	}
}

// maxThoughtTransitions is the hard ceiling on state-machine transitions per
// task. It is derived from MaxIterations/MaxContinuations so users who raise
// those knobs get proportionally more headroom in the thinking path, with a
// sane floor.
func (a *Agent) maxThoughtTransitions() int {
	n := a.config.MaxIterations * (1 + a.config.MaxContinuations)
	if n < 50 {
		n = 50
	}
	return n
}

// executeWithThinking drives the hybrid thinking state machine.
func (a *Agent) executeWithThinking(ctx context.Context, task string) (*kyoci.TaskResult, error) {
	a.logger.Info("thinking loop enabled", "task", task)

	// Build conversation context with the thinking-system prompts.
	// Order matters for small models (8B): injected skill markdown goes FIRST
	// so the JSON protocol + few-shot examples are the last (strongest) signal
	// before the task. Injecting 4-8k chars of markdown AFTER the protocol was
	// diluting it and causing the model to drift off-format, leading to
	// reflection-loop exhaustion. See plan: "thinking budget exhaustion."
	convo := NewContext()
	systemPrompt := ""
	if injected := a.injector.Inject(task); injected != "" {
		systemPrompt += injected + "\n\n"
		a.logger.Info("L3 context injected", "context_length", len(injected),
			"position", "pre-protocol")
	}
	systemPrompt += BaseSystemPrompt
	if a.config.ThinkingFewShot {
		systemPrompt += "\n\n" + FewShotExample
	}
	if a.config.SystemPrompt != "" {
		systemPrompt += "\n\n" + a.config.SystemPrompt
	}
	convo.AddMessage(kyoci.RoleSystem, systemPrompt)
	convo.AddMessage(kyoci.RoleUser, task)

	ls := a.newLoopState()
	taskStart := time.Now()
	maxTransitions := a.maxThoughtTransitions()
	parseNudgesInARow := 0
	narrativeFallbackUsed := false
	var lastError error

	currentState := StateAssess

transitionLoop:
	for iter := 0; iter < maxTransitions; iter++ {
		// Auto-compaction guard, matching the legacy loop's behavior.
		if convo.TokenCount() > a.config.MaxContextTokens {
			a.logger.Info("auto-compacting context",
				"tokens", convo.TokenCount(),
				"messages", convo.MessageCount())
			convo.SmartCompact(a.config.MaxContextTokens / 2)
		}

		// Fire progress event so UIs that consume the legacy loop's events
		// still see per-iteration heartbeat signals.
		if fn := getProgress(ctx); fn != nil {
			fn(ProgressEvent{Type: "think", Iteration: iter + 1})
		}

		var next LoopState
		var err error

		switch currentState {
		case StateAssess:
			_, next, err = a.assessStep(ctx, convo, ls, task)
		case StatePlan:
			_, next, err = a.planStep(ctx, convo, ls, task)
		case StateExecute:
			_, next, err = a.executeStep(ctx, convo, ls)
		case StateVerify:
			_, next, err = a.verifyStep(ctx, convo, ls)
		case StateReflect:
			_, next, err = a.reflectStep(ctx, convo, ls)
		case StateDone:
			break transitionLoop
		default:
			lastError = fmt.Errorf("thinking driver: unknown state %d", currentState)
			break transitionLoop
		}

		if err != nil {
			// Parse-failure recovery: nudge the model to re-emit valid JSON.
			// Transport errors (LLM call failed) are not recoverable this way.
			if isParseError(err) && parseNudgesInARow < maxParseNudgesInARow {
				parseNudgesInARow++
				a.logger.Warn("thought parse failure, injecting JSON reminder",
					"state", stateName(currentState),
					"nudge", parseNudgesInARow)
				convo.AddMessage(kyoci.RoleUser, ForcedJSONReminder)
				continue // retry the same state without advancing
			}
			// Graceful narrative fallback: if we haven't tried narrative mode
			// yet, give the model one plain-prose turn and use it as the
			// final answer. Better than terminating with "thinking budget
			// exhausted" for users on smaller models that can't reliably emit
			// the complex JSON schema. One-shot per task — no retries.
			if isParseError(err) && !narrativeFallbackUsed {
				narrativeFallbackUsed = true
				a.logger.Warn("parse nudges exhausted, falling back to narrative mode",
					"state", stateName(currentState),
					"parse_nudges", parseNudgesInARow)
				convo.AddMessage(kyoci.RoleUser, NarrativeFallbackPrompt)
				_, resp, ferr := a.think(ctx, convo)
				if ferr == nil && resp != nil && strings.TrimSpace(resp.Content) != "" {
					ls.finalContent = strings.TrimSpace(resp.Content)
					currentState = StateDone
					break transitionLoop
				}
				// Narrative call failed or returned empty — fall through to
				// terminate with the original parse error.
				a.logger.Warn("narrative fallback returned empty/error; terminating",
					"narrative_err", ferr)
			}
			lastError = err
			break transitionLoop
		}

		// A successful transition resets the consecutive-parse-failure counter.
		parseNudgesInARow = 0

		if next == StateDone {
			currentState = StateDone
			break transitionLoop
		}
		currentState = next
	}

	// Compose final content from whatever the loop produced.
	finalContent := ls.finalContent
	if finalContent == "" && lastError == nil {
		// Loop ran out of transitions without an explicit Done. Report honestly.
		finalContent = fmt.Sprintf(
			"Task ended: thinking loop exhausted after %d transitions (%d tool calls, %d thoughts).",
			maxTransitions, ls.toolCallsUsed, len(ls.thoughts))
		a.logger.Warn("thinking loop exhausted without reaching Done",
			"transitions", maxTransitions,
			"tool_calls", ls.toolCallsUsed,
			"thoughts", len(ls.thoughts))
	}

	// Defense in depth: strip any leaked Thought JSON from the surfaced text.
	finalContent = sanitizeContent(finalContent)

	// Done progress event, mirroring the legacy loop's terminal event.
	if fn := getProgress(ctx); fn != nil {
		msg := "completed"
		if lastError != nil {
			msg = "error: " + lastError.Error()
		}
		fn(ProgressEvent{Type: "done", Message: msg})
	}

	// Record experience for self-improvement (non-blocking), matching legacy.
	a.recorder.Record(ctx, TaskRecord{
		Task:       task,
		Iterations: len(ls.thoughts),
		ToolCalls:  ls.toolCallsUsed,
		Success:    finalContent != "" && lastError == nil,
		DurationMs: time.Since(taskStart).Milliseconds(),
		ErrorMsg: func() string {
			if lastError != nil {
				return lastError.Error()
			}
			return ""
		}(),
	})

	result := &kyoci.TaskResult{
		Content:       finalContent,
		ToolCallsMade: ls.toolCallsUsed,
		Iterations:    len(ls.thoughts),
		Usage:         kyoci.TokenUsage{},
		Error:         lastError,
	}

	if lastError != nil && finalContent == "" {
		return result, lastError
	}
	return result, nil
}

// isParseError reports whether an error returned by a step method is a
// Thought-parse failure (recoverable with a ForcedJSONReminder) rather than an
// LLM transport error (not recoverable).
func isParseError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "parse failed")
}

// stateName returns a human-readable name for a LoopState for logging.
func stateName(s LoopState) string {
	switch s {
	case StateAssess:
		return "Assess"
	case StatePlan:
		return "Plan"
	case StateExecute:
		return "Execute"
	case StateVerify:
		return "Verify"
	case StateReflect:
		return "Reflect"
	case StateDone:
		return "Done"
	default:
		return fmt.Sprintf("Unknown(%d)", s)
	}
}
