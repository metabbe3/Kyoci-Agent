package role

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/metabbe3/Kyoci-Agent/internal/agent"
	"github.com/metabbe3/Kyoci-Agent/internal/agentdef"
	"github.com/metabbe3/Kyoci-Agent/internal/config"
	"github.com/metabbe3/Kyoci-Agent/internal/llm"
	"github.com/metabbe3/Kyoci-Agent/internal/skill"
	"github.com/metabbe3/Kyoci-Agent/internal/tool"
	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// =============================================================================
// Role Registry
// =============================================================================

// RoleRegistry manages role configurations and creates role agents on demand.
// It is thread-safe and uses internal synchronization (RWMutex).
type RoleRegistry struct {
	roles      map[kyoci.RoleType]*RoleAgent
	router     *llm.Router
	toolReg    *tool.Registry
	skillReg   *skill.Registry
	memoryMgr  kyoci.MemoryStore
	thinkingCfg config.ThinkingConfig
	orchCfg     config.OrchestrationConfig
	mu         sync.RWMutex
	logger     *slog.Logger
}

// NewRoleRegistry creates a new role registry with the required dependencies.
// The thinking config is initialized with sensible defaults so that agents
// created without calling RegisterDefaults still have valid thinking budgets.
func NewRoleRegistry(
	router *llm.Router,
	toolReg *tool.Registry,
	skillReg *skill.Registry,
	memoryMgr kyoci.MemoryStore,
) *RoleRegistry {
	return &RoleRegistry{
		roles:     make(map[kyoci.RoleType]*RoleAgent),
		router:    router,
		toolReg:   toolReg,
		skillReg:  skillReg,
		memoryMgr: memoryMgr,
		thinkingCfg: config.ThinkingConfig{
			Enabled:             false,
			ToolBudget:          15,
			MaxReflections:      3,
			MaxReplans:          2,
			ConfidenceThreshold: 0.7,
			FewShot:             true,
		},
		orchCfg: config.OrchestrationConfig{
			Enabled:             false,
			MaxSteps:            6,
			MaxParallel:         3,
			WorkerMaxIterations: 8,
			WorkerMaxToolCalls:  8,
		},
		logger: slog.Default(),
	}
}

