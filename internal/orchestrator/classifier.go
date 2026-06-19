package orchestrator

import (
	"sync"

	"github.com/metabbe3/Kyoci-Agent/internal/agentdef"
	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// =====================================================================================
// Task → agent classifier
//
// Routes a task string to the best-matching agent by scoring each loaded
// AgentDef's triggers (keywords + anchors + regex) against the task. Pure Go —
// no LLM call.
//
// Routing philosophy, preserved from the pre-refactor hardcoded classifier:
//
//  1. Keywords score +1 each, anchors score +3 each, regex +1 each.
//  2. A specialist agent wins only if its score >= MinSpecialistScore (2) —
//     a single accidental substring match (e.g. "ui " inside "quit") is NOT
//     enough.
//  3. If no specialist clears the bar, the agent literally named "generalist"
//     wins as the fallback.
//  4. Ties are broken first by PriorityRank (high > normal > low), then by
//     load order (alphabetical filename).
//
// Why no LLM routing: a small model making a bad route decision costs a full
// pipeline run (planner + workers + synthesizer). A pure-Go heuristic is
// faster, deterministic, and debuggable.
//
// The triggers themselves are data-driven — each agent's keywords/anchors live
// in its agents/<name>.md frontmatter, not in Go source. Adding a new agent
// or retuning an existing one's routing is now a markdown edit.
// =====================================================================================

// defaultDefs is the package-global agent set used by the ClassifyRole wrapper.
// Populated once at orchestrator init via SetDefaultAgentDefs. Tests that need
// hermetic classification call ClassifyWithAgents directly with their own defs.
var (
	defaultDefs    []agentdef.AgentDef
	defaultDefsMu  sync.RWMutex
)

// SetDefaultAgentDefs installs the agent set the ClassifyRole wrapper consults.
// Called once at orchestrator construction after LoadAgents succeeds. Safe to
// call multiple times — the last call wins. Pass nil or empty to clear.
func SetDefaultAgentDefs(defs []agentdef.AgentDef) {
	defaultDefsMu.Lock()
	defer defaultDefsMu.Unlock()
	defaultDefs = defs
}

// ClassifyRole auto-detects which agent should handle a task using the
// package-global default agent set (installed via SetDefaultAgentDefs at
// orchestrator init). Returns RoleGeneralist if no def is loaded or no
// specialist clears the threshold.
//
// Thin wrapper preserved for backward compatibility — production callers
// (Orchestrator.Execute, delegation, the dashboard) keep their existing
// signatures. New tests should prefer ClassifyWithAgents for hermetic fixtures.
func ClassifyRole(task string) kyoci.RoleType {
	defaultDefsMu.RLock()
	defs := defaultDefs
	defaultDefsMu.RUnlock()
	if len(defs) == 0 {
		// No agents loaded — fall back to the generalist constant so the
		// caller still has somewhere to dispatch. This branch is hit when
		// the orchestrator boots without an agents/ dir or during early
		// tests that don't initialize the registry.
		return kyoci.RoleGeneralist
	}
	return ClassifyWithAgents(task, defs)
}

// ClassifyWithAgents scores every def's triggers against the task and returns
// the best match as a RoleType. The returned RoleType's string value is the
// agent's name — the role registry looks it up directly.
//
// Mirrors agentdef.BestMatch but returns a RoleType for orchestrator-side
// callers. When no specialist clears MinSpecialistScore and no agent is named
// "generalist", returns the RoleGeneralist constant so the orchestrator's
// fallback path still resolves to a registered agent.
func ClassifyWithAgents(task string, defs []agentdef.AgentDef) kyoci.RoleType {
	name := agentdef.BestMatch(task, defs)
	if name == "" {
		return kyoci.RoleGeneralist
	}
	return kyoci.RoleType(name)
}

// currentAgentDefs returns the package-global agent set under a read lock.
// Callers that only need to read the loaded defs (e.g. provenance scoring in
// autoresolve.go) should use this rather than touching defaultDefs directly.
// The returned slice shares its backing array; readers must not mutate it.
// SetDefaultAgentDefs replaces the variable wholesale under a write lock, so
// concurrent readers may observe the previous set — acceptable since agents are
// loaded once at boot.
func currentAgentDefs() []agentdef.AgentDef {
	defaultDefsMu.RLock()
	defer defaultDefsMu.RUnlock()
	return defaultDefs
}
