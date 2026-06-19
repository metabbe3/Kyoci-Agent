package agent

import (
	"log/slog"
	"testing"
)

func sdlcTestAgent() *Agent { return &Agent{logger: slog.Default()} }

// A code task gets a SETUP step (id 0) first — every other step depends on it —
// and a VERIFY step last.
func TestEnsureSDLCSteps_InjectsSetupAndVerify(t *testing.T) {
	a := sdlcTestAgent()
	steps := []OrchStep{
		{ID: 1, Description: "Create projects/calc/src/App.tsx", ToolHint: "file"},
		{ID: 2, Description: "Create projects/calc/src/main.tsx", ToolHint: "file", DependsOn: []int{1}},
	}
	out := a.ensureSDLCSteps("make a calculator", steps, 60)

	if len(out) < 3 || out[0].ID != -1 || out[0].ToolHint != "terminal" {
		t.Fatalf("expected SETUP step (id -1, terminal) first, got %+v", out)
	}
	for _, s := range out[1:] {
		if !intSliceContains(s.DependsOn, -1) {
			t.Errorf("step %d should depend on setup (-1), depends_on=%v", s.ID, s.DependsOn)
		}
	}
	last := out[len(out)-1]
	if last.ToolHint != "terminal" {
		t.Errorf("expected last step VERIFY (terminal), got tool_hint=%q", last.ToolHint)
	}
}

// A conversational task (no file-creation steps) is left unchanged.
func TestEnsureSDLCSteps_NoOpForConversational(t *testing.T) {
	a := sdlcTestAgent()
	steps := []OrchStep{{ID: 1, Description: "Answer directly — conversational question", ToolHint: ""}}
	out := a.ensureSDLCSteps("what is REST vs GraphQL?", steps, 60)
	if len(out) != 1 || out[0].ID != 1 {
		t.Fatalf("conversational task must be unchanged, got %+v", out)
	}
}

// No duplicate injection when the planner already emitted setup + verify steps.
func TestEnsureSDLCSteps_NoDuplicateWhenPresent(t *testing.T) {
	a := sdlcTestAgent()
	// Planner already emitted SETUP, VERIFY, AND QA → none should be re-injected.
	steps := []OrchStep{
		{ID: 1, Description: "SETUP: npm install dependencies", ToolHint: "terminal"},
		{ID: 2, Description: "Create projects/calc/src/App.tsx", ToolHint: "file"},
		{ID: 3, Description: "VERIFY: npm run build", ToolHint: "terminal"},
		{ID: 4, Description: "QA: independently review the result", ToolHint: "terminal"},
	}
	out := a.ensureSDLCSteps("make a calculator", steps, 60)
	if len(out) != 4 {
		t.Fatalf("expected no duplicate injection (4 steps), got %d: %+v", len(out), out)
	}
	for _, s := range out {
		if s.ID == 0 {
			t.Errorf("should not inject a second setup step (id 0)")
		}
	}
}

func TestIsStalledResult(t *testing.T) {
	stalled := []string{
		"", "   ",
		"[worker error: boom]",
		"[VERIFICATION FAILED: none found]",
		"[no tool evidence — memory]",
		"[circuit breaker: step 2 stopped]",
		"[step 3 hit tool budget]",
	}
	ok := []string{
		"Created projects/calc/App.tsx",
		"npm install completed successfully",
		"build passed",
	}
	for _, s := range stalled {
		if !isStalledResult(s) {
			t.Errorf("expected stalled for %q", s)
		}
	}
	for _, s := range ok {
		if isStalledResult(s) {
			t.Errorf("expected NOT stalled for %q", s)
		}
	}
}

func intSliceContains(xs []int, v int) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

// Injected VERIFY/QA must survive even when the planner output is at the cap —
// they are carved out of maxSteps, not truncated from the tail.
func TestEnsureSDLCSteps_TruncationKeepsQA(t *testing.T) {
	a := sdlcTestAgent()
	steps := []OrchStep{
		{ID: 1, Description: "Create projects/calc/src/App.tsx", ToolHint: "file"},
		{ID: 2, Description: "Create projects/calc/src/main.tsx", ToolHint: "file"},
	}
	out := a.ensureSDLCSteps("make a calculator", steps, 3)
	last := out[len(out)-1]
	if !isQAStep(last.Description) {
		t.Fatalf("QA step must survive (not be truncated); last=%q len=%d", last.Description, len(out))
	}
}

// A read/understand step must NOT be misclassified as file-creation, even though
// "implement" appears inside "implementation" (the planner's own few-shot pattern).
func TestIsFileCreationStep_ReadStepNotCreation(t *testing.T) {
	for _, d := range []string{
		"Read user_service.go to understand current implementation",
		"Review the implementation of the auth module in auth.go",
		"Investigate the generated output in build.go",
	} {
		if isFileCreationStep(d) {
			t.Errorf("expected isFileCreationStep(%q)=false (read/analysis step)", d)
		}
	}
	if !isFileCreationStep("Create projects/calc/src/App.tsx") {
		t.Error("expected isFileCreationStep('Create App.tsx')=true")
	}
}

// looksLikeBuildTask must send build tasks to the orchestrator (not a skill).
func TestLooksLikeBuildTask(t *testing.T) {
	yes := []string{
		"make me a personal website portfolio with light and blue color",
		"build a react app",
		"create a landing page for my startup",
	}
	no := []string{
		"sha256 of hello",
		"color #aaffee",
		"generate uuid",
		"create a hash of secret",
		"what is the capital of france",
	}
	for _, s := range yes {
		if !looksLikeBuildTask(s) {
			t.Errorf("expected looksLikeBuildTask(%q)=true", s)
		}
	}
	for _, s := range no {
		if looksLikeBuildTask(s) {
			t.Errorf("expected looksLikeBuildTask(%q)=false", s)
		}
	}
}

// isServeTask detects URL/preview requests.
func TestIsServeTask(t *testing.T) {
	yes := []string{"give me the working url to see it", "preview the result", "open it in browser"}
	no := []string{"make a website", "fix the bug in main.go", "what is REST"}
	for _, s := range yes {
		if !isServeTask(s) {
			t.Errorf("expected isServeTask(%q)=true", s)
		}
	}
	for _, s := range no {
		if isServeTask(s) {
			t.Errorf("expected isServeTask(%q)=false", s)
		}
	}
}