// Register creates and registers a role agent from the given configuration.
// If a role of the same type already exists, it will be replaced.
//
// Parameters:
//   - cfg: The role configuration to register
//
// Returns:
//   - error: nil on success, error if registration or agent creation fails
func (r *RoleRegistry) Register(cfg kyoci.RoleConfig) error {
	if err := cfg.Validate(); err != nil {
		r.logger.Error("role config validation failed", "type", cfg.Type, "error", err)
		return fmt.Errorf("invalid role config for type %s: %w", cfg.Type, err)
	}

	// Create agent config from role config + registry thinking config.
	// thinkingCfg is sourced from *config.Config via RegisterDefaults, or
	// falls back to the NewRoleRegistry defaults when Register is called
	// directly (e.g., in tests or programmatic usage).
	agentCfg := agent.AgentConfig{
		SystemPrompt:                 cfg.SystemPrompt,
		ToolChoice:                   "auto",
		Temperature:                  cfg.Temperature,
		MaxTokens:                    8192,
		PreferredProvider:            cfg.PreferredProvider,
		Model:                        cfg.Model,
		EnableSkills:                 true,
		EnableMemory:                 true,
		EnableStreaming:              true,
		EnableThinking:               r.thinkingCfg.Enabled,
		ThinkingToolBudget:           r.thinkingCfg.ToolBudget,
		ThinkingMaxReflections:       r.thinkingCfg.MaxReflections,
		ThinkingMaxReplans:           r.thinkingCfg.MaxReplans,
		ThinkingConfidenceThreshold:  r.thinkingCfg.ConfidenceThreshold,
		ThinkingFewShot:              r.thinkingCfg.FewShot,
		Orchestration: agent.OrchestratorConfig{
			Enabled:             r.orchCfg.Enabled,
			MaxSteps:            r.orchCfg.MaxSteps,
			MaxParallel:         r.orchCfg.MaxParallel,
			WorkerMaxIterations: r.orchCfg.WorkerMaxIterations,
			WorkerMaxToolCalls:  r.orchCfg.WorkerMaxToolCalls,
			ModelRouting: agent.ModelRouting{
				Planner:            agent.PhaseRoute{Provider: r.orchCfg.ModelRouting.Planner.Provider, Model: r.orchCfg.ModelRouting.Planner.Model},
				Worker:             agent.PhaseRoute{Provider: r.orchCfg.ModelRouting.Worker.Provider, Model: r.orchCfg.ModelRouting.Worker.Model},
				WorkerFileCreation: agent.PhaseRoute{Provider: r.orchCfg.ModelRouting.WorkerFileCreation.Provider, Model: r.orchCfg.ModelRouting.WorkerFileCreation.Model},
				Synthesizer:        agent.PhaseRoute{Provider: r.orchCfg.ModelRouting.Synthesizer.Provider, Model: r.orchCfg.ModelRouting.Synthesizer.Model},
				QA:                 agent.PhaseRoute{Provider: r.orchCfg.ModelRouting.QA.Provider, Model: r.orchCfg.ModelRouting.QA.Model},
			},
		},
	}

	if cfg.MaxIterations > 0 {
		agentCfg.MaxIterations = cfg.MaxIterations
	}

	// Create role agent
	roleAgent, err := r.createRoleAgent(cfg, agentCfg)
	if err != nil {
		r.logger.Error("failed to create role agent", "type", cfg.Type, "error", err)
		return fmt.Errorf("failed to create role agent for type %s: %w", cfg.Type, err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.roles[cfg.Type] = roleAgent
	r.logger.Info("role registered", "type", cfg.Type, "tools", len(cfg.Tools))
	return nil
}

// Get retrieves a registered role agent by type.
//
// Parameters:
//   - roleType: The role type to retrieve
//
// Returns:
//   - *RoleAgent: The role agent if found
//   - error: kyoci.ErrRoleNotFound if not registered
func (r *RoleRegistry) Get(roleType kyoci.RoleType) (*RoleAgent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	roleAgent, ok := r.roles[roleType]
	if !ok {
		return nil, kyoci.ErrRoleNotFound
	}
	return roleAgent, nil
}

// List returns all registered role configurations.
//
// Returns:
//   - []kyoci.RoleConfig: List of role configurations
func (r *RoleRegistry) List() []kyoci.RoleConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	configs := make([]kyoci.RoleConfig, 0, len(r.roles))
	for _, roleAgent := range r.roles {
		configs = append(configs, roleAgent.config)
	}
	return configs
}

// RegisterFromAgents registers one RoleAgent per loaded AgentDef. This is the
// markdown-driven replacement for RegisterDefaults — instead of importing six
// per-role Go packages, the registry accepts the []AgentDef the loader produced
// from agents/*.md and converts each to a RoleConfig before delegating to the
// existing Register path.
//
// As with RegisterDefaults, the cfg argument captures thinking/orchestration
// settings applied to every agent. Pass nil to keep the NewRoleRegistry
// defaults (thinking disabled, sane budgets).
//
// Returns an error on the first registration failure; prior successful
// registrations remain in place.
func (r *RoleRegistry) RegisterFromAgents(defs []agentdef.AgentDef, cfg *config.Config) error {
	r.logger.Info("registering agents from markdown definitions", "count", len(defs))

	if cfg != nil {
		r.thinkingCfg = cfg.Agent.Thinking
		r.orchCfg = cfg.Agent.Orchestration
	}

	registered := 0
	for _, def := range defs {
		roleCfg := kyoci.RoleConfig{
			Type:              kyoci.RoleType(def.Name),
			SystemPrompt:      def.SystemPrompt,
			Tools:             def.Tools,
			PreferredProvider: def.PreferredProvider,
			Model:             def.Model,
			MaxIterations:     def.MaxIterations,
			Temperature:       0.3,
		}
		if err := r.Register(roleCfg); err != nil {
			r.logger.Error("failed to register agent", "name", def.Name, "source", def.SourcePath, "error", err)
			return fmt.Errorf("failed to register agent %s: %w", def.Name, err)
		}
		// Stash the recall-depth on the role agent so the orchestrator /
		// memory injector can read it at dispatch time. Stored on a side map
		// rather than RoleConfig because the legacy config struct is shared
		// with non-agentdef callers.
		if def.Memory.Enabled || def.Memory.RecallDepth > 0 {
			r.mu.Lock()
			if ra, ok := r.roles[roleCfg.Type]; ok {
				ra.memorySpec = def.Memory
			}
			r.mu.Unlock()
		}
		registered++
	}

	r.logger.Info("agents registered from markdown", "count", registered)
	return nil
}

// SetIntelligenceHooks wires context injector and task recorder into all
// registered role agents. Called by the orchestrator after all roles are created.
func (r *RoleRegistry) SetIntelligenceHooks(injector agent.ContextInjector, recorder agent.TaskRecorder) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, roleAgent := range r.roles {
		if injector != nil {
			roleAgent.SetContextInjector(injector)
		}
		if recorder != nil {
			roleAgent.SetTaskRecorder(recorder)
		}
	}
	r.logger.Info("intelligence hooks wired into all roles")
}

