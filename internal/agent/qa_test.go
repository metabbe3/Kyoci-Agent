package agent

import (
	"context"
	"strings"
	"testing"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

func TestIsQAStep(t *testing.T) {
	yes := []string{"QA: review the result", "qa: check the build", "independently verify everything", "  QA: ..."}
	no := []string{"Create App.tsx", "VERIFY: run build", "Answer directly — conversational", "setup: install deps"}
	for _, s := range yes {
		if !isQAStep(s) {
			t.Errorf("expected isQAStep(%q)=true", s)
		}
	}
	for _, s := range no {
		if isQAStep(s) {
			t.Errorf("expected isQAStep(%q)=false", s)
		}
	}
}

// A code task gets a QA step appended last; it depends on every other step.
func TestEnsureSDLCSteps_InjectsQA(t *testing.T) {
	a := sdlcTestAgent()
	steps := []OrchStep{{ID: 1, Description: "Create projects/calc/src/App.tsx", ToolHint: "file"}}
	out := a.ensureSDLCSteps("make a calculator", steps, 60)

	last := out[len(out)-1]
	if !isQAStep(last.Description) {
		t.Fatalf("expected last step to be QA, got %q", last.Description)
	}
	for _, s := range out[:len(out)-1] {
		if !intSliceContains(last.DependsOn, s.ID) {
			t.Errorf("QA should depend on step %d, depends_on=%v", s.ID, last.DependsOn)
		}
	}
}

// QAToolAllowlist = explore read-set + terminal (so QA can re-run build/tests).
func TestQAToolAllowlist(t *testing.T) {
	if !QAToolAllowlist["terminal"] {
		t.Error("QA allowlist must include terminal (re-run build/tests)")
	}
	for _, k := range []string{"glob", "grep", "file", "git", "codesearch", "lsp"} {
		if !QAToolAllowlist[k] {
			t.Errorf("QA allowlist must include read tool %q", k)
		}
	}
}

// qaFilterStub counts Execute calls — proves whether the filter reached the
// inner tool.
type qaFilterStub struct{ execs int }

func (s *qaFilterStub) Register(kyoci.Tool) error    { return nil }
func (s *qaFilterStub) List() []kyoci.ToolDefinition { return nil }
func (s *qaFilterStub) Execute(context.Context, string, map[string]interface{}) (string, error) {
	s.execs++
	return "", nil
}

// The file tool's real parameter is "operation" (not "action"). A write via the
// correct key MUST be rejected by the read-only filter (the QA/explore
// "never modifies files" guarantee).
func TestReadOnlyToolFilter_OperationKeyWriteRejected(t *testing.T) {
	var stub qaFilterStub
	f := NewReadOnlyToolFilter(&stub, QAToolAllowlist)
	_, err := f.Execute(context.Background(), "file", map[string]interface{}{
		"operation": "write", "path": "x", "content": "y",
	})
	if err == nil {
		t.Fatal("expected file:write via 'operation' key to be rejected by the read-only filter")
	}
	if stub.execs != 0 {
		t.Errorf("inner file tool must NOT be reached on a blocked write; execs=%d", stub.execs)
	}
}

// The QA/explore terminal may run builds but must not write files via the shell.
func TestCommandWritesToFile(t *testing.T) {
	blocked := []string{"cat > x", "echo hi > file", "cmd >> log", "tee out", "sed -i s/a/b/", "rm -f x", "cp a b", "chmod +x x", "dd of=img"}
	allowed := []string{"npm run build", "npm run build 2>&1", "go test ./...", "npm install", "ls -la", "grep foo bar", "go build ./..."}
	for _, c := range blocked {
		if !commandWritesToFile(c) {
			t.Errorf("expected commandWritesToFile(%q)=true", c)
		}
	}
	for _, c := range allowed {
		if commandWritesToFile(c) {
			t.Errorf("expected commandWritesToFile(%q)=false (allowed build/test)", c)
		}
	}
}

// The Go-side honesty gate tags a VERIFY/QA step FAIL when a non-zero exit is
// present, regardless of what the model claimed — and leaves other steps alone.
func TestTagBuildFailureIfNeeded(t *testing.T) {
	verify := OrchStep{Description: "VERIFY: run npm run build"}
	impl := OrchStep{Description: "Create projects/calc/App.tsx"}

	got := tagBuildFailureIfNeeded(verify, []kyoci.Message{{Content: "...\n[exit_status: non-zero (code 1)]"}}, "PASS")
	if !strings.HasPrefix(got, "[VERIFICATION FAILED") {
		t.Errorf("verify step with non-zero exit must be tagged FAIL; got %q", got)
	}
	if got := tagBuildFailureIfNeeded(verify, nil, "build passed"); got != "build passed" {
		t.Errorf("clean verify step must be unchanged; got %q", got)
	}
	// Non-verify step is untouched even if a non-zero marker is present.
	if got := tagBuildFailureIfNeeded(impl, []kyoci.Message{{Content: "[exit_status: non-zero (code 1)]"}}, "done"); got != "done" {
		t.Errorf("non-verify step must be unchanged; got %q", got)
	}
}
