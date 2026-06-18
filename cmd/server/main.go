package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/metabbe3/Kyoci-Agent/internal/agentgrpc"
	"github.com/metabbe3/Kyoci-Agent/internal/apperr"
	"github.com/metabbe3/Kyoci-Agent/internal/config"
	"github.com/metabbe3/Kyoci-Agent/internal/dashboard"
	"github.com/metabbe3/Kyoci-Agent/internal/gateway"
	"github.com/metabbe3/Kyoci-Agent/internal/hitl"
	"github.com/metabbe3/Kyoci-Agent/internal/observability"
	"github.com/metabbe3/Kyoci-Agent/internal/orchestrator"
	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// version is the build version reported by /version and /health. Override at
// build time via -ldflags "-X main.version=...".
var version = "5.0.0"

// Server wraps the orchestrator with HTTP handlers.
type Server struct {
	orch *orchestrator.Orchestrator
	dash *dashboard.Server
	addr string
}

// NewServer creates a new HTTP server wrapping the orchestrator.
func NewServer(orch *orchestrator.Orchestrator, cfg *config.Config, cfgPath, addr string) *Server {
	dash := dashboard.NewServer(orch, cfg, cfgPath)
	// Wire the orchestrator's global activity publisher to the dashboard's
	// broker so Live Activity panel subscribers see every event.
	orch.SetActivityPublisher(dash.PublishActivity)
	return &Server{
		orch: orch,
		dash: dash,
		addr: addr,
	}
}

// TaskRequest is the JSON request body for task execution.
type TaskRequest struct {
	Task    string `json:"task"`
	Role    string `json:"role,omitempty"`
	Stream  bool   `json:"stream,omitempty"`
	Timeout int    `json:"timeout,omitempty"`
}

// TaskResponse is the JSON response for task execution.
type TaskResponse struct {
	Success    bool   `json:"success"`
	Result     string `json:"result,omitempty"`
	Role       string `json:"role,omitempty"`
	Iterations int    `json:"iterations,omitempty"`
	ToolCalls  int    `json:"tool_calls_made,omitempty"`
	Error      string `json:"error,omitempty"`
	DurationMs int64  `json:"duration_ms,omitempty"`
}

// parseRoleType converts a role string to kyoci.RoleType.
func parseRoleType(role string) kyoci.RoleType {
	switch role {
	case "developer", "dev":
		return kyoci.RoleDeveloper
	case "sre", "ops":
		return kyoci.RoleSRE
	case "qa", "test":
		return kyoci.RoleQA
	case "pm", "manager":
		return kyoci.RolePM
	case "frontend", "ui", "ux":
		return kyoci.RoleFrontend
	default:
		return kyoci.RoleCustom
	}
}

// roleTypeToString converts kyoci.RoleType to a string.
func roleTypeToString(rt kyoci.RoleType) string {
	switch rt {
	case kyoci.RoleDeveloper:
		return "developer"
	case kyoci.RoleSRE:
		return "sre"
	case kyoci.RoleQA:
		return "qa"
	case kyoci.RolePM:
		return "pm"
	case kyoci.RoleFrontend:
		return "frontend"
	default:
		return "custom"
	}
}

