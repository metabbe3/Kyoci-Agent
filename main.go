package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/nicholas/ai-agent/agent"
	"github.com/nicholas/ai-agent/api"
	"github.com/nicholas/ai-agent/classifier"
	"github.com/nicholas/ai-agent/codegraph"
	"github.com/nicholas/ai-agent/config"
	"github.com/nicholas/ai-agent/engine"
	"github.com/nicholas/ai-agent/gateway"
	"github.com/nicholas/ai-agent/llm"
	"github.com/nicholas/ai-agent/pool"
	"github.com/nicholas/ai-agent/security"
	"github.com/nicholas/ai-agent/selfskill"
	"github.com/nicholas/ai-agent/skill"
	"github.com/nicholas/ai-agent/thinking"
	"github.com/nicholas/ai-agent/tools"
)

// Shutdownable defines components that can be gracefully shut down
type Shutdownable interface {
	Name() string
	Shutdown(ctx context.Context) error
}

// gracefulShutdown handles the shutdown sequence when a signal is received
func gracefulShutdown(ctx context.Context, components ...Shutdownable) {
	<-ctx.Done()
	slog.Warn("shutdown signal received", "signal", ctx.Err())

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	for _, c := range components {
		if err := c.Shutdown(shutdownCtx); err != nil {
			slog.Error("shutdown error", "component", c.Name(), "error", err)
		}
	}
	slog.Info("agent safely powered down")
}

// WorkerPoolShutdown wraps pool.WorkerPool for graceful shutdown
type WorkerPoolShutdown struct {
	pool *pool.WorkerPool
}

func (w *WorkerPoolShutdown) Name() string {
	return "WorkerPool"
}

func (w *WorkerPoolShutdown) Shutdown(ctx context.Context) error {
	w.pool.Stop()
	return nil
}

// ConnPoolShutdown wraps gateway.ConnectionPool for graceful shutdown
type ConnPoolShutdown struct {
	pool *gateway.ConnectionPool
}

func (c *ConnPoolShutdown) Name() string {
	return "ConnectionPool"
}

func (c *ConnPoolShutdown) Shutdown(ctx context.Context) error {
	c.pool.CloseAll()
	return nil
}

// SkillLifecycleShutdown wraps selfskill.SkillLifecycle for graceful shutdown
type SkillLifecycleShutdown struct {
	sl *selfskill.SkillLifecycle
}

func (s *SkillLifecycleShutdown) Name() string {
	return "SkillLifecycle"
}

func (s *SkillLifecycleShutdown) Shutdown(ctx context.Context) error {
	if s.sl != nil {
		s.sl.Stop()
	}
	return nil
}

// HTTPServerShutdown wraps api.Server for graceful shutdown
type HTTPServerShutdown struct {
	srv *api.Server
}

func (h *HTTPServerShutdown) Name() string {
	return "HTTPServer"
}

func (h *HTTPServerShutdown) Shutdown(ctx context.Context) error {
	if h.srv != nil {
		return h.srv.Shutdown(ctx)
	}
	return nil
}

// Global state for REPL commands
var (
	codeKnowledge *codegraph.CodeKnowledge
	skillLifecycle *selfskill.SkillLifecycle
	skillIdent    *selfskill.Identifier
	pendingPlan   *thinking.Plan
)

func initLogger() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)
}

