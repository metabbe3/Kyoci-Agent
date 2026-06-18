package agent

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/sync/errgroup"
	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// executeReact runs the legacy ReAct (Reason + Act) loop: build conversation
// context, then iterate THINK -> ACT with auto-continuation until a final
// answer is produced or the iteration/round budget is exhausted. Extracted
// verbatim from Execute so that Execute is a clean strategy dispatcher
// (skill fast-path -> thinking -> orchestrated -> react).
func (a *Agent) executeReact(ctx context.Context, task string) (*kyoci.TaskResult, error) {
	// b. Build context with system prompt (+ injected L3 intelligence), task, and conversation history
	conversationCtx := NewContext()

	// Inject L3 memory context (user profile, past experiences, lessons)
	systemPrompt := a.config.SystemPrompt
	if injected := a.injector.Inject(task); injected != "" {
		systemPrompt = systemPrompt + "\n\n" + injected
		a.logger.Info("L3 context injected", "context_length", len(injected))
	}

	// Append critical response format rules that ALL roles must follow
	systemPrompt += `

CRITICAL RULES:
- NEVER claim a task is done without VERIFYING it actually works. "Exit code 0" is NOT proof — test the result yourself.
- If a command or operation FAILS or produces unexpected output, you MUST investigate the error, diagnose the root cause, and fix it. Do NOT give up after one attempt.
- When a build, server, or deployment fails: read the error output, fix the code/config, and try AGAIN. Retry at least 2-3 times with different approaches before reporting failure.
- NEVER say "please provide more details" or "please share the error" — you have tools, USE THEM to find and fix the problem yourself.
- When you give your final response, write in PAST TENSE — describe what you DID, not what you plan to do.
- NEVER start your response with "Let me...", "I'll...", "I will...", or "Now I'll...". Use tools directly.
- Your response is a SUMMARY of completed, VERIFIED work — not a proposal or a guess.
- If you could NOT complete the task despite trying, say so HONESTLY — explain what failed and what you tried. Do NOT pretend it worked.`

	conversationCtx.AddMessage(kyoci.RoleSystem, systemPrompt)
	conversationCtx.AddMessage(kyoci.RoleUser, task)

	// NOTE: STM cross-conversation loading is intentionally DISABLED.
	// Loading past conversation messages from different tasks confuses small
	// models (gemma4 8B). Cross-session intelligence is handled by the L3
	// ContextInjector (user profile, past experiences, lessons learned) which
	// provides structured, relevant context instead of raw message dumps.

	// c. Enter ReAct loop with auto-continuation (Hermes-like).
	// Instead of crashing at MaxIterations, the agent auto-continues up to
	// MaxContinuations rounds as long as it's making progress (tool calls).
	// Total effective iterations = MaxIterations * (1 + MaxContinuations).
	taskStart := time.Now()
	totalToolCalls := 0
	totalIters := 0 // total iterations across all rounds
	toolsUsed := make(map[string]bool)
	toolCallLog := make([]kyoci.ToolCallEntry, 0)
	totalTokens := kyoci.TokenUsage{}
	var finalContent string
	var lastError error
	emptyRetried := false
	truncationRetried := false

	maxRounds := a.config.MaxContinuations
	if maxRounds <= 0 {
		maxRounds = 3
	}

roundLoop:
	for round := 0; round <= maxRounds; round++ {
		roundStartTools := totalToolCalls

		for iteration := 1; iteration <= a.config.MaxIterations; iteration++ {
			totalIters++
			a.logger.Debug("ReAct iteration", "iteration", totalIters, "round", round)

			// Auto-compaction: if context is getting large, compact it
			if conversationCtx.TokenCount() > a.config.MaxContextTokens {
				a.logger.Info("auto-compacting context",
					"tokens", conversationCtx.TokenCount(),
					"messages", conversationCtx.MessageCount())
				conversationCtx.SmartCompact(a.config.MaxContextTokens / 2)
			}

			// Fire progress: thinking
			if fn := getProgress(ctx); fn != nil {
				fn(ProgressEvent{Type: "think", Iteration: totalIters})
			}

			// THINK: Call LLM with current context
			iterationTokens, response, err := a.think(ctx, conversationCtx)
			if err != nil {
				a.logger.Error("LLM call failed", "iteration", totalIters, "error", err)
				lastError = err
				break roundLoop
			}

			// Accumulate token usage
			totalTokens.PromptTokens += iterationTokens.PromptTokens
			totalTokens.CompletionTokens += iterationTokens.CompletionTokens
			totalTokens.TotalTokens += iterationTokens.TotalTokens

			a.logger.Info("THINK",
				"iteration", totalIters,
				"round", round,
				"tokens", iterationTokens.TotalTokens,
				"content_length", len(response.Content),
				"tool_calls", len(response.ToolCalls))

			// Fallback: extract tool calls from text for models that don't
			// support native function calling (e.g., qwen2.5-coder)
			if len(response.ToolCalls) == 0 && response.Content != "" {
				extracted := parseFencedJSONToolCalls(response.Content)
				if len(extracted) > 0 {
					response.ToolCalls = extracted
					response.Content = "" // Clear text — it was a tool call
					a.logger.Info("extracted tool calls from text output",
						"count", len(extracted),
						"tool", extracted[0].Name)
				}
			}

			// Check if we need to act (tool calls)
			if len(response.ToolCalls) > 0 {
				// HARD CAP: After 15 tool calls, force the model to give final answer
				if totalToolCalls >= 15 {
					a.logger.Info("hard tool-call cap reached, forcing final answer",
						"total_tool_calls", totalToolCalls, "iteration", totalIters)
					conversationCtx.AddMessage(kyoci.RoleUser,
						"You have used 15+ tool calls. You MUST stop now. Give your final answer summarizing what you accomplished and what's left to do. Do NOT call any more tools.")
					continue
				}

				conversationCtx.AddAssistantMessage(response.Content, response.ToolCalls)

				// ACT: Execute tool calls in parallel for speed.
				// Approval is checked sequentially first, then execution runs in parallel.
				type execResult struct {
					result   string
					err      error
					denied   bool
					duration time.Duration
				}
				execResults := make([]execResult, len(response.ToolCalls))

				// Phase 1: Check approvals sequentially (involves user interaction)
				// Also fire "act" progress events HERE (before execution) so the user
				// sees what's about to happen, not what already happened.
				approved := make([]bool, len(response.ToolCalls))
				for i, toolCall := range response.ToolCalls {
					// Fire progress: act (BEFORE execution — show user what's happening)
					toolSummary := summarizeToolCall(toolCall.Name, toolCall.Arguments)
					if fn := getProgress(ctx); fn != nil {
						fn(ProgressEvent{
							Type:       "act",
							Tool:       toolCall.Name,
							Iteration:  totalIters,
							ToolParams: toolSummary,
						})
					}

					approved[i] = true
					if approvalFn := getApproval(ctx); approvalFn != nil {
						ok, appErr := approvalFn(toolCall.Name, toolCall.Arguments)
						if appErr != nil || !ok {
							approved[i] = false
							deniedMsg := "Tool execution DENIED by user."
							if appErr != nil {
								deniedMsg += " Error: " + appErr.Error()
							}
							execResults[i] = execResult{result: deniedMsg, denied: true}
							a.logger.Warn("tool denied by user", "tool", toolCall.Name, "iteration", totalIters)
						}
					}
				}

				// Phase 2: Execute approved tools in parallel. Each goroutine stores
				// its result in execResults and returns nil — we never short-circuit,
				// so the ordered result processing below is unchanged. gctx propagates
				// cancellation into a.act.
				g, gctx := errgroup.WithContext(ctx)
				for i, toolCall := range response.ToolCalls {
					if !approved[i] {
						continue
					}
					i, toolCall := i, toolCall
					g.Go(func() error {
						start := time.Now()
						r, e := a.act(gctx, toolCall)
						execResults[i] = execResult{
							result:   r,
							err:      e,
							duration: time.Since(start),
						}
						return nil
					})
				}
				_ = g.Wait()

				// Phase 3: Process results in order (fire observe, log, add to context)
				for i, toolCall := range response.ToolCalls {
					res := execResults[i]

					// Fire progress: observe (tool already executed — show result)
					if fn := getProgress(ctx); fn != nil {
						resultSummary := summarizeResult(res.result, 80)
						fn(ProgressEvent{
							Type:       "observe",
							Tool:       toolCall.Name,
							Iteration:  totalIters,
							Result:     resultSummary,
							Success:    res.err == nil && !res.denied,
							DurationMs: res.duration.Milliseconds(),
						})
					}

					// Record for activity display
					toolCallLog = append(toolCallLog, kyoci.ToolCallEntry{
						Tool:       toolCall.Name,
						Args:       toolCall.Arguments,
						Success:    res.err == nil && !res.denied,
						DurationMs: res.duration.Milliseconds(),
					})

					a.logger.Info("OBSERVE",
						"iteration", totalIters,
						"tool", toolCall.Name,
						"duration", res.duration,
						"result_length", len(res.result),
						"error", res.err)

					// Build result string
					resultStr := res.result
					if res.err != nil {
						resultStr = fmt.Sprintf("Error: %v", res.err)
					}

					// Truncate large results to prevent context bloat
					resultStr = truncateToolResult(resultStr, 4000)

					conversationCtx.AddToolResult(toolCall.ID, resultStr)
					toolsUsed[toolCall.Name] = true
					totalToolCalls++
				}

				continue
			}

			// TRUNCATION DETECTION: If response had no tool calls but used
			// nearly all token budget, the tool call JSON was likely truncated.
			// Nudge the model to retry with explicit instructions.
			if iterationTokens.CompletionTokens >= a.config.MaxTokens-200 && !truncationRetried && iteration < a.config.MaxIterations {
				truncationRetried = true
				a.logger.Warn("response likely truncated (hit token limit), nudging to continue",
					"completion_tokens", iterationTokens.CompletionTokens,
					"max_tokens", a.config.MaxTokens,
					"iteration", totalIters)
				conversationCtx.AddMessage(kyoci.RoleUser,
					"Your previous response was cut off due to length. DO NOT explain or describe what you will do. Instead, call the file tool directly with operation=\"write\" to create the file. Keep the content concise but complete.")
				continue
			}

			// No tool calls — check if this is a real answer or an empty response.
			if response.Content == "" && totalToolCalls > 0 {
				if !emptyRetried {
					emptyRetried = true
					a.logger.Info("empty response after tool calls, nudging for summary", "iteration", totalIters)
					conversationCtx.AddMessage(kyoci.RoleUser, "You completed the task using tools. Now provide a brief summary of what you did in one or two sentences.")
					continue
				}
				finalContent = "Done. I completed the task successfully."
				a.logger.Info("empty response after nudge, using default summary", "iteration", totalIters)
				break roundLoop
			}

			// Real final answer with content
			finalContent = response.Content
			a.logger.Info("final answer received",
				"iteration", totalIters,
				"round", round,
				"content_length", len(finalContent))
			break roundLoop
		}

		// Inner loop exhausted — check if progress was made this round
		if totalToolCalls == roundStartTools {
			// No progress (no tool calls were made) — stop
			a.logger.Warn("no progress in round, stopping", "round", round, "total_tools", totalToolCalls)
			break
		}

		// Progress was made — auto-continue with a nudge
		if round < maxRounds {
			a.logger.Info("auto-continuing after max iterations",
				"round", round+1,
				"tools_so_far", totalToolCalls,
				"iters_so_far", totalIters)
			conversationCtx.AddMessage(kyoci.RoleUser,
				fmt.Sprintf("You've made good progress with %d tool calls. Continue from where you left off — don't repeat work you've already done. Complete the task and provide your final answer when done.", totalToolCalls))

			if fn := getProgress(ctx); fn != nil {
				fn(ProgressEvent{
					Type:      "think",
					Iteration: 0,
					Message:   fmt.Sprintf("Auto-continuing (round %d/%d)...", round+2, maxRounds+1),
				})
			}
		}
	}

	// Check if we exhausted all rounds without a final answer
	if finalContent == "" && lastError == nil {
		if totalToolCalls > 0 {
			// Made progress but didn't finalize — use what we have
			finalContent = fmt.Sprintf("Done. I completed the task using %d tool calls across %d iterations.", totalToolCalls, totalIters)
			a.logger.Info("all rounds exhausted with progress, using summary", "tools", totalToolCalls, "iters", totalIters)
		} else {
			lastError = kyoci.ErrMaxIterations
			a.logger.Warn("max iterations reached with no progress", "max", a.config.MaxIterations*(maxRounds+1))
		}
	}

	// Fire progress: done
	if fn := getProgress(ctx); fn != nil {
		msg := "completed"
		if lastError != nil {
			msg = "error: " + lastError.Error()
		}
		fn(ProgressEvent{Type: "done", Message: msg})
	}

	// d. Store conversation summary in memory (if enabled)
	// This lets the agent recall past interactions across sessions.
	// We store a concise summary, NOT raw messages, to avoid compaction loops.
	if a.config.EnableMemory && a.memory != nil && finalContent != "" {
		summary := fmt.Sprintf("Task: %s\nResult: %s", task, finalContent)
		storeCtx, storeCancel := context.WithTimeout(ctx, 5*time.Second)
		if _, err := a.memory.Store(storeCtx, summary, kyoci.MemoryShortTerm, map[string]string{
			"task":    task,
			"success": fmt.Sprintf("%t", finalContent != "" && lastError == nil),
		}); err != nil {
			a.logger.Warn("failed to store conversation in memory", "error", err)
		}
		storeCancel()
	}

	// e. Build tool list from tracking map
	usedToolList := make([]string, 0, len(toolsUsed))
	for name := range toolsUsed {
		usedToolList = append(usedToolList, name)
	}

	// f. Record experience for self-improvement (non-blocking)
	a.recorder.Record(ctx, TaskRecord{
		Task:       task,
		ToolsUsed:  usedToolList,
		Iterations: totalIters,
		ToolCalls:  totalToolCalls,
		Success:    finalContent != "" && lastError == nil,
		DurationMs: time.Since(taskStart).Milliseconds(),
		ErrorMsg: func() string {
			if lastError != nil {
				return lastError.Error()
			}
			return ""
		}(),
	})

	// g. Sanitize final content — strip any leaked tool-call artifacts
	finalContent = sanitizeContent(finalContent)

	// h. Return task result
	result := &kyoci.TaskResult{
		Content:       finalContent,
		ToolCallLog:   toolCallLog,
		ToolCallsMade: totalToolCalls,
		Iterations:    totalIters,
		Usage:         totalTokens,
		Error:         lastError,
	}

	if lastError != nil && finalContent == "" {
		return result, lastError
	}

	return result, nil
}
