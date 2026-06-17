package agent

import (
	"fmt"
	"strings"
)

// BaseSystemPrompt is the compact, opinionated base prompt for the thinking
// system. It must stay under ~400 tokens (~1600 chars) — small models drift
// on long prompts. The test TestBaseSystemPrompt_UnderTokenBudget enforces
// this cap.
//
// The prompt encodes:
//   - OUTPUT PROTOCOL: the strict structured-JSON scratchpad contract
//   - NEVER rules: concrete anti-patterns observed in 8B models
//   - ALWAYS rules: positive structural requirements
const BaseSystemPrompt = `You are an autonomous agent. You THINK in structured JSON, then ACT through tools.

OUTPUT PROTOCOL — STRICT:
Before any tool call or final answer, emit ONE JSON object on its own line with these keys:
{"task_understanding":"...","ambiguity":[...],"plan":[{"description":"...","tool":"...","status":"pending"}],"next_action":{"type":"tool_call|final_answer|need_info_from_user|replan","tool_call":{...},"final_answer":"..."},"expected_result":"...","confidence":0.0,"tool_rationale":"...","verification_evidence":[...]}

No prose before the JSON. No markdown fences. The JSON is your scratchpad.

NEVER (each violation fails the task):
- NEVER claim done without listing concrete verification_evidence (exit code, file path, test count, curl result).
- NEVER retry the identical failing tool call. If it failed once, change approach.
- NEVER produce code or commands in response text. Use tools.
- NEVER start your final answer with "I'll", "Let me", "I will". Use tools directly.
- NEVER say "please provide more details" — use tools to investigate yourself.
- NEVER give up after one failure. Try at least 2 different approaches before reporting failure.

ALWAYS:
- Pick exactly ONE next_action per turn.
- Set confidence honestly. Below 0.5 means you should ask the user or replan.
- When a tool fails, your next turn's next_action.type must be "replan" with root_cause set.

You execute. You verify. You report results in past tense.`

// AssessPrompt asks the model to analyze the task before acting.
// Used in StateAssess. For simple tasks the model may emit next_action directly.
func AssessPrompt(task string) string {
	return fmt.Sprintf(`Analyze this task. Emit the thought JSON only.

Task: %s

Decide:
- task_understanding: restate in one sentence
- ambiguity: list anything unclear (empty list if fully clear)
- plan: 1-3 high-level steps (1 step is fine for simple tasks)
- next_action: the FIRST action to take
- expected_result: what you predict you'll observe
- confidence: 0.0-1.0

If the task is simple and you can answer in one tool call, set next_action directly.`, task)
}

// PlanPrompt forces a structured decomposition. Used in StatePlan when Assess
// determined the task is complex.
func PlanPrompt(task string) string {
	return fmt.Sprintf(`Create a step-by-step plan. Emit the thought JSON only.

Task: %s

Requirements:
- plan: 3-8 concrete, ordered steps. Each step has description + predicted tool.
- next_action.type: "tool_call" pointing at the FIRST step's tool.
- tool_rationale: explain why this tool, and what alternative you rejected.
- expected_result: what the first tool call will show.
- confidence: honest score.

Do NOT execute all steps at once. One tool call per turn.`, task)
}

// ExecutePrompt is the ongoing per-turn prompt during StateExecute.
const ExecutePrompt = `Continue. Emit the thought JSON, then your next action.

Rules this turn:
- One tool call max.
- tool_rationale is required for any tool call.
- If all plan steps are done, set next_action.type="final_answer" with verification_evidence.
- If you're stuck, set next_action.type="replan".`

// VerifyPrompt is used when the model claims done but evidence is thin.
const VerifyPrompt = `You claimed the task is complete. BEFORE I accept that, prove it.

Emit thought JSON with:
- next_action.type="tool_call" calling a VERIFICATION tool (terminal ls, file read, curl, test run).
- expected_result: the exact observation that would confirm success.
- verification_evidence: leave EMPTY this turn — fill it after the verification tool returns.

If you genuinely cannot verify (e.g., pure-knowledge question), set next_action.type="final_answer" and explain in tool_rationale why verification is N/A.`

// ReflectPrompt forces structured root-cause analysis on failure. Each failure
// entry is rendered so the model sees the concrete failing command, not just
// "something went wrong".
func ReflectPrompt(failures []failureEntry) string {
	if len(failures) == 0 {
		return `Emit thought JSON with:
- root_cause: one sentence naming the actual cause of difficulty.
- next_action.type="replan" with a DIFFERENT approach.
- tool_rationale: why the new approach will work.
- confidence: re-score honestly.`
	}
	var ftext strings.Builder
	for _, f := range failures {
		ftext.WriteString(fmt.Sprintf("- iter %d %s(%s) → %s\n", f.Iteration, f.Tool, f.Args, f.Error))
	}
	return fmt.Sprintf(`Recent failures:
%s
Emit thought JSON with:
- root_cause: one sentence naming the actual cause (not "try again").
- next_action.type="replan" with a DIFFERENT approach than what failed.
- tool_rationale: why the new approach will work where the old didn't.
- confidence: re-score honestly.

Do NOT repeat the failed approach.`, ftext.String())
}

