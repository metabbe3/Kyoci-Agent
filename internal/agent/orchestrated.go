package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"

	"github.com/metabbe3/Kyoci-Agent/internal/tool"
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
	// MaxSteps caps the planner output. Default 6 — beyond this the plan is
	// usually over-decomposed and the synthesizer struggles to use it all.
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
		MaxSteps:            6,
		MaxParallel:         3,
		WorkerMaxIterations: 8,
		WorkerMaxToolCalls:  8,
		WorkerMaxTokens:     4096,
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

	// Phase 1 — Plan
	steps, err := a.planTask(ctx, task)
	if err != nil {
		return nil, fmt.Errorf("orchestrator planner failed: %w", err)
	}
	if len(steps) == 0 {
		return nil, fmt.Errorf("orchestrator planner returned no steps")
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

	// Phase 4 — Synthesize
	finalAnswer, synthErr := a.synthesize(ctx, task, steps, results)
	if synthErr != nil {
		// Last resort: concatenate the raw worker outputs so the user gets
		// evidence even if the synthesizer LLM call failed.
		a.logger.Warn("orchestrator: synthesizer failed; concatenating raw results", "err", synthErr)
		finalAnswer = fallbackConcatenate(task, steps, results)
	}

	return &kyoci.TaskResult{
		Content:    finalAnswer,
		Iterations: len(steps),
	}, nil
}

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

// filterTools returns the subset of `all` whose Name appears in `keep`. Order
// follows `all`. Used to physically strip competing tools from a worker's
// payload so the model cannot substitute away from the assigned tool family
// — the single highest-leverage enforcement against small-model tool-substitution
// defects (gemma-4 reaching for web_search/memory_recall instead of file write).
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

// filterToolsForMCP returns a tool list containing only the referenced MCP
// tool(s) plus the minimal set the worker needs to act on their output
// (`file` to write results, `terminal` for parity). All other tools —
// especially web_search and memory_recall, the substitutes the model reaches
// for — are stripped.
func filterToolsForMCP(all []kyoci.ToolDefinition, referenced []string) []kyoci.ToolDefinition {
	keep := append([]string{}, referenced...)
	keep = append(keep, "file", "terminal")
	return filterTools(all, keep)
}

// filterToolsForFileCreation restricts the worker payload to the tools that
// can actually produce an artifact on disk. Without this, the model reaches
// for web_search, memory_recall, browser, docs, etc. and reports success from
// parametric memory — the same substitution defect filterToolsForMCP solves
// for MCP-referenced steps.
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
var fileCreationVerbs = []string{"create", "write", "generate", "initialize", "implement", "build", "add"}

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
func isFileCreationStep(desc string) bool {
	low := strings.ToLower(desc)
	creates := false
	for _, v := range fileCreationVerbs {
		if strings.Contains(low, v) {
			creates = true
			break
		}
	}
	if !creates {
		return false
	}
	return descriptionMentionsFileTarget(desc)
}

// fileCreationDirective is the proactive nudge injected into a file-creation
// step's user content. It tells the model concretely which tool op to use so
// it doesn't reach for list/search/recall.
const fileCreationDirective = `FILE-CREATION DIRECTIVE: This step requires the 'file' tool with operation=write to produce a new artifact.
- Do NOT use operation=list, operation=search, operation=read, or memory_recall as your final action — those gather context, they do not create the file.
- When ready, emit a single file tool call shaped like: {"operation":"write","path":"<the target path>","content":"<the full file contents>"}.
- Only after the write returns successfully, report what you created in one or two sentences.`


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
		MaxTokens:   4096, // reasoning models (Gemma) burn ~2k tokens on CoT before the JSON
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
			"len", len(resp.Content), "raw_head", truncate(resp.Content, 1000))
		return nil, fmt.Errorf("planner output parse failed: %w (raw: %q)", err, truncate(resp.Content, 200))
	}

	// Enforce MaxSteps cap. Over-long plans confuse the synthesizer.
	cfg := a.effectiveOrchConfig()
	if len(steps) > cfg.MaxSteps {
		a.logger.Warn("orchestrator: planner over-produced steps; truncating",
			"got", len(steps), "cap", cfg.MaxSteps)
		steps = steps[:cfg.MaxSteps]
	}

	return steps, nil
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

