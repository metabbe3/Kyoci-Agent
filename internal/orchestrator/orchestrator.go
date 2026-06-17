package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
	"github.com/metabbe3/Kyoci-Agent/internal/agent"
	"github.com/metabbe3/Kyoci-Agent/internal/config"
	"github.com/metabbe3/Kyoci-Agent/internal/llm"
	"github.com/metabbe3/Kyoci-Agent/internal/mcp"
	"github.com/metabbe3/Kyoci-Agent/internal/memory"
	"github.com/metabbe3/Kyoci-Agent/internal/role"
	"github.com/metabbe3/Kyoci-Agent/internal/skill"
	"github.com/metabbe3/Kyoci-Agent/internal/tool"
	"github.com/metabbe3/Kyoci-Agent/internal/tool/builtin"
	"github.com/metabbe3/Kyoci-Agent/internal/tracing"
)

// SystemStatus represents the current system status.
type SystemStatus struct {
	Status      string              `json:"status"`
	Started     bool                `json:"started"`
	Timestamp   time.Time           `json:"timestamp"`
	Roles       []kyoci.RoleConfig  `json:"roles"`
	Tools       []kyoci.ToolDefinition `json:"tools"`
	Skills      []kyoci.SkillInfo   `json:"skills"`
	Providers   []string            `json:"providers"`
	MemoryStats *memory.MemoryStats `json:"memory_stats,omitempty"`
}

// Orchestrator is the central coordinator that ties all subsystems together.
// It manages the lifecycle of all components and routes tasks to the appropriate role agents.
// Goroutine-safe: All public methods are safe for concurrent use.
type Orchestrator struct {
	config           *config.Config
	roleRegistry     *role.RoleRegistry
	llmRouter        *llm.Router
	providerReg      *llm.ProviderRegistry
	toolReg          *tool.Registry
	skillReg         *skill.Registry
	memoryMgr        *memory.MemoryManager
	// Intelligence subsystems
	contextInjector  *memory.ContextInjector
	experienceEngine *memory.ExperienceEngine
	profileStore     *memory.ProfileStore
	reflectionEngine *memory.ReflectionEngine
	tracer           *tracing.Tracer
	mcpManager       *mcp.MCPManager
	logger           *slog.Logger
	mu               sync.RWMutex
	started          bool
	shutdownChan     chan struct{}
}

