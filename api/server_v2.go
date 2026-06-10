package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/nicholas/ai-agent/agent"
	"github.com/nicholas/ai-agent/config"
	"github.com/nicholas/ai-agent/llm"
	"github.com/nicholas/ai-agent/skill"
	"github.com/nicholas/ai-agent/tools"
)

// ServerV2 is the HTTP API v2 server with security middleware
type ServerV2 struct {
	agent       *agent.Agent  // Global agent for non-session operations (deprecated, unused in chat)
	router      *llm.Router
	toolReg     *tools.Registry
	skillReg    *skill.Registry
	config      *config.Config
	apiKey      string
	rateLimiter *RateLimiter
	sessions    *SessionManager
	agentFactory func() *agent.Agent  // Factory to create per-session agents
	httpServer  *http.Server
}

// NewServerV2 creates a new API v2 server
func NewServerV2(cfg *config.Config, ag *agent.Agent, r *llm.Router, tr *tools.Registry, apiKey string) *ServerV2 {
	// Create agent factory for per-session agents
	agentFactory := func() *agent.Agent {
		return agent.NewV2(cfg, r, tr)
	}

	return &ServerV2{
		agent:       ag,
		router:      r,
		toolReg:     tr,
		config:      cfg,
		apiKey:      apiKey,
		rateLimiter: NewRateLimiter(100, time.Minute), // 100 requests per minute
		sessions:    NewSessionManager(agentFactory),
		agentFactory: agentFactory,
	}
}

// SetSkillRegistry sets the skill registry for Tier 0 matching
func (s *ServerV2) SetSkillRegistry(sr *skill.Registry) {
	s.skillReg = sr
}

// Start begins serving the HTTP API v2
func (s *ServerV2) Start() error {
	mux := http.NewServeMux()

	// v2 REST endpoints (wrapped with security)
	mux.HandleFunc("POST /v2/chat", s.withSecurity(s.handleV2Chat))
	mux.HandleFunc("POST /v2/stream", s.withSecurity(s.handleV2Stream))
	mux.HandleFunc("POST /v2/tool", s.withSecurity(s.handleV2Tool))
	mux.HandleFunc("GET /v2/status", s.withSecurity(s.handleV2Status))
	mux.HandleFunc("GET /v2/tools", s.withSecurity(s.handleV2Tools))
	mux.HandleFunc("GET /v2/memory", s.withSecurity(s.handleV2Memory))
	mux.HandleFunc("POST /v2/session/new", s.withSecurity(s.handleV2SessionNew))
	mux.HandleFunc("DELETE /v2/session/", s.withSecurity(s.handleV2SessionDelete))

	// WebSocket endpoint (with security)
	mux.HandleFunc("/v2/ws", s.withSecurity(s.handleWebSocketUpgrade))

	// CORS handler
	addr := fmt.Sprintf("%s:%d", s.config.Server.Host, s.config.Server.Port)
	slog.Info("starting server", "version", "v2", "address", addr)

	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      corsMiddleware(mux),
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 300 * time.Second, // Longer for streaming
	}

	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully shuts down the server
