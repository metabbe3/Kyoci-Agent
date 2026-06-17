package tool

import "testing"

// =============================================================================
// Step 7a — Shared built-in tool name set
//
// The per-role tool filter in internal/role/registry.go drops any tool whose
// name isn't in the role's hardcoded Tools allowlist. That filter is correct
// for built-ins (so a frontend agent can't reach security_scan), but it
// incorrectly drops dynamically-loaded MCP tools (e.g. kyoci_fetch_user_schema)
// because those names cannot be known ahead of time and listed in a role
// config.
//
// IsBuiltinName is the single source of truth that lets the role filter
// distinguish "this is a built-in tool that should be gated by the allowlist"
// from "this is a user-installed MCP tool that should pass through to every
// role unconditionally".
//
// This set must stay in sync with Registry.RegisterBuiltin() and the
// intelligence tools registered by the orchestrator. If you add a new
// built-in tool, add its name here AND to RegisterBuiltin().
// =============================================================================

func TestIsBuiltinName(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		// Built-in tools registered by Registry.RegisterBuiltin().
		{"terminal", true},
		{"file", true},
		{"http_client", true},
		{"web_search", true},
		{"calculator", true},
		{"browser", true},
		{"docs", true},
		{"todo", true},
		{"skill", true},
		{"process", true},

		// Intelligence hooks registered by the orchestrator. These live in
		// the same registry as the built-ins, so they must be treated as
		// built-in for allowlisting purposes.
		{"memory_recall", true},
		{"remember", true},
		{"security_scan", true},
		{"delegation", true},

		// Dynamically loaded MCP tools. These MUST return false — they are
		// the entire reason IsBuiltinName exists. The role filter uses this
		// signal to pass them through.
		{"kyoci_fetch_user_schema", false},
		{"kyoci_other_mcp", false},
		{"fetch_user_schema", false}, // bare MCP name (planner may emit this)
		{"github_create_issue", false},
		{"slack_send_message", false},

		// Edge cases.
		{"", false},              // empty is not a built-in (and not a valid tool)
		{"Terminal", false},      // case-sensitive — uppercase is NOT built-in
		{"file_v2", false},       // suffix on a built-in name is a different tool
		{"my_custom_tool", false}, // user-defined
		{"random_string", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := IsBuiltinName(tc.name)
			if got != tc.want {
				t.Errorf("IsBuiltinName(%q) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

// TestBuiltinNameSet_CoversRegisterBuiltin is the load-bearing sync guard:
// if a future change adds a tool to RegisterBuiltin but forgets to add its
// name to the builtinNames set, the role filter would wrongly pass it through
// to roles that shouldn't have it. We can't import the builtin package here
// without an import cycle, so this test instead pins the known set by name —
// if RegisterBuiltin ever adds a tool, this test must be updated alongside
// the builtinNames set.
func TestBuiltinNameSet_CoversKnownRegisterBuiltinNames(t *testing.T) {
	// These names must match Registry.RegisterBuiltin() in registry.go.
	// If RegisterBuiltin changes, update BOTH that function AND this list.
	knownBuiltinNames := []string{
		"terminal", "file", "http_client", "web_search",
		"calculator", "browser", "docs", "todo", "skill", "process",
	}
	for _, name := range knownBuiltinNames {
		if !IsBuiltinName(name) {
			t.Errorf("RegisterBuiltin registers %q but IsBuiltinName returns false — "+
				"the role filter would pass it through to roles that shouldn't have it", name)
		}
	}
}