// truncate is a small helper for error messages so we don't dump huge model
// outputs into logs.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

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

		var wg sync.WaitGroup
		for _, step := range batch {
			wg.Add(1)
			sem <- struct{}{}
			go func(s OrchStep) {
				defer wg.Done()
				defer func() { <-sem }()

				// Skill fast-path: when the planner explicitly assigned
				// tool_hint="skill", skip the worker LLM call entirely
				// and run the matching zero-AI skill from the registry.
				// A skill match is deterministic and instant — saves a
				// full worker turn (system+user messages + tool round-trip)
				// on every trivial computation (json format, color convert,
				// hash, uuid, subnet calc, cron parse, etc.).
				if strings.TrimSpace(s.ToolHint) == "skill" && a.skills != nil {
					if sk, ok := a.skills.Match(task); ok {
						a.logger.Info("orchestrator: skill fast-path",
							"step", s.ID, "skill", sk.Name())
						if out, err := a.skills.Execute(ctx, sk.Name(), task); err == nil {
							mu.Lock()
							results[s.ID] = out
							done[s.ID] = true
							mu.Unlock()
							return
						} else {
							a.logger.Warn("orchestrator: skill fast-path failed, falling back to worker",
								"step", s.ID, "skill", sk.Name(), "error", err)
						}
					}
				}

				// Snapshot the prior results this worker is allowed to see.
				mu.Lock()
				prior := snapshotPrior(s, results)
				mu.Unlock()

				out, werr := worker(ctx, task, s, prior)
				if werr != nil {
					out = fmt.Sprintf("[worker error: %v]", werr)
				}

				mu.Lock()
				results[s.ID] = out
				done[s.ID] = true
				mu.Unlock()
			}(step)
		}
		wg.Wait()
	}
	return results, nil
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
// Phase 3: Worker (one focused LLM call per step)
// -----------------------------------------------------------------------------