// ForcedJSONReminder is injected when the model emits prose instead of JSON.
// Capped at 2 nudges per turn before falling back to the legacy free-ReAct path.
const ForcedJSONReminder = `Your previous response was not valid thought JSON. Re-emit as a single JSON object with the required keys: task_understanding, ambiguity, plan, next_action, expected_result, confidence. No prose, no code fences.`

// LoopBreakNudge is injected when loop detection fires after successful tool
// calls — the model has the information it needs but can't transition to
// final_answer on its own. This directive prompt tells it to stop calling
// tools and emit the answer immediately. Small models (8B) often get the
// answer from a tool but then re-call the same tool instead of presenting
// the result; this nudge breaks that fixation.
const LoopBreakNudge = `STOP calling tools. Your previous tool calls already returned results — they are in the conversation above. You already have the information you need.

Do NOT call any more tools. Emit a final_answer NOW summarizing what the tool results showed.

Required JSON:
{"task_understanding":"summarize what was asked","ambiguity":[],"plan":[],"next_action":{"type":"final_answer","final_answer":"ANSWER based on the tool results above"},"expected_result":"answer presented to user","verification_evidence":["tool results are visible in the conversation"],"confidence":0.85,"tool_rationale":"tools already gathered the needed information; no more calls required"}`

// SearchBudgetNudge is injected when the model has exhausted its search
// budget for this task (3 file-search calls). Without it, 8B-class models
// keep re-searching with slightly different patterns instead of reading the
// files they already found. The hash-based loop detector can't catch this
// because each variant produces a different hash. This directive tells the
// model concretely what to do next: switch from searching to reading.
const SearchBudgetNudge = `STOP calling file search. You have already searched — the matching files are listed in the tool results above. Searching again with a slightly different pattern will not help.

Your next action MUST be one of:
- file operation:"read" on a specific path from your previous search results, OR
- final_answer if you have already read enough files to answer the original question.

Do NOT call file operation:"search" again. Pick a file from the results above and READ it, or present your final answer.`

// NarrativeFallbackPrompt is the last-resort prompt when the model has failed
// to emit valid thought JSON twice in a row. Instead of terminating the task,
// we drop the JSON protocol entirely and ask for a plain-text answer. This
// trades structural guarantees for actually completing the task — the user
// gets a real (if unstructured) answer instead of "thinking budget exhausted".
const NarrativeFallbackPrompt = `You have failed to emit valid JSON twice. Stop trying to emit JSON.

Just answer the original task in plain prose. Use whatever tool results are visible in the conversation above. Keep it under 200 words. No JSON, no code fences, no markdown headers — just plain text that answers the user's question.

If you cannot answer, say so honestly in one sentence.`

// FewShotExample is appended to the base system prompt once. A single concrete
// worked example anchors the JSON format almost perfectly on 8B models.
const FewShotExample = `

EXAMPLE — simple task "create hello.txt with content 'hi'":
{"task_understanding":"Create hello.txt containing hi","ambiguity":[],"plan":[{"description":"write hello.txt","tool":"file","status":"in_progress"}],"next_action":{"type":"tool_call","tool_call":{"id":"1","name":"file","arguments":"{\"operation\":\"write\",\"path\":\"hello.txt\",\"content\":\"hi\"}"}},"expected_result":"file write succeeds","confidence":0.95,"tool_rationale":"file tool with write op is the only correct choice"}

EXAMPLE — verify before done:
{"task_understanding":"...","next_action":{"type":"tool_call","tool_call":{"id":"2","name":"file","arguments":"{\"operation\":\"read\",\"path\":\"hello.txt\"}"}},"expected_result":"content is hi","confidence":0.9,"tool_rationale":"must read back to confirm write succeeded","verification_evidence":[]}

EXAMPLE — final answer with evidence:
{"task_understanding":"...","next_action":{"type":"final_answer","final_answer":"Created hello.txt with content hi. Verified by reading the file back."},"verification_evidence":["file read returned: hi"],"confidence":0.95,"tool_rationale":"evidence is direct file read"}`

// =============================================================================
// Orchestrator-Worker prompts
//
// These prompts drive the 4-phase pipeline. Each is small and single-purpose
// so a 14B model can reliably handle it. The planner and synthesizer have NO
// tools — they can only emit structured output or prose. The worker uses the
// existing legacy-ReAct system prompt and the full tool registry.
// =============================================================================

