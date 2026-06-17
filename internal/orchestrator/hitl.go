package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"

	"github.com/metabbe3/Kyoci-Agent/internal/hitl"
	"github.com/metabbe3/Kyoci-Agent/internal/memory"
)

// =====================================================================================
// Human-In-The-Loop retry loop
//
// When a task carries a "VERIFY:" directive, the orchestrator runs the directive
// as a shell command after each agent attempt. Non-zero exit triggers a retry.
// After MaxRetries failed attempts, the orchestrator emits a HelpRequest via
// the HITL hook; the operator's hint is injected into the next attempt. After
// a successful post-HITL resolution, a "lesson learned" entry is recorded in
// L3 SQLite so future sessions benefit.
//
// Tasks WITHOUT a VERIFY directive skip this loop entirely and behave exactly
// as before: one agent call, return result on success, return error on failure.
// =====================================================================================

// HITLConfig bundles the per-orchestrator HITL settings. Set via SetHITL.
// A nil config (or MaxRetries=0) leaves the legacy single-shot behavior intact.
type HITLConfig struct {
	// MaxRetries caps the number of retries before HITL fallback. Default 0
	// (disabled). The L4 benchmark sets this to 2.
	MaxRetries int

	// Hook is the HITL hook the orchestrator uses to request operator hints.
	// If nil, retries proceed without HITL; exhaustion returns an error.
	Hook hitl.HITLHook
}

// ErrVerifyFailed is returned when the VERIFY command exits non-zero.
type ErrVerifyFailed struct {
	Attempt int
	Output  string
}

func (e *ErrVerifyFailed) Error() string {
	return fmt.Sprintf("verify failed on attempt %d: %s", e.Attempt, truncateForHitl(e.Output, 200))
}

// ErrHITLExhausted is returned when the post-HITL attempt still fails verification.
type ErrHITLExhausted struct {
	LastOutput string
}

func (e *ErrHITLExhausted) Error() string {
	return fmt.Sprintf("task failed verification even after HITL hint: %s", truncateForHitl(e.LastOutput, 200))
}

// verifyDirectiveRe matches a line starting with VERIFY: and captures the
// command. Case-insensitive, multiline. The first match wins.
var verifyDirectiveRe = regexp.MustCompile(`(?im)^[ \t]*VERIFY:[ \t]*(.+?)[ \t]*$`)

// extractVerifyCommand returns the VERIFY command from the task text, or "" if
// none. The task is left unchanged; stripVerifyDirective removes the line for
// the agent-facing prompt.
func extractVerifyCommand(task string) string {
	m := verifyDirectiveRe.FindStringSubmatch(task)
	if m == nil {
		return ""
	}
	return strings.TrimSpace(m[1])
}

// stripVerifyDirective removes all VERIFY: lines from the task text. The agent
// never sees the directive — it would confuse the planner into emitting a
// separate "run the verify command" step instead of fixing the bug.
func stripVerifyDirective(task string) string {
	return strings.TrimSpace(verifyDirectiveRe.ReplaceAllString(task, ""))
}

// runVerify runs the verify command under bash and returns (pass, output, err).
// pass is true iff exit code is 0. output is combined stdout+stderr regardless
// of pass. err is the exec error (nil on success).
//
// The command runs with up to a 60s timeout — enough for `go test` on a
// toy package, short enough to keep the overall retry loop responsive.
func runVerify(ctx context.Context, cmd string) (bool, string, error) {
	verifyCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	c := exec.CommandContext(verifyCtx, "bash", "-c", cmd)
	out, err := c.CombinedOutput()
	passed := err == nil
	return passed, string(out), err
}