func (s *ServerV2) Shutdown(ctx context.Context) error {
	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

// withSecurity wraps a handler with API key and rate limiting
func (s *ServerV2) withSecurity(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check API key
		if s.apiKey != "" {
			receivedKey := r.Header.Get("X-API-Key")
			if receivedKey != s.apiKey {
				jsonError(w, http.StatusUnauthorized, "invalid API key")
				return
			}
		}

		// Rate limiting by IP
		ip := r.RemoteAddr
		if !s.rateLimiter.Allow(ip) {
			w.Header().Set("Retry-After", "60")
			jsonError(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}

		next(w, r)
	}
}

// handleV2Chat handles POST /v2/chat
func (s *ServerV2) handleV2Chat(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionID  string  `json:"session_id"`
		Message    string  `json:"message"`
		Mode       string  `json:"mode"`
		Model      string  `json:"model"`
		MaxTokens  int     `json:"max_tokens"`
		Temperature float64 `json:"temperature"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Message == "" {
		jsonError(w, http.StatusBadRequest, "message is required")
		return
	}

	// Get or create session
	sess := s.sessions.GetOrCreate(req.SessionID)

	// Set mode if specified
	if req.Mode != "" {
		sess.SetMode(req.Mode)
	}

	// Override model if specified (requires temporary config adjustment)
	originalModel := ""
	if req.Model != "" && req.Model != s.config.LLM.DefaultProvider {
		// Note: In production, you'd want to make this more robust
		originalModel = s.config.LLM.DefaultProvider
		if _, ok := s.router.GetProvider(req.Model); ok {
			// Successfully using alternate provider
			slog.Info("Using alternate provider", "provider", req.Model)
		}
	}

	ctx := r.Context()

	// ── Tier 0: Zero-AI skill (instant, free) ──
	if s.skillReg != nil {
		if output, matched, _ := s.skillReg.Execute(ctx, req.Message); matched {
			jsonResp(w, http.StatusOK, map[string]interface{}{
				"message":    output,
				"tier":       0,
				"model":      "builtin",
				"tokens":     0,
				"session_id": sess.ID,
			})
			return
		}
	}

	ag := sess.GetAgent()
	response, err := ag.Run(ctx, req.Message)

	// Restore original config
	if originalModel != "" {
		_ = s.router.SetDefault(originalModel)
	}

	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("agent error: %v", err))
		return
	}

	memory := ag.GetMemory()
	tokens := memory.TokenCount()

	jsonResp(w, http.StatusOK, map[string]interface{}{
		"message":    response,
		"model":      s.config.LLM.DefaultProvider,
		"tier":       1,
		"tokens":     tokens,
		"session_id": sess.ID,
	})
}

// handleV2Stream handles POST /v2/stream with SSE
func (s *ServerV2) handleV2Stream(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionID   string  `json:"session_id"`
		Message     string  `json:"message"`
		Mode        string  `json:"mode"`
		Model       string  `json:"model"`
		MaxTokens   int     `json:"max_tokens"`
		Temperature float64 `json:"temperature"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Message == "" {
		jsonError(w, http.StatusBadRequest, "message is required")
		return
	}

	// Get or create session
	sess := s.sessions.GetOrCreate(req.SessionID)

	// Set mode if specified
	if req.Mode != "" {
		sess.SetMode(req.Mode)
	}

	ctx := r.Context()
	ch, err := sess.GetAgent().Stream(ctx, req.Message)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("stream error: %v", err))
		return
	}

	// SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		jsonError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	memory := sess.GetAgent().GetMemory()
	tokens := memory.TokenCount()

	for chunk := range ch {
		fmt.Fprintf(w, "data: %s\n\n", toJson(map[string]interface{}{
			"content":    chunk,
			"session_id": sess.ID,
		}))
		flusher.Flush()
	}
	fmt.Fprintf(w, "data: %s\n\n", toJson(map[string]interface{}{
		"done":       true,
		"tokens":     tokens,
		"session_id": sess.ID,
	}))
	flusher.Flush()
}

