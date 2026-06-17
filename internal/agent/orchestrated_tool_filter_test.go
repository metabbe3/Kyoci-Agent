package agent

import (
	"sort"
	"testing"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// =============================================================================
// Go-side MCP tool enforcement (Step 6)
//
// When a plan step description references an MCP tool by name, the worker's
// function-calling payload is filtered to contain ONLY the referenced MCP
// tool(s) + `file` + `terminal`. Physically removing competing tools (notably
// web_search and file-search-as-substitute) is the only reliable way to make
// gemma4:12b call the MCP tool — prompt constraints alone were ignored because
// the model could "guess" the schema from training data.
//
// These tests pin the two pure helper functions that drive the filter.
// =============================================================================

// sampleToolDefs builds a registry-shaped slice covering the cases that matter:
// built-in tools the filter must DROP (web_search, memory_recall), the two
// built-ins the filter must KEEP (file, terminal), and the MCP tool under test.
func sampleToolDefs() []kyoci.ToolDefinition {
	return []kyoci.ToolDefinition{
		{Name: "file", Description: "read/write files"},
		{Name: "terminal", Description: "run shell commands"},
		{Name: "web_search", Description: "search the web"},
		{Name: "memory_recall", Description: "recall memories"},
		{Name: "remember", Description: "store a memory"},
		{Name: "kyoci_fetch_user_schema", Description: "MANDATORY: fetch user schema"},
		{Name: "kyoci_other_mcp", Description: "another MCP tool"},
	}
}

// --- mcpToolsReferencedBy ---

func TestMCPToolsReferencedBy_HitOnExactName(t *testing.T) {
	all := sampleToolDefs()
	desc := "Use the 'kyoci_fetch_user_schema' tool to fetch the user profile schema."

	got := mcpToolsReferencedBy(desc, all)

	want := []string{"kyoci_fetch_user_schema"}
	if !equalStringSet(got, want) {
		t.Fatalf("mcpToolsReferencedBy = %v, want %v", got, want)
	}
}

func TestMCPToolsReferencedBy_HitsMultipleMCPNames(t *testing.T) {
	all := sampleToolDefs()
	desc := "Call kyoci_fetch_user_schema then kyoci_other_mcp to combine results."

	got := mcpToolsReferencedBy(desc, all)

	want := []string{"kyoci_fetch_user_schema", "kyoci_other_mcp"}
	if !equalStringSet(got, want) {
		t.Fatalf("mcpToolsReferencedBy = %v, want %v", got, want)
	}
}

func TestMCPToolsReferencedBy_EmptyWhenNoMCPName(t *testing.T) {
	all := sampleToolDefs()
	// Description references only built-in concepts — the filter must NOT fire,
	// otherwise L1/L2 behavior regresses.
	desc := "Search files in the working directory for an existing user schema."

	got := mcpToolsReferencedBy(desc, all)

	if len(got) != 0 {
		t.Fatalf("mcpToolsReferencedBy = %v, want empty (filter must not fire on built-in concepts)", got)
	}
}

func TestMCPToolsReferencedBy_IgnoresBuiltinNames(t *testing.T) {
	all := sampleToolDefs()
	// Even though "file" appears in the registry and the description, it is a
	// built-in tool and must never be reported as a referenced MCP tool.
	desc := "Use the file tool to read config."

	got := mcpToolsReferencedBy(desc, all)

	if len(got) != 0 {
		t.Fatalf("mcpToolsReferencedBy = %v, want empty (built-in names must be ignored)", got)
	}
}

// TestMCPToolsReferencedBy_MatchesUnprefixedName is the load-bearing regression
// test for the case where the planner emits the bare tool name (e.g.
// `fetch_user_schema`) without the MCP manager prefix (`kyoci_`). The filter
// MUST still fire, otherwise the model keeps the full tool list and substitutes
// file/web_search for the MCP tool — exactly the failure observed in benchmark
// run b7cf42b, where mcp_calls=1 was a false positive from the plan-step text
// and the tool was never actually executed.
func TestMCPToolsReferencedBy_MatchesUnprefixedName(t *testing.T) {
	all := sampleToolDefs()
	desc := "Use the fetch_user_schema tool to retrieve the user profile schema."

	got := mcpToolsReferencedBy(desc, all)

	want := []string{"kyoci_fetch_user_schema"}
	if !equalStringSet(got, want) {
		t.Fatalf("mcpToolsReferencedBy(unprefixed) = %v, want %v (filter must fire on bare name too)", got, want)
	}
}

// --- filterToolsForMCP ---

func TestFilterToolsForMCP_KeepsReferencedPlusFileTerminal(t *testing.T) {
	all := sampleToolDefs()
	referenced := []string{"kyoci_fetch_user_schema"}

	got := filterToolsForMCP(all, referenced)

	gotNames := namesFromDefs(got)
	wantNames := []string{"kyoci_fetch_user_schema", "file", "terminal"}
	if !equalStringSet(gotNames, wantNames) {
		t.Fatalf("filterToolsForMCP names = %v, want %v", gotNames, wantNames)
	}
	// Sanity: the substitutes the model reaches for MUST be absent.
	for _, banned := range []string{"web_search", "memory_recall", "remember"} {
		if sliceContains(gotNames, banned) {
			t.Fatalf("filterToolsForMCP kept banned tool %q; expected it stripped", banned)
		}
	}
}

func TestFilterToolsForMCP_KeepsMultipleMCPTools(t *testing.T) {
	all := sampleToolDefs()
	referenced := []string{"kyoci_fetch_user_schema", "kyoci_other_mcp"}

	got := filterToolsForMCP(all, referenced)

	gotNames := namesFromDefs(got)
	wantNames := []string{"kyoci_fetch_user_schema", "kyoci_other_mcp", "file", "terminal"}
	if !equalStringSet(gotNames, wantNames) {
		t.Fatalf("filterToolsForMCP names = %v, want %v", gotNames, wantNames)
	}
}

func TestFilterToolsForMCP_EmptyWhenReferencedNotRegistered(t *testing.T) {
	all := sampleToolDefs()
	// Defensive: if the referenced tool isn't actually registered, we still
	// keep file+terminal so the worker can make progress (rather than getting
	// an empty tool list, which some providers reject).
	referenced := []string{"nonexistent_mcp_tool"}

	got := filterToolsForMCP(all, referenced)

	gotNames := namesFromDefs(got)
	if !equalStringSet(gotNames, []string{"file", "terminal"}) {
		t.Fatalf("filterToolsForMCP names = %v, want [file terminal] fallback", gotNames)
	}
}

// --- helpers used by the tests ---

func namesFromDefs(defs []kyoci.ToolDefinition) []string {
	out := make([]string, 0, len(defs))
	for _, d := range defs {
		out = append(out, d.Name)
	}
	return out
}

func equalStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	a2 := append([]string(nil), a...)
	b2 := append([]string(nil), b...)
	sort.Strings(a2)
	sort.Strings(b2)
	for i := range a2 {
		if a2[i] != b2[i] {
			return false
		}
	}
	return true
}

func sliceContains(xs []string, target string) bool {
	for _, x := range xs {
		if x == target {
			return true
		}
	}
	return false
}
