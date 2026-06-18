package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/metabbe3/Kyoci-Agent/internal/agent"
	"github.com/metabbe3/Kyoci-Agent/internal/agentdef"
	"github.com/metabbe3/Kyoci-Agent/internal/apperr"
	"github.com/metabbe3/Kyoci-Agent/internal/config"
	"github.com/metabbe3/Kyoci-Agent/internal/hitl"
	"github.com/metabbe3/Kyoci-Agent/internal/llm"
	"github.com/metabbe3/Kyoci-Agent/internal/mcp"
	"github.com/metabbe3/Kyoci-Agent/internal/memory"
	"github.com/metabbe3/Kyoci-Agent/internal/role"
	"github.com/metabbe3/Kyoci-Agent/internal/skill"
	"github.com/metabbe3/Kyoci-Agent/internal/taskctx"
	"github.com/metabbe3/Kyoci-Agent/internal/tool"
	"github.com/metabbe3/Kyoci-Agent/internal/tool/builtin"
	"github.com/metabbe3/Kyoci-Agent/internal/tracing"
	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// SystemStatus represents the current system status.
type SystemStatus struct {
	Status      string                 `json:"status"`
	Started     bool                   `json:"started"`
	Timestamp   time.Time              `json:"timestamp"`
	Roles       []kyoci.RoleConfig     `json:"roles"`
	Tools       []kyoci.ToolDefinition `json:"tools"`
	Skills      []kyoci.SkillInfo      `json:"skills"`
	Providers   []string               `json:"providers"`
	MemoryStats *memory.MemoryStats    `json:"memory_stats,omitempty"`
}

