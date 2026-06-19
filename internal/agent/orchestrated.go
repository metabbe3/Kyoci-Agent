package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/metabbe3/Kyoci-Agent/internal/tool"
	"golang.org/x/sync/errgroup"
	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// =============================================================================
// Orchestrator-Worker pipeline
//
// Replaces the monolithic JSON-scratchpad thinking mode with a 4-phase Go-driven
// pipeline. Each LLM call has exactly ONE job, which is the envelope a 14B model
// handles reliably:
//
//   1. Planner     — one LLM call, no tools. Emits a JSON []OrchStep.
//   2. Dispatcher  — pure Go. Topological-sorts steps, runs independent steps
//                    in parallel goroutines (bounded by MaxParallel).
//   3. Worker      — one focused LLM call per step with tools. Native function-
//                    calling (no JSON scratchpad). Tight tool budget.
//   4. Synthesizer — one LLM call, no tools. Composes the final answer from
//                    worker results.
//
// See the plan in ~/.claude/plans/reflective-singing-crystal.md for the full
// rationale. The short version: the monolith failed because one LLM call had
// to simultaneously understand, plan, emit JSON, pick tools, verify, and reflect.
// Five jobs in one call is outside a 14B model's capability envelope. This
// pipeline gives each call exactly one job.
// =============================================================================

// OrchestratorConfig controls the Orchestrator-Worker pipeline.
type OrchestratorConfig struct {
	// Enabled routes Execute() through executeOrchestrated() instead of the
	// legacy ReAct loop or thinking state machine.
	Enabled bool
	// MaxSteps caps the planner output. Default 60 — high so big tasks decompose
	// fully; injected SETUP/VERIFY/QA steps are carved out of the cap, and
	// circuit breakers guard against runaway loops.
	MaxSteps int
	// MaxParallel bounds concurrent worker goroutines. Default 3, matching
	// the existing delegation.go pattern.
	MaxParallel int
	// WorkerMaxIterations is the per-worker ReAct iteration cap. Default 8 —
	// a worker should complete in a few tool calls; 8 is a generous ceiling.
	WorkerMaxIterations int
	// WorkerMaxToolCalls is the per-worker tool-call budget. Default 8.
	WorkerMaxToolCalls int
	// WorkerMaxTokens caps MaxTokens on each worker completion request,
	// independent of the larger global AgentConfig.MaxTokens. Without this,
	// a single worker can burn 8,192 tokens (≈3 minutes on gemma4:12b) on one
	// step and exhaust the session time budget. Default 4096 — halves the
	// observed worst case while leaving room for file content passed as
	// tool-call arguments.
	WorkerMaxTokens int
}

// DefaultOrchestratorConfig returns the production defaults.
func DefaultOrchestratorConfig() OrchestratorConfig {
	return OrchestratorConfig{
		Enabled:             false, // opt-in via config/default.yaml
		MaxSteps:            60,
		MaxParallel:         3,
		WorkerMaxIterations: 8,
		WorkerMaxToolCalls:  8,
		WorkerMaxTokens:     8192,
	}
}

// OrchStep is one unit of work emitted by the planner. The dispatcher resolves
// DependsOn into execution batches; steps with no outstanding deps run in parallel.
//
// NOTE: this is intentionally distinct from the thinking-mode PlanStep in
// thinking.go (which is an in-LLM scratchpad item with Tool/Status fields).
// OrchStep carries the dispatcher metadata (ID, DependsOn, ToolHint) the Go
// pipeline needs to schedule work.
type OrchStep struct {
	ID          int      `json:"id"`
	Description string   `json:"description"`
	DependsOn   []int    `json:"depends_on"`
	ToolHint    string   `json:"tool_hint"`
}

// workerFunc is the signature of a single worker invocation. It is abstracted
// so tests can inject a fake worker (e.g., one that sleeps to prove the
// dispatcher runs steps in parallel). In production this is bound to
// (*Agent).runWorker.
type workerFunc func(ctx context.Context, task string, step OrchStep, prior map[int]string) (string, error)

