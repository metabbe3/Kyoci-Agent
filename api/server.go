package api

// This file is intentionally minimal.
// The main API implementation has moved to server_v2.go and websocket.go.
//
// For backward compatibility, NewServer creates a v2 server with no API key.
//
// API v2 Features:
// - REST endpoints for chat, streaming, tools, status, memory, sessions
// - WebSocket support at /v2/ws for real-time bidirectional communication
// - Security middleware: API key validation (X-API-Key header)
// - Rate limiting per IP using sliding window algorithm
// - Session management with automatic cleanup

import (
	"context"

	"github.com/nicholas/ai-agent/agent"
	"github.com/nicholas/ai-agent/config"
	"github.com/nicholas/ai-agent/llm"
	"github.com/nicholas/ai-agent/selfimprove"
	"github.com/nicholas/ai-agent/skill"
	"github.com/nicholas/ai-agent/tools"
)

// Server is the HTTP API server (v2)
type Server struct {
	v2    *ServerV2
	shutdownChan chan struct{}
}

// NewServer creates a new API server (v2 by default)
func NewServer(cfg *config.Config, ag *agent.Agent, r *llm.Router, tr *tools.Registry) *Server {
	return &Server{
		v2:    NewServerV2(cfg, ag, r, tr, ""),
		shutdownChan: make(chan struct{}),
	}
}

// NewServerWithAPIKey creates a new API server with API key protection
func NewServerWithAPIKey(cfg *config.Config, ag *agent.Agent, r *llm.Router, tr *tools.Registry, apiKey string) *Server {
	return &Server{
		v2:    NewServerV2(cfg, ag, r, tr, apiKey),
		shutdownChan: make(chan struct{}),
	}
}

// Start begins serving the HTTP API
func (s *Server) Start() error {
	return s.v2.Start()
}

// SetSkillRegistry sets the skill registry for Tier 0 matching
func (s *Server) SetSkillRegistry(sr *skill.Registry) {
	s.v2.SetSkillRegistry(sr)
}

// SetSelfImprovePipeline sets the self-improvement pipeline
func (s *Server) SetSelfImprovePipeline(pipeline *selfimprove.SelfImprovePipeline) {
	s.v2.SetSelfImprovePipeline(pipeline)
}

// Shutdown gracefully stops the server
func (s *Server) Shutdown(ctx context.Context) error {
	close(s.shutdownChan)
	if s.v2.httpServer != nil {
		return s.v2.httpServer.Shutdown(ctx)
	}
	return nil
}

// V2 returns the v2 server instance
func (s *Server) V2() *ServerV2 {
	return s.v2
}