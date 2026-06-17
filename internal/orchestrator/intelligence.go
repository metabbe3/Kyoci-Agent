package orchestrator

import (
	"context"
	"log/slog"
	"time"

	"github.com/metabbe3/Kyoci-Agent/internal/agent"
	"github.com/metabbe3/Kyoci-Agent/internal/memory"
	"github.com/metabbe3/Kyoci-Agent/internal/promptskill"
	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// =============================================================================
// Intelligence Adapter — Bridges Agent Hooks to Memory Intelligence Systems
// =============================================================================
//
// This adapter implements the agent package's hook interfaces (ContextInjector
// and TaskRecorder) using the concrete implementations from the memory package.
// It lives in the orchestrator package because that's the only place that
// imports both agent and memory.
//

// contextInjectorAdapter wraps memory.ContextInjector to satisfy agent.ContextInjector.
type contextInjectorAdapter struct {
	injector *memory.ContextInjector
}

func (a *contextInjectorAdapter) Inject(task string) string {
	if a.injector == nil {
		return ""
	}
	return a.injector.Inject(task)
}

// taskRecorderAdapter wraps the experience + reflection engines to satisfy
// agent.TaskRecorder. Recording is async to never block the response path.
type taskRecorderAdapter struct {
	experience *memory.ExperienceEngine
	reflection *memory.ReflectionEngine
	patterns   *memory.PatternDetector
	memoryMgr  *memory.MemoryManager
	logger     *slog.Logger
}

func (a *taskRecorderAdapter) Record(ctx context.Context, rec agent.TaskRecord) {
	// Run async — never block the response
	go func() {
		// Convert agent.TaskRecord → memory.ExperienceRecord
		expRec := memory.ExperienceRecord{
			Task:       rec.Task,
			Role:       rec.Role,
			ToolsUsed:  rec.ToolsUsed,
			Iterations: rec.Iterations,
			ToolCalls:  rec.ToolCalls,
			Success:    rec.Success,
			DurationMs: rec.DurationMs,
			ErrorMsg:   rec.ErrorMsg,
		}

		// 1. Record the experience
		if a.experience != nil {
			if err := a.experience.Record(expRec); err != nil {
				a.logger.Warn("failed to record experience", "error", err)
			}
		}

		// 2. Check for patterns and auto-generate skills
		if a.patterns != nil && expRec.Success {
			if skillName := a.patterns.CheckAndGenerate(expRec); skillName != "" {
				a.logger.Info("auto-skill generated after task", "skill", skillName)
			}
		}

		// 3. Trigger reflection for complex tasks
		if a.reflection != nil {
			insights, err := a.reflection.Reflect(ctx, expRec)
			if err != nil {
				a.logger.Warn("reflection failed", "error", err)
			} else if len(insights) > 0 {
				a.logger.Info("reflection produced insights", "count", len(insights))
			}
		}

		// 4. Trigger post-task compaction (L3 auto-compaction backup).
		// Uses a fresh context — the caller's ctx may be cancelled by the time
		// this async goroutine runs.
		if a.memoryMgr != nil {
			compactCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := a.memoryMgr.CompactIfNeeded(compactCtx); err != nil {
				a.logger.Warn("post-task compaction failed", "error", err)
			}
		}
	}()
}

// resolveInjector creates the agent hook adapters from the orchestrator's
// intelligence components. Called during New() to wire everything together.
// It composes the L3 memory context injector with the prompt-skill knowledge
// injector via a CompositeInjector so both contribute to the system prompt.
func (o *Orchestrator) resolveInjector() (agent.ContextInjector, agent.TaskRecorder) {
	var memoryInjector agent.ContextInjector = nil
	var recorder agent.TaskRecorder = nil

	if o.contextInjector != nil {
		memoryInjector = &contextInjectorAdapter{injector: o.contextInjector}
	}
	if o.experienceEngine != nil || o.reflectionEngine != nil {
		recorder = &taskRecorderAdapter{
			experience: o.experienceEngine,
			reflection: o.reflectionEngine,
			memoryMgr:  o.memoryMgr,
			logger:     o.logger,
		}
		// Wire pattern detector for auto-skill generation
		if o.experienceEngine != nil {
			recorder.(*taskRecorderAdapter).patterns = memory.NewPatternDetector(
				o.experienceEngine.Storage(),
				memory.DefaultSkillGeneratorConfig(),
				o.logger,
			)
		}
	}

	// Build the prompt-skill injector from config. When disabled or empty, it
	// contributes nothing (Inject returns ""). When enabled alongside the
	// memory injector, both are composed.
	var injectors []promptskill.Injector
	if memoryInjector != nil {
		injectors = append(injectors, memoryInjector)
	}
	if skillInjector := o.buildPromptSkillInjector(); skillInjector != nil {
		injectors = append(injectors, skillInjector)
	}

	var injector agent.ContextInjector = nil
	switch {
	case len(injectors) == 1:
		// Single injector — wrap to satisfy the agent interface without nesting.
		injector = injectorFunc(injectors[0].Inject)
	case len(injectors) > 1:
		injector = promptskill.CompositeInjector{Injectors: injectors}
	}

	return injector, recorder
}

// buildPromptSkillInjector loads the skill registry from config and returns an
// injector, or nil if the feature is disabled. A missing/empty dir yields a
// no-op injector (Inject returns ""), which is harmless but wastes a slot —
// so we return nil when the registry is empty to keep the chain tight.
func (o *Orchestrator) buildPromptSkillInjector() *promptskill.PromptSkillInjector {
	if o.config == nil || !o.config.PromptSkill.Enabled {
		return nil
	}
	reg, err := promptskill.LoadWithLogger(o.config.PromptSkill.Dir, o.logger)
	if err != nil {
		o.logger.Warn("prompt skill load failed; skipping injector", "dir", o.config.PromptSkill.Dir, "error", err)
		return nil
	}
	if reg.Len() == 0 {
		return nil
	}
	opts := promptskill.MatchOptions{
		MaxSkills:    o.config.PromptSkill.MaxSkillsPerTask,
		MaxTotalChars: o.config.PromptSkill.MaxTotalChars,
	}
	return promptskill.NewInjectorWithOptions(reg, o.logger, opts)
}

// injectorFunc adapts a bare Inject function to the agent.ContextInjector
// interface. Used when there's a single injector so we avoid an unnecessary
// CompositeInjector wrapper.
type injectorFunc func(task string) string

func (f injectorFunc) Inject(task string) string { return f(task) }

// routerLLMAdapter wraps the orchestrator's LLM Router to satisfy the
// memory.LLMClient interface. This lets the compactor's LLM summarizer use
// the same provider pool as the agent without the memory package importing
// internal/llm.
type routerLLMAdapter struct {
	router    llmRouter
	preferred string
}

// llmRouter is the minimal subset of *llm.Router that routerLLMAdapter needs.
type llmRouter interface {
	Route(ctx context.Context, req kyoci.CompletionRequest, preferredProvider string) (*kyoci.CompletionResponse, error)
}

func (a *routerLLMAdapter) Complete(ctx context.Context, req kyoci.CompletionRequest) (*kyoci.CompletionResponse, error) {
	return a.router.Route(ctx, req, a.preferred)
}