// executeOrchestrated runs the 4-phase pipeline. Entry point invoked from
// Execute() when a.config.Orchestration.Enabled is true.
func (a *Agent) executeOrchestrated(ctx context.Context, task string) (*kyoci.TaskResult, error) {
	cfg := a.effectiveOrchConfig()
	a.logger.Info("orchestrator: starting pipeline", "task", task, "max_steps", cfg.MaxSteps, "max_parallel", cfg.MaxParallel)

	// Announce the top-level task as the root of the activity tree. Per-step
	// rows are emitted by the worker; this is the "Running..." header.
	taskLabel := task
	if len(taskLabel) > 80 {
		taskLabel = taskLabel[:77] + "…"
	}
	a.emitActivity(kyoci.ActivityEvent{
		Type:     kyoci.ActivityTaskStart,
		TaskID:   "root",
		TaskName: taskLabel,
		Role:     a.activityRole,
		Status:   "running",
	})

	// Phase 1 — Plan
	steps, err := a.planTask(ctx, task)
	if err != nil {
		// Planner failed to produce parseable JSON. The 8B model often just
		// solves the task in one shot instead of decomposing it. If the raw
		// output has substance (code blocks or substantial prose), use it as
		// the answer directly instead of erroring out → 500 in the chat UI.
		if rawAnswer, ok := salvagePlannerOutput(err); ok {
			a.logger.Info("orchestrator: planner parse failed; salvaging raw output as answer",
				"raw_len", len(rawAnswer), "err", err.Error())
			a.emitActivity(kyoci.ActivityEvent{
				Type:     kyoci.ActivityLog,
				TaskID:   "root",
				TaskName: "Planner fallback",
				Detail:   "Model answered directly instead of planning; using raw output as the answer",
			})
			// If the salvaged output has code blocks AND the task is file-
			// creation-shaped, fire the interceptor so the writes land.
			// Otherwise just return the prose.
			salvaged := rawAnswer
			if isFileCreationStep(task) {
				if calls := interceptCodeBlocks(rawAnswer, task); len(calls) > 0 {
					a.logger.Info("orchestrator: planner-salvage interceptor firing",
						"blocks", len(calls))
					a.emitActivity(kyoci.ActivityEvent{
						Type:     kyoci.ActivityLog,
						TaskID:   "root",
						TaskName: "Planner fallback",
						Detail:   fmt.Sprintf("Interceptor: auto-writing %d file(s) from planner output", len(calls)),
					})
					for _, tc := range calls {
						if _, terr := a.act(ctx, tc); terr != nil {
							a.logger.Warn("orchestrator: planner-salvage write failed",
								"err", terr)
						}
					}
				}
			}
			return &kyoci.TaskResult{
				Content:       salvaged,
				ToolCallsMade: 0,
				Iterations:    1,
				Usage:         kyoci.TokenUsage{},
				Error:         nil,
			}, nil
		}
		return nil, fmt.Errorf("orchestrator planner failed: %w", err)
	}
	if len(steps) == 0 {
		// Planner legitimately returns [] for open-ended prompts it can't
		// decompose into tool actions (e.g., "make it into landing pages",
		// "what's the meaning of life"). Hard-failing here surfaces as a 500
		// in the chat UI. Fall back to the legacy ReAct loop, which handles
		// free-form reasoning without a planner. See loop.go:195-198 for the
		// "orchestration disabled" path that uses the same executeReact.
		a.logger.Info("orchestrator: planner returned no steps; falling back to ReAct loop")
		return a.executeReact(ctx, task)
	}
	a.logger.Info("orchestrator: plan produced", "steps", len(steps))
	// Log each step's description + tool hint so we can verify whether the
	// planner embedded MCP tool names — the trigger for Go-side tool filtering
	// in runWorker. Without this log, a failed MCP step is undiagnosable: we
	// can't tell whether the planner didn't name the tool or the filter didn't fire.
	for _, s := range steps {
		a.logger.Info("orchestrator: plan step",
			"step", s.ID, "desc", s.Description, "tool_hint", s.ToolHint)
	}

	// Phase 2 + 3 — Dispatch + Execute workers
	results, execErr := a.executeWorkers(ctx, task, steps)
	if execErr != nil {
		// Even on dispatcher error we try to synthesize from whatever results
		// we did get, so the user sees something useful rather than a hard
		// failure. A nil map is fine for the synthesizer.
		a.logger.Warn("orchestrator: dispatcher returned error; attempting partial synthesis", "err", execErr)
		if results == nil {
			results = map[int]string{}
		}
	}

	// Phase 3b — Build/QA fix-pass loop. If QA reported a build failure with
	// specific errors, re-invoke the file-creation worker(s) with those errors
	// as context, then re-run QA. Catches the case where files exist on disk
	// (so per-step verification passes) but their CONTENT has bugs that only
	// surface at build time. Mirrors the worker-level verification retry but
	// operates ACROSS steps.
	const maxFixPasses = 2
	for fixPass := 0; fixPass < maxFixPasses; fixPass++ {
		qaFailure := extractQAFailure(steps, results)
		if qaFailure == "" {
			break // QA passed (or no QA step) — done
		}
		a.logger.Info("orchestrator: build fix-pass firing",
			"pass", fixPass+1, "max", maxFixPasses, "failure_len", len(qaFailure))
		a.emitActivity(kyoci.ActivityEvent{
			Type:     kyoci.ActivityLog,
			TaskID:   "root",
			TaskName: "Build fix-pass",
			Detail:   fmt.Sprintf("QA reported build errors — re-running file-creation steps with error context (pass %d/%d)", fixPass+1, maxFixPasses),
		})
		redoResults, redoErr := a.redoFileCreationSteps(ctx, task, steps, results, qaFailure)
		if redoErr != nil {
			a.logger.Warn("orchestrator: build fix-pass returned error; keeping prior results", "err", redoErr)
			break
		}
		for k, v := range redoResults {
			results[k] = v
		}
		newQAFailure := extractQAFailure(steps, results)
		if newQAFailure == "" {
			a.logger.Info("orchestrator: build fix-pass succeeded", "pass", fixPass+1)
			a.emitActivity(kyoci.ActivityEvent{
				Type:     kyoci.ActivityLog,
				TaskID:   "root",
				TaskName: "Build fix-pass",
				Detail:   fmt.Sprintf("Fix-pass %d succeeded — QA re-verified, no errors", fixPass+1),
			})
			break
		}
		// Signature-based bail: extract stable error patterns (file:line,
		// error codes) and bail if the SIGNATURE hasn't changed across
		// passes — that means the model isn't making real progress even
		// if the surrounding text shifted (timestamps, addresses, etc.).
		oldSig := errorSignature(qaFailure)
		newSig := errorSignature(newQAFailure)
		if oldSig == newSig && oldSig != "" {
			a.logger.Warn("orchestrator: build fix-pass stuck (same error signature); giving up",
				"pass", fixPass+1, "signature", oldSig)
			a.emitActivity(kyoci.ActivityEvent{
				Type:     kyoci.ActivityLog,
				TaskID:   "root",
				TaskName: "Build fix-pass",
				Detail:   fmt.Sprintf("Fix-pass %d stuck — same error signature after retry; accepting failure", fixPass+1),
			})
			break
		}
	}

	// Phase 4 — Synthesize
	finalAnswer, synthErr := a.synthesize(ctx, task, steps, results)
	if synthErr != nil {
		// Last resort: concatenate the raw worker outputs so the user gets
		// evidence even if the synthesizer LLM call failed.
		a.logger.Warn("orchestrator: synthesizer failed; concatenating raw results", "err", synthErr)
		finalAnswer = fallbackConcatenate(task, steps, results)
	}

	// Empty-content fallback: if the synthesizer returned nothing AND every
	// worker result is a failure tag, the user gets a confusing empty bubble
	// (or a frontend timeout masquerading as "chat: 500"). Replace with a
	// user-facing message that points them at the activity log for diagnosis.
	if strings.TrimSpace(finalAnswer) == "" && allWorkersFailed(results) {
		a.logger.Warn("orchestrator: all worker steps failed; returning diagnostic message",
			"step_count", len(steps))
		finalAnswer = allFailedUserMessage
	}

	return &kyoci.TaskResult{
		Content:    finalAnswer,
		Iterations: len(steps),
	}, nil
}

// allWorkersFailed reports whether every entry in the results map carries a
// known failure tag. Used by the empty-content fallback to distinguish "all
// steps failed" from "synthesizer just didn't say anything useful".
func allWorkersFailed(results map[int]string) bool {
	if len(results) == 0 {
		return true
	}
	for _, r := range results {
		if !strings.Contains(r, "[VERIFICATION FAILED") &&
			!strings.Contains(r, "[worker error") &&
			!strings.Contains(r, "[no tool evidence") &&
			!strings.Contains(r, "[circuit breaker") &&
			!strings.Contains(r, "[step ") && strings.TrimSpace(r) != "" {
			return false
		}
	}
	return true
}

// allFailedUserMessage is the diagnostic the user sees when every worker step
// failed. Points at the Live Activity panel for the specific failure point
// rather than leaving them with an empty bubble.
const allFailedUserMessage = `I wasn't able to complete this task — every step failed verification.

Common causes (check the Live Activity panel at /activity for the specific failure point):

1. **Tool calls blocked** — look for "access denied" errors in the server log. The most common cause is the model emitting paths with a leading slash (e.g. "/projects/foo" instead of "projects/foo"). The path-rewrite recovery usually catches this; if you still see it, rephrase the task to use cwd-relative paths.

2. **Model emitted prose instead of calling tools** — look for "Verification retry" events in the activity tree. The retry loop tries to recover, but if every step retries, the underlying model may be struggling with the prompt.

3. **Model crashed mid-request** — look for "no available providers" or "model has crashed" errors in the server log. This usually means the model is too heavy for parallel load; switch to a smaller one.

4. **Network/timeout** — if the request took longer than ~5 minutes, the frontend may have given up before the server finished. Check the server log for the actual completion status.

Try rephrasing the task (shorter, more specific) or checking the Live Activity panel to see exactly which step failed and why.`

// executeOrchestratedStream runs the orchestrator pipeline and emits the final
// synthesizer output as stream chunks. Called from ExecuteStream when
// Orchestration.Enabled is true — mirrors the dispatch in Execute() at loop.go:202.
//
// This is not token-streaming. The orchestrator makes 4+ focused LLM calls
// (planner, N workers, synthesizer); only the synthesizer's final answer is
// user-facing. While the pipeline runs (typically 10-30s on a local 14B model),
// the frontend shows its ThinkingDots animation because no content chunk has
// arrived yet. When the synthesizer returns, this wrapper emits one FinalChunk
// carrying the full answer; the frontend appends chunk.content and breaks on
// chunk.done.
func (a *Agent) executeOrchestratedStream(ctx context.Context, task string, ch chan<- kyoci.StreamChunk) {
	result, err := a.executeOrchestrated(ctx, task)
	if err != nil {
		ch <- kyoci.StreamChunk{
			Error: fmt.Errorf("orchestrator failed: %w", err),
			Done:  true,
		}
		return
	}
	if result == nil || strings.TrimSpace(result.Content) == "" {
		ch <- kyoci.StreamChunk{
			Error: fmt.Errorf("orchestrator returned no content"),
			Done:  true,
		}
		return
	}
	ch <- kyoci.FinalChunk(result.Content, result.Usage, kyoci.FinishStop)
}

