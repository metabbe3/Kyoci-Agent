package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// =====================================================================================
// ReadOnlyToolFilter + Explore dispatch tests.
//
// The filter is the security boundary for the explore worker — it must be
// airtight. These tests verify:
//   1. Blocked tools are invisible in List() (the model can't even see them).
//   2. Blocked tools are rejected at Execute() (defense in depth).
//   3. The "file" tool's write actions are rejected even though the tool name
//      is in the allowlist (file is allowed for read but not for write).
//   4. Allowed tools pass through transparently.
// =====================================================================================

// stubToolProvider is a minimal ToolProvider for filter tests. Returns whatever
// ToolDefinitions it was constructed with and records Execute calls.
type stubToolProvider struct {
	defs     []kyoci.ToolDefinition
	execs    []execCall
	execResp string
	execErr  error
}

type execCall struct {
	name   string
	params map[string]interface{}
}

func (s *stubToolProvider) Register(kyoci.Tool) error { return nil }
func (s *stubToolProvider) List() []kyoci.ToolDefinition {
	return append([]kyoci.ToolDefinition(nil), s.defs...)
}
func (s *stubToolProvider) Execute(_ context.Context, name string, params map[string]interface{}) (string, error) {
	s.execs = append(s.execs, execCall{name: name, params: params})
	return s.execResp, s.execErr
}

func defaultExploreDefs() []kyoci.ToolDefinition {
	// A realistic mix: allowed explore tools + dangerous ones that must be filtered.
	return []kyoci.ToolDefinition{
		{Name: "glob"},
		{Name: "grep"},
		{Name: "file"},
		{Name: "git"},
		{Name: "codesearch"},
		{Name: "lsp"},
		{Name: "todo"},
		{Name: "patch"},   // must be filtered out
		{Name: "terminal"}, // must be filtered out
		{Name: "web_fetch"},
		{Name: "secret_scan"},
	}
}

func TestReadOnlyToolFilter_BlockedToolsHidden(t *testing.T) {
	inner := &stubToolProvider{defs: defaultExploreDefs()}
	filter := NewReadOnlyToolFilter(inner, nil)

	visible := filter.List()
	names := map[string]bool{}
	for _, td := range visible {
		names[td.Name] = true
	}
	// Allowed tools should be visible.
	for _, allowed := range []string{"glob", "grep", "file", "git", "codesearch", "lsp", "todo"} {
		if !names[allowed] {
			t.Errorf("allowed tool %q should be visible", allowed)
		}
	}
	// Blocked tools should be invisible.
	for _, blocked := range []string{"patch", "terminal", "web_fetch", "secret_scan"} {
		if names[blocked] {
			t.Errorf("blocked tool %q should be hidden, got visible", blocked)
		}
	}
}

func TestReadOnlyToolFilter_BlockedExecuteRejected(t *testing.T) {
	inner := &stubToolProvider{defs: defaultExploreDefs(), execResp: "should not be returned"}
	filter := NewReadOnlyToolFilter(inner, nil)

	// Attempting to execute a blocked tool name returns ErrExploreToolNotAllowed.
	_, err := filter.Execute(context.Background(), "patch", map[string]interface{}{})
	if !errors.Is(err, ErrExploreToolNotAllowed) {
		t.Errorf("patch execute: got err=%v, want ErrExploreToolNotAllowed", err)
	}
	_, err = filter.Execute(context.Background(), "terminal", map[string]interface{}{})
	if !errors.Is(err, ErrExploreToolNotAllowed) {
		t.Errorf("terminal execute: got err=%v, want ErrExploreToolNotAllowed", err)
	}
	if len(inner.execs) != 0 {
		t.Errorf("inner Execute called %d times; want 0 (filter should short-circuit)", len(inner.execs))
	}
}

func TestReadOnlyToolFilter_FileWriteActionsRejected(t *testing.T) {
	inner := &stubToolProvider{defs: defaultExploreDefs(), execResp: "ok"}
	filter := NewReadOnlyToolFilter(inner, nil)

	// "file" tool is allowed, but write actions inside it are not.
	for _, action := range []string{"write", "append", "delete", "mkdir", "move", "copy"} {
		_, err := filter.Execute(context.Background(), "file", map[string]interface{}{
			"action": action,
			"path":   "/tmp/x",
		})
		if !errors.Is(err, ErrExploreToolNotAllowed) {
			t.Errorf("file action %q: got err=%v, want ErrExploreToolNotAllowed", action, err)
		}
	}

	// file:read should pass through to inner.
	_, err := filter.Execute(context.Background(), "file", map[string]interface{}{
		"action": "read",
		"path":   "/tmp/x",
	})
	if err != nil {
		t.Errorf("file read: got err=%v, want nil", err)
	}
	if len(inner.execs) != 1 || inner.execs[0].name != "file" {
		t.Errorf("inner not called for read: %+v", inner.execs)
	}
}

