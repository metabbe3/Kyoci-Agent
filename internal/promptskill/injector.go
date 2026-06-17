package promptskill

import (
	"log/slog"
	"strings"
)

// Injector is the structural interface satisfied by anything that can inject
// context into the system prompt. It matches agent.ContextInjector from the
// agent package (Inject(task string) string), so a PromptSkillInjector can be
// passed wherever an agent.ContextInjector is expected without importing the
// agent package here (avoids a dependency cycle).
type Injector interface {
	Inject(task string) string
}

// PromptSkillInjector matches loaded skills against a task and returns a
// concatenated prompt fragment containing the relevant skill bodies. It
// implements Injector (and by structural typing, agent.ContextInjector).
type PromptSkillInjector struct {
	registry *Registry
	log      *slog.Logger
	opts     MatchOptions
}

// NewInjector builds an injector over reg with default caps (max 4 skills,
// 12k chars). Pass a non-nil logger to observe matches.
func NewInjector(reg *Registry, log *slog.Logger) *PromptSkillInjector {
	if log == nil {
		log = slog.Default()
	}
	return &PromptSkillInjector{
		registry: reg,
		log:      log,
		opts:     MatchOptions{MaxSkills: 4, MaxTotalChars: 12000},
	}
}

// NewInjectorWithOptions is like NewInjector but lets the caller set the caps
// (e.g. from config).
func NewInjectorWithOptions(reg *Registry, log *slog.Logger, opts MatchOptions) *PromptSkillInjector {
	if log == nil {
		log = slog.Default()
	}
	return &PromptSkillInjector{registry: reg, log: log, opts: opts}
}

// Inject implements Injector. Returns "" when nothing matches so the caller
// leaves the system prompt untouched. The output is hard-truncated to
// MaxTotalChars so that even a single large skill body can't blow the budget.
func (p *PromptSkillInjector) Inject(task string) string {
	if p == nil || p.registry == nil || strings.TrimSpace(task) == "" {
		return ""
	}
	matched := p.registry.Match(task, p.opts)
	if len(matched) == 0 {
		return ""
	}
	p.log.Debug("prompt skills matched", "count", len(matched), "task", task)
	var b strings.Builder
	b.WriteString("\n\n# Relevant Skills\nThe following skill guidance is relevant to this task. Follow it.")
	for _, s := range matched {
		b.WriteString("\n\n## Skill: ")
		b.WriteString(s.Name)
		b.WriteString("\n\n")
		b.WriteString(s.Body)
	}
	out := b.String()
	// Hard cap: the matcher includes the first skill unconditionally even if
	// its body alone exceeds MaxTotalChars. Truncate here so the prompt never
	// grows beyond the configured budget — critical for 8B-class models that
	// drift off-format when the system prompt is too long.
	if p.opts.MaxTotalChars > 0 && len(out) > p.opts.MaxTotalChars {
		out = out[:p.opts.MaxTotalChars]
		p.log.Debug("prompt skill injection truncated",
			"original_chars", b.Len(), "capped_chars", p.opts.MaxTotalChars)
	}
	return out
}