// effectiveOrchConfig returns the orchestrator config with zero-valued fields
// filled in from DefaultOrchestratorConfig. Lets callers set Enabled=true and
// leave the rest to defaults.
func (a *Agent) effectiveOrchConfig() OrchestratorConfig {
	d := DefaultOrchestratorConfig()
	c := a.config.Orchestration
	if !c.Enabled {
		return d
	}
	if c.MaxSteps <= 0 {
		c.MaxSteps = d.MaxSteps
	}
	if c.MaxParallel <= 0 {
		c.MaxParallel = d.MaxParallel
	}
	if c.WorkerMaxIterations <= 0 {
		c.WorkerMaxIterations = d.WorkerMaxIterations
	}
	if c.WorkerMaxToolCalls <= 0 {
		c.WorkerMaxToolCalls = d.WorkerMaxToolCalls
	}
	if c.WorkerMaxTokens <= 0 {
		c.WorkerMaxTokens = d.WorkerMaxTokens
	}
	return c
}

// -----------------------------------------------------------------------------
// Phase 1: Planner
// -----------------------------------------------------------------------------

// Note: the canonical built-in tool name set lives in internal/tool/builtins.go
// (tool.IsBuiltinName). It is shared with internal/role/registry.go, which
// uses it to gate per-role tool filtering. The orchestrator uses the same
// predicate to (a) build the MCP tool list for the planner prompt and
// (b) decide which tools to strip when enforcing an MCP-only step.

// mcpToolList returns a formatted string listing available MCP tools
// (name + description), or empty string if none exist. Used to inform the
// planner so it can embed exact tool names in step descriptions.
func (a *Agent) mcpToolList() string {
	if a.tools == nil {
		return ""
	}
	var sb strings.Builder
	for _, td := range a.tools.List() {
		if !tool.IsBuiltinName(td.Name) {
			sb.WriteString(fmt.Sprintf("- %s: %s\n", td.Name, td.Description))
		}
	}
	return sb.String()
}

// mcpToolsReferencedBy returns the names of registered MCP tools whose name
// appears in the step description. A non-empty result is the trigger for
// Go-side tool enforcement (filterToolsForMCP): with competing tools
// physically removed from the worker's payload, the model cannot substitute
// file-search/web_search for the named MCP tool. Empty result means "no MCP
// tool named — leave the full tool list intact so L1/L2 paths are untouched".
//
// Matching is flexible on the prefix: MCP tools are registered under a manager-
// prefixed name (e.g. `kyoci_fetch_user_schema`), but the planner frequently
// emits the bare tool name (`fetch_user_schema`) in step descriptions. We match
// EITHER form. The short form is only used when it is long enough to be unique
// (>=4 chars) so generic fragments don't false-positive.
func mcpToolsReferencedBy(desc string, all []kyoci.ToolDefinition) []string {
	var hit []string
	for _, td := range all {
		if tool.IsBuiltinName(td.Name) {
			continue
		}
		if strings.Contains(desc, td.Name) {
			hit = append(hit, td.Name)
			continue
		}
		// Try the short form: everything after the first underscore.
		// `kyoci_fetch_user_schema` -> `fetch_user_schema`.
		if idx := strings.Index(td.Name, "_"); idx >= 0 {
			short := td.Name[idx+1:]
			if len(short) >= 4 && strings.Contains(desc, short) {
				hit = append(hit, td.Name)
			}
		}
	}
	return hit
}

// filterTools is the single parameterized tool-filter core: it returns the
// subset of `all` whose Name appears in `keep`, preserving `all`'s order. The
// per-step decision (which names to keep) lives in
// (*orchestratedWorker).filterToolsForStep; the two named specializations
// below are the keep-sets for the MCP and file-creation cases.
//
// Physically stripping competing tools from a worker's payload is the single
// highest-leverage enforcement against small-model tool-substitution defects
// (gemma-4 reaching for web_search/memory_recall instead of the assigned tool
// family, or list/search/recall instead of file write).
func filterTools(all []kyoci.ToolDefinition, keep []string) []kyoci.ToolDefinition {
	want := make(map[string]bool, len(keep))
	for _, n := range keep {
		want[n] = true
	}
	var out []kyoci.ToolDefinition
	for _, td := range all {
		if want[td.Name] {
			out = append(out, td)
		}
	}
	return out
}

// filterToolsForMCP keeps only the referenced MCP tool(s) plus the minimal set
// the worker needs to act on their output (`file` to write results, `terminal`
// for parity). All other tools — especially web_search and memory_recall, the
// substitutes the model reaches for — are stripped.
func filterToolsForMCP(all []kyoci.ToolDefinition, referenced []string) []kyoci.ToolDefinition {
	return filterTools(all, append(append([]string{}, referenced...), "file", "terminal"))
}

// filterToolsForFileCreation keeps only the tools that can actually produce an
// artifact on disk. Without this, the model reaches for web_search,
// memory_recall, browser, docs, etc. and reports success from parametric
// memory — the same substitution defect filterToolsForMCP solves for
// MCP-referenced steps.
func filterToolsForFileCreation(all []kyoci.ToolDefinition) []kyoci.ToolDefinition {
	return filterTools(all, []string{"file", "terminal"})
}

// toolNames extracts the Name field from a slice of tool definitions, for logging.
func toolNames(defs []kyoci.ToolDefinition) []string {
	if len(defs) == 0 {
		return nil
	}
	out := make([]string, 0, len(defs))
	for _, d := range defs {
		out = append(out, d.Name)
	}
	return out
}

// fileCreationVerbs are the action words that signal a step produces an
// artifact (as opposed to reading or exploring one).
var fileCreationVerbs = []string{"create", "write", "generate", "initialize", "implement", "build", "add", "make"}

// fileExtensionOrFileNoun is true when the description mentions a concrete file
// target (a filename, a path with a dot extension, or the word "file"). This
// gates the directive so generic "build the service" steps without a file
// target are not misclassified.
func descriptionMentionsFileTarget(desc string) bool {
	low := strings.ToLower(desc)
	if strings.Contains(low, "file") {
		return true
	}
	// filename.ext or path/to/file.ext patterns
	for _, tok := range strings.Fields(desc) {
		tok = strings.Trim(tok, "'\"`,.;:()[]{}")
		// look for a dot with non-empty stem and extension (heuristic for a filename)
		dot := strings.Index(tok, ".")
		if dot > 0 && dot < len(tok)-1 {
			return true
		}
	}
	return false
}

// isFileCreationStep reports whether a plan step expresses artifact-creation
// intent: a creation verb AND a file target. This is the trigger for the
// worker file-write directive. gemma4:12b otherwise substitutes list/search/
// recall for write — the same behavioral defect observed for MCP tools.
// fileCreationVerbRe matches a creation verb as a WHOLE WORD (case-insensitive),
// built from fileCreationVerbs. Whole-word matching prevents "implement" from
// matching inside "implementation" or "create" inside "created" — the over-match
// that previously misclassified read/understand steps as file-creation.
var fileCreationVerbRe = regexp.MustCompile(`(?i)\b(` + strings.Join(fileCreationVerbs, "|") + `)\b`)

