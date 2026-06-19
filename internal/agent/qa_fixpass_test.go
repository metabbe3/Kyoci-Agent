package agent

import (
	"strings"
	"testing"
)

// =====================================================================================
// extractQAFailure — detects when the QA/verify step reported a build failure.
// Drives the orchestrator-level fix-pass loop. Pinned because the loop is
// useless if this misses real failures or false-positives on successes.
// =====================================================================================

func TestExtractQAFailure_DetectsVerificationFailedTag(t *testing.T) {
	steps := []OrchStep{
		{ID: 1, Description: "Create src/main.ts"},
		{ID: 6, Description: "VERIFY: run npm run build"},
		{ID: 7, Description: "QA: independently re-run the build"},
	}
	results := map[int]string{
		1: "Created src/main.ts",
		6: "[VERIFICATION FAILED: a build/test command exited non-zero]\nnpm run build failed:\nTS18028: jsx is not enabled",
		7: "**QA VERDICT**: FAIL — build broken",
	}
	got := extractQAFailure(steps, results)
	if got == "" {
		t.Fatal("expected failure extraction, got empty")
	}
	if !strings.Contains(got, "TS18028") {
		t.Errorf("extracted text missing the TS error; got: %q", got)
	}
}

func TestExtractQAFailure_DetectsQAVerdictFail(t *testing.T) {
	// QA reports FAIL without the [VERIFICATION FAILED] tag — model phrased
	// it as "**FAIL**" instead.
	steps := []OrchStep{
		{ID: 7, Description: "QA: re-check the build"},
	}
	results := map[int]string{
		7: "**FAIL**: webpack-cli missing",
	}
	got := extractQAFailure(steps, results)
	if got == "" {
		t.Fatal("expected failure extraction for **FAIL** marker")
	}
}

func TestExtractQAFailure_PassReturnsEmpty(t *testing.T) {
	steps := []OrchStep{
		{ID: 7, Description: "QA: re-check the build"},
	}
	results := map[int]string{
		7: "**PASS**: build succeeded, all tests green",
	}
	if got := extractQAFailure(steps, results); got != "" {
		t.Errorf("PASS marker should not trigger extraction, got: %q", got)
	}
}

func TestExtractQAFailure_NoQAStepReturnsEmpty(t *testing.T) {
	// Plan with no QA/verify step — extraction should return "" (loop no-ops).
	steps := []OrchStep{
		{ID: 1, Description: "Create main.go"},
		{ID: 2, Description: "Explain the design"},
	}
	results := map[int]string{
		1: "Created main.go",
		2: "Here's the design...",
	}
	if got := extractQAFailure(steps, results); got != "" {
		t.Errorf("no QA step should yield empty, got: %q", got)
	}
}

func TestExtractQAFailure_QAStepMissingFromResults(t *testing.T) {
	// Plan HAS a QA step but it didn't run (or errored before producing output).
	steps := []OrchStep{
		{ID: 7, Description: "QA: re-check"},
	}
	results := map[int]string{} // empty
	if got := extractQAFailure(steps, results); got != "" {
		t.Errorf("missing QA result should yield empty, got: %q", got)
	}
}

// =====================================================================================
// BuildFixNudge — the prompt appended during a fix-pass. Must surface the
// actual errors and demand tool calls.
// =====================================================================================

func TestBuildFixNudge_ContainsErrors(t *testing.T) {
	errors := "TS18028: jsx not enabled\ntsconfig.json:5:3\nnpm ERR! missing webpack-cli"
	nudge := BuildFixNudge(errors)
	for _, want := range []string{"TS18028", "webpack-cli", "file:write", "BUILD FAILURE"} {
		if !strings.Contains(nudge, want) {
			t.Errorf("nudge missing %q:\n%s", want, nudge)
		}
	}
}

func TestBuildFixNudge_TruncatesLongErrors(t *testing.T) {
	// 5000-char stack trace should get truncated to fit the worker's budget.
	long := strings.Repeat("error line\n", 500) // ~5500 chars
	nudge := BuildFixNudge(long)
	if !strings.Contains(nudge, "truncated") {
		t.Errorf("long failure should be truncated; nudge length = %d", len(nudge))
	}
	if len(nudge) > 3500 { // 2000 cap + ~1500 prompt boilerplate
		t.Errorf("nudge too long after truncation: %d chars", len(nudge))
	}
}

func TestBuildFixNudge_ForbitsProseOnlyResponses(t *testing.T) {
	nudge := BuildFixNudge("some error")
	if !strings.Contains(nudge, "ANTI-PATTERNS") {
		t.Errorf("nudge should list anti-patterns to forbid prose-only responses")
	}
	if !strings.Contains(nudge, "Do NOT explain the fix in prose") {
		t.Errorf("nudge should explicitly forbid prose-only responses")
	}
}
