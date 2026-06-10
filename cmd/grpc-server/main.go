package main

import (
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	grpcserver "google.golang.org/grpc"

	"github.com/nicholas/ai-agent/agent"
	"github.com/nicholas/ai-agent/config"
	grpc "github.com/nicholas/ai-agent/grpc"
	"github.com/nicholas/ai-agent/llm"
	"github.com/nicholas/ai-agent/proto"
	"github.com/nicholas/ai-agent/tools"
)

func main() {
	// Load configuration
	cfg, err := config.Load("")
	if err != nil {
		slog.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	// Create LLM router
	router := llm.NewRouter(cfg)

	// Create tool registry
	toolReg := tools.NewRegistry()
	// Register all tools here
	// For example:
	// toolReg.Register(tools.NewTerminal())
	// toolReg.Register(tools.NewCalculator())
	// etc.

	// Create agent
	ag := agent.NewV2(cfg, router, toolReg)

	// Create gRPC server implementation
	impl := grpc.NewServer(cfg, router, toolReg, ag)

	// Start listening
	addr := ":50051"
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		slog.Error("Failed to listen", "error", err)
		os.Exit(1)
	}

	s := grpcserver.NewServer()
	proto.RegisterAgentServiceServer(s, impl)

	// Handle graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		slog.Info("shutting down")
		s.GracefulStop()
	}()

	slog.Info("server listening", "address", addr)
	if err := s.Serve(lis); err != nil {
		slog.Error("Failed to serve", "error", err)
		os.Exit(1)
	}
}