// executeWithRetry wraps the role's Execute method with the HITL retry loop.
// If the task has no VERIFY directive (or HITL is disabled), it delegates
// directly to agentRole.Execute — single shot, unchanged behavior.
//
// The retry budget:
//   - maxRetries=0, no hook: 1 attempt, no retry
//   - maxRetries=N, no hook: 1 + N attempts (initial + N retries)
//   - maxRetries=N, hook set: 1 + N + 1 attempts (initial + N retries + 1 post-HITL)
//
// The post-HITL attempt is the agent's last chance with operator guidance.
// On success after HITL, a lesson is recorded via ReflectionEngine.RecordLesson.
func (o *Orchestrator) executeWithRetry(
	ctx context.Context,
	task string,
	agentRole kyoci.Role,
	roleType kyoci.RoleType,
) (*kyoci.TaskResult, error) {
	verifyCmd := extractVerifyCommand(task)
	cleanTask := stripVerifyDirective(task)

	// Fast path: no verify directive, no retry loop. Single shot, unchanged.
	if verifyCmd == "" {
		return agentRole.Execute(ctx, task, o.memoryMgr)
	}

	maxRetries := 0
	var hook hitl.HITLHook
	o.mu.RLock()
	if o.hitlCfg != nil {
		maxRetries = o.hitlCfg.MaxRetries
		hook = o.hitlCfg.Hook
	}
	o.mu.RUnlock()

	// Even with no HITL hook, a MaxRetries > 0 still gives us a verify-loop
	// (just no human fallback). totalAttempts covers both shapes.
	preHITLAttempts := maxRetries + 1
	totalAttempts := preHITLAttempts
	if hook != nil {
		totalAttempts++ // post-HITL attempt
	}

	taskID := hitl.NewTaskID()
	o.logger.Info("orchestrator: verify loop enabled",
		"task_id", taskID,
		"verify_cmd", verifyCmd,
		"max_retries", maxRetries,
		"total_attempts", totalAttempts,
		"hitl", hook != nil,
		"role", roleType.String(),
	)

	var (
		attempts      []string
		lastResult    *kyoci.TaskResult
		lastVerifyOut string
		hint          string
		hintInjected  bool
	)

	for attempt := 1; attempt <= totalAttempts; attempt++ {
		// Compose this iteration's task text.
		taskForRun := cleanTask
		switch {
		case attempt == 1:
			// pristine
		case hintInjected:
			taskForRun += fmt.Sprintf(
				"\n\n---\nHINT FROM HUMAN OPERATOR (attempt %d of %d):\n%s\n---\nApply this hint to your next fix. Do not repeat previous mistakes.",
				attempt, totalAttempts, hint,
			)
		default:
			taskForRun += fmt.Sprintf(
				"\n\n---\nPREVIOUS ATTEMPT %d FAILED verification (attempt %d of %d).\nVerify output:\n%s\n---\nTry a different approach. Do not repeat the same fix.",
				attempt-1, attempt, totalAttempts, truncateForHitl(lastVerifyOut, 1500),
			)
		}

		o.logger.Info("orchestrator: attempt",
			"task_id", taskID, "attempt", attempt, "total", totalAttempts, "hint_injected", hintInjected)

		// Run the agent. Errors here are not fatal to the retry loop — the
		// verify command is the source of truth. But we capture the result
		// for the success-path return.
		result, runErr := agentRole.Execute(ctx, taskForRun, o.memoryMgr)
		lastResult = result
		if runErr != nil {
			o.logger.Warn("orchestrator: agent returned error",
				"task_id", taskID, "attempt", attempt, "error", runErr)
		}

		// Run verification.
		pass, verifyOut, verifyErr := runVerify(ctx, verifyCmd)
		lastVerifyOut = verifyOut
		attempts = append(attempts, fmt.Sprintf(
			"attempt %d: pass=%v verify_err=%v output_len=%d",
			attempt, pass, verifyErr, len(verifyOut),
		))
		o.logger.Info("orchestrator: verify result",
			"task_id", taskID, "attempt", attempt, "pass", pass, "output_len", len(verifyOut))

		if pass {
			// SUCCESS. If HITL contributed, record a lesson.
			if hintInjected && o.reflectionEngine != nil {
				lessonErr := o.reflectionEngine.RecordLesson(ctx, memory.LessonInput{
					TaskID:    taskID,
					Task:      cleanTask,
					Failure:   attempts[0],
					Hint:      hint,
					Fix:       lessonFixSummary(result),
					VerifyCmd: verifyCmd,
				})
				if lessonErr != nil {
					o.logger.Warn("orchestrator: failed to record HITL lesson", "error", lessonErr)
				} else {
					o.logger.Info("orchestrator: HITL lesson recorded", "task_id", taskID)
				}
			}
			if result == nil {
				result = &kyoci.TaskResult{Content: "verification passed"}
			}
			result.Role = roleType
			return result, nil
		}

		// Failure path. Decide what to do next.

		// Time to trigger HITL? Only on the boundary between pre-HITL and post-HITL.
		if !hintInjected && attempt == preHITLAttempts && hook != nil {
			req := hitl.HelpRequest{
				TaskID:         taskID,
				Role:           roleType.String(),
				Attempt:        attempt,
				Question: fmt.Sprintf(
					"I am stuck on task: %s\n\nLast verification output:\n%s\n\nI've tried %d attempt(s). Can you provide a hint?",
					truncateForHitl(cleanTask, 300),
					truncateForHitl(lastVerifyOut, 800),
					attempt,
				),
				LastError:      truncateForHitl(lastVerifyOut, 1500),
				AttemptedFixes: attempts,
			}
			hintVal, herr := hook.RequestHelp(ctx, req)
			if herr != nil {
				o.logger.Warn("orchestrator: HITL hook failed; giving up",
					"task_id", taskID, "error", herr)
				if errors.Is(herr, hitl.ErrNoSubscriber) {
					return lastResult, fmt.Errorf("HITL unavailable (no operator subscribed); verify failed: %w",
						&ErrVerifyFailed{Attempt: attempt, Output: lastVerifyOut})
				}
				return lastResult, fmt.Errorf("HITL hook error: %w (verify output: %s)",
					herr, truncateForHitl(lastVerifyOut, 200))
			}
			hint = hintVal
			hintInjected = true
			o.logger.Info("orchestrator: HITL hint received, applying to next attempt",
				"task_id", taskID, "hint_len", len(hint))
			continue
		}

		// No HITL fallback (or already used). If this was the last attempt, give up.
		if attempt == totalAttempts {
			if hintInjected {
				return lastResult, &ErrHITLExhausted{LastOutput: lastVerifyOut}
			}
			return lastResult, &ErrVerifyFailed{Attempt: attempt, Output: lastVerifyOut}
		}

		// Otherwise, fall through to the next retry.
	}

	// Unreachable — the loop returns from inside. Defensive default.
	return lastResult, &ErrVerifyFailed{Attempt: totalAttempts, Output: lastVerifyOut}
}

