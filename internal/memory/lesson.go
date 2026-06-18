package memory

import (
	"context"
	"fmt"
	"strings"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// LessonInput is the payload for ReflectionEngine.RecordLesson. It captures
// the full context of a HITL-assisted resolution so the rule synthesizer has
// the original failure, the operator's hint, and the resulting fix available.
type LessonInput struct {
	// TaskID correlates to the orchestrator's HITL task ID. Stored as metadata.
	TaskID string
	// Task is the original task text (cleaned of VERIFY: directive).
	Task string
	// Failure is the verification output from the most recent failed attempt
	// (or the agent's error if no verifier was set). Should include enough of
	// the test output for future recall to surface this lesson on similar tasks.
	Failure string
	// Hint is the operator's hint text. Stored verbatim — short hints are most
	// useful in recall, so callers should keep this tight.
	Hint string
	// Fix is the agent's final successful output (the TaskResult content).
	// Truncated internally to keep the SQLite row bounded.
	Fix string
	// VerifyCmd is the VERIFY: directive that ultimately passed. Stored so
	// future recall can identify which verification pattern this lesson relates to.
	VerifyCmd string
}

// RecordLesson writes a permanent lesson to L3 memory after a successful
// post-HITL resolution. The lesson content includes the original task, the
// failure, the hint, the fix, and a synthesized one-sentence rule. Stored
// with metadata category=lesson, source=postmortem so it is retrievable via
// the existing ReflectionEngine.GetRelevantLessons path.
//
// Unlike Reflect(), this method does NOT make rule-based inferences about
// what to remember — every HITL-assisted fix is worth a lesson. The lesson
// text itself encodes the failure → hint → fix chain.
//
// Returns nil on success. Storage errors are returned wrapped.
func (re *ReflectionEngine) RecordLesson(ctx context.Context, input LessonInput) error {
	if re == nil || re.storage == nil {
		return fmt.Errorf("reflection engine not initialized")
	}

	rule := synthesizeLessonRule(input)
	content := buildLessonContent(input, rule)

	metadata := map[string]string{
		"category": string(CategoryLesson),
		"source":   "postmortem",
		"task_id":  input.TaskID,
		"task":     truncateForLog(input.Task, 200),
	}

	id, err := re.storage.Store(ctx, content, kyoci.MemoryLongTerm, metadata)
	if err != nil {
		return fmt.Errorf("failed to store lesson: %w", err)
	}

	re.logger.Info("lesson recorded",
		"id", id,
		"task_id", input.TaskID,
		"rule_len", len(rule),
		"content_len", len(content))
	return nil
}

// synthesizeLessonRule produces a one-sentence rule from the lesson inputs.
// Pure-Go heuristic — no LLM call — so the post-HITL lesson recording is
// cheap and deterministic. A future LLM-driven rule synthesizer can layer
// on top by overriding the Rule field before calling RecordLesson.
//
// The rule embeds the failure summary and the hint verbatim. This makes the
// lesson self-describing in recall: a future agent seeing this lesson reads
// both what went wrong and how to fix it without needing to consult the
// surrounding context.
func synthesizeLessonRule(input LessonInput) string {
	hint := strings.TrimSpace(input.Hint)
	failure := strings.TrimSpace(input.Failure)
	task := strings.TrimSpace(input.Task)

	// Pull the first line of the failure for a tight rule.
	firstFailLine := failure
	if idx := strings.Index(failure, "\n"); idx > 0 {
		firstFailLine = strings.TrimSpace(failure[:idx])
	}
	if firstFailLine == "" {
		firstFailLine = "(no failure output)"
	}

	if hint != "" {
		return fmt.Sprintf(
			"Rule: For tasks like '%s', when verification fails with '%s', apply: %s",
			truncateForLog(task, 80),
			truncateForLog(firstFailLine, 80),
			truncateForLog(hint, 120),
		)
	}
	return fmt.Sprintf(
		"Rule: Avoid the failure pattern '%s' for tasks like '%s'",
		truncateForLog(firstFailLine, 100),
		truncateForLog(task, 80),
	)
}

// buildLessonContent formats the full lesson row. The format is line-delimited
// so it reads cleanly in SQLite CLI queries and in recall-formatted prompts.
func buildLessonContent(input LessonInput, rule string) string {
	var b strings.Builder
	b.WriteString(rule)
	b.WriteString("\n\n--- Lesson Details ---\n")
	fmt.Fprintf(&b, "Task: %s\n", truncateForLog(input.Task, 300))
	fmt.Fprintf(&b, "Failure: %s\n", truncateForLog(input.Failure, 600))
	fmt.Fprintf(&b, "Hint received: %s\n", truncateForLog(input.Hint, 300))
	fmt.Fprintf(&b, "Fix applied: %s\n", truncateForLog(input.Fix, 400))
	if input.VerifyCmd != "" {
		fmt.Fprintf(&b, "Verified by: %s\n", truncateForLog(input.VerifyCmd, 120))
	}
	if input.TaskID != "" {
		fmt.Fprintf(&b, "Task ID: %s\n", input.TaskID)
	}
	return b.String()
}