// handleV2Tool handles POST /v2/tool
func (s *ServerV2) handleV2Tool(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ToolName   string                 `json:"tool_name"`
		Parameters map[string]interface{} `json:"parameters"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.ToolName == "" {
		jsonError(w, http.StatusBadRequest, "tool_name is required")
		return
	}

	ctx := r.Context()

	// Convert parameters to JSON
	paramsBytes, err := json.Marshal(req.Parameters)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid parameters")
		return
	}

	result, err := s.toolReg.ExecuteTool(ctx, req.ToolName, paramsBytes)
	if err != nil {
		jsonResp(w, http.StatusOK, map[string]interface{}{
			"result": nil,
			"error":  err.Error(),
		})
		return
	}

	jsonResp(w, http.StatusOK, map[string]interface{}{
		"result": result,
		"error":  nil,
	})
}

// handleV2Status handles GET /v2/status
func (s *ServerV2) handleV2Status(w http.ResponseWriter, r *http.Request) {
	memory := s.agent.GetMemory()
	longTermMem := s.agent.GetLongTermMemory()

	providers := s.router.ListProviders()
	tools := s.toolReg.List()

	jsonResp(w, http.StatusOK, map[string]interface{}{
		"status": "ok",
		"version": "2.0.0",
		"providers": map[string]interface{}{
			"default": s.config.LLM.DefaultProvider,
			"available": providers,
			"count": len(providers),
		},
		"tools": map[string]interface{}{
			"count": len(tools),
			"enabled": len(s.config.Tools.Enabled),
		},
		"memory": map[string]interface{}{
			"tokens_used": memory.TokenCount(),
			"max_tokens": memory.GetMaxTokens(),
			"usage_percent": memory.GetTokenUsage() * 100,
		},
		"sessions": s.sessions.Count(),
		"long_term_memory": map[string]interface{}{
			"enabled": longTermMem != nil,
		},
	})
}

// handleV2Tools handles GET /v2/tools
func (s *ServerV2) handleV2Tools(w http.ResponseWriter, r *http.Request) {
	toolList := s.toolReg.List()
	schemas := s.toolReg.Schemas()

	jsonResp(w, http.StatusOK, map[string]interface{}{
		"tools":   schemas,
		"count":   len(toolList),
		"enabled": s.config.Tools.Enabled,
	})
}

// handleV2Memory handles GET /v2/memory
func (s *ServerV2) handleV2Memory(w http.ResponseWriter, r *http.Request) {
	memory := s.agent.GetMemory()
	longTermMem := s.agent.GetLongTermMemory()

	shortTermStats := map[string]interface{}{
		"tokens_used":   memory.TokenCount(),
		"max_tokens":    memory.GetMaxTokens(),
		"usage_percent": memory.GetTokenUsage() * 100,
		"message_count": len(memory.GetMessages()),
	}

	response := map[string]interface{}{
		"short_term": shortTermStats,
		"long_term": map[string]interface{}{
			"enabled": longTermMem != nil,
		},
	}

	if longTermMem != nil {
		// Try to get some entries count (may not be available in interface)
		response["long_term"] = map[string]interface{}{
			"enabled": true,
		}
	}

	jsonResp(w, http.StatusOK, response)
}

// handleV2SessionNew handles POST /v2/session/new
func (s *ServerV2) handleV2SessionNew(w http.ResponseWriter, r *http.Request) {
	sess := s.sessions.Create()
	jsonResp(w, http.StatusCreated, map[string]interface{}{
		"session_id": sess.ID,
		"created_at": sess.CreatedAt,
	})
}

// handleV2SessionDelete handles DELETE /v2/session/{id}
func (s *ServerV2) handleV2SessionDelete(w http.ResponseWriter, r *http.Request) {
	// Extract session ID from path
	sessionID := r.URL.Path[len("/v2/session/"):]
	if sessionID == "" {
		jsonError(w, http.StatusBadRequest, "session ID required")
		return
	}

	if s.sessions.Delete(sessionID) {
		jsonResp(w, http.StatusOK, map[string]interface{}{
			"session_id": sessionID,
			"deleted":    true,
		})
	} else {
		jsonError(w, http.StatusNotFound, "session not found")
	}
}

// ── CORS Middleware ──

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")
		w.Header().Set("Access-Control-Allow-Credentials", "true")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// ── Response Helpers ──

func jsonResp(w http.ResponseWriter, code int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(data)
}

func jsonError(w http.ResponseWriter, code int, msg string) {
	jsonResp(w, code, map[string]interface{}{"error": msg})
}

func toJson(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// ── Rate Limiter (Sliding Window) ──

type RateLimiter struct {
	requests map[string]*timeWindow
	maxRequests int
	windowDuration time.Duration
	mu sync.RWMutex
}

type timeWindow struct {
	timestamps []time.Time
	mu         sync.Mutex
}

func NewRateLimiter(maxRequests int, windowDuration time.Duration) *RateLimiter {
	return &RateLimiter{
		requests:      make(map[string]*timeWindow),
		maxRequests:  maxRequests,
		windowDuration: windowDuration,
	}
}

func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	window, exists := rl.requests[ip]
	if !exists {
		window = &timeWindow{
			timestamps: make([]time.Time, 0),
		}
		rl.requests[ip] = window
	}
	rl.mu.Unlock()

	window.mu.Lock()
	defer window.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.windowDuration)

	// Remove old timestamps
	valid := make([]time.Time, 0)
	for _, ts := range window.timestamps {
		if ts.After(cutoff) {
			valid = append(valid, ts)
		}
	}
	window.timestamps = valid

	// Check if under limit
	if len(window.timestamps) >= rl.maxRequests {
		return false
	}

	// Add current request
	window.timestamps = append(window.timestamps, now)
	return true
}

// ── Session Manager ──

type Session struct {
	ID        string
	CreatedAt time.Time
	LastUsed  time.Time
	mu        sync.RWMutex
	agent     *agent.Agent  // Per-session agent instance
}

type SessionManager struct {
	sessions      sync.Map
	agentFactory  func() *agent.Agent  // Factory to create per-session agents
}

func NewSessionManager(agentFactory func() *agent.Agent) *SessionManager {
	sm := &SessionManager{
		agentFactory: agentFactory,
	}
	go sm.cleanup()
	return sm
}

func (sm *SessionManager) Create() *Session {
	id := generateSessionID()
	// Create a new agent for this session
	agentInstance := sm.agentFactory()
	sess := &Session{
		ID:        id,
		CreatedAt: time.Now(),
		LastUsed:  time.Now(),
		agent:     agentInstance,
	}
	sm.sessions.Store(id, sess)
	return sess
}

// GetAgent returns the session's agent instance
func (s *Session) GetAgent() *agent.Agent {
	return s.agent
}

// SetMode sets the mode on the session's agent
func (s *Session) SetMode(mode string) {
	s.agent.SetMode(mode)
}

func (sm *SessionManager) GetOrCreate(id string) *Session {
	if id == "" {
		return sm.Create()
	}

	val, ok := sm.sessions.Load(id)
	if !ok {
		return sm.Create()
	}

	sess := val.(*Session)
	sess.mu.Lock()
	sess.LastUsed = time.Now()
	sess.mu.Unlock()
	return sess
}

func (sm *SessionManager) Delete(id string) bool {
	_, ok := sm.sessions.LoadAndDelete(id)
	return ok
}

func (sm *SessionManager) Count() int {
	count := 0
	sm.sessions.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	return count
}

func (sm *SessionManager) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		sm.sessions.Range(func(key, val interface{}) bool {
			sess := val.(*Session)
			sess.mu.RLock()
			age := now.Sub(sess.LastUsed)
			sess.mu.RUnlock()

			// Delete sessions unused for 1 hour
			if age > time.Hour {
				sm.sessions.Delete(key)
			}
			return true
		})
	}
}

func generateSessionID() string {
	return fmt.Sprintf("sess_%d", time.Now().UnixNano())
}