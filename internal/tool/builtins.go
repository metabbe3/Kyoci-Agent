package tool

// =============================================================================
// Step 7a — Shared built-in tool name set
//
// IsBuiltinName is the single source of truth for distinguishing tools that
// ship with the agent (and should be gated by the per-role allowlist) from
// dynamically-loaded MCP tools (which should pass through to every role).
//
// Why this matters: internal/role/registry.go filters tools through each
// role's hardcoded Tools list. MCP tools (e.g. kyoci_fetch_user_schema) are
// registered at runtime by the MCP manager and therefore cannot appear in
// any role config. Before Step 7 the filter dropped them silently, which
// meant orchestrated workers never saw MCP tools — the L3 benchmark's
// `mcp_calls=1` was a grader false positive matching plan-step text.
//
// The fix: the role filter treats any tool whose name is NOT in this set as
// a user-installed MCP tool and passes it through unconditionally.
//
// KEEP IN SYNC with Registry.RegisterBuiltin() in registry.go and the
// intelligence tools registered by the orchestrator (memory_recall,
// remember, security_scan, delegation). If you add a new built-in tool,
// add its name here too — internal/tool/builtins_test.go has a guard test.
// =============================================================================

var builtinNames = map[string]bool{
	// Tools registered by Registry.RegisterBuiltin().
	"terminal":    true,
	"file":        true,
	"http_client": true,
	"web_search":  true,
	"calculator":  true,
	"browser":     true,
	"docs":        true,
	"todo":        true,
	"skill":       true,
	"process":     true,

	// Intelligence hooks registered by the orchestrator into the same
	// registry. Treated as built-in for allowlisting purposes.
	"memory_recall": true,
	"remember":      true,
	"security_scan": true,
	"delegation":    true,
}

// IsBuiltinName reports whether name is a built-in (non-MCP) tool. Returns
// false for empty strings, MCP-prefixed names (e.g. "kyoci_*"),
// user-defined tools, and any name not in the canonical set above.
func IsBuiltinName(name string) bool {
	return builtinNames[name]
}
