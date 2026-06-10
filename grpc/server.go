package grpc

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"time"

	"github.com/nicholas/ai-agent/agent"
	"github.com/nicholas/ai-agent/config"
	"github.com/nicholas/ai-agent/llm"
	"github.com/nicholas/ai-agent/proto"
	"github.com/nicholas/ai-agent/tools"
)

var (
	startTime = time.Now()
)

// Server implements the AgentService gRPC interface
type Server struct {
	proto.UnimplementedAgentServiceServer
	agent  *agent.Agent
	router *llm.Router
	config *config.Config
	tools  *tools.Registry
}

// NewServer creates a new gRPC server
func NewServer(cfg *config.Config, router *llm.Router, toolReg *tools.Registry, ag *agent.Agent) *Server {
	return &Server{
		agent:  ag,
		router: router,
		config: cfg,
		tools:  toolReg,
	}
}

// Chat handles a single chat request and returns a complete response
func (s *Server) Chat(ctx context.Context, req *proto.ChatRequest) (*proto.ChatResponse, error) {
	response, err := s.agent.Run(ctx, req.Message)
	if err != nil {
		return nil, fmt.Errorf("chat failed: %w", err)
	}

	// Get default provider for model info
	defaultProvider, ok := s.router.GetProvider(s.config.LLM.DefaultProvider)
	modelUsed := s.config.LLM.DefaultProvider
	if ok {
		modelUsed = defaultProvider.Name()
	}

	return &proto.ChatResponse{
		Message:    response,
		SessionId:  req.SessionId,
		ModelUsed:  modelUsed,
		TokensIn:   0, // TODO: Extract from agent response
		TokensOut:  0,
		StopReason: "stop",
	}, nil
}

// ChatStream handles a chat request and returns a streaming response
func (s *Server) ChatStream(req *proto.ChatRequest, stream proto.AgentService_ChatStreamServer) error {
	ch, err := s.agent.Stream(context.Background(), req.Message)
	if err != nil {
		return fmt.Errorf("stream failed: %w", err)
	}

	for chunk := range ch {
		if err := stream.Send(&proto.ChatChunk{
			Content: chunk,
			Done:    false,
		}); err != nil {
			return fmt.Errorf("send chunk failed: %w", err)
		}
	}

	// Send final done marker
	if err := stream.Send(&proto.ChatChunk{
		Content: "",
		Done:    true,
	}); err != nil {
		return fmt.Errorf("send done marker failed: %w", err)
	}

	return nil
}

// ExecuteTool executes a tool directly without AI processing
func (s *Server) ExecuteTool(ctx context.Context, req *proto.ToolRequest) (*proto.ToolResponse, error) {
	result, err := s.tools.ExecuteTool(ctx, req.ToolName, json.RawMessage(req.ParametersJson))
	if err != nil {
		return &proto.ToolResponse{
			Result: result,
			Error:  true,
		}, nil
	}

	return &proto.ToolResponse{
		Result: result,
		Error:  false,
	}, nil
}

// GetStatus returns system status information
func (s *Server) GetStatus(ctx context.Context, req *proto.StatusRequest) (*proto.StatusResponse, error) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// Build provider info list from config
	providers := make([]*proto.ProviderInfo, 0)
	for name, providerCfg := range s.config.LLM.Providers {
		if provider, ok := s.router.GetProvider(name); ok {
			providers = append(providers, &proto.ProviderInfo{
				Name:      provider.Name(),
				Model:     providerCfg.Model,
				Available: ok,
			})
		}
	}

	// Get tool count
	toolList := s.tools.List()
	toolsCount := int32(len(toolList))

	uptime := int64(time.Since(startTime).Seconds())

	return &proto.StatusResponse{
		Providers:     providers,
		ToolsCount:    toolsCount,
		UptimeSeconds: uptime,
		Memory: &proto.MemoryStats{
			TotalAlloc: int64(m.TotalAlloc),
			HeapAlloc:  int64(m.HeapAlloc),
			Sys:        int64(m.Sys),
			GcCount:    int32(m.NumGC),
		},
	}, nil
}