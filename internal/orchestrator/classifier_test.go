package orchestrator

import (
	"path/filepath"
	"testing"

	"github.com/metabbe3/Kyoci-Agent/internal/agentdef"
	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// loadRealDefs is a shared helper for tests that need the production agent
// set. Reads from the repo-root agents/ directory.
func loadRealDefs(t *testing.T) []agentdef.AgentDef {
	t.Helper()
	defs, err := agentdef.LoadAgents(filepath.Join("..", "..", "agents"))
	if err != nil {
		t.Fatalf("LoadAgents: %v", err)
	}
	if len(defs) == 0 {
		t.Fatal("no agents loaded — agents/ dir is empty or missing")
	}
	return defs
}

// TestClassifyWithAgents_SpecialistRouting locks in the contract that clear
// specialist signals route to the matching specialist. Strong anchors (file
// extensions, framework names) and any two weak hits should both be enough.
//
// Migrated from the pre-refactor TestClassifyRole_SpecialistRouting: the
// assertions are identical, only the call site changed from the wrapper to
// the explicit ClassifyWithAgents so the test is hermetic (doesn't depend on
// package-global state).
func TestClassifyWithAgents_SpecialistRouting(t *testing.T) {
	defs := loadRealDefs(t)
	cases := []struct {
		name string
		task string
		want kyoci.RoleType
	}{
		// Frontend — strong anchors
		{"react component", "build a react dashboard with charts", kyoci.RoleFrontend},
		{"css file", "fix the broken css in styles.css", kyoci.RoleFrontend},
		{"tsx file", "refactor this .tsx component", kyoci.RoleFrontend},
		{"tailwind", "switch the page to tailwind utilities", kyoci.RoleFrontend},
		// Frontend — two weak hits
		{"ui component", "build a button component for the ui", kyoci.RoleFrontend},

		// SRE — strong anchors
		{"k8s deploy", "deploy to kubernetes with health checks", kyoci.RoleSRE},
		{"docker container", "debug the docker container that won't start", kyoci.RoleSRE},
		{"nginx config", "fix the nginx config for ssl termination", kyoci.RoleSRE},
		// SRE — two weak hits
		{"disk + memory", "check disk space and memory usage on the server", kyoci.RoleSRE},

		// QA — strong anchors
		{"pytest file", "add a pytest file for the auth module", kyoci.RoleQA},
		{"go test file", "write tests in parser_test.go for the new branch", kyoci.RoleQA},
		// QA — two weak hits
		{"write test cases", "write test cases for the user service", kyoci.RoleQA},

		// Developer — strong anchors
		{"go build", "run go build and fix any errors", kyoci.RoleDeveloper},
		{"python file", "refactor this .py file to use async", kyoci.RoleDeveloper},
		// Developer — two weak hits
		{"refactor function", "refactor the function and add error handling", kyoci.RoleDeveloper},

		// PM — strong anchors
		{"project plan", "draft a project plan for the Q3 roadmap", kyoci.RolePM},
		{"scrum sprint", "plan the next scrum sprint", kyoci.RolePM},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ClassifyWithAgents(c.task, defs)
			if got != c.want {
				t.Errorf("ClassifyWithAgents(%q) = %q, want %q", c.task, got, c.want)
			}
		})
	}
}

// TestClassifyWithAgents_GeneralistFallback confirms ambiguous / research
// tasks route to generalist, not a specialist that would mishandle them.
// This is the critical fix the original classifier made: developer used to be
// the fallback, but its prompt forbids prose — research tasks were broken.
func TestClassifyWithAgents_GeneralistFallback(t *testing.T) {
	defs := loadRealDefs(t)
	cases := []struct {
		name string
		task string
	}{
		{"open research", "what's the difference between async and sync"},
		{"explanation", "explain how the rust async runtime works"},
		{"comparison", "compare postgres and mysql for small projects"},
		{"arithmetic", "what is 2 + 2"},
		{"capital city", "what's the capital of France"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ClassifyWithAgents(c.task, defs)
			if got != kyoci.RoleGeneralist {
				t.Errorf("ClassifyWithAgents(%q) = %q, want generalist", c.task, got)
			}
		})
	}
}

// TestClassifyWithAgents_FrontendPriorityTiebreaker confirms the priority
// field on AgentDef breaks ties correctly: react/css/tsx match both frontend
// (high priority) and developer (normal priority) — frontend must win.
func TestClassifyWithAgents_FrontendPriorityTiebreaker(t *testing.T) {
	defs := loadRealDefs(t)
	// "build a react component" — frontend anchor " react " (score 3),
	// no developer anchors. Frontend wins outright by score, not just priority.
	got := ClassifyWithAgents("build a react component for the user profile", defs)
	if got != kyoci.RoleFrontend {
		t.Errorf("got %q, want frontend", got)
	}
}

// TestClassifyRole_WrapperUsesDefaultDefs confirms the package-global wrapper
// picks up the agent set installed via SetDefaultAgentDefs. Production code
// (Orchestrator.Execute) calls ClassifyRole; this test pins the contract that
// the wrapper is a thin shim over ClassifyWithAgents.
func TestClassifyRole_WrapperUsesDefaultDefs(t *testing.T) {
	// Save and restore global state so this test doesn't leak.
	defaultDefsMu.RLock()
	prev := defaultDefs
	defaultDefsMu.RUnlock()
	defer SetDefaultAgentDefs(prev)

	defs := loadRealDefs(t)
	SetDefaultAgentDefs(defs)

	got := ClassifyRole("deploy to kubernetes with health checks")
	if got != kyoci.RoleSRE {
		t.Errorf("ClassifyRole with default defs: got %q, want sre", got)
	}
}

// TestClassifyRole_EmptyDefaultDefsFallsBackToGeneralist confirms the wrapper
// is safe to call before SetDefaultAgentDefs has run (e.g. during early init
// or in tests that don't load agents). Returns RoleGeneralist rather than
// panicking.
func TestClassifyRole_EmptyDefaultDefsFallsBackToGeneralist(t *testing.T) {
	defaultDefsMu.RLock()
	prev := defaultDefs
	defaultDefsMu.RUnlock()
	defer SetDefaultAgentDefs(prev)

	SetDefaultAgentDefs(nil)
	got := ClassifyRole("literally anything")
	if got != kyoci.RoleGeneralist {
		t.Errorf("ClassifyRole with no defs: got %q, want generalist", got)
	}
}