func main() {
	initLogger()

	configPath := flag.String("config", "config/config.yaml", "Path to config file")
	serverMode := flag.Bool("serve", false, "Start HTTP API server")
	grpcMode := flag.Bool("grpc", false, "Start gRPC server")
	provider := flag.String("provider", "", "Override default provider")
	prompt := flag.String("prompt", "", "Single prompt (non-interactive)")
	maxWorkers := flag.Int("workers", 4, "Max worker goroutines")
	queueSize := flag.Int("queue", 100, "Job queue size")
	verbose := flag.Bool("v", false, "Verbose output")
	flag.Parse()

	// ── Config ──
	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("Failed to load config", "error", err)
		os.Exit(1)
	}
	if *provider != "" {
		cfg.LLM.DefaultProvider = *provider
	}
	if *verbose {
		cfg.Agent.Verbose = true
	}

	// ── Security ──
	_ = security.NewAPIKeyAuth()
	_ = security.NewRateLimiter(100)

	// ── Memory Pool ──
	_ = pool.NewBufferPool()

	// ── LLM Router ──
	router := llm.NewRouter(cfg)
	if len(router.ListProviders()) == 0 {
		slog.Error("No LLM providers configured")
		os.Exit(1)
	}
	slog.Info("Providers loaded", "providers", strings.Join(router.ListProviders(), ", "), "default", cfg.LLM.DefaultProvider)

	// ── Tiered Router (Circuit Breaker per provider) ──
	tieredRouter := gateway.NewTieredRouter(cfg)

	// ── gRPC Connection Pool ──
	connPool := gateway.NewConnectionPool()
	connPool.StartHealthCheck()
	defer connPool.CloseAll()

	// ── DAG Executor ──
	_ = gateway.NewDAGExecutor(*maxWorkers, 5*time.Minute, nil)

	// ── Tools ──
	toolReg := tools.NewRegistry()
	for _, t := range []tools.Tool{
		tools.NewWebSearchTool(),
		tools.NewCalculatorTool(),
		tools.NewFileHandlerTool(cfg.Tools.WorkDir),
		tools.NewTerminalTool(),
		tools.NewBrowserTool(),
		tools.NewHTTPClientTool(),
		tools.NewWebScraperTool(),
		tools.NewPDFTool(),
		tools.NewCodeExecTool(),
		tools.NewDatabaseTool(),
		tools.NewEmailTool(tools.EmailConfig{}),
		tools.NewImageGenTool(nil),
		tools.NewSchedulerTool(),
		tools.NewDelegationTool(),
		tools.NewVisionTool(nil),
	} {
		if err := toolReg.Register(t); err != nil {
			slog.Warn("Failed to register tool", "tool", t.Name(), "error", err)
		}
	}
	slog.Info("Tools registered", "count", len(toolReg.List()))

	// ── Zero-AI Skills ──
	skillReg := skill.NewRegistry()
	if err := skill.RegisterBuiltinSkills(skillReg); err != nil {
		slog.Warn("Some skills failed to register", "error", err)
	}
	slog.Info("Skills loaded", "count", skillReg.Count())

	// ── Agent v2 ──
	ag := agent.NewV2(cfg, router, toolReg)

	// ── Code Knowledge (AST + Vector Search) ──
	codeKnowledge = codegraph.NewCodeKnowledge(".")
	if err := codeKnowledge.Initialize(); err != nil {
		slog.Warn("Code knowledge init failed", "error", err)
	} else {
		codeKnowledge.WatchChanges(30 * time.Second)
		stats := codeKnowledge.Stats()
		slog.Info("Code knowledge initialized", "stats", stats)
	}

	// ── Slow Skill Creation Pipeline ──
	skillIdent = selfskill.NewIdentifier(5) // threshold: 5 occurrences
	skillGen := selfskill.NewSkillGenerator(skillIdent, "skill/auto", skillReg)
	skillLifecycle = selfskill.NewSkillLifecycle(skillGen, skillIdent)
	skillLifecycle.RunPeriodically(10 * time.Minute) // check every 10 min
	slog.Info("Self-skill pipeline active", "threshold", 5)

	// ── Worker Pool ──
	wp := pool.NewWorkerPool(*maxWorkers, *queueSize, func(ctx context.Context, task interface{}) (interface{}, error) {
		et, ok := task.(*engine.EngineTask)
		if !ok {
			return nil, fmt.Errorf("invalid task type")
		}
		return processTask(ctx, skillReg, ag, tieredRouter, et, cfg.Agent.Verbose), nil
	})
	wp.Start()
	defer wp.Stop()
	slog.Info("Worker pool started", "workers", *maxWorkers, "queue", *queueSize)

	// ── Signal handling for graceful shutdown ──
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Prepare shutdown components
	shutdownComponents := []Shutdownable{
		&WorkerPoolShutdown{pool: wp},
		&ConnPoolShutdown{pool: connPool},
	}
	if skillLifecycle != nil {
		shutdownComponents = append(shutdownComponents, &SkillLifecycleShutdown{sl: skillLifecycle})
	}

	// Start graceful shutdown goroutine
	go gracefulShutdown(ctx, shutdownComponents...)

	// ── Single prompt ──
	if *prompt != "" {
		task := engine.NewEngineTask(engine.SourceREPL, *prompt)
		result := processTask(ctx, skillReg, ag, tieredRouter, task, cfg.Agent.Verbose)
		fmt.Println(result.Message)
		return
	}

	// ── gRPC mode ──
	if *grpcMode {
		slog.Info("Starting gRPC server")
		select {}
	}

	// ── HTTP API mode ──
	if *serverMode {
		srv := api.NewServer(cfg, ag, router, toolReg)
		slog.Info("Starting API server", "host", cfg.Server.Host, "port", cfg.Server.Port)

		// Start server in a goroutine
		serverErr := make(chan error, 1)
		go func() {
			if err := srv.Start(); err != nil {
				serverErr <- err
			}
		}()

		// Wait for either signal or server error
		select {
		case <-ctx.Done():
			// Graceful shutdown
			slog.Info("Shutting down HTTP server")
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer shutdownCancel()
			if err := srv.Shutdown(shutdownCtx); err != nil {
				slog.Error("HTTP server shutdown error", "error", err)
			}
			return
		case err := <-serverErr:
			slog.Error("Server error", "error", err)
			os.Exit(1)
		}
	}

	// ── Interactive REPL ──
	printBanner(cfg, toolReg, skillReg, wp)

	scanner := bufio.NewScanner(os.Stdin)
	for {
		// Check if context is cancelled
		select {
		case <-ctx.Done():
			fmt.Println("\nReceived shutdown signal. Exiting...")
			return
		default:
		}

		fmt.Print("You: ")
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		// ── REPL Commands ──
		switch {
		case input == "quit" || input == "exit":
			printMemAndPoolStats(wp)
			fmt.Println("Goodbye!")
			return
		case input == "reset":
			ag.Reset()
			fmt.Println("🧹 Memory cleared.")
			continue
		case input == "providers":
			fmt.Printf("Available: %v (default: %s)\n", router.ListProviders(), cfg.LLM.DefaultProvider)
			continue
		case input == "skills":
			for _, s := range skillReg.List() {
				fmt.Printf("  ⚡ %s\n", s)
			}
			continue
		case input == "stats":
			printMemAndPoolStats(wp)
			continue
		case input == "pool":
			ps := wp.Stats()
			fmt.Printf("📊 Workers: active=%d | processed=%d | failed=%d | queue=%d/%d\n",
				ps.ActiveWorkers, ps.TotalProcessed, ps.TotalFailed, ps.QueueLength, ps.QueueCapacity)
			continue
		case input == "circuits":
			for _, tier := range tieredRouter.AvailableTiers() {
				fmt.Printf("  Tier %d: available\n", tier)
			}
			continue
		case input == "code":
			if codeKnowledge != nil {
				fmt.Printf("📚 %s\n", codeKnowledge.Stats())
			} else {
				fmt.Println("📚 Code knowledge not initialized")
			}
			continue
		case input == "pipeline":
			st := skillLifecycle.Status()
			fmt.Printf("🔧 Skill pipeline: candidates=%d | generated=%d | failed=%d | active=%d\n",
				st.Candidates, st.Generated, st.Failed, st.Active)
			continue
		case input == "approve":
			if pendingPlan != nil {
				pendingPlan.Approve("user")
				fmt.Printf("✅ Plan approved: %s (%d steps)\n", pendingPlan.Task, len(pendingPlan.Steps))
				// Execute approved plan
				for _, step := range pendingPlan.Steps {
					fmt.Printf("  → Step %d: %s\n", step.Number, step.Action)
					task := engine.NewEngineTask(engine.SourceREPL, step.Action)
					result := processTask(ctx, skillReg, ag, tieredRouter, task, cfg.Agent.Verbose)
					fmt.Printf("    %s\n", result.Message)
				}
				pendingPlan = nil
			} else {
				fmt.Println("❌ No pending plan to approve")
			}
			continue
		case input == "reject":
			if pendingPlan != nil {
				pendingPlan.Reject()
				fmt.Printf("❌ Plan rejected: %s\n", pendingPlan.Task)
				pendingPlan = nil
			} else {
				fmt.Println("❌ No pending plan to reject")
			}
			continue
		}

		if strings.HasPrefix(input, "use ") {
			name := strings.TrimSpace(strings.TrimPrefix(input, "use "))
			if err := router.SetDefault(name); err != nil {
				fmt.Printf("❌ %v\n", err)
			} else {
				cfg.LLM.DefaultProvider = name
				fmt.Printf("✅ Switched to: %s\n", name)
			}
			continue
		}

		// ── Code search command ──
		if strings.HasPrefix(input, "search ") {
			query := strings.TrimPrefix(input, "search ")
			if codeKnowledge != nil {
				sc, err := codeKnowledge.Query(query)
				if err != nil {
					fmt.Printf("❌ %v\n", err)
				} else {
					fmt.Printf("📚 Found %d results for \"%s\"\n", len(sc.Results), query)
					for _, r := range sc.Results {
						fmt.Printf("  %.3f %s\n", r.Score, r.Path)
					}
					if len(sc.ImpactFiles) > 0 {
						fmt.Printf("🔗 Impact: %v\n", sc.ImpactFiles)
					}
				}
			}
			continue
		}

		// ── Record task for skill pipeline ──
		skillLifecycle.RecordTask(input)

		// ── Complex task → Plan first ──
		cl := classifier.Classify(input)
		if cl.Level >= 4 {
			fmt.Printf("📋 Complex task detected (Level %d). Generating plan...\n", cl.Level)
			// Check for ambiguity
			if cr := thinking.DetectAmbiguity(input); cr != nil {
				fmt.Printf("❓ Clarification needed:\n%s\n", cr.Format())
				fmt.Print("Your answer: ")
				if scanner.Scan() {
					input = input + " " + scanner.Text()
				}
			}
			// Generate plan via AI
			planPrompt := fmt.Sprintf("Create a step-by-step execution plan for this task:\n%s\n\nOutput as numbered steps with action and expected result.", input)
			planResp, err := ag.Run(ctx, planPrompt)
			if err == nil {
				parsedPlan, err := thinking.ParsePlanFromAI(planResp)
				if err == nil && len(parsedPlan.Steps) > 0 {
					pendingPlan = parsedPlan
					fmt.Printf("\n📋 PLAN (awaiting approval):\n%s\n", pendingPlan.Format())
					fmt.Println("Type 'approve' to execute or 'reject' to cancel")
					continue
				}
			}
			// If plan parsing failed, just execute normally
		}

		// ── Submit to worker pool ──
		task := engine.NewEngineTask(engine.SourceREPL, input)
		result, err := wp.Submit(ctx, task)
		if err != nil {
			fmt.Printf("❌ %v\n", err)
			continue
		}
		if tr, ok := result.(*engine.TaskResult); ok {
			fmt.Printf("\n🤖 %s [tier=%d %v]\n\n", tr.Message, tr.Tier, tr.Duration)
		}
	}
}