// handleExecute handles POST /api/v1/execute
func (s *Server) handleExecute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		apperr.WriteHTTPError(w, http.StatusBadRequest, "failed to read body: "+err.Error())
		return
	}
	defer r.Body.Close()

	var req TaskRequest
	if err := json.Unmarshal(body, &req); err != nil {
		apperr.WriteHTTPError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if req.Task == "" {
		http.Error(w, `{"error":"task is required"}`, http.StatusBadRequest)
		return
	}

	if req.Timeout <= 0 {
		req.Timeout = 600 // 10 min — delegation tasks need more time
	}

	timeout := time.Duration(req.Timeout) * time.Second
	// Use Background() context, NOT r.Context() — so delegation sub-agents
	// survive even if the HTTP client disconnects/times out.
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	roleType := parseRoleType(req.Role)
	start := time.Now()

	if req.Stream {
		s.handleStream(w, r, ctx, req.Task, roleType)
		return
	}

	slog.Info("executing task", "task", req.Task, "role", roleType.String(), "timeout", timeout)

	result, err := s.orch.Execute(ctx, req.Task, roleType)
	elapsed := time.Since(start)

	resp := TaskResponse{DurationMs: elapsed.Milliseconds()}

	if err != nil {
		resp.Success = false
		resp.Error = err.Error()
		resp.Role = roleTypeToString(roleType)
		slog.Error("task failed", "error", err, "duration", elapsed)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(resp)
		return
	}

	resp.Success = true
	resp.Result = result.Content
	resp.Role = roleTypeToString(result.Role)
	resp.Iterations = result.Iterations
	resp.ToolCalls = result.ToolCallsMade

	slog.Info("task completed", "role", resp.Role, "iterations", result.Iterations, "duration", elapsed)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleStream handles streaming task execution via SSE.
func (s *Server) handleStream(w http.ResponseWriter, r *http.Request, ctx context.Context, task string, roleType kyoci.RoleType) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, `{"error":"streaming not supported"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	chunkChan, err := s.orch.ExecuteStream(ctx, task, roleType)
	if err != nil {
		fmt.Fprintf(w, "data: {\"error\":\"%s\"}\n\n", err.Error())
		flusher.Flush()
		return
	}

	for chunk := range chunkChan {
		data, _ := json.Marshal(chunk)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
		if chunk.Done {
			break
		}
	}

	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

// handleStatus handles GET /api/v1/status
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	status := s.orch.Status()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// handleHealth handles GET /health
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"service": "kyoci-agent",
		"version": "5.0.0",
	})
}

// Routes sets up all HTTP routes.
func (s *Server) Routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/api/v1/execute", s.handleExecute)
	mux.HandleFunc("/api/v1/status", s.handleStatus)
	mux.HandleFunc("/api/v1/webhook", s.handleWebhook)

	// Dashboard API + embedded SPA. The SPA fallback is registered LAST so
	// the more specific /api/* routes win on Go's longest-prefix mux.
	if s.dash != nil {
		for path, handler := range s.dash.Routes() {
			mux.HandleFunc(path, handler)
		}
	}
	mux.HandleFunc("/", dashboard.SPAHandler())

	return mux
}

// handleWebhook handles POST /api/v1/webhook — external apps can call this
func (s *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// Check webhook secret if configured
	// Read body
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		apperr.WriteHTTPError(w, http.StatusBadRequest, "failed to read body: "+err.Error())
		return
	}
	defer r.Body.Close()

	var req TaskRequest
	if err := json.Unmarshal(body, &req); err != nil {
		apperr.WriteHTTPError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if req.Task == "" {
		http.Error(w, `{"error":"task is required"}`, http.StatusBadRequest)
		return
	}

	if req.Timeout <= 0 {
		req.Timeout = 600 // 10 min — delegation tasks need more time
	}

	timeout := time.Duration(req.Timeout) * time.Second
	// Use Background() context, NOT r.Context() — so delegation sub-agents
	// survive even if the HTTP client disconnects/times out.
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	roleType := parseRoleType(req.Role)
	start := time.Now()

	result, err := s.orch.Execute(ctx, req.Task, roleType)
	elapsed := time.Since(start)

	resp := TaskResponse{DurationMs: elapsed.Milliseconds()}

	if err != nil {
		resp.Success = false
		resp.Error = err.Error()
		resp.Role = roleTypeToString(roleType)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(resp)
		return
	}

	resp.Success = true
	resp.Result = result.Content
	resp.Role = roleTypeToString(result.Role)
	resp.Iterations = result.Iterations
	resp.ToolCalls = result.ToolCallsMade

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func main() {
	configPath := flag.String("config", "config/default.yaml", "path to config file")
	port := flag.Int("port", 0, "override server port")
	logLevel := flag.String("log-level", "", "override log level (debug/info/warn/error)")
	flag.Parse()

	logLevelStr := "info"
	if *logLevel != "" {
		logLevelStr = *logLevel
	}
	setupLogger(logLevelStr)

	// Initialize observability (OpenTelemetry traces + metrics, Prometheus
	// /metrics). OFF by default — enabled via env vars so the benchmark suite
	// (which does not set them) incurs zero exporter overhead. otelShutdown
	// flushes providers after the explicit component shutdown below (deferred
	// defers run last, i.e. after the inline shutdown sequence returns).
	otelShutdown, otelErr := observability.Setup(context.Background(), observability.Config{
		OTLPEndpoint:   os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
		ServiceName:    "kyoci-agent",
		ServiceVersion: version,
		TracesEnabled:  os.Getenv("KYOCI_OTEL_TRACES") == "1",
		MetricsEnabled: os.Getenv("KYOCI_OTEL_METRICS") == "1",
	})
	if otelErr != nil {
		slog.Warn("observability setup failed; continuing without telemetry", "error", otelErr)
	}
	defer func() {
		if otelShutdown != nil {
			_ = otelShutdown(context.Background())
		}
	}()

	slog.Info("starting Kyoci Agent v5", "version", version)

	// started anchors uptime reporting for the optional AgentService gRPC API.
	started := time.Now()

	// Load configuration
	slog.Info("loading configuration", "path", *configPath)
	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Warn("config file not found, using defaults", "path", *configPath, "error", err)
		cfg = config.Default()
	}

	if *port > 0 {
		cfg.Server.GRPCPort = *port
	}

	// Determine listen address
	addr := fmt.Sprintf(":%d", cfg.Server.GRPCPort)
	if cfg.Server.RESTPort > 0 {
		addr = fmt.Sprintf(":%d", cfg.Server.RESTPort)
	}

	// Create orchestrator (initializes ALL subsystems)
	orch, err := orchestrator.New(cfg)
	if err != nil {
		slog.Error("failed to initialize orchestrator", "error", err)
		os.Exit(1)
	}
	orch.Start()

	// Start the HITL gRPC server if enabled. The orchestrator's retry loop
	// uses the Hub directly (in-process); the gRPC server is the network
	// bridge for operator clients (cmd/hitlctl).
	var hitlHub *hitl.Hub
	var hitlServer *hitl.Server
	if cfg.HITL.Enabled {
		timeout := time.Duration(cfg.HITL.RequestTimeout) * time.Second
		if timeout <= 0 {
			timeout = 5 * time.Minute
		}
		hitlHub = hitl.NewHub(timeout, slog.Default())
		hitlServer = hitl.NewServer(hitlHub, slog.Default())

		hitlAddr := fmt.Sprintf(":%d", cfg.HITL.Port)
		ln, err := net.Listen("tcp", hitlAddr)
		if err != nil {
			slog.Error("failed to bind HITL gRPC listener", "addr", hitlAddr, "error", err)
			os.Exit(1)
		}
		go func() {
			if err := hitlServer.Serve(ln); err != nil {
				slog.Error("HITL gRPC server stopped", "error", err)
			}
		}()
		slog.Info("HITL gRPC server listening", "addr", hitlAddr, "request_timeout", timeout)

		// Wire the HITL hook into the orchestrator. MaxRetries comes from the
		// orchestration config so it travels with the rest of the retry budget.
		orch.SetHITL(&orchestrator.HITLConfig{
			MaxRetries: cfg.Agent.Orchestration.MaxRetries,
			Hook:       hitlHub,
		})
	} else {
		slog.Info("HITL gRPC server disabled (set hitl.enabled=true to enable)")
	}

	// Start the AgentService gRPC server (Execute/ExecuteStream/GetStatus) if
	// configured. Default-off: server.agent_grpc_port <= 0 means the server is
	// NOT started, so the HTTP API and the benchmark suite are unaffected. When
	// enabled, it runs alongside the HTTP server on its own port and delegates
	// to the same orchestrator.
	var agentGRPCServer *agentgrpc.Server
	if cfg.Server.AgentGRPCPort > 0 {
		backend := agentgrpc.NewOrchestratorBackend(orch, version, started)
		agentGRPCServer = agentgrpc.NewServer(backend, version, slog.Default())
		agentAddr := fmt.Sprintf(":%d", cfg.Server.AgentGRPCPort)
		agentLn, err := net.Listen("tcp", agentAddr)
		if err != nil {
			slog.Error("failed to bind AgentService gRPC listener", "addr", agentAddr, "error", err)
			os.Exit(1)
		}
		go func() {
			if err := agentGRPCServer.Serve(agentLn); err != nil {
				slog.Error("AgentService gRPC server stopped", "error", err)
			}
		}()
		slog.Info("AgentService gRPC server listening", "addr", agentAddr)
	}

	// Create HTTP server
	server := NewServer(orch, cfg, *configPath, addr)
	mux := server.Routes()
	// Debug/health endpoints: /healthz /readyz /metrics /version /debug/pprof.
	observability.MountDebug(mux, version, nil)
	httpServer := &http.Server{
		Addr:         addr,
		Handler:      observability.HTTPMiddleware(mux, slog.Default()),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 360 * time.Second, // 6 min — must exceed 5 min task timeout
		IdleTimeout:  120 * time.Second,
	}

	// Start HTTP server
	go func() {
		slog.Info("HTTP server listening", "addr", addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// Start Telegram gateway if configured
	var tgGateway *gateway.TelegramGateway
	var tgCtx context.Context
	var tgCancel context.CancelFunc

	if cfg.Telegram.Enabled && cfg.Telegram.Token != "" {
		slog.Info("initializing Telegram gateway")
		tgAdapter := gateway.NewOrchestratorAdapter(orch)
		tgGateway = gateway.NewTelegramGateway(gateway.TelegramConfig{
			Enabled:      cfg.Telegram.Enabled,
			Token:        cfg.Telegram.Token,
			AllowedUsers: cfg.Telegram.AllowedUsers,
			PollTimeout:  cfg.Telegram.PollTimeout,
		}, tgAdapter, slog.Default())

		tgCtx, tgCancel = context.WithCancel(context.Background())
		go func() {
			if err := tgGateway.Start(tgCtx); err != nil {
				slog.Error("Telegram gateway failed", "error", err)
			}
		}()
	}

	// Print startup banner
	printBanner(addr, cfg)

	// Graceful lifecycle: SIGINT/SIGTERM-driven, ordered, per-component
	// timeout-bounded shutdown via observability.Manager (replaces the former
	// inline sequence). Components are added in the reverse of the desired
	// shutdown order — Manager shuts down in reverse-add order, so this yields
	// telegram → http → orchestrator → hitl-grpc, matching the original sequence.
	// OTel providers flush via the deferred otelShutdown after components stop.
	mgr := observability.NewManager(slog.Default(), nil)
	mgr.Add("hitl-grpc", 10*time.Second, func(context.Context) error {
		if hitlServer != nil {
			hitlServer.GracefulStop()
		}
		return nil
	})
	mgr.Add("agent-grpc", 10*time.Second, func(context.Context) error {
		if agentGRPCServer != nil {
			agentGRPCServer.GracefulStop()
		}
		return nil
	})
	mgr.Add("orchestrator", 15*time.Second, func(context.Context) error {
		return orch.Shutdown()
	})
	mgr.Add("http", 10*time.Second, httpServer.Shutdown)
	mgr.Add("telegram", 5*time.Second, func(ctx context.Context) error {
		if tgCancel != nil {
			tgCancel()
		}
		if tgGateway != nil {
			return tgGateway.Stop(ctx)
		}
		return nil
	})
	if err := mgr.Run(context.Background(), func(ctx context.Context) error {
		<-ctx.Done() // block until SIGINT/SIGTERM (Manager installs the handler)
		return nil
	}); err != nil {
		slog.Error("shutdown completed with error", "error", err)
	}
	slog.Info("Kyoci Agent v5 stopped")
}

func setupLogger(level string) {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "info":
		lvl = slog.LevelInfo
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})
	slog.SetDefault(slog.New(handler))
}

func printBanner(addr string, cfg *config.Config) {
	fmt.Println()
	fmt.Println("  ╔═══════════════════════════════════════╗")
	fmt.Println("  ║       Kyoci Agent v5 — MVP            ║")
	fmt.Println("  ║       Plug & Play AI Agent Platform    ║")
	fmt.Println("  ╚═══════════════════════════════════════╝")
	fmt.Println()
	fmt.Printf("  HTTP API:  http://localhost%s\n", addr)

	if cfg.HITL.Enabled {
		fmt.Printf("  HITL gRPC: localhost:%d  (max_retries=%d)\n",
			cfg.HITL.Port, cfg.Agent.Orchestration.MaxRetries)
	} else {
		fmt.Println("  HITL:      ❌ Disabled")
	}

	if cfg.Server.AgentGRPCPort > 0 {
		fmt.Printf("  Agent gRPC: localhost:%d  (Execute/ExecuteStream/GetStatus)\n",
			cfg.Server.AgentGRPCPort)
	} else {
		fmt.Println("  Agent gRPC: ❌ Disabled")
	}

	if cfg.Telegram.Enabled {
		fmt.Println("  Telegram:  ✅ Connected")
	} else {
		fmt.Println("  Telegram:  ❌ Disabled")
	}

	fmt.Println()
	fmt.Println("  Endpoints:")
	fmt.Printf("    POST  http://localhost%s/api/v1/execute\n", addr)
	fmt.Printf("    GET   http://localhost%s/api/v1/status\n", addr)
	fmt.Printf("    GET   http://localhost%s/health\n", addr)
	fmt.Println()
	fmt.Println("  Press Ctrl+C to stop")
	fmt.Println()
}
