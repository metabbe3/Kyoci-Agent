package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
	"github.com/metabbe3/Kyoci-Agent/internal/config"
	"github.com/metabbe3/Kyoci-Agent/internal/gateway"
	"github.com/metabbe3/Kyoci-Agent/internal/orchestrator"
)

// Server wraps the orchestrator with HTTP handlers.
type Server struct {
	orch *orchestrator.Orchestrator
	addr string
}

// NewServer creates a new HTTP server wrapping the orchestrator.
func NewServer(orch *orchestrator.Orchestrator, addr string) *Server {
	return &Server{orch: orch, addr: addr}
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
		http.Error(w, fmt.Sprintf(`{"error":"failed to read body: %s"}`, err), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req TaskRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"invalid JSON: %s"}`, err), http.StatusBadRequest)
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
		http.Error(w, fmt.Sprintf(`{"error":"failed to read body: %s"}`, err), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req TaskRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"invalid JSON: %s"}`, err), http.StatusBadRequest)
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

	slog.Info("starting Kyoci Agent v5")

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

	// Create HTTP server
	server := NewServer(orch, addr)
	mux := server.Routes()
	httpServer := &http.Server{
		Addr:         addr,
		Handler:      mux,
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

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	slog.Info("shutdown signal received", "signal", sig.String())

	// Stop Telegram gateway first
	if tgCancel != nil {
		slog.Info("stopping Telegram gateway")
		tgCancel()
		time.Sleep(1 * time.Second) // Let polling goroutine finish
	}

	// Graceful HTTP shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("server forced shutdown", "error", err)
	}

	if err := orch.Shutdown(); err != nil {
		slog.Error("orchestrator shutdown error", "error", err)
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