// createRoleAgent creates a new RoleAgent with the given configuration.
func (r *RoleRegistry) createRoleAgent(
	cfg kyoci.RoleConfig,
	agentCfg agent.AgentConfig,
) (*RoleAgent, error) {
	// Create the underlying agent with kyoci.ToolRegistry for proper integration
	// IMPORTANT: Only register tools that are listed in the role's Tools config.
	// This prevents a frontend agent from accidentally using security_scan, etc.
	toolRegistry := kyoci.NewToolRegistry()
	if r.toolReg != nil {
		// Build a set of allowed tool names for O(1) lookup
		allowedTools := make(map[string]bool, len(cfg.Tools))
		for _, name := range cfg.Tools {
			allowedTools[name] = true
		}

		toolDefs := r.toolReg.List()
		for _, def := range toolDefs {
			// Built-in tools respect the role's allowlist (so a frontend
			// agent cannot reach security_scan, etc.). Non-built-in tools
			// (MCP / dynamically loaded) bypass the allowlist — they are
			// user-installed extensions that should be available to any
			// role, and their names cannot be known ahead of time to be
			// listed in role config. Without this bypass, MCP tools are
			// silently dropped and never reach the orchestrated worker.
			if len(allowedTools) > 0 && !allowedTools[def.Name] && tool.IsBuiltinName(def.Name) {
				continue
			}
			t, err := r.toolReg.Get(def.Name)
			if err != nil {
				r.logger.Warn("failed to get tool from registry", "name", def.Name, "error", err)
				continue
			}
			if err := toolRegistry.Register(t); err != nil {
				r.logger.Warn("failed to register tool", "name", def.Name, "error", err)
			}
		}
		r.logger.Info("role tool filtering applied",
			"role", cfg.Type,
			"total_tools_available", len(toolDefs),
			"tools_registered", toolRegistry.Count(),
		)
	}

	// Share the global, fully-populated skill registry with every agent. The
	// underlying *kyoci.SkillRegistry is goroutine-safe and read-only after
	// RegisterBuiltin runs at boot, so one shared instance is safe across all
	// agents and avoids copying ~180 skills per role. This is what makes the
	// zero-AI skill fast path (agent loop) and the planner's tool_hint="skill"
	// path actually fire — previously each agent got an empty registry and no
	// skills were ever available, so the fast path could never match.
	// IMPORTANT: every agent, worker, and explore-clone holds this SAME pointer,
	// so it MUST stay read-only after boot — never call a.skills.Register(...)
	// through the agent port, or you mutate every concurrent agent's skills.
	// Keep skill registration to RegisterBuiltin at init only.
	skillRegistry := kyoci.NewSkillRegistry()
	if r.skillReg != nil {
		skillRegistry = r.skillReg.Kyoci()
	}

	// Create agent
	agt := agent.NewAgent(
		agentCfg,
		r.router,
		toolRegistry,
		skillRegistry,
		r.memoryMgr,
	)

	return &RoleAgent{
		config: cfg,
		agent:  agt,
	}, nil
}

