// Package promptskill implements a markdown-driven knowledge layer that
// injects relevant workflow guidance into the agent's system prompt based on
// the task. Each skill is a self-contained markdown file with YAML
// frontmatter; the loader discovers them at startup, the matcher selects the
// most relevant ones per task, and the injector appends their bodies to the
// system prompt via the existing agent.ContextInjector hook.
//
// This mirrors the Hermes Agent skill model: skills are prompt-level guidance
// that teaches the LLM how to use the tools Kyoci already has (terminal, file,
// http_client, search). They are distinct from Kyoci's zero-AI deterministic
// skills (math, time, hash) which bypass the LLM entirely.
package promptskill

// Triggers holds the matching rules for a skill. A task matches if ANY
// keyword appears as a substring (case-insensitive) OR any regex matches.
type Triggers struct {
	Keywords []string `yaml:"keywords"`
	Regex    []string `yaml:"regex"`
}

// PromptSkill is a single loaded skill. Body holds the markdown content that
// follows the YAML frontmatter; it is what gets injected into the system prompt.
type PromptSkill struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Category    string   `yaml:"category"`
	Triggers    Triggers `yaml:"triggers"`
	Requires    []string `yaml:"requires"`
	Priority    string   `yaml:"priority"`
	Body        string   `yaml:"-"`
}
