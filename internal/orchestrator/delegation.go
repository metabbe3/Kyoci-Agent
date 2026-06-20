package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/metabbe3/Kyoci-Agent/internal/agent"
	"github.com/metabbe3/Kyoci-Agent/internal/tool/builtin"
	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// subAgentPrefix is prepended to EVERY delegated sub-agent task.
// This ensures sub-agents follow the same quality standards as the main agent.
// This is the KEY to making 8B sub-agents produce complete, correct output.
const subAgentPrefix = `SUB-AGENT TASK — COMPLETE EVERY STEP, NO SHORTCUTS.

QUALITY RULES (violating these = task failure):
1. You MUST produce complete, working output. Empty files or placeholder content = FAILURE.
2. You MUST use ALL steps in the task. If the task says "create file with tests", the file MUST have real tests.
3. Do NOT summarize what you would do. DO IT. Create the actual file with real content.
4. Every file you create must be complete and syntactically valid.
5. If you create a file, read it back to VERIFY it was written correctly.
6. After creating files, verify each one exists and has content (use file operation=read or terminal ls -la).

`

// wireDelegation connects the delegation tool's callback to the orchestrator.
// When an agent spawns a sub-task, it calls orch.Execute() directly.
// The sub-agent gets a quality enforcement prefix to ensure complete output.
//
// Explore dispatch: if the goal starts with "explore:" or "explore ", route to
// the read-only explore worker instead of the regular recursive orchestrator.
// The explore worker has a restricted toolset (glob/grep/file:read/git/etc.)
// and returns only a Markdown summary, mirroring Claude Code's context-isolated
// Task tool pattern. This keeps the parent's context window clean.
func (o *Orchestrator) wireDelegation(tool *builtin.DelegationTool) {
	tool.SetCallback(func(ctx context.Context, goal string, contextInfo string) (string, error) {
		slog.Info("delegation callback invoked", "goal", goal, "context_len", len(contextInfo))

		// Emit a task_start row for this delegation. The orchestrator's worker
		// will fill in sub-activity + progress events as the sub-agent runs.
		o.publishActivity(kyoci.ActivityEvent{
			Type:     kyoci.ActivityTaskStart,
			TaskID:   "delegation:" + delegationID(goal),
			TaskName: delegationLabel(goal),
			ParentID: "root",
			Role:     delegationRole(goal),
			Status:   "running",
		})
		finishOK := true
		defer func() {
			status := "done"
			if !finishOK {
				status = "error"
			}
			o.publishActivity(kyoci.ActivityEvent{
				Type:     kyoci.ActivityTaskComplete,
				TaskID:   "delegation:" + delegationID(goal),
				TaskName: delegationLabel(goal),
				ParentID: "root",
				Role:     delegationRole(goal),
				Status:   status,
			})
		}()

		// Explore dispatch — read-only sub-agent with context isolation.
		if agent.HasExplorePrefix(goal) {
			question := agent.StripExplorePrefix(goal)
			if contextInfo != "" {
				question = question + "\n\nAdditional context: " + contextInfo
			}
			slog.Info("delegation: routing to explore worker", "question", question)
			out, err := o.RunExplore(ctx, question)
			if err != nil {
				finishOK = false
				return "", err
			}
			return out, nil
		}

		// Research dispatch — multi-source web research with cited report.
		if agent.HasResearchPrefix(goal) {
			question := agent.StripResearchPrefix(goal)
			if contextInfo != "" {
				question = question + "\n\nAdditional context: " + contextInfo
			}
			slog.Info("delegation: routing to research worker", "question", question)
			o.publishActivity(kyoci.ActivityEvent{
				Type:     kyoci.ActivityLog,
				TaskID:   "delegation:" + delegationID(goal),
				TaskName: "Deep Research",
				Detail:   fmt.Sprintf("Researching: %s", question[:min(80, len(question))]),
			})
			out, err := o.RunResearch(ctx, question)
			if err != nil {
				finishOK = false
				return "", err
			}
			return out, nil
		}

		// Compare dispatch — blind side-by-side model comparison.
		if agent.HasComparePrefix(goal) {
			prompt := agent.StripComparePrefix(goal)
			slog.Info("delegation: routing to compare worker", "prompt", prompt)
			o.publishActivity(kyoci.ActivityEvent{
				Type:     kyoci.ActivityLog,
				TaskID:   "delegation:" + delegationID(goal),
				TaskName: "Model Compare",
				Detail:   fmt.Sprintf("Comparing: %s", prompt[:min(80, len(prompt))]),
			})
			result, err := o.RunCompare(ctx, prompt)
			if err != nil {
				finishOK = false
				return "", err
			}
			return agent.FormatCompareReport(result), nil
		}

		// Build the task string with quality prefix
		taskStr := subAgentPrefix + goal
		if contextInfo != "" {
			taskStr += "\n\nContext: " + contextInfo
		}

		result, err := o.Execute(ctx, taskStr, kyoci.RoleCustom)
		if err != nil {
			finishOK = false
			return "", err
		}

		// Append verification info so the main agent can check completeness
		verification := fmt.Sprintf("\n\n[SUB-AGENT METRICS] iterations=%d tool_calls=%d",
			result.Iterations, result.ToolCallsMade)
		if result.Content == "" || len(strings.TrimSpace(result.Content)) < 20 {
			verification = "\n\n[WARNING] Sub-agent produced very short output. The task may be incomplete."
		}

		return result.Content + verification, nil
	})
}

// delegationID returns a stable short ID for a delegation goal. Used as the
// TaskID so multiple delegations with the same goal collapse into one tree
// row (rare but possible).
func delegationID(goal string) string {
	// First 8 hex chars of a stable hash — good enough for grouping in the
	// activity tree. Not crypto-relevant.
	h := uint64(1469598103934665603) // FNV offset basis
	for i := 0; i < len(goal); i++ {
		h ^= uint64(goal[i])
		h *= 1099511628211 // FNV prime
	}
	return fmt.Sprintf("%016x", h)[:8]
}

// delegationLabel returns the human-readable label for the delegation row.
// For explore: prefixes, strips the prefix and truncates. Otherwise uses the
// raw goal (truncated to 80 chars).
func delegationLabel(goal string) string {
	if agent.HasExplorePrefix(goal) {
		q := agent.StripExplorePrefix(goal)
		if len(q) > 80 { return q[:77] + "…" }
		return "Explore: " + q
	}
	if agent.HasResearchPrefix(goal) {
		q := agent.StripResearchPrefix(goal)
		if len(q) > 80 { return q[:77] + "…" }
		return "Research: " + q
	}
	if agent.HasComparePrefix(goal) {
		q := agent.StripComparePrefix(goal)
		if len(q) > 80 { return q[:77] + "…" }
		return "Compare: " + q
	}
	if len(goal) > 80 { return goal[:77] + "…" }
	return goal
}

func delegationRole(goal string) string {
	if agent.HasExplorePrefix(goal) { return "explore" }
	if agent.HasResearchPrefix(goal) { return "research" }
	if agent.HasComparePrefix(goal) { return "compare" }
	return "generalist"
}

func min(a, b int) int {
	if a < b { return a }
	return b
}