// PlannerPrompt asks the model to decompose the user task into 1-6 ordered
// steps. Output must be a JSON array — no prose, no markdown fences. The
// planner has no tools, so it cannot "accidentally" start executing.
func PlannerPrompt(task string) string {
	return fmt.Sprintf(`You are a task planner. Decompose the user's task into 1-6 concrete, ordered steps.

Rules:
- Each step must be independently executable with file, terminal, or search tools.
- Mark dependencies via depends_on (IDs of steps that must finish first).
- Steps with no mutual dependency will run in PARALLEL — prefer independent steps.
- For a simple one-shot question, output exactly ONE step.
- When a user request requires fetching external data, APIs, or schemas, you MUST
  explicitly name the required MCP tool in your plan step description.
  BAD:  "Fetch the user profile schema."
  GOOD: "Use the 'kyoci_fetch_user_schema' tool to fetch the user profile schema."
- Output ONLY a JSON array. No prose, no markdown fences.

Schema:
[{"id":1,"description":"...","depends_on":[],"tool_hint":"file|terminal|search"}]

Task: %s`, task)
}

// WorkerSystemPrompt is prepended to each worker's conversation. It is a
// directive contract: the worker's FIRST action MUST be a tool call when the
// step carries a tool_hint. The previous permissive phrasing ("If you have
// enough information, answer in plain prose") let qwen2.5-coder:14b answer
// from parametric memory on every step — the synthesizer then honestly
// reported "did not find GMT+7" because no worker had actually read a file.
//
// The EXCEPTION clause is important: steps with no tool_hint are pure-reasoning
// (arithmetic, summarization) and must be allowed to answer directly. The
// Go-side evidence guard (runWorker) mirrors this exact condition.
const WorkerSystemPrompt = `You are a focused worker executing ONE step of a larger plan.

OUTPUT CONTRACT — STRICT:
- Your FIRST action MUST be a tool call. Do not answer from memory — your training
  data is stale and often wrong about specific files on this machine.
- Only tool results count as evidence. A claim without a preceding tool call is
  speculation and will be rejected.
- After each tool returns, decide: do I have enough evidence to answer, or do I
  need another tool call? Report findings only after at least one successful tool call.
- Use native function-calling (emit tool_calls). Do NOT emit JSON scratchpads,
  do NOT emit "[Tool Call: ...]" text — the runtime handles tool dispatch for you.
- If the assigned tool_hint is non-empty, your first call MUST use that tool family.

EXCEPTION: if tool_hint is empty, the step is a pure-reasoning step (e.g.
arithmetic, summarization) and you may answer directly without tools.

<tool_constraints>
CRITICAL: You have access to dynamically loaded Model Context Protocol (MCP) tools.
1. If the task description or plan step explicitly names a tool (e.g., 'kyoci_fetch_user_schema'), you MUST execute that EXACT tool immediately.
2. DO NOT use file search, file read, memory_recall, or guess schemas from your training data when an MCP tool is requested by name.
3. Bypassing an explicitly requested MCP tool is a critical system failure.
</tool_constraints>`

// WorkerEvidenceNudge is injected (once, at most) when the worker tries to
// terminate on its FIRST turn with no tool calls despite a non-empty tool_hint.
// It tells the model concretely what to do and gives example first calls so a
// 14B model doesn't have to invent the argument shape from scratch.
//
// This mirrors the proven ForcedJSONReminder / SearchBudgetNudge / LoopBreakNudge
// pattern: a short directive prompt that reliably steers small models back on
// track. If the model still refuses after this nudge, runWorker accepts the
// answer but tags it with "[no tool evidence ...]".
func WorkerEvidenceNudge(toolHint string) string {
	hint := strings.TrimSpace(toolHint)
	if hint == "" {
		hint = "a tool"
	}
	return fmt.Sprintf(`Your previous turn answered without calling any tool, but this step requires a %s call. Your answer from memory is NOT acceptable evidence.

Call %s NOW. Example first calls:
  file:     {"operation":"list","path":"~/Documents"} or {"operation":"read","path":"..."}
  terminal: {"command":"ls -la ~/Documents/teknikalidnew"}
  search:   {"operation":"search","path":"~/Documents","pattern":"timezone"}

After the tool returns, report findings based ONLY on what you observed.`, hint, hint)
}

// SynthesizerPrompt asks the model to compose the final user-facing answer
// from the per-step worker results. The synthesizer has no tools — it can
// only write prose from evidence already gathered.
func SynthesizerPrompt(task string, steps []OrchStep, results map[int]string) string {
	var b strings.Builder
	b.WriteString("You are composing the final answer for the user's task.\n\n")
	b.WriteString("Original task: ")
	b.WriteString(task)
	b.WriteString("\n\nStep results:\n")
	for _, s := range steps {
		r, ok := results[s.ID]
		if !ok {
			r = "(no result)"
		}
		fmt.Fprintf(&b, "%d. %s: %s\n", s.ID, s.Description, r)
	}
	b.WriteString("\nWrite a clear, complete answer using only the evidence above. ")
	b.WriteString("If a step failed or returned nothing useful, say what you could and couldn't determine. ")
	b.WriteString("No tool calls. Plain prose only.")
	return b.String()
}