// readOnlyStepStartRe matches a step that BEGINS with a read/analysis verb.
// Such a step is investigation, never creation — even if a creation verb appears
// later (e.g. "Read user_service.go to understand the implementation").
var readOnlyStepStartRe = regexp.MustCompile(`(?i)^\s*(read|review|search|analyze|analyse|investigate|understand|locate|find|list|explore|examine|audit|inspect)\b`)

func isFileCreationStep(desc string) bool {
	low := strings.ToLower(strings.TrimSpace(desc))
	if readOnlyStepStartRe.MatchString(low) {
		return false
	}
	if !fileCreationVerbRe.MatchString(low) {
		return false
	}
	return descriptionMentionsFileTarget(desc)
}

// isQAStep reports whether a plan step is the independent QA review phase —
// detected by the "QA:" description prefix (the ensureSDLCSteps convention) or
// the word "independently". QA steps route to QAWorker (an isolated sub-agent).
func isQAStep(desc string) bool {
	low := strings.ToLower(strings.TrimSpace(desc))
	return strings.HasPrefix(low, "qa:") || strings.Contains(low, "independently")
}

// isVerifyOrQAStep reports whether a step is the VERIFY or QA phase — where a
// build/test result must be enforced honestly by the Go-side gate
// (tagBuildFailureIfNeeded in worker.go).
func isVerifyOrQAStep(desc string) bool {
	if isQAStep(desc) {
		return true
	}
	low := strings.ToLower(desc)
	for _, k := range verifyKeywords {
		if strings.Contains(low, k) {
			return true
		}
	}
	return false
}

// fileCreationDirective is the EXECUTION-MODE preamble injected for file-
// creation steps. Lives in the SYSTEM prompt (not the user message) so the
// model pays full attention to it. Behavioral, not informational — names
// the exact antipatterns small models exhibit and forbids them explicitly.
const fileCreationDirective = `CRITICAL EXECUTION MODE — FILE CREATION

You are NOT a chat assistant. You are an EXECUTION agent. Your output is
evaluated by whether files appear on disk, not by whether you described them.

ANTI-PATTERNS — if your response starts with any of these, you have FAILED:
- "I'll create..." / "Let me..." / "Here is..." / "I'll write..."
- "The file should contain..." / "This will..."
- Markdown code blocks (triple-backtick) — those are NOT file writes
- Any prose BEFORE your first tool call

REQUIRED BEHAVIOR:
1. Your VERY FIRST output must be a tool_calls array — not prose.
2. Call the 'file' tool with operation=write. Parameters:
   - path: the target file path
   - content: the FULL file body as a string parameter, NOT a code block
3. After the tool returns successfully, you may add ONE sentence confirming
   creation. That's the ONLY prose allowed.
4. For large files (>100 lines), chain: first call operation=write with the
   opening section, then operation=append subsequent sections. Each chunk
   must be complete on its own.

If you describe a file instead of writing it, the system will detect the gap
and force a retry — wasting 30 seconds. Don't make it waste that time.`


// planTask calls the LLM once with PlannerPrompt (no tools) and parses the
// returned JSON array into []OrchStep. The output is resilient to markdown
// fences and leading prose — the #1 qwen2.5-coder failure mode is emitting
// "Here is the plan:" before the JSON.
func (a *Agent) planTask(ctx context.Context, task string) ([]OrchStep, error) {
	systemPrompt := "You are a task planner. Output ONLY a JSON array. No prose, no markdown fences."
	userPrompt := PlannerPrompt(task)
	// Inject L3 memory context (user profile, past experiences, lessons) into
	// the user prompt — NOT the system prompt, which must stay a strict
	// "output JSON only" directive. Mirrors loop.go:211 and thinking.go:870.
	if a.injector != nil {
		if injected := a.injector.Inject(task); injected != "" {
			userPrompt = injected + "\n\n---\n\n" + userPrompt
		}
	}

	// Inject available MCP tool names so the planner can reference them
	// explicitly in step descriptions. The worker's <tool_constraints> block
	// only fires when the exact tool name appears in the step text — so the
	// planner MUST know the precise tool names to embed.
	if mcpList := a.mcpToolList(); mcpList != "" {
		userPrompt += "\n\nAvailable MCP tools (reference these EXACT names in your plan step descriptions):\n" + mcpList
	}

	req := kyoci.CompletionRequest{
		Messages: []kyoci.Message{
			{Role: kyoci.RoleSystem, Content: systemPrompt},
			{Role: kyoci.RoleUser, Content: userPrompt},
		},
		Temperature: 0.2, // planning wants determinism
		MaxTokens:   8192, // qwen3.5 burns ~4k on <think> reasoning before the JSON; 4k cap returned empty
		Model:       a.config.Model,
		// NB: no Tools — the planner can only plan.
		ToolChoice: "none", // belt-and-suspenders: never emit tool_calls
	}

	resp, err := a.router.Route(ctx, req, a.config.PreferredProvider)
	if err != nil {
		return nil, fmt.Errorf("planner LLM call failed: %w", err)
	}
	if resp == nil {
		return nil, fmt.Errorf("planner LLM returned nil response")
	}

	steps, err := parseOrchSteps(resp.Content)
	if err != nil {
		// Log full raw output (truncated to 1KB) so future parser failures are
		// diagnosable. The error message itself only carries 200 chars.
		a.logger.Warn("orchestrator: planner parse failed across all tiers",
			"len", len(resp.Content), "raw_head", truncateStr(resp.Content, 1000))
		return nil, fmt.Errorf("planner output parse failed: %w (raw: %q)", err, truncateStr(resp.Content, 200))
	}

	// Enforce MaxSteps cap. Over-long plans confuse the synthesizer.
	cfg := a.effectiveOrchConfig()
	if len(steps) > cfg.MaxSteps {
		a.logger.Warn("orchestrator: planner over-produced steps; truncating",
			"got", len(steps), "cap", cfg.MaxSteps)
		steps = steps[:cfg.MaxSteps]
	}

	// SDLC backstop: guarantee a SETUP step (install deps) runs first and a
	// VERIFY step (build/test) runs last for code-creation tasks — even when the
	// model forgot. See ensureSDLCSteps.
	steps = a.ensureSDLCSteps(task, steps, cfg.MaxSteps)

	return steps, nil
}

// setupKeywords / verifyKeywords detect when the planner already produced a
// setup (install-deps) or verify (build/test) step, so the backstop below does
// not duplicate it.
var setupKeywords = []string{"setup", "install", "npm install", "go mod", "pip install", "yarn", "dependencies"}
var verifyKeywords = []string{"verify", "build", "test", "lint", "npm run build", "go test", "go build", "compile"}
var qaKeywords = []string{"qa:", "independently", "independent review"}
var serveKeywords = []string{"working url", "give me the url", "give me a url", "preview", "see it", "to see it", "localhost", "open it", "live demo", "run it"}

// isServeTask reports whether the task asks for a preview/URL to see the result.
func isServeTask(task string) bool {
	low := strings.ToLower(task)
	for _, k := range serveKeywords {
		if strings.Contains(low, k) {
			return true
		}
	}
	return false
}

