package agent

import (
	"context"
	"fmt"
	"strings"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// =============================================================================
// Orchestrator worker (Phase 3)
//
// runWorker is the production orchestrator worker: it builds the worker's
// message context, filters the tool set to the step's needs, runs a focused
// sub-agent loop, and (for file-producing steps) verifies the artifact was
// actually created. Tests inject a.orchWorkerFn to bypass it for
// dispatcher-focused tests. Lives in worker.go to keep orchestrated.go focused
// on planning/dispatch/synthesis.
//
// The work is split across an orchestratedWorker helper value with focused
// responsibilities so runWorker stays a thin shim:
//   - buildMessages      assembles the system+user conversation for the step
//   - filterToolsForStep collapses MCP / file-creation / default filtering into
//                        one parameterized decision
//   - reactLoop          the per-worker ReAct sub-loop (evidence nudge + tools)
// runWorker preserves the orchWorkerFn test seam exactly: same signature, same
// observable message sequence, same logging.
// =============================================================================

// runWorker is the production worker. Tests may inject a.orchWorkerFn to
// bypass it for dispatcher-focused tests.
func (a *Agent) runWorker(ctx context.Context, task string, step OrchStep, prior map[int]string) (string, error) {
	// QA step: run as an ISOLATED sub-agent (fresh context, read+terminal tools,
	// QA system prompt) so it independently verifies the deliverable without
	// seeing the implement worker's claims. "Never trust the author."
	if isQAStep(step.Description) {
		out, err := a.QAWorker(ctx, task)
		if err != nil {
			return out, err
		}
		return tagBuildFailureIfNeeded(step, nil, out), nil
	}

	w := &orchestratedWorker{agent: a, task: task, step: step, prior: prior, cfg: a.effectiveOrchConfig()}

	messages := w.buildMessages()
	toolDefs, filterReason := w.filterToolsForStep()
	if filterReason != "" {
		a.logger.Info("orchestrator: worker tool list filtered",
			"step", step.ID, "reason", filterReason, "kept", toolNames(toolDefs))
	}

	// reactLoop returns the FULLY-GROWN message slice (including the assistant
	// tool-call messages it appended during the loop) so verifyArtifacts can see
	// the file-write calls and verify them. Passing the original `messages` here
	// previously made every file-creation step false-fail verification.
	grownMessages, out, err := w.reactLoop(ctx, messages, toolDefs)
	if err != nil {
		return "", err
	}
	// verifyArtifacts preserves the verifyFileCreation test seam: it is the
	// load-bearing defense against hallucinated file creation, run only for
	// file-creation steps after the sub-loop returns.
	out = w.verifyArtifacts(ctx, grownMessages, out)
	return tagBuildFailureIfNeeded(step, grownMessages, out), nil
}

// tagBuildFailureIfNeeded enforces build/test honesty: if this is a VERIFY or QA
// step and any terminal command it ran exited non-zero, prepend a failure tag.
// This is a GO-SIDE check (independent of the model's claim) — a failing build
// can never be reported as success.
func tagBuildFailureIfNeeded(step OrchStep, messages []kyoci.Message, out string) string {
	if !isVerifyOrQAStep(step.Description) {
		return out
	}
	if !containsNonZeroExit(messages, out) {
		return out
	}
	return "[VERIFICATION FAILED: a build/test command exited non-zero] " + out
}

// containsNonZeroExit reports whether any tool-result message Content or the
// worker's final output carries the terminal's non-zero-exit marker.
func containsNonZeroExit(messages []kyoci.Message, out string) bool {
	if strings.Contains(out, "[exit_status: non-zero") {
		return true
	}
	for _, m := range messages {
		if strings.Contains(m.Content, "[exit_status: non-zero") {
			return true
		}
	}
	return false
}

// orchestratedWorker carries the per-step inputs that every phase of the
// worker needs. Constructed once per runWorker call; not shared across steps.
type orchestratedWorker struct {
	agent *Agent
	task  string
	step  OrchStep
	prior map[int]string
	cfg   OrchestratorConfig
}

// buildMessages assembles the initial worker conversation: the worker system
// prompt (with L3 memory appended) and a user turn carrying the step, the
// planner's tool hint, prior-step results, and (for file-creation steps) the
// file-write directive.
func (w *orchestratedWorker) buildMessages() []kyoci.Message {
	step, task := w.step, w.task

	// Layer 1b: surface the planner's tool_hint so the model has a concrete
	// default rather than having to choose a tool family from the registry.
	// Empty hint → "any" signals a pure-reasoning step (see WorkerSystemPrompt EXCEPTION).
	hintLabel := strings.TrimSpace(step.ToolHint)
	if hintLabel == "" {
		hintLabel = "any (pure-reasoning step — you may answer without tools)"
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Step %d: %s\n\nOriginal task: %s", step.ID, step.Description, task)
	sb.WriteString("\n\nExpected tool family for this step: " + hintLabel)
	if len(w.prior) > 0 {
		sb.WriteString("\n\nPrior context from earlier steps:\n")
		for id, r := range w.prior {
			fmt.Fprintf(&sb, "- step %d result: %s\n", id, truncateStr(r, 400))
		}
	}
	sb.WriteString("\n\nComplete this step now using the available tools. When done, report your findings in one or two sentences.")

	// File-creation directive: when the step expresses creation intent,
	// proactively steer the model toward the `file` tool's write operation.
	// Without this, gemma4:12b substitutes list/search/recall and never
	// produces the artifact — the same substitution defect observed for MCP
	// tools before Go-side enforcement was added.
	if isFileCreationStep(step.Description) {
		sb.WriteString("\n\n" + fileCreationDirective)
		w.agent.logger.Info("orchestrator: file-creation directive injected", "step", step.ID)
	}

	// Inject L3 memory context (user profile, past experiences, lessons) into
	// the worker system prompt — appended AFTER WorkerSystemPrompt so the
	// strict output contract stays the leading signal. Mirrors loop.go:210-214.
	systemPrompt := WorkerSystemPrompt
	if w.agent.injector != nil {
		if injected := w.agent.injector.Inject(task); injected != "" {
			systemPrompt = systemPrompt + "\n\n" + injected
		}
	}

	return []kyoci.Message{
		{Role: kyoci.RoleSystem, Content: systemPrompt},
		{Role: kyoci.RoleUser, Content: sb.String()},
	}
}

// filterToolsForStep is the single parameterized tool-filter for the worker.
// It decides, from the step description + tool hint, which tools survive into
// the worker's function-calling payload:
//
//   - MCP-referenced step → only the named MCP tool(s) + file + terminal
//     (physically removing web_search/memory_recall so the model cannot
//     substitute away from the assigned tool family).
//   - file-creation step  → only file + terminal (same substitution defense
//     for write vs list/search/recall).
//   - otherwise           → the full tool list (L1/L2 paths untouched).
//
// Returns the filtered defs plus a short reason string for logging (empty when
// no filtering applied, i.e. the default full-list case).
func (w *orchestratedWorker) filterToolsForStep() ([]kyoci.ToolDefinition, string) {
	if w.agent.tools == nil {
		return nil, ""
	}
	all := w.agent.tools.List()
	referenced := mcpToolsReferencedBy(w.step.Description, all)
	switch {
	case len(referenced) > 0:
		return filterToolsForMCP(all, referenced), fmt.Sprintf("mcp tools=%v", referenced)
	case isFileCreationStep(w.step.Description):
		return filterToolsForFileCreation(all), "file-creation"
	default:
		return all, ""
	}
}

// reactLoop runs the focused ReAct sub-loop for one step: prompt the model,
// honor tool calls, apply the one-shot evidence nudge, and terminate when the
// model emits a tool-free answer or a budget is hit. Returns the worker's
// finding string. The message sequence is prompt-sensitive — see the thinking
// tests — so this preserves the exact append order of the legacy loop.
func (w *orchestratedWorker) reactLoop(ctx context.Context, messages []kyoci.Message, toolDefs []kyoci.ToolDefinition) ([]kyoci.Message, string, error) {
	a, step, cfg := w.agent, w.step, w.cfg
	maxIter := cfg.WorkerMaxIterations
	toolCallsMade := 0
	tokensUsed := 0
	nudged := false   // Layer 2: evidence guard fires at most once per worker
	lastToolSig := "" // circuit breaker: consecutive identical tool calls
	toolSigRepeat := 0

	// Announce this worker as a new row in the activity tree. TaskID is the
	// step ID; TaskName is the step description; ParentID stays empty for
	// top-level orchestrator steps (set by delegation for sub-agent rows).
	a.emitActivity(kyoci.ActivityEvent{
		Type:     kyoci.ActivityTaskStart,
		TaskID:   fmt.Sprintf("step-%d", step.ID),
		TaskName: step.Description,
		Status:   "running",
	})

	// needsEvidence is constant for the loop: a step whose tool_hint is set OR
	// which expresses file-creation intent must gather real evidence (a tool
	// call) before answering.
	hint := strings.TrimSpace(step.ToolHint)
	needsEvidence := hint != "" || isFileCreationStep(step.Description)

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
		if iter == 0 && needsEvidence {
			req.ToolChoice = "required"
		}

		resp, err := a.router.Route(ctx, req, a.config.PreferredProvider)
		if err != nil {
			a.emitActivity(kyoci.ActivityEvent{
				Type:     kyoci.ActivityTaskComplete,
				TaskID:   fmt.Sprintf("step-%d", step.ID),
				TaskName: step.Description,
				Status:   "error",
				Detail:   fmt.Sprintf("LLM call failed: %v", err),
			})
			return messages, "", fmt.Errorf("worker LLM call failed (iter %d): %w", iter, err)
		}
		if resp == nil {
			return messages, "", fmt.Errorf("worker LLM returned nil response (iter %d)", iter)
		}
		// Surface running metrics for the activity tree. Some providers only
		// report usage on the final chunk; we accumulate what we have.
		tokensUsed += int(resp.Usage.TotalTokens)
		a.emitActivity(kyoci.ActivityEvent{
			Type:       kyoci.ActivityTaskProgress,
			TaskID:     fmt.Sprintf("step-%d", step.ID),
			TaskName:   step.Description,
			ToolUses:   toolCallsMade,
			TokensUsed: tokensUsed,
			Status:     "running",
		})

		// No tool calls → the model wants to terminate. Decide whether to accept.
		if len(resp.ToolCalls) == 0 {
			out := strings.TrimSpace(resp.Content)
			if out == "" {
				out = fmt.Sprintf("[step %d produced no output]", step.ID)
			}

			// 8B-optimization Strategy 2: code-block interceptor. If the model
			// emitted markdown code blocks instead of calling file:write, AND
			// this is a file-creation step, synthesize the writes on its behalf.
			// Combats the "chatty developer" failure mode where small models
			// default to Markdown output. Conservative: only fires on
			// isFileCreationStep and only when we can guess a filename.
			if isFileCreationStep(step.Description) && !nudged {
				if synthCalls := interceptCodeBlocks(out, step.Description); len(synthCalls) > 0 {
					a.logger.Info("orchestrator: code-block interceptor firing",
						"step", step.ID, "blocks", len(synthCalls))
					a.emitActivity(kyoci.ActivityEvent{
						Type:     kyoci.ActivityLog,
						TaskID:   fmt.Sprintf("step-%d", step.ID),
						TaskName: step.Description,
						Detail:   fmt.Sprintf("Interceptor: model emitted %d code block(s) as markdown; auto-writing", len(synthCalls)),
					})
					for _, tc := range synthCalls {
						result, terr := a.act(ctx, tc)
						if terr != nil {
							a.logger.Warn("orchestrator: interceptor write failed",
								"step", step.ID, "err", terr)
							result = fmt.Sprintf("[interceptor write failed: %v]", terr)
						}
						messages = append(messages, kyoci.Message{
							Role:       kyoci.RoleTool,
							Content:    result,
							ToolCallID: tc.ID,
							Name:       tc.Name,
						})
						toolCallsMade++
					}
					// Continue the loop so the model can verify the writes and
					// produce its final finding — don't accept `out` as final yet.
					continue
				}
			}

			// Layer 2: evidence guard. On the FIRST turn, if the model answered
			// from memory, inject the WorkerEvidenceNudge once and re-run instead
			// of accepting. This is the load-bearing fix: qwen2.5-coder:14b
			// otherwise answers every step from parametric memory and the
			// synthesizer honestly reports "did not find" because no file was
			// ever read.
			if iter == 0 && needsEvidence && !nudged {
				nudgeHint := hint
				if nudgeHint == "" {
					nudgeHint = "file"
				}
				messages = append(messages, kyoci.Message{
					Role:    kyoci.RoleAssistant,
					Content: out,
				}, kyoci.Message{
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
			if nudged && needsEvidence && toolCallsMade == 0 {
				out = "[no tool evidence — answer is from model memory] " + out
			}

			a.logger.Info("orchestrator: worker done",
				"step", step.ID, "iters", iter+1, "tool_calls", toolCallsMade, "nudged", nudged)
			a.emitActivity(kyoci.ActivityEvent{
				Type:       kyoci.ActivityTaskComplete,
				TaskID:     fmt.Sprintf("step-%d", step.ID),
				TaskName:   step.Description,
				ToolUses:   toolCallsMade,
				TokensUsed: tokensUsed,
				Status:     "done",
			})
			return messages, out, nil
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
				return messages, fmt.Sprintf("[partial: step %d hit tool budget — partial findings in conversation]", step.ID), nil
			}
			// Circuit breaker: a worker stuck repeating the EXACT same tool call
			// (same name + args) is in a loop (e.g. hammering a failing command).
			// After 3 consecutive identical calls, finalize the worker.
			sig := tc.Name + ":" + tc.Arguments
			if sig == lastToolSig {
				toolSigRepeat++
			} else {
				toolSigRepeat = 1
				lastToolSig = sig
			}
			if toolSigRepeat >= 3 {
				a.logger.Warn("orchestrator: circuit breaker — repeated tool call, finalizing worker",
					"step", step.ID, "tool", tc.Name)
				return messages, fmt.Sprintf("[circuit breaker: step %d stopped — repeated tool call %q]", step.ID, tc.Name), nil
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
	return messages, lastContent, nil
}

// verifyArtifacts runs the file-creation verification gate for file-producing
// steps. It delegates to verifyFileCreation (the test-seen name) so the gate's
// behavior and its direct tests stay anchored on one implementation.
//
// RETRY LOOP: when verification fails because the model emitted prose claiming
// file creation but didn't call file:write, this method re-invokes the worker
// with a sharper, file-specific nudge (VerificationRetryNudge). The model
// sees its own claimed filenames and the verification failure, then has another
// chance to actually emit the tool calls. Cap at 2 retries so we don't burn
// the worker's iteration budget on a model that won't comply.
func (w *orchestratedWorker) verifyArtifacts(ctx context.Context, messages []kyoci.Message, out string) string {
	if !isFileCreationStep(w.step.Description) {
		return out
	}
	tagged := w.agent.verifyFileCreation(ctx, w.step, messages, out)
	if !strings.HasPrefix(tagged, "[VERIFICATION FAILED") {
		return tagged
	}

	// Failure: claimed files but didn't write them. Try to recover.
	const maxRetries = 2
	currentMessages := messages
	currentOut := out
	currentTagged := tagged
	seenClaims := map[string]bool{} // bail if the same set repeats

	for attempt := 0; attempt < maxRetries; attempt++ {
		claimed := extractClaimedFiles(currentOut)
		if len(claimed) == 0 {
			break // can't sharpen without targets
		}

		// Bail if the model claimed the same set last time (stuck).
		claimKey := strings.Join(claimed, ",")
		if seenClaims[claimKey] {
			w.agent.logger.Warn("orchestrator: verification retry — same claims as prior attempt; giving up",
				"step", w.step.ID, "claims", claimed)
			break
		}
		seenClaims[claimKey] = true

		w.agent.logger.Info("orchestrator: verification retry",
			"step", w.step.ID, "attempt", attempt+1, "claimed_count", len(claimed))
		w.agent.emitActivity(kyoci.ActivityEvent{
			Type:     kyoci.ActivityLog,
			TaskID:   fmt.Sprintf("step-%d", w.step.ID),
			TaskName: w.step.Description,
			Detail:   fmt.Sprintf("Verification retry %d/%d: model claimed %d file(s) in prose but didn't call file:write — sharpening prompt", attempt+1, maxRetries, len(claimed)),
		})

		// Append the failure context as a tool message (so the model sees
		// the [VERIFICATION FAILED] tag) + the sharper nudge as a user turn.
		retryMessages := append(append([]kyoci.Message{}, currentMessages...),
			kyoci.Message{
				Role:    kyoci.RoleTool,
				Content: currentTagged,
				Name:    "verification",
			},
			kyoci.Message{
				Role:    kyoci.RoleUser,
				Content: VerificationRetryNudge(claimed),
			},
		)

		// Re-filter tools (deterministic — returns the same set as the original
		// worker invocation) and re-run the loop.
		toolDefs, _ := w.filterToolsForStep()
		// Re-run the loop; verify against the GROWN messages it returns (which
		// include the retry's file-write tool calls), not the pre-loop retryMessages.
		retryGrown, retryOut, err := w.reactLoop(ctx, retryMessages, toolDefs)
		if err != nil {
			w.agent.logger.Warn("orchestrator: verification retry failed (reactLoop error)",
				"step", w.step.ID, "attempt", attempt+1, "err", err)
			break
		}

		retryTagged := w.agent.verifyFileCreation(ctx, w.step, retryGrown, retryOut)
		if !strings.HasPrefix(retryTagged, "[VERIFICATION FAILED") {
			w.agent.logger.Info("orchestrator: verification retry succeeded",
				"step", w.step.ID, "attempt", attempt+1)
			w.agent.emitActivity(kyoci.ActivityEvent{
				Type:     kyoci.ActivityLog,
				TaskID:   fmt.Sprintf("step-%d", w.step.ID),
				TaskName: w.step.Description,
				Detail:   fmt.Sprintf("Verification retry %d succeeded — files now on disk", attempt+1),
			})
			return retryTagged
		}

		// Still failing — extract any newly-claimed files and try once more.
		// Build the next retry on the grown messages (includes prior writes).
		currentMessages = retryGrown
		currentOut = retryOut
		currentTagged = retryTagged
	}

	w.agent.logger.Warn("orchestrator: verification retries exhausted",
		"step", w.step.ID, "retries", maxRetries)
	w.agent.emitActivity(kyoci.ActivityEvent{
		Type:     kyoci.ActivityLog,
		TaskID:   fmt.Sprintf("step-%d", w.step.ID),
		TaskName: w.step.Description,
		Detail:   fmt.Sprintf("Verification failed after %d retries — accepting failure", maxRetries),
	})
	return currentTagged
}
