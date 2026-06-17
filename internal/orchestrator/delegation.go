package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
	"github.com/metabbe3/Kyoci-Agent/internal/tool/builtin"
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
func (o *Orchestrator) wireDelegation(tool *builtin.DelegationTool) {
	tool.SetCallback(func(ctx context.Context, goal string, contextInfo string) (string, error) {
		slog.Info("delegation callback invoked", "goal", goal, "context_len", len(contextInfo))

		// Build the task string with quality prefix
		taskStr := subAgentPrefix + goal
		if contextInfo != "" {
			taskStr += "\n\nContext: " + contextInfo
		}

		result, err := o.Execute(ctx, taskStr, kyoci.RoleCustom)
		if err != nil {
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