func TestReadOnlyToolFilter_FileDefaultActionIsRead(t *testing.T) {
	// If action param is missing entirely, the filter assumes "read" and lets
	// it through. This matches the file tool's own default behavior.
	inner := &stubToolProvider{defs: defaultExploreDefs(), execResp: "data"}
	filter := NewReadOnlyToolFilter(inner, nil)

	_, err := filter.Execute(context.Background(), "file", map[string]interface{}{
		"path": "/tmp/x",
	})
	if err != nil {
		t.Errorf("file with no action: got err=%v, want nil (default to read)", err)
	}
}

func TestReadOnlyToolFilter_AllowedToolsPassThrough(t *testing.T) {
	inner := &stubToolProvider{defs: defaultExploreDefs(), execResp: "result"}
	filter := NewReadOnlyToolFilter(inner, nil)

	for _, name := range []string{"glob", "grep", "git", "codesearch", "lsp"} {
		_, err := filter.Execute(context.Background(), name, map[string]interface{}{"q": "foo"})
		if err != nil {
			t.Errorf("%s execute: got err=%v, want nil", name, err)
		}
	}
	if len(inner.execs) != 5 {
		t.Errorf("inner execs: got %d, want 5", len(inner.execs))
	}
}

func TestReadOnlyToolFilter_CustomAllowlist(t *testing.T) {
	// Caller can restrict further with a custom allowlist (e.g. grep-only).
	inner := &stubToolProvider{defs: defaultExploreDefs()}
	custom := map[string]bool{"grep": true}
	filter := NewReadOnlyToolFilter(inner, custom)

	visible := filter.List()
	if len(visible) != 1 || visible[0].Name != "grep" {
		names := []string{}
		for _, td := range visible {
			names = append(names, td.Name)
		}
		t.Errorf("custom allowlist visible = %v, want [grep]", names)
	}
}

// =====================================================================================
// Explore dispatch helpers — prefix detection and stripping.
// =====================================================================================

func TestHasExplorePrefix(t *testing.T) {
	positives := []string{
		"explore: find all uses of context.Background()",
		"explore find the auth flow",
		"EXPLORE: find foo",
		"  explore:  something  ",
	}
	negatives := []string{
		"find all uses",
		"exploration of foo", // close but not a prefix
		"",
		"delegation: do something",
	}
	for _, in := range positives {
		if !HasExplorePrefix(in) {
			t.Errorf("HasExplorePrefix(%q) = false, want true", in)
		}
	}
	for _, in := range negatives {
		if HasExplorePrefix(in) {
			t.Errorf("HasExplorePrefix(%q) = true, want false", in)
		}
	}
}

func TestStripExplorePrefix(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"explore: find foo", "find foo"},
		{"explore find foo", "find foo"},
		{"EXPLORE: find foo", "find foo"},
		{"  explore:  find foo  ", "find foo"},
		{"find foo", "find foo"}, // no prefix → unchanged
		{"", ""},
	}
	for _, c := range cases {
		if got := StripExplorePrefix(c.in); got != c.want {
			t.Errorf("StripExplorePrefix(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// =====================================================================================
// ExploreSystemPrompt — sanity check it has the directives we depend on.
// =====================================================================================

func TestExploreSystemPrompt_Directives(t *testing.T) {
	low := strings.ToLower(ExploreSystemPrompt)
	for _, want := range []string{"read-only", "file:line", "markdown", "cannot edit"} {
		if !strings.Contains(low, want) {
			t.Errorf("ExploreSystemPrompt missing %q", want)
		}
	}
}

// =====================================================================================
// ExploreToolAllowlist — verify the allowlist contents are tight.
// =====================================================================================

func TestExploreToolAllowlist_Tight(t *testing.T) {
	// Allowed tools are read-only by design.
	for _, allowed := range []string{"glob", "grep", "file", "git", "codesearch", "lsp", "todo"} {
		if !ExploreToolAllowlist[allowed] {
			t.Errorf("ExploreToolAllowlist should include %q", allowed)
		}
	}
	// Dangerous tools must NOT be in the allowlist.
	for _, blocked := range []string{"patch", "terminal", "web_fetch", "secret_scan", "delegation", "memory_recall"} {
		if ExploreToolAllowlist[blocked] {
			t.Errorf("ExploreToolAllowlist must NOT include %q (would let explore mutate or escape)", blocked)
		}
	}
}