// New creates a new Orchestrator and initializes ALL subsystems.
// This is the single entry point for setting up the entire agent platform.
//
// Parameters:
//   - cfg: Application configuration (use config.Default() or config.Load())
//
// Returns:
//   - *Orchestrator: Fully initialized orchestrator ready to Start()
//   - error: If any subsystem fails to initialize
func New(cfg *config.Config) (*Orchestrator, error) {
	logger := slog.Default().With("component", "orchestrator")
	logger.Info("initializing orchestrator")

	// 1. Create memory store
	logger.Info("initializing memory store", "db_path", cfg.Memory.DBPath)
	memoryMgr, err := memory.NewStore(cfg.Memory)
	if err != nil {
		return nil, fmt.Errorf("failed to create memory store: %w", err)
	}

	// 2. Initialize LLM providers
	logger.Info("initializing LLM providers")
	providerReg, err := llm.InitProviders(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize LLM providers: %w", err)
	}

	// 3. Create LLM router
	llmRouter := llm.NewRouter(providerReg, llm.StrategyFallback)

	// 4. Create tool registry and register builtins
	logger.Info("initializing tool registry")
	toolReg := tool.NewRegistry()
	if err := toolReg.RegisterBuiltin(); err != nil {
		return nil, fmt.Errorf("failed to register builtin tools: %w", err)
	}

	// 4b. Register intelligence tools (memory_recall, remember)
	// These need the memory store, which is available via memoryMgr
	toolReg.Register(builtin.NewMemoryRecallTool(memoryMgr))
	toolReg.Register(builtin.NewProfileSetTool(memoryMgr))
	logger.Info("intelligence tools registered", "tools", "memory_recall, remember")

	// 4b2. Register security scan tool (OWASP vulnerability scanner)
	toolReg.Register(builtin.NewSecurityScanTool())
	logger.Info("security scan tool registered")

	// 4c. Register MCP tools (if configured)
	var mcpMgr *mcp.MCPManager
	if len(cfg.MCP.Servers) > 0 {
		logger.Info("initializing MCP servers", "count", len(cfg.MCP.Servers))
		mcpMgr = mcp.NewMCPManager("kyoci")
		mcpCtx, mcpCancel := context.WithTimeout(context.Background(), 30*time.Second)
		mcpCount := 0
		for name, sc := range cfg.MCP.Servers {
			if !sc.Enabled {
				continue
			}
			err := mcpMgr.AddServer(mcpCtx, mcp.MCPServerConfig{
				Name:    name,
				Command: sc.Command,
				Args:    sc.Args,
				Env:     sc.Env,
			})
			if err != nil {
				logger.Warn("failed to add MCP server", "server", name, "error", err)
			} else {
				mcpCount++
			}
		}
		mcpCancel()
		if mcpCount > 0 {
			mcpTools := mcpMgr.GetTools()
			for _, t := range mcpTools {
				toolReg.Register(t)
			}
			logger.Info("MCP tools registered", "servers", mcpCount, "tools", len(mcpTools))
		}
	}

	// 5. Create skill registry and register builtins
	logger.Info("initializing skill registry")
	skillReg := skill.NewRegistry()
	if err := skillReg.RegisterBuiltin(); err != nil {
		return nil, fmt.Errorf("failed to register builtin skills: %w", err)
	}

	// 6. Register delegation tool BEFORE roles (roles snapshot tools at creation)
	delegationTool := builtin.NewDelegationTool()
	if err := toolReg.Register(delegationTool); err != nil {
		logger.Warn("failed to register delegation tool", "error", err)
	}
	logger.Info("delegation tool registered")

	// 7. Create role registry and register defaults
	logger.Info("initializing role registry")
	roleRegistry := role.NewRoleRegistry(llmRouter, toolReg, skillReg, memoryMgr)
	if err := roleRegistry.RegisterDefaults(cfg); err != nil {
		return nil, fmt.Errorf("failed to register default roles: %w", err)
	}

	// 7. Create tracer
	tracer := tracing.New("kyoci-orchestrator")

	// 8. Initialize intelligence subsystems (L3 memory, experience, profile, reflection)
	ltm := memoryMgr.GetLongTermMemory()
	experienceEngine := memory.NewExperienceEngine(ltm, logger)
	profileStore := memory.NewProfileStore(ltm, logger)
	reflectionEngine := memory.NewReflectionEngine(ltm, profileStore, logger)
	contextInjector := memory.NewContextInjector(experienceEngine, profileStore, reflectionEngine, logger)

	logger.Info("intelligence subsystems initialized",
		"profile_entries", len(profileStore.GetAll()))

	// 8b. Wire LLM-backed summarizer into the compactor for L3 auto-compaction.
	// The summarizer uses the same provider pool as the agent. If no providers
	// are available, the compactor falls back to legacy concatenation.
	if len(providerReg.GetAvailable()) > 0 {
		summarizerModel := ""
		for _, pc := range cfg.Providers {
			if pc.Enabled && pc.DefaultModel != "" {
				summarizerModel = pc.DefaultModel
				break
			}
		}
		summarizer := memory.NewLLMSummarizer(
			&routerLLMAdapter{router: llmRouter, preferred: ""},
			memory.LLMSummarizerConfig{Model: summarizerModel, MaxEntries: 50},
			logger,
		)
		memoryMgr.GetCompactor().SetSummarizer(summarizer)
		logger.Info("LLM summarizer installed for auto-compaction", "model", summarizerModel)
	}

	orch := &Orchestrator{
		config:           cfg,
		roleRegistry:     roleRegistry,
		llmRouter:        llmRouter,
		providerReg:      providerReg,
		toolReg:          toolReg,
		skillReg:         skillReg,
		memoryMgr:        memoryMgr,
		contextInjector:  contextInjector,
		experienceEngine: experienceEngine,
		profileStore:     profileStore,
		reflectionEngine: reflectionEngine,
		tracer:           tracer,
		logger:           logger,
		shutdownChan:     make(chan struct{}),
	}

	// Wire delegation callback (tool already registered above)
	orch.wireDelegation(delegationTool)

	// Store MCP manager reference if it was created
	if mcpMgr != nil {
		orch.mcpManager = mcpMgr
	}

	providerCount := len(providerReg.List())
	roleCount := len(roleRegistry.List())
	toolCount := len(toolReg.List())
	skillCount := len(skillReg.List())

	logger.Info("orchestrator initialized successfully",
		"providers", providerCount,
		"roles", roleCount,
		"tools", toolCount,
		"skills", skillCount)

	// 9. Wire intelligence hooks into all role agents
	injector, recorder := orch.resolveInjector()
	if injector != nil || recorder != nil {
		roleRegistry.SetIntelligenceHooks(injector, recorder)
	}

	return orch, nil
}