// runWorker executes ONE step as a focused LLM call with native function-calling.
// No JSON scratchpad, no thinking state machine — the model emits tool_calls
// directly (or a plain-text finding). A tight ReAct-style loop handles any
// tool calls and re-prompts until the model produces a final answer or the
// iteration cap is hit.
//
// runWorker is the production worker. Tests may inject a.orchWorkerFn to
// bypass it for dispatcher-focused tests.
func (a *Agent) runWorker(ctx context.Context, task string, step OrchStep, prior map[int]string) (string, error) {
	cfg := a.effectiveOrchConfig()

	// Build the initial conversation: system + user with step + prior context.
	userContent := fmt.Sprintf("Step %d: %s\n\nOriginal task: %s", step.ID, step.Description, task)
	// Layer 1b: surface the planner's tool_hint so the model has a concrete
	// default rather than having to choose a tool family from the registry.
	// Empty hint → "any" signals a pure-reasoning step (see WorkerSystemPrompt EXCEPTION).
	hintLabel := strings.TrimSpace(step.ToolHint)
	if hintLabel == "" {
		hintLabel = "any (pure-reasoning step — you may answer without tools)"
	}
	userContent += "\n\nExpected tool family for this step: " + hintLabel
	if len(prior) > 0 {
		userContent += "\n\nPrior context from earlier steps:\n"
		for id, r := range prior {
			userContent += fmt.Sprintf("- step %d result: %s\n", id, truncate(r, 400))
		}
	}
	userContent += "\n\nComplete this step now using the available tools. When done, report your findings in one or two sentences."

	// File-creation directive: when the step expresses creation intent,
	// proactively steer the model toward the `file` tool's write operation.
	// Without this, gemma4:12b substitutes list/search/recall and never
	// produces the artifact — the same substitution defect observed for MCP
	// tools before Go-side enforcement was added.
	if isFileCreationStep(step.Description) {
		userContent += "\n\n" + fileCreationDirective
		a.logger.Info("orchestrator: file-creation directive injected", "step", step.ID)
	}

	// Inject L3 memory context (user profile, past experiences, lessons) into
	// the worker system prompt — appended AFTER WorkerSystemPrompt so the
	// strict output contract stays the leading signal. Mirrors loop.go:210-214.
	systemPrompt := WorkerSystemPrompt
	if a.injector != nil {
		if injected := a.injector.Inject(task); injected != "" {
			systemPrompt = systemPrompt + "\n\n" + injected
		}
	}

	messages := []kyoci.Message{
		{Role: kyoci.RoleSystem, Content: systemPrompt},
		{Role: kyoci.RoleUser, Content: userContent},
	}

	// Build tool definitions (native function-calling). When the step
	// description references an MCP tool by name, enforce Go-side tool
	// filtering: with competing tools (web_search, memory_recall, etc.)
	// physically removed from the payload, the model cannot substitute them
	// for the named MCP tool — the #1 failure mode that prompt constraints
	// alone could not fix in gemma4:12b. File-creation steps get the same
	// treatment for the same reason — without enforcement the model substitutes
	// list/search/recall for write and then hallucinates success. When neither
	// condition applies, the full tool list is used so L1/L2 paths are unaffected.
	var toolDefs []kyoci.ToolDefinition
	if a.tools != nil {
		all := a.tools.List()
		referenced := mcpToolsReferencedBy(step.Description, all)
		switch {
		case len(referenced) > 0:
			toolDefs = filterToolsForMCP(all, referenced)
			a.logger.Info("orchestrator: worker tool list filtered for MCP step",
				"step", step.ID, "mcp_tools", referenced, "kept", toolNames(toolDefs))
		case isFileCreationStep(step.Description):
			toolDefs = filterToolsForFileCreation(all)
			a.logger.Info("orchestrator: worker tool list filtered for file-creation step",
				"step", step.ID, "kept", toolNames(toolDefs))
		default:
			toolDefs = all
		}
	}

	maxIter := cfg.WorkerMaxIterations
	toolCallsMade := 0
	nudged := false // Layer 2: evidence guard fires at most once per worker

	for iter := 0; iter < maxIter; iter++ {
		req := kyoci.CompletionRequest{
			Messages:    messages,
			Temperature: a.config.Temperature,
			MaxTokens:   cfg.WorkerMaxTokens,
			Model:       a.config.Model,
			Tools:       toolDefs,
		}
		// Layer 3c: on the FIRST turn of an evidence step, ask the provider to
		// force a tool call. qwen2.5-coder:14b ignores this (it emits text-JSON),
		// but Layer 2 catches that case — this is the belt-and-suspenders layer
		// for providers that DO honor tool_choice. On later turns, leave it
		// "" (auto) so the model can decide when it has enough evidence.
		// Fires when either the planner assigned a tool_hint OR the step was
		// detected as file-creation (which needs the `file` write op specifically).
		if iter == 0 && (strings.TrimSpace(step.ToolHint) != "" || isFileCreationStep(step.Description)) {
			req.ToolChoice = "required"
		}

		resp, err := a.router.Route(ctx, req, a.config.PreferredProvider)
		if err != nil {
			return "", fmt.Errorf("worker LLM call failed (iter %d): %w", iter, err)
		}
		if resp == nil {
			return "", fmt.Errorf("worker LLM returned nil response (iter %d)", iter)
		}

		// No tool calls → the model wants to terminate. Decide whether to accept.
		if len(resp.ToolCalls) == 0 {
			out := strings.TrimSpace(resp.Content)
			if out == "" {
				out = fmt.Sprintf("[step %d produced no output]", step.ID)
			}

			// Layer 2: evidence guard. A worker whose step carries a non-empty
			// tool_hint OR expresses file-creation intent must gather real
			// evidence (a tool call) before answering. On the FIRST turn, if
			// the model answered from memory, inject the WorkerEvidenceNudge
			// once and re-run instead of accepting. This is the load-bearing
			// fix: qwen2.5-coder:14b otherwise answers every step from
			// parametric memory and the synthesizer honestly reports
			// "did not find" because no file was ever read. The file-creation
			// case extends the same defense to steps the planner didn't assign
			// a tool_hint for but which clearly need a file write.
			hint := strings.TrimSpace(step.ToolHint)
			needsEvidence := hint != "" || isFileCreationStep(step.Description)
			if iter == 0 && needsEvidence && !nudged {
				nudgeHint := hint
				if nudgeHint == "" {
					nudgeHint = "file"
				}
				messages = append(messages, kyoci.Message{
					Role:    kyoci.RoleAssistant,
					Content: out,
				})
				messages = append(messages, kyoci.Message{
					Role:    kyoci.RoleUser,
					Content: WorkerEvidenceNudge(nudgeHint),
				})
				nudged = true
				a.logger.Info("orchestrator: worker evidence nudge injected",
					"step", step.ID, "tool_hint", nudgeHint)
				continue
			}

			// Already nudged (or past turn 0) and the model STILL refuses tools.
			// Accept the answer so the pipeline makes progress, but tag it so the
			// synthesizer can honestly report that this step produced no observed
			// evidence — the user sees the gap instead of mistaking memory for fact.
			// The file-creation case is also caught here; the verification gate
			// below will additionally rewrite the output with [VERIFICATION FAILED].
			if nudged && needsEvidence && toolCallsMade == 0 {
				out = "[no tool evidence — answer is from model memory] " + out
			}

			a.logger.Info("orchestrator: worker done",
				"step", step.ID, "iters", iter+1, "tool_calls", toolCallsMade, "nudged", nudged)
			if isFileCreationStep(step.Description) {
				out = a.verifyFileCreation(ctx, step, messages, out)
			}
			return out, nil
		}

		// Append the assistant message (with tool_calls) to the conversation.
		messages = append(messages, kyoci.Message{
			Role:      kyoci.RoleAssistant,
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		// Execute each tool call and append the results as tool messages.
		for _, tc := range resp.ToolCalls {
			toolCallsMade++
			if toolCallsMade > cfg.WorkerMaxToolCalls {
				// Budget hit — return what we have so far.
				a.logger.Warn("orchestrator: worker hit tool budget",
					"step", step.ID, "budget", cfg.WorkerMaxToolCalls)
				return fmt.Sprintf("[step %d hit tool budget; partial findings in conversation]", step.ID), nil
			}
			result, terr := a.act(ctx, tc)
			if terr != nil {
				result = fmt.Sprintf("[tool %s error: %v]", tc.Name, terr)
			}
			messages = append(messages, kyoci.Message{
				Role:       kyoci.RoleTool,
				Content:    result,
				ToolCallID: tc.ID,
				Name:       tc.Name,
			})
		}
	}

	// Iteration cap hit without a final answer. Return the last content as the
	// finding — better than failing outright.
	a.logger.Warn("orchestrator: worker hit iteration cap",
		"step", step.ID, "cap", maxIter)
	lastContent := ""
	if len(messages) > 0 {
		lastContent = messages[len(messages)-1].Content
	}
	if strings.TrimSpace(lastContent) == "" {
		lastContent = fmt.Sprintf("[step %d did not produce a final finding within %d iterations]", step.ID, maxIter)
	}
	return lastContent, nil
}

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
		MaxTokens:   4096, // reasoning models (Gemma) burn tokens on CoT before the prose
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

// keepLogger warms the unused-import guard for slog if future helpers need it.
var _ = slog.Default()