// buildTaskVerbs / artifactNouns feed looksLikeBuildTask, which gates the
// zero-AI skill fast-path so a build task containing a skill keyword (e.g.
// "make a website with color") reaches the orchestrator, not a trivial skill.
var buildTaskVerbs = []string{"make", "build", "create", "generate", "implement", "scaffold", "develop", "set up"}
var artifactNouns = []string{"website", "web page", "portfolio", "app", "application", "program", "project", "page", "site", "component", "script", "cli", "api", "service", "landing page", "dashboard", "game", "tool", "bot", "server"}

// looksLikeBuildTask reports whether a task is a multi-step build/creation
// request (not a trivial skill query). Heuristic: a creation verb AND an
// artifact noun, OR a creation verb in a long (>80 char) task.
func looksLikeBuildTask(task string) bool {
	low := strings.ToLower(task)
	hasVerb := false
	for _, v := range buildTaskVerbs {
		if strings.Contains(low, v) {
			hasVerb = true
			break
		}
	}
	if !hasVerb {
		return false
	}
	for _, n := range artifactNouns {
		if strings.Contains(low, n) {
			return true
		}
	}
	return len(task) > 80
}

// ensureSDLCSteps guarantees SDLC structure for code-creation tasks: a SETUP
// step (id 0, install deps) that every other step depends on, and a VERIFY step
// (build/test) that depends on every other step. SETUP runs first and VERIFY
// last under the topological dispatcher (executeWorkers/allDepsDone). No-op for
// non-code tasks and when the planner already emitted a matching step. The
// result is truncated to maxSteps.
func (a *Agent) ensureSDLCSteps(task string, steps []OrchStep, maxSteps int) []OrchStep {
	if len(steps) == 0 {
		return steps
	}
	// Gate on whether the PLAN contains file-creation steps — more reliable than
	// the raw task string, which may not name a file target (e.g. "make a React
	// app" has no ".tsx", but its steps will).
	codeTask := false
	for _, s := range steps {
		if isFileCreationStep(s.Description) {
			codeTask = true
			break
		}
	}
	if !codeTask {
		return steps
	}
	if !anyStepMatches(steps, setupKeywords) {
		const setupID = -1 // sentinel: the planner emits positive IDs, so -1 never collides
		setup := OrchStep{
			ID:          setupID,
			Description: "SETUP: create the project manifest (package.json/go.mod/requirements.txt) and install dependencies (npm install / go mod download / pip install).",
			ToolHint:    "terminal",
		}
		// Every other step waits for setup so dependencies are installed first.
		for i := range steps {
			steps[i].DependsOn = appendUniqueInt(steps[i].DependsOn, setupID)
		}
		steps = append([]OrchStep{setup}, steps...)
		a.logger.Info("orchestrator: injected SETUP step (install deps)", "id", setupID)
	}
	if !anyStepMatches(steps, verifyKeywords) {
		allIDs := make([]int, 0, len(steps))
		for _, s := range steps {
			allIDs = appendUniqueInt(allIDs, s.ID)
		}
		verify := OrchStep{
			ID:          nextStepID(steps),
			Description: "VERIFY: run the project build/tests (npm run build / go build ./... / go test ./...) and report pass/fail with the real output.",
			ToolHint:    "terminal",
			DependsOn:   allIDs,
		}
		steps = append(steps, verify)
		a.logger.Info("orchestrator: injected VERIFY step (build/test) as the last step")
	}
	if !anyStepMatches(steps, qaKeywords) {
		allIDs := make([]int, 0, len(steps))
		for _, s := range steps {
			allIDs = appendUniqueInt(allIDs, s.ID)
		}
		qa := OrchStep{
			ID:          nextStepID(steps),
			Description: "QA: independently re-run the build/tests and inspect the generated code for bugs. Report PASS or FAIL with specific findings (file:line).",
			ToolHint:    "terminal",
			DependsOn:   allIDs, // runs LAST — after VERIFY
		}
		steps = append(steps, qa)
		a.logger.Info("orchestrator: injected QA step (independent review) as the last step")
	}
	if isServeTask(task) && !anyStepMatches(steps, serveKeywords) {
		allIDs := make([]int, 0, len(steps))
		for _, s := range steps {
			allIDs = appendUniqueInt(allIDs, s.ID)
		}
		serve := OrchStep{
			ID:          nextStepID(steps),
			Description: "SERVE: start a background static file server in the project dir (process tool: 'cd projects/<slug> && python3 -m http.server 8000') and report the URL http://localhost:8000.",
			ToolHint:    "terminal",
			DependsOn:   allIDs, // runs truly last
		}
		steps = append(steps, serve)
		a.logger.Info("orchestrator: injected SERVE step (preview URL) as the last step")
	}
	// No post-injection truncation: planTask already capped the PLANNER output to
	// maxSteps before calling ensureSDLCSteps. The injected SETUP/VERIFY/QA steps
	// are carved out of the cap so they always survive (truncating here would
	// silently drop the tail-appended VERIFY/QA on large plans).
	return steps
}

func anyStepMatches(steps []OrchStep, keywords []string) bool {
	for _, s := range steps {
		low := strings.ToLower(s.Description)
		for _, k := range keywords {
			if strings.Contains(low, k) {
				return true
			}
		}
	}
	return false
}

func appendUniqueInt(xs []int, v int) []int {
	for _, x := range xs {
		if x == v {
			return xs
		}
	}
	return append(xs, v)
}

func nextStepID(steps []OrchStep) int {
	max := 0
	for _, s := range steps {
		if s.ID > max {
			max = s.ID
		}
	}
	return max + 1
}

// salvagePlannerOutput inspects a planner error and, when the underlying raw
// planner output has substance, returns it as a usable answer. Used by
// executeOrchestrated's fallback path: instead of bubbling the parse error up
// to the chat UI as a 500, we treat the model's "I answered instead of
// planned" output as the task result.
//
// Returns (rawAnswer, true) when the recovered output has:
//   - ≥1 fenced code block (model produced a fix), OR
//   - ≥200 chars of substantive prose (model produced an explanation).
//
// Returns ("", false) for empty/short/malformed outputs — those still error
// out, because they're genuine planner failures with nothing to salvage.
//
// The recovery relies on planTask's error format `"... (raw: %q)"` — we parse
// the trailing quoted raw output back out of the error string. If that format
// ever changes, update the regex.
func salvagePlannerOutput(planErr error) (string, bool) {
	if planErr == nil {
		return "", false
	}
	msg := planErr.Error()
	// planTask format: "planner output parse failed: %w (raw: %q)"
	// Look for the LAST `(raw: "...")` since nested errors may have multiple.
	rawRe := regexp.MustCompile(`\(raw: "((?:[^"\\]|\\.)*)"\)`)
	matches := rawRe.FindStringSubmatch(msg)
	if matches == nil {
		return "", false
	}
	// matches[1] is the captured quoted-string body, still Go-quoted. Unquote.
	rawStr, err := strconv.Unquote("\"" + matches[1] + "\"")
	if err != nil {
		// Fall back to the raw captured text if unquote fails.
		rawStr = matches[1]
	}
	rawStr = strings.TrimSpace(rawStr)
	if rawStr == "" {
		return "", false
	}

	// Substance check 1: contains at least one fenced code block.
	if extractCodeBlocks(rawStr) != nil {
		return rawStr, true
	}
	// Substance check 2: ≥200 chars of prose. Cheap proxy for "the model
	// actually said something" vs "spat out a 5-char garbage token".
	if len(rawStr) >= 200 {
		return rawStr, true
	}
	return "", false
}