// Execute routes a task to the correct role agent and executes it.
// If roleType is RoleCustom, auto-detects the best role based on task content.
func (o *Orchestrator) Execute(ctx context.Context, task string, roleType kyoci.RoleType) (*kyoci.TaskResult, error) {
	o.mu.RLock()
	started := o.started
	o.mu.RUnlock()

	if !started {
		return nil, fmt.Errorf("orchestrator not started — call Start() first")
	}

	// Start tracing span
	ctx, span := o.tracer.StartSpan(ctx, "Orchestrator.Execute")
	defer span.End()

	span.SetAttribute("task", task)
	span.SetAttribute("role_type", roleType.String())

	o.logger.Info("executing task", "task", task, "role_type", roleType.String())

	// Auto-detect role if not specified
	if roleType == kyoci.RoleCustom {
		roleType = ClassifyRole(task)
		o.logger.Info("auto-detected role", "detected_role", roleType.String())
		span.SetAttribute("detected_role", roleType.String())
	}

	// Get the role agent
	agentRole, err := o.roleRegistry.Get(roleType)
	if err != nil {
		o.logger.Error("failed to get role", "role", roleType.String(), "error", err)
		return nil, fmt.Errorf("failed to get role %s: %w", roleType.String(), err)
	}

	// Delegate to the role agent's Execute
	result, err := agentRole.Execute(ctx, task, o.memoryMgr)
	if err != nil {
		o.logger.Error("task execution failed", "error", err, "role", roleType.String())
		span.SetAttribute("error", err.Error())
		return nil, err
	}

	// Set the actual role used (may differ from request if auto-detected)
	result.Role = roleType

	o.logger.Info("task completed",
		"role", roleType.String(),
		"iterations", result.Iterations,
		"tool_calls", result.ToolCallsMade)

	return result, nil
}

// ExecuteStream routes a task to the correct role agent with streaming response.
// Returns a channel that emits StreamChunk messages as they are generated.
func (o *Orchestrator) ExecuteStream(ctx context.Context, task string, roleType kyoci.RoleType) (<-chan kyoci.StreamChunk, error) {
	o.mu.RLock()
	started := o.started
	o.mu.RUnlock()

	if !started {
		return nil, fmt.Errorf("orchestrator not started — call Start() first")
	}

	ctx, span := o.tracer.StartSpan(ctx, "Orchestrator.ExecuteStream")
	defer span.End()

	span.SetAttribute("task", task)
	span.SetAttribute("role_type", roleType.String())

	o.logger.Info("executing streaming task", "task", task, "role_type", roleType.String())

	// Auto-detect role if not specified
	if roleType == kyoci.RoleCustom {
		roleType = ClassifyRole(task)
		o.logger.Info("auto-detected role", "detected_role", roleType.String())
		span.SetAttribute("detected_role", roleType.String())
	}

	// Get the role agent
	agentRole, err := o.roleRegistry.Get(roleType)
	if err != nil {
		o.logger.Error("failed to get role", "role", roleType.String(), "error", err)
		return nil, fmt.Errorf("failed to get role %s: %w", roleType.String(), err)
	}

	// Delegate to the role agent's ExecuteStream
	chunkChan, err := agentRole.ExecuteStream(ctx, task)
	if err != nil {
		o.logger.Error("stream execution failed", "error", err)
		span.SetAttribute("error", err.Error())
		return nil, err
	}

	return chunkChan, nil
}

