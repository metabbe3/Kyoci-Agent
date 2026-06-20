package agent

import "context"

// =====================================================================================
// QA reviewer — an isolated, skeptical sub-agent that independently verifies a
// task's deliverable. Mirrors ExploreWorker's context-isolation pattern.
//
// The QA agent runs as the final SDLC phase (after SETUP → IMPLEMENT → VERIFY).
// It executes in a FRESH context — it never sees the implement worker's claims —
// and derives truth only from the filesystem and the real build/test output.
// This is the "never trust the author" guarantee: the worker that built the code
// cannot also judge it.
//
// Tools are restricted to the explore read-set PLUS terminal, so QA can re-run
// `npm run build` / `go test` / etc. itself. File writes stay blocked by
// ReadOnlyToolFilter (QA reviews; it does not modify).
// =====================================================================================

// QAToolAllowlist is the explore read-set plus terminal (to re-run build/tests).
// file writes remain blocked by ReadOnlyToolFilter's write-action check.
var QAToolAllowlist = func() map[string]bool {
	m := make(map[string]bool, len(ExploreToolAllowlist)+1)
	for k, v := range ExploreToolAllowlist {
		m[k] = v
	}
	m["terminal"] = true
	return m
}()

// QAWorker runs an independent QA review using the parent agent's infrastructure
// but a fresh, isolated context + read-and-terminal-only tools + the QA system
// prompt. Returns the QA agent's verdict ("PASS" | "FAIL: <bugs>").
func (a *Agent) QAWorker(ctx context.Context, goal string) (string, error) {
	qa := NewAgent(
		a.config,
		a.router,
		NewReadOnlyToolFilter(a.tools, QAToolAllowlist),
		a.skills,
		a.memory,
		WithLogger(a.logger),
	)
	// Force the legacy ReAct loop so QASystemPrompt is the actual system prompt
	// (orchestration/thinking would override it) and the review is one focused pass.
	qa.config.SystemPrompt = QASystemPrompt
	qa.config.Orchestration.Enabled = false
	qa.config.EnableThinking = false
	qa.config.EnableSkills = false
	qa.config.MaxIterations = 15
	// Route QA to the configured cloud provider if set. Without this, QA
	// always uses the global default (local) which crashes on large contexts.
	if route := a.config.Orchestration.ModelRouting.QA; route.Provider != "" {
		qa.config.PreferredProvider = route.Provider
		qa.config.Model = route.Model
	}

	result, err := qa.Execute(ctx, goal)
	if err != nil {
		return "", err
	}
	return result.Content, nil
}