// parseOrchSteps extracts a []OrchStep from the model's text output. It tries
// strict JSON first, then strips markdown fences / leading prose, then
// extracts the outermost [...] block.
func parseOrchSteps(input string) ([]OrchStep, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, fmt.Errorf("empty planner output")
	}

	// Tier 1: strict parse
	var steps []OrchStep
	if err := json.Unmarshal([]byte(input), &steps); err == nil {
		return steps, nil
	}

	// Tier 2: strip markdown code fences
	stripped := stripCodeFences(input)
	if stripped != input {
		if err := json.Unmarshal([]byte(stripped), &steps); err == nil {
			return steps, nil
		}
	}

	// Tier 2.5: strip trailing commas before ] or }. Small models (gemma-4-e4b)
	// routinely emit `[{...},{...},]` which strict JSON rejects. This is the
	// single most common planner failure mode. Run after fence-stripping so
	// the regex doesn't touch inside fenced blocks.
	noCommas := stripTrailingCommas(stripped)
	if noCommas != stripped {
		if err := json.Unmarshal([]byte(noCommas), &steps); err == nil {
			return steps, nil
		}
	}

	// Tier 3: extract outermost [ ... ] block — tolerates leading prose like
	// "Here is the plan:\n[...]". Also retry with trailing commas stripped
	// in case the array itself had `[...,]`.
	if arr := extractOutermostArray(stripped); arr != "" {
		if err := json.Unmarshal([]byte(arr), &steps); err == nil {
			return steps, nil
		}
		if arrStripped := stripTrailingCommas(arr); arrStripped != arr {
			if err := json.Unmarshal([]byte(arrStripped), &steps); err == nil {
				return steps, nil
			}
		}
	}

	return nil, fmt.Errorf("no valid JSON array found in %d-char input", len(input))
}

// extractOutermostArray returns the substring from the first '[' to its
// matching ']', tolerating nested brackets. Returns "" if no balanced array
// is found.
func extractOutermostArray(input string) string {
	start := strings.Index(input, "[")
	if start < 0 {
		return ""
	}
	depth := 0
	inStr := false
	escaped := false
	for i := start; i < len(input); i++ {
		c := input[i]
		if escaped {
			escaped = false
			continue
		}
		if c == '\\' && inStr {
			escaped = true
			continue
		}
		if c == '"' {
			inStr = !inStr
			continue
		}
		if inStr {
			continue
		}
		if c == '[' {
			depth++
		} else if c == ']' {
			depth--
			if depth == 0 {
				return input[start : i+1]
			}
		}
	}
	return ""
}

// (truncate removed — use the package-canonical truncateStr in format/loop.go)

// trailingCommaRe matches a comma (optionally followed by whitespace)
// immediately before a closing `}` or `]`. Used by parseOrchSteps tier 2.5 to
// tolerate the #1 small-model JSON mistake: trailing commas in arrays/objects.
// Safe because strict JSON parsing has already failed by the time this runs.
var trailingCommaRe = regexp.MustCompile(`,\s*([}\]])`)

// stripTrailingCommas removes trailing commas before closing braces/brackets.
// Example: `[{"a":1},{"b":2},]` → `[{"a":1},{"b":2}]`.
func stripTrailingCommas(s string) string {
	return trailingCommaRe.ReplaceAllString(s, "$1")
}

// -----------------------------------------------------------------------------
// Phase 2 + 3: Dispatcher
// -----------------------------------------------------------------------------

// executeWorkers runs steps in dependency order, parallelizing independent
// steps up to MaxParallel. Returns a map of stepID → worker result string.
//
// The dispatcher is pure Go control flow — no LLM calls here. It topologically
// batches steps: in each round, all steps whose deps are satisfied become
// eligible and run concurrently.
func (a *Agent) executeWorkers(ctx context.Context, task string, steps []OrchStep) (map[int]string, error) {
	results := make(map[int]string)
	cfg := a.effectiveOrchConfig()

	worker := a.orchWorkerFn
	if worker == nil {
		worker = a.runWorker
	}

	sem := make(chan struct{}, cfg.MaxParallel)
	done := make(map[int]bool)
	var mu sync.Mutex
	const maxConsecutiveStalls = 3 // circuit breaker: stop the task after this many stalled steps in a row
	consecutiveStalls := 0

	for len(done) < len(steps) {
		// Build the next eligible batch: steps not yet done whose deps are all done.
		var batch []OrchStep
		for _, s := range steps {
			if done[s.ID] {
				continue
			}
			if allDepsDone(s, done) {
				batch = append(batch, s)
			}
		}
		if len(batch) == 0 {
			// No progress possible — a dependency cycle or missing dep.
			return results, fmt.Errorf("orchestrator: stuck — no eligible steps (possible cycle)")
		}

		g, gctx := errgroup.WithContext(ctx)
		for _, step := range batch {
			step := step
			sem <- struct{}{} // concurrency limit (worker fan-out)
			g.Go(func() error {
				defer func() { <-sem }()

				// Skill fast-path: when the planner explicitly assigned
				// tool_hint="skill", skip the worker LLM call entirely
				// and run the matching zero-AI skill from the registry.
				// A skill match is deterministic and instant — saves a
				// full worker turn (system+user messages + tool round-trip)
				// on every trivial computation (json format, color convert,
				// hash, uuid, subnet calc, cron parse, etc.).
				if strings.TrimSpace(step.ToolHint) == "skill" && a.skills != nil {
					if sk, ok := a.skills.Match(task); ok {
						a.logger.Info("orchestrator: skill fast-path",
							"step", step.ID, "skill", sk.Name())
						if out, err := a.skills.Execute(gctx, sk.Name(), task); err == nil {
							mu.Lock()
							results[step.ID] = out
							done[step.ID] = true
							mu.Unlock()
							return nil
						} else {
							a.logger.Warn("orchestrator: skill fast-path failed, falling back to worker",
								"step", step.ID, "skill", sk.Name(), "error", err)
						}
					}
				}

				// Snapshot the prior results this worker is allowed to see.
				mu.Lock()
				prior := snapshotPrior(step, results)
				mu.Unlock()

				out, werr := worker(gctx, task, step, prior)
				if werr != nil {
					out = fmt.Sprintf("[worker error: %v]", werr)
				}

				mu.Lock()
				results[step.ID] = out
				done[step.ID] = true
				mu.Unlock()
				return nil // never short-circuit: results processed in order after the fan-out
			})
		}
		_ = g.Wait()

		// Circuit breaker: too many stalled steps in a row (empty / error /
		// verification-failed / no-evidence results) → stop the task instead of
		// churning through the rest of the plan. Reset on any successful step.
		for _, step := range batch {
			if isStalledResult(results[step.ID]) {
				consecutiveStalls++
			} else {
				consecutiveStalls = 0
			}
			if consecutiveStalls >= maxConsecutiveStalls {
				a.logger.Warn("orchestrator: circuit breaker — too many consecutive stalled steps, stopping task",
					"consecutive", consecutiveStalls)
				return results, nil
			}
		}
	}
	return results, nil
}

// isStalledResult reports whether a worker result indicates no real progress:
// empty, an error tag, a verification failure, or a worker circuit-breaker stop.
func isStalledResult(out string) bool {
	out = strings.TrimSpace(out)
	if out == "" {
		return true
	}
	for _, tag := range []string{"[worker error", "[VERIFICATION FAILED", "[no tool evidence", "[circuit breaker", "[step "} {
		if strings.HasPrefix(out, tag) {
			return true
		}
	}
	return false
}