// ExecuteDirect runs a task directly using the agent without a specific role.
// This is useful for quick one-off tasks that don't need role-based routing.
func (o *Orchestrator) ExecuteDirect(ctx context.Context, task string, systemPrompt string) (*kyoci.TaskResult, error) {
	o.mu.RLock()
	started := o.started
	o.mu.RUnlock()

	if !started {
		return nil, fmt.Errorf("orchestrator not started — call Start() first")
	}

	ctx, span := o.tracer.StartSpan(ctx, "Orchestrator.ExecuteDirect")
	defer span.End()

	if systemPrompt == "" {
		systemPrompt = "You are a helpful AI assistant. Complete the task using available tools."
	}

	agentCfg := agent.AgentConfig{
		SystemPrompt:      systemPrompt,
		MaxIterations:     10,
		ToolChoice:        "auto",
		Temperature:       0.7,
		MaxTokens:         8192,
		PreferredProvider: "",
		EnableSkills:      true,
		EnableMemory:      true,
		EnableStreaming:    false,
	}

	ag := agent.NewAgent(agentCfg, o.llmRouter, o.toolReg.Kyoci(), o.skillReg.Kyoci(), o.memoryMgr)

	result, err := ag.Execute(ctx, task)
	if err != nil {
		o.logger.Error("direct execution failed", "error", err)
		return nil, err
	}

	o.logger.Info("direct task completed", "iterations", result.Iterations)
	return result, nil
}

// Status returns the current system status.
func (o *Orchestrator) Status() *SystemStatus {
	o.mu.RLock()
	defer o.mu.RUnlock()

	// Collect provider names
	providerMap := o.providerReg.List()
	providerNames := make([]string, 0, len(providerMap))
	for name := range providerMap {
		providerNames = append(providerNames, name)
	}

	// Collect tool definitions
	toolDefs := o.toolReg.List()

	// Collect skill info
	skillInfos := o.skillReg.List()

	// Collect role configs
	roleConfigs := o.roleRegistry.List()

	// Get memory stats
	memStats := o.memoryMgr.GetStats()

	return &SystemStatus{
		Status:      "running",
		Started:     o.started,
		Timestamp:   time.Now(),
		Roles:       roleConfigs,
		Tools:       toolDefs,
		Skills:      skillInfos,
		Providers:   providerNames,
		MemoryStats: &memStats,
	}
}

// Start marks the orchestrator as ready to accept requests.
func (o *Orchestrator) Start() {
	o.mu.Lock()
	o.started = true
	o.mu.Unlock()
	o.logger.Info("orchestrator started — ready to accept requests")
}

// Shutdown gracefully shuts down all subsystems.
// Closes memory store, clears traces, and signals shutdown.
func (o *Orchestrator) Shutdown() error {
	o.logger.Info("shutting down orchestrator")

	o.mu.Lock()
	o.started = false
	o.mu.Unlock()

	// Signal shutdown
	select {
	case <-o.shutdownChan:
		// Already closed
	default:
		close(o.shutdownChan)
	}

	// Close memory manager
	if err := o.memoryMgr.Close(); err != nil {
		o.logger.Error("failed to close memory manager", "error", err)
		return fmt.Errorf("failed to close memory manager: %w", err)
	}

	// Close MCP servers
	if o.mcpManager != nil {
		if err := o.mcpManager.Close(); err != nil {
			o.logger.Error("failed to close MCP manager", "error", err)
		}
	}

	// Clear traces
	o.tracer.Clear()

	o.logger.Info("orchestrator shutdown complete")
	return nil
}

// GetTracer returns the tracer instance.
func (o *Orchestrator) GetTracer() *tracing.Tracer {
	return o.tracer
}

// GetLogger returns the logger instance.
func (o *Orchestrator) GetLogger() *slog.Logger {
	return o.logger
}

// GetConfig returns the current configuration.
func (o *Orchestrator) GetConfig() *config.Config {
	return o.config
}

// GetProfileStore returns the profile store for external access.
func (o *Orchestrator) GetProfileStore() *memory.ProfileStore {
	return o.profileStore
}

// GetExperienceStats returns aggregate experience statistics.
func (o *Orchestrator) GetExperienceStats() memory.ExperienceStats {
	if o.experienceEngine == nil {
		return memory.ExperienceStats{}
	}
	return o.experienceEngine.GetStats()
}
