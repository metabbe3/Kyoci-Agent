package agentdef

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeAgent writes a single agent MD file under a fresh tmp dir + subname.
func writeAgent(t *testing.T, dir, name, body string) {
	t.Helper()
	path := filepath.Join(dir, name+".md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// minimalValidAgent is the smallest valid frontmatter+body for tests.
const minimalValidAgent = `---
name: test-agent
description: "test fixture"
triggers:
  keywords: [foo, bar]
  anchors: []
  regex: []
tools: []
max_iterations: 5
memory:
  enabled: true
  recall_depth: 3
priority: normal
---
# Soul

You are a test agent. Do things.
`

func TestLoadAgents_ParsesMinimalAgent(t *testing.T) {
	dir := t.TempDir()
	writeAgent(t, dir, "test-agent", minimalValidAgent)

	defs, err := LoadAgents(dir)
	if err != nil {
		t.Fatalf("LoadAgents: %v", err)
	}
	if len(defs) != 1 {
		t.Fatalf("got %d defs, want 1", len(defs))
	}
	d := defs[0]
	if d.Name != "test-agent" {
		t.Errorf("Name: got %q, want test-agent", d.Name)
	}
	if d.MaxIterations != 5 {
		t.Errorf("MaxIterations: got %d, want 5", d.MaxIterations)
	}
	if !d.Memory.Enabled {
		t.Errorf("Memory.Enabled: got false, want true")
	}
	if d.Memory.RecallDepth != 3 {
		t.Errorf("Memory.RecallDepth: got %d, want 3", d.Memory.RecallDepth)
	}
	if !strings.Contains(d.Body, "test agent") {
		t.Errorf("Body missing content: %q", d.Body)
	}
	// SystemPrompt must have shared closing blocks appended via Compose.
	if !strings.Contains(d.SystemPrompt, "VERIFICATION RULES:") {
		t.Errorf("SystemPrompt missing VerificationRules")
	}
	if !strings.Contains(d.SystemPrompt, "DELEGATION:") {
		t.Errorf("SystemPrompt missing DelegationBlock")
	}
	if !strings.Contains(d.SystemPrompt, "Keep responses SHORT") {
		t.Errorf("SystemPrompt missing ClosingDirective")
	}
}

func TestLoadAgents_SkipsMissingName(t *testing.T) {
	dir := t.TempDir()
	missing := strings.Replace(minimalValidAgent, "name: test-agent", "", 1)
	writeAgent(t, dir, "bad", missing)

	defs, err := LoadAgents(dir)
	if err != nil {
		t.Fatalf("LoadAgents: %v", err)
	}
	if len(defs) != 0 {
		t.Fatalf("got %d defs, want 0 (missing name should skip)", len(defs))
	}
}

func TestLoadAgents_SkipsEmptyBody(t *testing.T) {
	dir := t.TempDir()
	empty := `---
name: empty-body
description: "no soul"
triggers:
  keywords: [x]
---
`
	writeAgent(t, dir, "empty-body", empty)

	defs, err := LoadAgents(dir)
	if err != nil {
		t.Fatalf("LoadAgents: %v", err)
	}
	if len(defs) != 0 {
		t.Fatalf("got %d defs, want 0 (empty body should skip)", len(defs))
	}
}

func TestLoadAgents_DuplicateNameSkipsSecond(t *testing.T) {
	dir := t.TempDir()
	writeAgent(t, dir, "first", minimalValidAgent)
	// Same name in a different file.
	writeAgent(t, dir, "second", strings.Replace(minimalValidAgent, "name: test-agent", "name: dup\ndescription: x", 1))
	// Re-rename to test-agent to force duplicate.
	dup := strings.Replace(minimalValidAgent, "name: test-agent", "name: test-agent", 1)
	_ = dup
	writeAgent(t, dir, "zzz-second", minimalValidAgent)

	defs, err := LoadAgents(dir)
	if err != nil {
		t.Fatalf("LoadAgents: %v", err)
	}
	if len(defs) != 1 {
		t.Fatalf("got %d defs, want 1 (duplicate name should skip)", len(defs))
	}
}

func TestLoadAgents_MissingDirReturnsEmpty(t *testing.T) {
	defs, err := LoadAgents("/this/does/not/exist/agents")
	if err != nil {
		t.Fatalf("LoadAgents on missing dir: %v", err)
	}
	if len(defs) != 0 {
		t.Fatalf("got %d defs, want 0", len(defs))
	}
}

func TestLoadAgents_RecursesSubdirectories(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "engineering")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeAgent(t, sub, "nested", minimalValidAgent)

	defs, err := LoadAgents(dir)
	if err != nil {
		t.Fatalf("LoadAgents: %v", err)
	}
	if len(defs) != 1 {
		t.Fatalf("got %d defs, want 1 (recursive)", len(defs))
	}
}

func TestParseAgentFile_SubstitutesPlatformToken(t *testing.T) {
	dir := t.TempDir()
	withToken := strings.Replace(minimalValidAgent,
		"You are a test agent. Do things.",
		"You are a test agent.\n\n{{platform}}\n\nDo things.",
		1)
	writeAgent(t, dir, "test-agent", withToken)

	defs, err := LoadAgents(dir)
	if err != nil {
		t.Fatalf("LoadAgents: %v", err)
	}
	if len(defs) != 1 {
		t.Fatalf("got %d defs, want 1", len(defs))
	}
	if strings.Contains(defs[0].SystemPrompt, "{{platform}}") {
		t.Errorf("{{platform}} token not substituted in SystemPrompt")
	}
	// platformSection is OS-specific; on macOS it includes "macOS", on Linux
	// "Linux". Just assert non-empty substitution happened.
	if defs[0].Body == defs[0].SystemPrompt {
		t.Errorf("SystemPrompt should differ from Body (Compose + substitution)")
	}
}

func TestSubstitutePlatformTokens_ReplacesAllOccurrences(t *testing.T) {
	in := "before {{platform}} middle {{platform}} after"
	out := SubstitutePlatformTokens(in)
	if strings.Contains(out, "{{platform}}") {
		t.Errorf("token remains: %q", out)
	}
	if out == in {
		t.Errorf("no substitution occurred")
	}
}

func TestCompose_AppendsSharedBlocks(t *testing.T) {
	got := Compose("agent body")
	for _, want := range []string{"agent body", "VERIFICATION RULES:", "DELEGATION:", "Keep responses SHORT"} {
		if !strings.Contains(got, want) {
			t.Errorf("Compose missing %q in:\n%s", want, got)
		}
	}
}