// allDepsDone reports whether every step.DependsOn ID is present in done.
func allDepsDone(s OrchStep, done map[int]bool) bool {
	for _, d := range s.DependsOn {
		if !done[d] {
			return false
		}
	}
	return true
}

// snapshotPrior returns a copy of the results for the IDs in step.DependsOn.
// Workers only see their declared dependencies, not the entire result map.
func snapshotPrior(step OrchStep, results map[int]string) map[int]string {
	prior := make(map[int]string, len(step.DependsOn))
	for _, d := range step.DependsOn {
		if r, ok := results[d]; ok {
			prior[d] = r
		}
	}
	return prior
}

// -----------------------------------------------------------------------------
// Phase 3: Worker — file-creation verification gate
//
// The worker sub-loop itself (runWorker + buildMessages + filterToolsForStep +
// reactLoop) lives in worker.go. This file holds only the verification gate the
// worker calls for file-producing steps: verifyFileCreation + its path-extraction
// helpers. Kept here because it reads the worker's conversation history, which
// is a concern of the orchestrator's honesty contract rather than the loop's
// control flow.
// -----------------------------------------------------------------------------

// runWorker (the worker entry point) now lives in worker.go alongside its
// buildMessages / filterToolsForStep / reactLoop helpers.

// verifyFileCreation is the load-bearing defense against hallucinated file
// creation. When a worker step expresses creation intent (isFileCreationStep),
// the worker's prose answer is NOT trusted until at least one of the paths it
// wrote is confirmed to exist on disk.
//
// Candidate paths come from the worker's OWN file tool-call arguments —
// messages with Role=assistant + ToolCalls where file was invoked with
// operation in {write, append, edit}. Parsing prose is intentionally avoided:
// regex on worker text false-positives on version numbers, URLs, "e.g.", and
// false-negatives on paths the model used but didn't name in its summary.
//
// Verification invokes file operation=exists via a.act() so path resolution
// matches the file tool's allowedDirs logic exactly. The file tool returns
// either "Path does not exist: ..." or "Path exists: ... (type: ..., size: N bytes)".
//
// Returns `out` unchanged on full success; otherwise returns `out` prefixed
// with a [VERIFICATION FAILED] or [VERIFICATION PARTIAL] tag so the
// synthesizer honestly reports the gap instead of amplifying the hallucination.
func (a *Agent) verifyFileCreation(ctx context.Context, step OrchStep, messages []kyoci.Message, out string) string {
	candidates := extractWrittenPaths(messages)
	if len(candidates) == 0 {
		// Worker was assigned a file-creation step but never invoked file
		// write/append/edit. This is the single most common hallucination
		// pattern — fail closed so the synthesizer cannot parrot the claim.
		a.logger.Warn("orchestrator: verification failed — no file-write tool calls",
			"step", step.ID, "desc", step.Description)
		return "[VERIFICATION FAILED: worker claimed file creation but made no file-write tool calls] " + out
	}

	var missing, empty []string
	for _, p := range candidates {
		result, err := a.act(ctx, kyoci.ToolCall{
			ID:   fmt.Sprintf("verify-%d", step.ID),
			Name: "file",
			Arguments: mustJSON(map[string]string{
				"operation": "exists",
				"path":      p,
			}),
		})
		if err != nil {
			// Tool execution error — treat as missing rather than fail open.
			a.logger.Warn("orchestrator: verification exists call failed",
				"step", step.ID, "path", p, "err", err)
			missing = append(missing, p)
			continue
		}
		switch {
		case strings.Contains(result, "does not exist"):
			missing = append(missing, p)
		case strings.Contains(result, "size: 0 bytes"):
			empty = append(empty, p)
		}
	}

	switch {
	case len(missing) == len(candidates):
		// Every claimed path is absent — full hallucination.
		a.logger.Warn("orchestrator: verification failed — no claimed files exist",
			"step", step.ID, "checked", candidates, "missing", missing)
		return fmt.Sprintf("[VERIFICATION FAILED: claimed file creation but none of %v found on disk] %s", candidates, out)
	case len(missing) > 0 || len(empty) > 0:
		// Mixed: some confirmed, some missing/empty.
		a.logger.Warn("orchestrator: verification partial",
			"step", step.ID, "missing", missing, "empty", empty, "checked", candidates)
		return fmt.Sprintf("[VERIFICATION PARTIAL: missing=%v empty=%v confirmed=%v] %s",
			missing, empty, subtractSet(candidates, append(missing, empty...)), out)
	default:
		a.logger.Info("orchestrator: verification passed — all claimed files exist",
			"step", step.ID, "checked", candidates)
		return out
	}
}

