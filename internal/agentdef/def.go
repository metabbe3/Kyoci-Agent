// Package agentdef implements markdown-driven agent definitions — the
// Kyoci equivalent of Claude Code's .claude/agents/<name>.md convention.
//
// Each agent is a self-contained markdown file with YAML frontmatter that
// declares its identity, dispatch triggers, tool allowlist, LLM knobs,
// memory configuration, and prompt-skill allowlist. The markdown body is the
// agent's "soul" — the system prompt that defines its behavior.
//
// At startup the orchestrator calls LoadAgents(dir) to discover every agent
// in agents/, then RegisterFromAgents wires each into the role registry.
// The classifier scores agents via MatchScore(def, task) using keyword and
// anchor triggers (no LLM call) and routes the task to the top match.
//
// This mirrors the existing internal/promptskill/ pattern (markdown + YAML
// frontmatter + keyword/regex matching) extended with agent-specific fields.
package agentdef

// TriggerSpec holds the dispatch rules for an agent. A task matches if any
// keyword or anchor appears as a substring (case-insensitive) or any regex
// matches.
//
// Keywords are weak signals (score +1 each); anchors are strong domain
// signals (score +3 each). The classifier requires total score >= 2 to
// dispatch to a specialist agent, so one accidental substring hit is never
// enough on its own.
type TriggerSpec struct {
	Keywords []string `yaml:"keywords"`
	Anchors  []string `yaml:"anchors"`
	Regex    []string `yaml:"regex"`
}

// MemorySpec configures how an agent uses the shared SQLite memory store.
// Today the store is process-global — every agent reads and writes the same
// DB — so the knobs here are read-side only: whether the agent recalls past
// context at all, and how many lessons to inject per task. Storage budgets
// are owned by config.MemoryConfig and cannot vary per agent.
type MemorySpec struct {
	// Enabled is the master kill switch. When false, the agent skips
	// memory_recall and long-term context injection entirely. Default true.
	Enabled bool `yaml:"enabled"`

	// RecallDepth caps the number of long-term-memory lessons injected per
	// task. Becomes the limit argument to memory.Recall(ctx, query, limit).
	// Default 5; 0 falls back to a sensible package default at call site.
	RecallDepth int `yaml:"recall_depth"`
}

// AgentDef is one loaded agent definition. Fields map 1:1 to the YAML
// frontmatter keys; Body holds the markdown body verbatim and SystemPrompt
// holds the final composed prompt (body + platform substitution + shared
// closing blocks) that becomes the agent's system prompt at runtime.
type AgentDef struct {
	// Identity
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Category    string `yaml:"category"`

	// Dispatch
	Triggers TriggerSpec `yaml:"triggers"`

	// Tool allowlist. Matches internal/tool/builtins.go builtin names.
	// Empty = all builtin tools allowed (MCP tools always pass through).
	Tools []string `yaml:"tools"`

	// LLM
	PreferredProvider string `yaml:"preferred_provider"`
	Model             string `yaml:"model"`
	MaxIterations     int    `yaml:"max_iterations"`

	// Memory
	Memory MemorySpec `yaml:"memory"`

	// PromptSkills filters the global data/skills/ prompt-skill matches to
	// this allowlist. Empty = take all matches (today's behavior).
	PromptSkills []string `yaml:"prompt_skills"`

	// Priority breaks dispatch ties. "high" beats "normal" beats "low".
	// Empty defaults to "normal".
	Priority string `yaml:"priority"`

	// Body is the raw markdown body (after the frontmatter, before any
	// composition). Read-only after Load.
	Body string `yaml:"-"`

	// SystemPrompt is the final composed prompt: Body with platform tokens
	// substituted, then VerificationRules + DelegationBlock + ClosingDirective
	// appended via Compose. This is what the role registry installs as the
	// agent's system prompt. Read-only after Load.
	SystemPrompt string `yaml:"-"`

	// SourcePath is the absolute path the def was loaded from, for diagnostics
	// (manifests, "edit this agent" hints in the dashboard). Read-only.
	SourcePath string `yaml:"-"`
}
