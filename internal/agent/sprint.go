package agent

import (
	"context"
	"fmt"
	"time"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// =====================================================================================
// Sprint mode — 8B-optimization Strategy 3.
//
// Runs the agent as a pure function: ONE LLM call, no tools, no chat history
// carried forward across calls. Best for pure-reasoning task shapes where the
// 8B model is more reliable as a one-shot responder than as an autonomous
// agent (explain, summarize, classify, convert, format).
//
// Why: 8B models degrade quickly in autonomous Plan→Search→Read→Write→Verify
// loops. By step 4 of a multi-step task they often forget the original
// constraints. Sprint mode sidesteps that — there is no step 4.
//
// When to use: planner marks a step with tool_hint="sprint" (no file ops
// needed). The orchestrator dispatcher routes that step to a sprint worker
// via EnableSprint=true on the per-step agent config.
//
// When NOT to use: anything that needs file reads/writes, multi-step
// reasoning over external state, or tool calls. Use the regular worker.
// =====================================================================================

// executeSprint runs the agent as a single-shot responder. No tools, no
// memory append, no iteration. Returns the model's output as the task result.
//
// The system prompt is whatever the agent config carries — callers typically
// set a focused "explain/summarize/classify" prompt when enabling sprint.
func (a *Agent) executeSprint(ctx context.Context, task string) (*kyoci.TaskResult, error) {
	a.logger.Info("sprint: single-shot execution", "task", task)
	a.emitActivity(kyoci.ActivityEvent{
		Type:     kyoci.ActivityTaskStart,
		TaskID:   "sprint",
		TaskName: truncateTask(task, 80),
		Role:     a.activityRole,
		Status:   "running",
		Detail:   "Sprint mode — one LLM call, no tools",
	})

	sysPrompt := a.config.SystemPrompt
	if sysPrompt == "" {
		sysPrompt = "You are a helpful assistant. Answer the user's question directly and concisely."
	}

	req := kyoci.CompletionRequest{
		Messages: []kyoci.Message{
			{Role: kyoci.RoleSystem, Content: sysPrompt},
			{Role: kyoci.RoleUser, Content: task},
		},
		Temperature: a.config.Temperature,
		MaxTokens:   a.config.MaxTokens,
		Model:       a.config.Model,
		Tools:       nil, // no tools in sprint mode
	}

	start := time.Now()
	resp, err := a.router.Route(ctx, req, a.config.PreferredProvider)
	elapsed := time.Since(start)
	if err != nil {
		a.emitActivity(kyoci.ActivityEvent{
			Type:     kyoci.ActivityTaskComplete,
			TaskID:   "sprint",
			TaskName: truncateTask(task, 80),
			Status:   "error",
			Detail:   fmt.Sprintf("LLM call failed after %s: %v", elapsed.Round(time.Millisecond), err),
		})
		return &kyoci.TaskResult{
			Error:         fmt.Errorf("sprint LLM call failed: %w", err),
			ToolCallsMade: 0,
			Iterations:    1,
			Usage:         kyoci.TokenUsage{},
		}, err
	}

	content := ""
	usage := kyoci.TokenUsage{}
	if resp != nil {
		content = resp.Content
		usage = resp.Usage
	}

	a.emitActivity(kyoci.ActivityEvent{
		Type:       kyoci.ActivityTaskComplete,
		TaskID:     "sprint",
		TaskName:   truncateTask(task, 80),
		Status:     "done",
		TokensUsed: int(usage.TotalTokens),
		Detail:     fmt.Sprintf("Completed in %s", elapsed.Round(time.Millisecond)),
	})

	return &kyoci.TaskResult{
		Content:       content,
		ToolCallsMade: 0,
		Iterations:    1,
		Usage:         usage,
		Error:         nil,
	}, nil
}

// truncateTask clips a task string to n chars for display in activity events.
func truncateTask(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