// lessonFixSummary extracts a short summary of the fix from the TaskResult
// content. If content is empty, returns "(no agent output)".
func lessonFixSummary(result *kyoci.TaskResult) string {
	if result == nil {
		return "(no agent output)"
	}
	c := strings.TrimSpace(result.Content)
	if c == "" {
		return "(empty agent output)"
	}
	return truncateForHitl(c, 500)
}

// truncateForHitl is the local truncation helper — keeps verify output and
// lesson text bounded so logs and SQLite rows don't blow up.
func truncateForHitl(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}

// SetHITL installs the Human-In-The-Loop configuration on the orchestrator.
// Pass nil to disable. Safe to call before Start(); not safe to call
// concurrently with Execute.
//
// The hook is typically *hitl.Hub constructed in main.go alongside the gRPC
// server. The MaxRetries field controls how many attempts the orchestrator
// makes before requesting human help.
func (o *Orchestrator) SetHITL(cfg *HITLConfig) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.hitlCfg = cfg
	if cfg != nil {
		o.logger.Info("HITL configured",
			"max_retries", cfg.MaxRetries, "hook_set", cfg.Hook != nil)
	} else {
		o.logger.Info("HITL disabled")
	}
}

// GetHITL returns the current HITL config (nil if unset). Read-only.
func (o *Orchestrator) GetHITL() *HITLConfig {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.hitlCfg
}