// =============================================================================
// Role Agent
// =============================================================================

// RoleAgent wraps an agent with role-specific configuration and behavior.
// It implements the kyoci.Role interface and delegates to the underlying agent.
// Goroutine-safe: All methods are safe for concurrent use.
type RoleAgent struct {
	config     kyoci.RoleConfig
	agent      *agent.Agent
	logger     *slog.Logger
	memorySpec agentdef.MemorySpec // from AgentDef.Memory; zero-value when registered via legacy RegisterDefaults
}

// Type returns the role type.
func (ra *RoleAgent) Type() kyoci.RoleType {
	return ra.config.Type
}

// SystemPrompt returns the role's system prompt.
func (ra *RoleAgent) SystemPrompt() string {
	return ra.config.SystemPrompt
}

// Tools returns the list of tool names this role can use.
func (ra *RoleAgent) Tools() []string {
	return ra.config.Tools
}

// PreferredProvider returns the preferred LLM provider name.
func (ra *RoleAgent) PreferredProvider() string {
	return ra.config.PreferredProvider
}

// MaxIterations returns the maximum number of iterations.
func (ra *RoleAgent) MaxIterations() int {
	return ra.config.MaxIterations
}

// Execute executes a task through this role.
// Delegates to the underlying agent's Execute method.
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - task: The task description to execute
//   - memory: The memory store for context and recall
//
// Returns:
//   - *kyoci.TaskResult: The result of task execution
//   - error: Any error that occurred
func (ra *RoleAgent) Execute(ctx context.Context, task string, memory kyoci.MemoryStore) (*kyoci.TaskResult, error) {
	if ra.logger == nil {
		ra.logger = slog.Default()
	}

	ra.logger.Info("role agent executing task", "role", ra.Type(), "task", task)
	return ra.agent.Execute(ctx, task)
}

// ExecuteStream executes a task through this role with streaming.
// Delegates to the underlying agent's ExecuteStream method.
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - task: The task description to execute
//
// Returns:
//   - <-chan kyoci.StreamChunk: Channel of streaming chunks
//   - error: Any error that occurred
func (ra *RoleAgent) ExecuteStream(ctx context.Context, task string) (<-chan kyoci.StreamChunk, error) {
	if ra.logger == nil {
		ra.logger = slog.Default()
	}

	ra.logger.Info("role agent executing stream task", "role", ra.Type(), "task", task)
	return ra.agent.ExecuteStream(ctx, task)
}

// SetContextInjector sets the L3 context injector on the underlying agent.
func (ra *RoleAgent) SetContextInjector(injector agent.ContextInjector) {
	ra.agent.SetContextInjector(injector)
}

// SetTaskRecorder sets the experience recorder on the underlying agent.
func (ra *RoleAgent) SetTaskRecorder(recorder agent.TaskRecorder) {
	ra.agent.SetTaskRecorder(recorder)
}

// WithRunLogger returns a clone of this RoleAgent whose underlying agent uses
// the supplied logger for the duration of one task. Used by the orchestrator
// to fan per-task events into a per-run log file without disturbing the
// shared role agent (which other concurrent tasks may be using).
//
// Returns the receiver unchanged when ra is nil or l is nil. The clone shares
// the role's config and the underlying tools/router/skills/memory; only the
// logger pointer is swapped.
func (ra *RoleAgent) WithRunLogger(l *slog.Logger) *RoleAgent {
	if ra == nil || l == nil {
		return ra
	}
	return &RoleAgent{
		config:     ra.config,
		agent:      ra.agent.WithLogger(l),
		logger:     l,
		memorySpec: ra.memorySpec,
	}
}

// MemorySpec returns the per-agent memory configuration declared in the
// agent's markdown frontmatter (memory.enabled, memory.recall_depth). Returns
// the zero value (Enabled=false, RecallDepth=0) for roles registered via the
// legacy RegisterDefaults path, in which case callers fall back to the
// hardcoded recall limit they always used.
func (ra *RoleAgent) MemorySpec() agentdef.MemorySpec {
	return ra.memorySpec
}