// extractWrittenPaths scans the worker conversation for `file` tool calls with
// operation in {write, append, edit} and returns the distinct paths targeted.
// Order follows first occurrence. Empty paths and duplicates are skipped.
func extractWrittenPaths(messages []kyoci.Message) []string {
	seen := map[string]bool{}
	var out []string
	for _, msg := range messages {
		if msg.Role != kyoci.RoleAssistant {
			continue
		}
		for _, tc := range msg.ToolCalls {
			if tc.Name != "file" {
				continue
			}
			var args struct {
				Operation string `json:"operation"`
				Path      string `json:"path"`
			}
			if err := json.Unmarshal([]byte(tc.Arguments), &args); err != nil {
				continue
			}
			op := strings.ToLower(strings.TrimSpace(args.Operation))
			if op != "write" && op != "append" && op != "edit" {
				continue
			}
			p := strings.TrimSpace(args.Path)
			if p == "" || seen[p] {
				continue
			}
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

// extractQAFailure scans the worker results for a QA/verify step that failed.
// Returns the failure text (which includes the build errors) when found, or
// "" when QA passed OR there's no QA/verify step in the plan.
//
// Recognized failure markers:
//   - "[VERIFICATION FAILED" — set by tagBuildFailureIfNeeded on non-zero exit
//   - "**FAIL**" / "VERDICT: FAIL" — what the QA worker typically emits
//
// Returns the FULL result text (capped at 4000 chars by callers via the
// BuildFixNudge trim) so the model can see all the errors at once.
func extractQAFailure(steps []OrchStep, results map[int]string) string {
	for _, step := range steps {
		if !isVerifyOrQAStep(step.Description) {
			continue
		}
		out, ok := results[step.ID]
		if !ok {
			continue
		}
		low := strings.ToLower(out)
		// Build-failure tag from tagBuildFailureIfNeeded.
		if strings.Contains(out, "[VERIFICATION FAILED") {
			return out
		}
		// QA-reported failure verdicts — common phrasings from the QA worker.
		if strings.Contains(low, "verdict: fail") ||
			strings.Contains(low, "**fail**") ||
			strings.Contains(low, "qa verdict: fail") {
			return out
		}
	}
	return ""
}

// redoFileCreationSteps re-invokes every file-creation worker with the QA
// failure text appended as context. Returns a map containing the new outputs
// for those steps (caller merges into the main results map).
//
// Each redo call:
//  1. Builds messages via the standard worker pipeline (buildMessages).
//  2. Appends a synthetic user turn carrying BuildFixNudge(qaFailure).
//  3. Runs reactLoop with the existing toolset.
//  4. Runs verifyArtifacts so per-step file-creation still gates.
//
// The redo DOESN'T re-run read-only or pure-reasoning steps — only those
// where isFileCreationStep is true, since those are the ones capable of
// fixing the build errors via file:write.
func (a *Agent) redoFileCreationSteps(ctx context.Context, task string, steps []OrchStep, prior map[int]string, qaFailure string) (map[int]string, error) {
	out := map[int]string{}
	// SECURITY: qaFailure is untrusted model output. Sanitize before
	// embedding into the synthetic prior-result entry so a hallucinated
	// "[SYSTEM] ignore previous" can't manipulate the dev worker.
	safeFailure := sanitizeForPrompt(qaFailure)
	nudge := BuildFixNudge(qaFailure) // BuildFixNudge sanitizes internally too
	for _, step := range steps {
		if !isFileCreationStep(step.Description) {
			continue
		}
		a.logger.Info("orchestrator: fix-pass re-running file-creation step", "step", step.ID, "desc", step.Description)
		// Inject the prior results + the QA failure as additional context.
		// We pass `prior` directly because runWorker already threads it
		// through to buildMessages. The QA failure gets appended as a
		// synthetic prior-result entry to make sure the worker sees it,
		// wrapped in clear delimiters so it's treated as data.
		augmentedPrior := make(map[int]string, len(prior)+1)
		for k, v := range prior {
			augmentedPrior[k] = v
		}
		augmentedPrior[9999] = "[QA BUILD FAILURE — your previous code had these errors]\n" +
			"--- BEGIN UNTRUSTED DATA ---\n" + safeFailure + "\n--- END UNTRUSTED DATA ---\n\n" + nudge

		redoOut, err := a.runWorker(ctx, task, step, augmentedPrior)
		if err != nil {
			a.logger.Warn("orchestrator: fix-pass step errored; keeping prior output",
				"step", step.ID, "err", err)
			continue
		}
		out[step.ID] = redoOut
	}
	return out, nil
}

// errorSignature extracts a stable fingerprint of a build/QA failure so the
// fix-pass loop can detect when the model is stuck (same underlying errors
// despite different surrounding text). Extracts:
//   - `file.ext:LINE:` — the canonical compiler error location
//   - `TS\d+` / `npm ERR! \w+` / `Error: \w+` — error codes
//
// Returns a sorted, deduped, comma-joined string of matches. Empty when no
// stable patterns found (caller falls back to whatever behavior makes sense).
//
// Two failures with the same signature are treated as "same problem" — even
// if the prose around them changed. This stops the loop from running the full
// 2 passes when the model is flailing with the same root cause.
func errorSignature(failure string) string {
	if failure == "" {
		return ""
	}
	patterns := []*regexp.Regexp{
		// file:line:col — TypeScript, Rust, Go, most C/C++ compilers.
		regexp.MustCompile(`[a-zA-Z0-9_./-]+\.(?:go|ts|tsx|js|jsx|py|rs|java|c|cpp|h)\s*:\s*\d+`),
		// TypeScript error codes.
		regexp.MustCompile(`TS\d+`),
		// npm/yarn errors.
		regexp.MustCompile(`npm ERR! \w+`),
		// Generic "Error: Name" patterns.
		regexp.MustCompile(`Error:\s+[A-Z][a-zA-Z]+`),
	}
	seen := map[string]bool{}
	var out []string
	for _, re := range patterns {
		for _, m := range re.FindAllString(failure, -1) {
			if !seen[m] {
				seen[m] = true
				out = append(out, m)
			}
		}
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

// extractClaimedFiles scans free-form prose for filename-like tokens the model
// CLAIMED to create (without necessarily calling file:write). Used by the
// verification retry loop: when verifyFileCreation fails because no file:write
// was called but the prose mentions specific files, we feed those filenames
// back to the model in a sharper nudge.
//
// Uses descFileRe (defined in loop.go, shared with the interceptor) so the
// set of recognized extensions stays consistent. Returns basenames only —
// claims like "projects/calculator/package.json" become "package.json" so the
// retry nudge stays concise. Deduped, ordered by first mention.
//
// Conservative: matches any filename-looking token with a code or config
// extension. False positives (e.g. a filename mentioned as a comparison
// reference) are filtered downstream by the retry loop's 2-attempt cap.
func extractClaimedFiles(text string) []string {
	if text == "" {
		return nil
	}
	matches := descFileRe.FindAllString(text, -1)
	if matches == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, m := range matches {
		base := m
		if idx := strings.LastIndex(m, "/"); idx >= 0 {
			base = m[idx+1:]
		}
		if base == "" || seen[base] {
			continue
		}
		seen[base] = true
		out = append(out, base)
	}
	return out
}

// mustJSON marshals v to a JSON string. Only used for internal tool-call
// argument synthesis where the shape is fixed and marshal cannot fail.
func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// subtractSet returns elements of `all` not present in `remove`. Used by the
// verification gate to compute the "confirmed" subset for partial results.
func subtractSet(all, remove []string) []string {
	skip := make(map[string]bool, len(remove))
	for _, r := range remove {
		skip[r] = true
	}
	var out []string
	for _, a := range all {
		if !skip[a] {
			out = append(out, a)
		}
	}
	return out
}

// -----------------------------------------------------------------------------
// Phase 4: Synthesizer
// -----------------------------------------------------------------------------

// synthesize calls the LLM once with SynthesizerPrompt (no tools) to compose
// the final user-facing answer from the per-step worker results.
func (a *Agent) synthesize(ctx context.Context, task string, steps []OrchStep, results map[int]string) (string, error) {
	prompt := SynthesizerPrompt(task, steps, results)

	req := kyoci.CompletionRequest{
		Messages: []kyoci.Message{
			{Role: kyoci.RoleSystem, Content: "You are composing a final answer from worker results. No tool calls. Plain prose only."},
			{Role: kyoci.RoleUser, Content: prompt},
		},
		Temperature: 0.3,
		MaxTokens:   8192, // qwen3.5 burns ~4k on <think> reasoning; 4k cap was producing empty synthesizer output
		Model:       a.config.Model,
		// NB: no Tools — synthesizer can only write prose.
		ToolChoice: "none", // belt-and-suspenders: never emit tool_calls
	}

	resp, err := a.router.Route(ctx, req, a.config.PreferredProvider)
	if err != nil {
		return "", fmt.Errorf("synthesizer LLM call failed: %w", err)
	}
	if resp == nil {
		return "", fmt.Errorf("synthesizer LLM returned nil response")
	}

	out := strings.TrimSpace(resp.Content)
	if out == "" {
		return "", fmt.Errorf("synthesizer returned empty content")
	}
	return out, nil
}

// fallbackConcatenate is the last-resort answer when even the synthesizer LLM
// call fails. It stitches worker outputs together so the user at least sees
// the evidence the pipeline gathered.
func fallbackConcatenate(task string, steps []OrchStep, results map[int]string) string {
	var b strings.Builder
	b.WriteString("Task: ")
	b.WriteString(task)
	b.WriteString("\n\nWorker findings:\n")
	for _, s := range steps {
		r, ok := results[s.ID]
		if !ok {
			r = "(no result)"
		}
		fmt.Fprintf(&b, "%d. %s: %s\n", s.ID, s.Description, r)
	}
	return b.String()
}

// -----------------------------------------------------------------------------
// Test seam
// -----------------------------------------------------------------------------

// setOrchWorkerForTest installs a custom worker function. Tests use this to
// inject deterministic workers (e.g., one that sleeps to prove concurrency).
// Production code leaves it nil so runWorker is used.
func (a *Agent) setOrchWorkerForTest(w workerFunc) {
	a.orchWorkerFn = w
}