// processTask — unified processing pipeline
func processTask(ctx context.Context, skillReg *skill.Registry, ag *agent.Agent, tr *gateway.TieredRouter, task *engine.EngineTask, verbose bool) *engine.TaskResult {
	start := time.Now()
	result := engine.NewTaskResult(task.ID)

	// ── Tier 0: Zero-AI skill (instant, free) ──
	if output, matched, _ := skillReg.Execute(ctx, task.Message); matched {
		result.Success = true
		result.Message = output
		result.Tier = 0
		result.Duration = time.Since(start)
		if verbose {
			slog.Debug("Tier 0 skill matched", "duration", result.Duration)
		}
		return result
	}

	// ── Classify ──
	cl := classifier.Classify(task.Message)
	if verbose {
		slog.Debug("Task classified", "level", cl.Level, "category", cl.Category, "needsAI", cl.NeedsAI, "budget", cl.TokenBudget)
	}

	// ── Tier 0 fallback: non-AI tasks ──
	if !cl.NeedsAI {
		result.Success = true
		result.Message = fmt.Sprintf("Task classified as %s (non-AI). No matching skill handler.", cl.Category)
		result.Tier = 0
		result.Duration = time.Since(start)
		return result
	}

	// ── Tier 1-2: AI path ──
	tier := 1
	if cl.Level >= 4 {
		tier = 2
	}

	// Set timeout by tier
	timeout := 150 * time.Second // Tier 1 (2.5 min)
	if tier == 2 {
		timeout = 300 * time.Second // Tier 2 (5 min)
	}
	if task.Timeout > 0 {
		timeout = task.Timeout
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Route through tiered router (with circuit breaker)
	provider, err := tr.Route(tier)
	if err == nil {
		result.ModelUsed = fmt.Sprintf("%s/%s", provider.Name, provider.Model)
		if verbose {
			slog.Debug("Route", "tier", tier, "provider", provider.Name, "model", provider.Model, "timeout", timeout)
		}
	}
	_ = provider

	// Inject code context for code-related queries
	if codeKnowledge != nil && (strings.Contains(strings.ToLower(task.Message), "code") ||
		strings.Contains(strings.ToLower(task.Message), "function") ||
		strings.Contains(strings.ToLower(task.Message), "struct") ||
		strings.Contains(strings.ToLower(task.Message), "implement")) {
		codeCtx, _ := codeKnowledge.GetContext(task.Message, 2000)
		if codeCtx != "" {
			task.Message = fmt.Sprintf("[Code Context]\n%s\n\n[Question]\n%s", codeCtx, task.Message)
		}
	}

	// Execute via agent
	response, err := ag.Run(ctx, task.Message)
	if err != nil {
		tr.ReportFailure(result.ModelUsed)
		result.Error = err.Error()
		result.Duration = time.Since(start)
		return result
	}

	tr.RecordSuccess(result.ModelUsed)

	result.Success = true
	result.Message = response
	result.Tier = tier
	result.Duration = time.Since(start)
	return result
}

func printBanner(cfg *config.Config, toolReg *tools.Registry, skillReg *skill.Registry, wp *pool.WorkerPool) {
	fmt.Println("═══════════════════════════════════════════════════════")
	fmt.Println("  🤖 Kyoci Agent v4 — Self-Aware Intelligence")
	fmt.Printf("  Provider: %s | Tools: %d | Skills: %d | Workers: %d\n",
		cfg.LLM.DefaultProvider, len(toolReg.List()), skillReg.Count(), wp.Stats().QueueCapacity)
	if codeKnowledge != nil {
		fmt.Printf("  Code: %s\n", codeKnowledge.Stats())
	}
	fmt.Println("  Commands: quit, reset, providers, use <n>, skills, stats,")
	fmt.Println("            pool, circuits, code, pipeline, search <q>,")
	fmt.Println("            approve, reject")
	fmt.Println("═══════════════════════════════════════════════════════")
	fmt.Println()
}

func printMemAndPoolStats(wp *pool.WorkerPool) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	ps := wp.Stats()
	fmt.Printf("📊 Heap: %.1fMB | Sys: %.1fMB | GC: %d | Goroutines: %d\n",
		float64(m.HeapAlloc)/1024/1024, float64(m.Sys)/1024/1024, m.NumGC, runtime.NumGoroutine())
	fmt.Printf("📊 Pool: active=%d | processed=%d | failed=%d | queue=%d/%d\n",
		ps.ActiveWorkers, ps.TotalProcessed, ps.TotalFailed, ps.QueueLength, ps.QueueCapacity)
	if skillLifecycle != nil {
		st := skillLifecycle.Status()
		fmt.Printf("📊 Skills: candidates=%d | generated=%d | active=%d\n",
			st.Candidates, st.Generated, st.Active)
	}
}
