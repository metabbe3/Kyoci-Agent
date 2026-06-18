//go:build !race

package agentdef

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadRealAgentsDir loads the real agents/ directory at the repo root and
// validates every file. This catches drift between the MD authoring and the
// loader contract — e.g. malformed frontmatter, missing required fields,
// empty bodies — before the orchestrator hits them at boot.
//
// Path is relative to the test file's package directory (internal/agentdef),
// so ../../agents reaches the repo-root agents/ folder.
func TestLoadRealAgentsDir(t *testing.T) {
	dir := filepath.Join("..", "..", "agents")
	defs, err := LoadAgents(dir)
	if err != nil {
		t.Fatalf("LoadAgents(%s): %v", dir, err)
	}
	want := map[string]bool{
		"developer":  true,
		"sre":        true,
		"qa":         true,
		"pm":         true,
		"frontend":   true,
		"generalist": true,
	}
	if len(defs) != len(want) {
		t.Fatalf("got %d defs, want %d", len(defs), len(want))
	}
	for _, d := range defs {
		if !want[d.Name] {
			t.Errorf("unexpected agent name %q", d.Name)
			continue
		}
		if d.Body == "" {
			t.Errorf("agent %q: empty body", d.Name)
		}
		if d.SystemPrompt == "" {
			t.Errorf("agent %q: empty SystemPrompt", d.Name)
		}
		if !strings.Contains(d.SystemPrompt, "VERIFICATION RULES:") {
			t.Errorf("agent %q: SystemPrompt missing shared VerificationRules block", d.Name)
		}
		if !strings.Contains(d.SystemPrompt, "DELEGATION:") {
			t.Errorf("agent %q: SystemPrompt missing shared DelegationBlock", d.Name)
		}
		if d.MaxIterations == 0 {
			t.Errorf("agent %q: MaxIterations should be set (was 0)", d.Name)
		}
		if len(d.Tools) == 0 {
			t.Errorf("agent %q: empty tools list", d.Name)
		}
		t.Logf("loaded %-12s tools=%d triggers=%d/%d/%d priority=%s",
			d.Name, len(d.Tools),
			len(d.Triggers.Keywords), len(d.Triggers.Anchors), len(d.Triggers.Regex),
			d.Priority)
	}
}

// TestRealDeveloper_RoutesFileExtensionTask confirms the developer agent wins
// when the task names a code file (anchor hit on ".go ") or build command
// (anchor hit on " go build"). Language names alone ("go", "python") are
// deliberately NOT anchors — "explain the rust async ecosystem" should route
// to generalist. This mirrors the original classifier's behavior exactly.
func TestRealDeveloper_RoutesFileExtensionTask(t *testing.T) {
	dir := filepath.Join("..", "..", "agents")
	defs, err := LoadAgents(dir)
	if err != nil {
		t.Fatalf("LoadAgents: %v", err)
	}
	// Anchor ".go " + keyword "function" → score 4.
	got := BestMatch("refactor the function in auth.go to use proper error handling", defs)
	if got != "developer" {
		t.Errorf("got %q, want developer", got)
	}
	// Anchor " go build" + keyword "function" → score 4.
	got = BestMatch("run go build and fix the function signature", defs)
	if got != "developer" {
		t.Errorf("got %q, want developer", got)
	}
}

// TestRealGeneralist_RoutesLanguageNameOnly confirms a task mentioning only a
// language name (no file extension, no build command) routes to generalist,
// not developer. This is the deliberate design: "explain rust async" should
// be research, not coding.
func TestRealGeneralist_RoutesLanguageNameOnly(t *testing.T) {
	dir := filepath.Join("..", "..", "agents")
	defs, err := LoadAgents(dir)
	if err != nil {
		t.Fatalf("LoadAgents: %v", err)
	}
	got := BestMatch("write me a Go function to parse a config file", defs)
	if got != "generalist" {
		t.Errorf("got %q, want generalist (language name without anchor should fallback)", got)
	}
}

// TestRealSRE_RoutesKubernetesTask confirms sre dispatches on a k8s task.
func TestRealSRE_RoutesKubernetesTask(t *testing.T) {
	dir := filepath.Join("..", "..", "agents")
	defs, err := LoadAgents(dir)
	if err != nil {
		t.Fatalf("LoadAgents: %v", err)
	}
	got := BestMatch("investigate why my kubernetes pod is crash-looping", defs)
	if got != "sre" {
		t.Errorf("got %q, want sre", got)
	}
}

// TestRealQA_RoutesPytestTask confirms qa dispatches on a pytest task.
func TestRealQA_RoutesPytestTask(t *testing.T) {
	dir := filepath.Join("..", "..", "agents")
	defs, err := LoadAgents(dir)
	if err != nil {
		t.Fatalf("LoadAgents: %v", err)
	}
	got := BestMatch("add a pytest test case for the auth function", defs)
	if got != "qa" {
		t.Errorf("got %q, want qa", got)
	}
}

// TestRealFrontend_WinsOverDeveloperOnReact confirms the priority tiebreaker
// works: "react component" matches both frontend (anchor " react ") and
// developer (no anchors, but might match keywords). Frontend should win.
func TestRealFrontend_WinsOverDeveloperOnReact(t *testing.T) {
	dir := filepath.Join("..", "..", "agents")
	defs, err := LoadAgents(dir)
	if err != nil {
		t.Fatalf("LoadAgents: %v", err)
	}
	got := BestMatch("build a react component for the user profile card", defs)
	if got != "frontend" {
		t.Errorf("got %q, want frontend (priority tiebreaker)", got)
	}
}

// TestRealGeneralist_FallbackForResearch confirms generalist wins for an
// open-ended research task that no specialist should claim.
func TestRealGeneralist_FallbackForResearch(t *testing.T) {
	dir := filepath.Join("..", "..", "agents")
	defs, err := LoadAgents(dir)
	if err != nil {
		t.Fatalf("LoadAgents: %v", err)
	}
	got := BestMatch("what's the difference between async and sync programming", defs)
	if got != "generalist" {
		t.Errorf("got %q, want generalist (fallback)", got)
	}
}