// Orchestrator is the central coordinator that ties all subsystems together.
// It manages the lifecycle of all components and routes tasks to the appropriate role agents.
// Goroutine-safe: All public methods are safe for concurrent use.
type Orchestrator struct {
	config       *config.Config
	roleRegistry *role.RoleRegistry
	llmRouter    *llm.Router
	providerReg  *llm.ProviderRegistry
	toolReg      *tool.Registry
	skillReg     *skill.Registry
	memoryMgr    *memory.MemoryManager
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

	// hitlCfg holds the optional Human-In-The-Loop retry configuration. Set
	// via SetHITL from main.go when the operator wires up the gRPC server.
	// When nil (or MaxRetries=0), tasks execute single-shot as before.
	hitlCfg *HITLConfig

	// activityPublisher fans out global activity events to the dashboard's
	// broker (for the Live Activity panel). Nil = no publisher wired; calls
	// to publishActivity are no-ops. Set via SetActivityPublisher from
	// cmd/server/main.go after both orchestrator + dashboard exist.
	activityPublisher func(kyoci.ActivityEvent)
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

	// 7. Create role registry and register agents from markdown.
	// The loader walks cfg.AgentsDir for *.md files at startup; each becomes
	// an agent registered with the role registry. An empty or missing dir is
	// non-fatal — the orchestrator still boots, but ClassifyRole returns
	// RoleGeneralist for every task (no specialists available). Operators
	// who hit this should create agents/*.md files (see agents/ in the repo
	// for the six built-in specialists).
	logger.Info("initializing role registry")
	roleRegistry := role.NewRoleRegistry(llmRouter, toolReg, skillReg, memoryMgr)

	agentDefs, loadErr := agentdef.LoadAgents(cfg.AgentsDir)
	if loadErr != nil {
		logger.Warn("agents dir load failed; continuing with empty agent set",
			"dir", cfg.AgentsDir, "error", loadErr)
	}
	if len(agentDefs) > 0 {
		SetDefaultAgentDefs(agentDefs)
		if err := roleRegistry.RegisterFromAgents(agentDefs, cfg); err != nil {
			return nil, fmt.Errorf("failed to register agents from %s: %w", cfg.AgentsDir, err)
		}
		logger.Info("agents registered from markdown",
			"dir", cfg.AgentsDir, "count", len(agentDefs))
	} else {
		logger.Warn("no agent definitions loaded; ClassifyRole will return generalist for every task",
			"dir", cfg.AgentsDir,
			"hint", "create *.md files in agents/ to define specialists")
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
		return nil, apperr.ErrNotStarted
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
	roleAgent, err := o.roleRegistry.Get(roleType)
	if err != nil {
		o.logger.Error("failed to get role", "role", roleType.String(), "error", err)
		return nil, fmt.Errorf("failed to get role %s: %w", roleType.String(), err)
	}

	// Per-task setup: mint an ID for log/manifest correlation, open the
	// per-run log file, and prepare the workspace folder. Each of these
	// degrades gracefully — a task must never fail because logging or
	// workspace setup hit an I/O error.
	startedAt := time.Now()
	taskID := hitl.NewTaskID()
	span.SetAttribute("task_id", taskID)

	runLogger, runLoggerCloser, logPath := OpenRunLogger(o.config, taskID, startedAt)
	defer runLoggerCloser()

	taskDir, wsErr := PrepareWorkspace(o.config, taskID)
	if wsErr != nil {
		// Workspace setup failure is non-fatal — log and continue without one.
		// The agent will still write to its legacy allowedDirs (".", home).
		runLogger.Warn("orchestrator: workspace setup failed; continuing without per-task isolation",
			"task_id", taskID, "err", wsErr)
		taskDir = ""
	}

	// Carry the workspace + per-run logger through ctx so the file tool and
	// the agent's log path pick them up without explicit plumbing.
	ctx = taskctx.WithWorkspace(ctx, taskDir)
	ctx = WithRunLogger(ctx, runLogger)

	// 8B-optimization Strategy 1: if the task references a specific subdir,
	// lock the file/glob/grep tools to that subtree. Combats the "wandering
	// agent" failure mode where small models burn their attention budget
	// scanning irrelevant sibling dirs. Detection is intentionally simple —
	// we look for a path-like token in the task string. If absent, no
	// sandbox is set (full access, current behavior).
	if sandbox := detectTaskSandbox(task); sandbox != "" {
		ctx = taskctx.WithSandbox(ctx, sandbox)
		o.logger.Info("orchestrator: task sandbox set", "sandbox", sandbox, "task_id", taskID)
	}

	// Treat the role as the abstract interface from here on so the per-task
	// clone returned by roleWithRunLogger can be a different concrete type.
	var agentRole kyoci.Role = roleAgent

	// Clone the role with the per-run logger so agent events (planner steps,
	// worker tool calls, synthesizer output) land in the per-run file as well
	// as stdout. The clone shares everything else with the role template.
	agentRole = o.roleWithRunLogger(agentRole, runLogger)

	runLogger.Info("orchestrator: task dispatched",
		"task_id", taskID, "role", roleType.String(),
		"workspace", taskDir, "log_path", logPath)

	// Delegate to the role agent's Execute — wrapped in the HITL retry loop
	// when the task carries a VERIFY: directive. executeWithRetry handles the
	// single-shot fast path internally when no directive is present.
	result, err := o.executeWithRetry(ctx, task, agentRole, roleType)

	completedAt := time.Now()
	status := "completed"
	if err != nil {
		status = "failed"
	}

	// Record what the task produced. extractFilesWritten scans the result's
	// tool-call log for `file` writes — these are the paths the manifest
	// advertises. Empty list (research/Q&A task) → cleanup the empty workspace
	// folder so tasks/ only holds folders with actual deliverables.
	filesWritten := []string{}
	if result != nil {
		filesWritten = extractFilesWritten(result.ToolCallLog)
	}
	o.finalizeTaskWorkspace(taskID, roleType.String(), task, startedAt, completedAt, status, filesWritten, logPath, result, err)

	if err != nil {
		runLogger.Error("task execution failed", "task_id", taskID, "error", err, "role", roleType.String())
		span.SetAttribute("error", err.Error())
		return nil, err
	}

	// Set the actual role used (may differ from request if auto-detected)
	result.Role = roleType

	runLogger.Info("task completed",
		"task_id", taskID,
		"role", roleType.String(),
		"iterations", result.Iterations,
		"tool_calls", result.ToolCallsMade,
		"files_written", len(filesWritten),
		"duration_ms", completedAt.Sub(startedAt).Milliseconds())

	return result, nil
}

// roleWithRunLogger returns a role scoped to the supplied logger, when the
// concrete role type supports it. *role.RoleAgent implements WithRunLogger;
// any future role type that wants per-task logging opts in by implementing
// the same method. Unsupported roles return unchanged — per-task agent
// logging silently no-ops for them rather than failing the task.
func (o *Orchestrator) roleWithRunLogger(r kyoci.Role, l *slog.Logger) kyoci.Role {
	if r == nil || l == nil {
		return r
	}
	type runLoggerAble interface {
		WithRunLogger(*slog.Logger) kyoci.Role
	}
	// First try the concrete type — *role.RoleAgent returns *RoleAgent which
	// is assignable to kyoci.Role. The interface assertion handles future
	// implementations that return the abstract type directly.
	if rl, ok := r.(runLoggerAble); ok {
		return rl.WithRunLogger(l)
	}
	if rl, ok := r.(*role.RoleAgent); ok {
		return rl.WithRunLogger(l)
	}
	return r
}

// finalizeTaskWorkspace writes the per-task manifest when files were produced
// and removes the empty workspace folder when they weren't. Errors here are
// logged but never propagated — manifest I/O failure must not fail a task
// that already succeeded.
func (o *Orchestrator) finalizeTaskWorkspace(
	taskID, roleLabel, task string,
	startedAt, completedAt time.Time,
	status string,
	filesWritten []string,
	logPath string,
	result *kyoci.TaskResult,
	taskErr error,
) {
	if TaskDir(o.config, taskID) == "" {
		return // workspaces disabled
	}

	summary := ""
	if result != nil {
		summary = truncateForHitl(result.Content, 500)
	} else if taskErr != nil {
		summary = truncateForHitl(taskErr.Error(), 500)
	}

	// Research/Q&A task — no files. Drop the empty folder so tasks/ stays clean.
	if len(filesWritten) == 0 {
		if err := CleanupIfEmpty(o.config, taskID); err != nil {
			slog.Warn("orchestrator: workspace cleanup failed",
				"task_id", taskID, "err", err)
		}
		return
	}

	manifest := taskManifest{
		TaskID:       taskID,
		Role:         roleLabel,
		Task:         truncateForHitl(task, 1000),
		StartedAt:    startedAt,
		CompletedAt:  completedAt,
		Status:       status,
		Summary:      summary,
		FilesCreated: filesWritten,
		LogPath:      formatLogPath(logPath),
	}
	if err := writeManifest(o.config, taskID, manifest); err != nil {
		slog.Warn("orchestrator: manifest write failed",
			"task_id", taskID, "err", err)
	}
}

// writeManifest atomically writes the per-task manifest.json next to the
// deliverable/ folder. The tmp-file-then-rename pattern ensures a partial
// write never leaves a corrupt manifest on disk.
func writeManifest(cfg *config.Config, taskID string, m taskManifest) error {
	root := TaskDir(cfg, taskID)
	if root == "" {
		return nil
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("manifest mkdir %s: %w", root, err)
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("manifest marshal: %w", err)
	}
	final := filepath.Join(root, "manifest.json")
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("manifest write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("manifest rename %s: %w", final, err)
	}
	return nil
}

// extractFilesWritten returns the distinct paths targeted by `file` tool calls
// with operation in {write, append, edit}. Order follows first occurrence.
// Used to populate the per-task manifest from the result's tool-call log.
func extractFilesWritten(log []kyoci.ToolCallEntry) []string {
	seen := map[string]bool{}
	var out []string
	for _, e := range log {
		if e.Tool != "file" {
			continue
		}
		var args struct {
			Operation string `json:"operation"`
			Path      string `json:"path"`
		}
		if err := json.Unmarshal([]byte(e.Args), &args); err != nil {
			continue
		}
		op := args.Operation
		if op != "write" && op != "append" && op != "edit" {
			continue
		}
		if args.Path == "" || seen[args.Path] {
			continue
		}
		seen[args.Path] = true
		out = append(out, args.Path)
	}
	return out
}

// ExecuteStream routes a task to the correct role agent with streaming response.
// Returns a channel that emits StreamChunk messages as they are generated.
func (o *Orchestrator) ExecuteStream(ctx context.Context, task string, roleType kyoci.RoleType) (<-chan kyoci.StreamChunk, error) {
	o.mu.RLock()
	started := o.started
	o.mu.RUnlock()

	if !started {
		return nil, apperr.ErrNotStarted
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
		return nil, apperr.ErrNotStarted
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
		EnableStreaming:   false,
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

// publishActivity ships an event to the dashboard's activity broker (if
// wired). The dashboard subscribes the Live Activity panel to the broker;
// this is the global path. The per-request SSE stream (chat client inline
// tree) is separate — agents emit there directly via their activity sink.
//
// No-op when no dashboard is wired (e.g. in tests or headless mode).
func (o *Orchestrator) publishActivity(evt kyoci.ActivityEvent) {
	if o.activityPublisher == nil {
		return
	}
	o.activityPublisher(evt)
}

// SetActivityPublisher wires a callback that the orchestrator uses to publish
// global activity events. Called from cmd/server/main.go after both the
// orchestrator and dashboard are constructed.
func (o *Orchestrator) SetActivityPublisher(fn func(kyoci.ActivityEvent)) {
	o.activityPublisher = fn
}

// RunExplore dispatches a read-only investigation using the Explore sub-agent
// worker. The worker shares the orchestrator's LLM router, skills, memory, and
// logger but gets a filtered tool provider that ONLY exposes glob, grep,
// file:read, git, codesearch, lsp — no write/patch/terminal. Returns the
// worker's Markdown summary directly (no metrics appended).
//
// This mirrors Claude Code's context-isolated Task tool: the parent agent's
// context window sees only the summary, not the raw file dumps the explore
// worker reads during its investigation.
func (o *Orchestrator) RunExplore(ctx context.Context, question string) (string, error) {
	o.mu.RLock()
	started := o.started
	o.mu.RUnlock()

	if !started {
		return "", apperr.ErrNotStarted
	}

	ctx, span := o.tracer.StartSpan(ctx, "Orchestrator.RunExplore")
	defer span.End()

	agentCfg := agent.AgentConfig{
		SystemPrompt:    agent.ExploreSystemPrompt,
		MaxIterations:   15,
		ToolChoice:      "auto",
		Temperature:     0.3, // lower temp → more deterministic exploration
		MaxTokens:       4096,
		PreferredProvider: "",
		EnableSkills:    false, // skills don't apply to exploration
		EnableMemory:    true,
		EnableStreaming:  false,
	}

	// Wrap the tool registry with the read-only filter. The filter is the
	// airtight defense: the explore worker literally cannot call write tools.
	tools := agent.NewReadOnlyToolFilter(o.toolReg.Kyoci(), nil)
	ag := agent.NewAgent(agentCfg, o.llmRouter, tools, o.skillReg.Kyoci(), o.memoryMgr)

	o.logger.Info("explore worker dispatched", "question", question)
	result, err := ag.Execute(ctx, question)
	if err != nil {
		o.logger.Error("explore worker failed", "error", err)
		return "", err
	}
	o.logger.Info("explore worker completed", "iterations", result.Iterations,
		"tool_calls", result.ToolCallsMade, "summary_len", len(result.Content))
	return result.Content, nil
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
	// Wire the agent package's global publisher so per-agent activity emits
	// fan out to the dashboard broker (Live Activity panel) without each
	// agent needing a per-request sink.
	agent.SetGlobalActivityPublisher(o.publishActivity)
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

// GetProviderRegistry returns the LLM provider registry for direct provider
// access from the dashboard (model enumeration, IsAvailable checks, direct
// streaming chat). Read-only access is safe; mutating the registry requires
// a server restart.
func (o *Orchestrator) GetProviderRegistry() *llm.ProviderRegistry {
	return o.providerReg
}

// GetSkillRegistry returns the prompt-skill registry. Read-only; the set of
// registered skills only changes at startup.
func (o *Orchestrator) GetSkillRegistry() *skill.Registry {
	return o.skillReg
}

// detectTaskSandbox inspects a task string for a path-like reference to a
// specific subdir. Returns the resolved absolute path when found, "" otherwise.
// Used to set per-task filesystem sandbox for the 8B "wandering agent" fix.
//
// Recognized patterns:
//   - Absolute path: "/projects/calculator", "/Users/foo/proj"
//   - Relative with subdir marker: "./projects/calculator", "projects/calculator/"
//   - Quote-delimited: 'fix the bug in "/projects/calculator"'
//
// Returns "" for ambiguous tasks (no clear subdir target) so we don't
// over-lock agents working at the project root.
func detectTaskSandbox(task string) string {
	// Strip newlines so multi-line tasks scan as one.
	task = strings.ReplaceAll(task, "\n", " ")

	// Try quoted paths first — highest confidence.
	quoted := regexp.MustCompile(`"((?:/|\./|\.\./|[A-Za-z]:\\)[^"]+)"`)
	if m := quoted.FindStringSubmatch(task); m != nil {
		if abs, err := filepath.Abs(m[1]); err == nil {
			if isDir(abs) {
				return abs
			}
		}
	}

	// Bare absolute Unix path with at least one slash and a directory suffix
	// or middle slash. Avoid matching file paths (those usually have extensions).
	absPath := regexp.MustCompile(`(?:^|\s)((?:/)[a-zA-Z0-9_./-]+/[a-zA-Z0-9_.-]*)`)
	for _, m := range absPath.FindAllStringSubmatch(task, -1) {
		candidate := strings.TrimRight(m[1], ".,;:!?")
		// Skip if it looks like a file (has a dot extension).
		if filepath.Ext(candidate) != "" {
			continue
		}
		if abs, err := filepath.Abs(candidate); err == nil {
			if isDir(abs) {
				return abs
			}
		}
	}

	// Relative "./foo/bar" or "foo/bar/" with directory suffix.
	relPath := regexp.MustCompile(`(?:^|\s)(\.\.?/[a-zA-Z0-9_./-]+/[a-zA-Z0-9_.-]*|projects/[a-zA-Z0-9_./-]+)`)
	if m := relPath.FindStringSubmatch(task); m != nil {
		candidate := strings.TrimRight(m[1], ".,;:!?/")
		if abs, err := filepath.Abs(candidate); err == nil {
			if isDir(abs) {
				return abs
			}
		}
	}

	return ""
}

// isDir reports whether path exists and is a directory.
func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
