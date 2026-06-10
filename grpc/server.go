package grpc

import (
	"context"
	"fmt"
	"time"

	"github.com/nicholas/ai-agent/agent"
	"github.com/nicholas/ai-agent/config"
	"github.com/nicholas/ai-agent/llm"
	"github.com/nicholas/ai-agent/proto"
	"github.com/nicholas/ai-agent/skill"
	"github.com/nicholas/ai-agent/tools"
	"google.golang.org/grpc"
)

var startTime = time.Now()

// Server implements the AgentService gRPC interface
type Server struct {
	proto.UnimplementedAgentServiceServer
	agent      *agent.Agent
	router     *llm.Router
	config     *config.Config
	tools      *tools.Registry
	skillReg   *skill.Registry
}

// NewServer creates a new gRPC server
func NewServer(cfg *config.Config, router *llm.Router, toolReg *tools.Registry, skillReg *skill.Registry, ag *agent.Agent) *Server {
	return &Server{
		agent:    ag,
		router:   router,
		config:   cfg,
		tools:    toolReg,
		skillReg: skillReg,
	}
}

// Chat handles a single chat request and returns a complete response
func (s *Server) Chat(ctx context.Context, req *proto.ChatRequest) (*proto.ChatResponse, error) {
	// Try skill registry first (Tier 0)
	if output, matched, _ := s.skillReg.Execute(ctx, req.Message); matched {
		defaultProvider, ok := s.router.GetProvider(s.config.LLM.DefaultProvider)
		modelName := s.config.LLM.DefaultProvider
		if ok {
			modelName = defaultProvider.Name()
		}

		return &proto.ChatResponse{
			Message:    output,
			Model:      modelName,
			Tier:       0,
			Tokens:     0,
			SessionId:  req.SessionId,
		}, nil
	}

	// Fall back to agent (Tier 1)
	response, err := s.agent.Run(ctx, req.Message)
	if err != nil {
		return nil, fmt.Errorf("chat failed: %w", err)
	}

	// Get default provider for model info
	defaultProvider, ok := s.router.GetProvider(s.config.LLM.DefaultProvider)
	modelName := s.config.LLM.DefaultProvider
	if ok {
		modelName = defaultProvider.Name()
	}

	return &proto.ChatResponse{
		Message:    response,
		Model:      modelName,
		Tier:       1,
		Tokens:     0, // TODO: Extract from agent response
		SessionId:  req.SessionId,
	}, nil
}

// StreamChat handles a chat request and returns a streaming response
func (s *Server) StreamChat(req *proto.ChatRequest, stream grpc.ServerStreamingServer[proto.ChatResponse]) error {
	ch, err := s.agent.Stream(context.Background(), req.Message)
	if err != nil {
		return fmt.Errorf("stream failed: %w", err)
	}

	defaultProvider, ok := s.router.GetProvider(s.config.LLM.DefaultProvider)
	modelName := s.config.LLM.DefaultProvider
	if ok {
		modelName = defaultProvider.Name()
	}

	for chunk := range ch {
		if err := stream.Send(&proto.ChatResponse{
			Message:    chunk,
			Model:      modelName,
			Tier:       1,
			Tokens:     0,
			SessionId:  req.SessionId,
		}); err != nil {
			return fmt.Errorf("send chunk failed: %w", err)
		}
	}

	return nil
}

// Status returns system status information
func (s *Server) Status(ctx context.Context, req *proto.StatusRequest) (*proto.StatusResponse, error) {
	providers := len(s.router.ListProviders())
	tools := len(s.tools.List())

	return &proto.StatusResponse{
		Status:   "running",
		Version:  "1.0.0",
		Providers: int32(providers),
		Tools:     int32(tools),
	}, nil
}

// Shutdown gracefully stops the gRPC server
func (s *Server) Shutdown() error {
	// Add any cleanup logic here
	return nil
}

// Name returns the server name for graceful shutdown
func (s *Server) Name() string {
	return "gRPCServer"
}