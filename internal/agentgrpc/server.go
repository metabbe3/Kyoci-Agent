// Package agentgrpc exposes the orchestrator's task execution, streaming, and
// status operations over a gRPC AgentService. It is the network counterpart to
// the HTTP /api/v1/execute and /api/v1/status endpoints and is opt-in: main.go
// starts it only when server.agent_grpc_port > 0, so the shipping benchmark
// suite is unaffected unless explicitly enabled.
//
// The package depends on a Backend interface rather than the concrete
// orchestrator, so it stays testable in isolation and free of the
// orchestrator's heavy dependency graph. *orchestrator.Orchestrator satisfies
// Backend via the OrchestratorBackend adapter (see adapter.go).
package agentgrpc

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"

	pb "github.com/metabbe3/Kyoci-Agent/internal/agentpb"
)

// TaskResult is the execution outcome Backend.Execute returns. It mirrors the
// fields of kyoci.TaskResult the gRPC layer needs, without forcing the agentgrpc
// package to import the orchestrator package (whose SystemStatus type carries
// further internal deps). Callers fill it from kyoci.TaskResult.
type TaskResult struct {
	Content       string
	Role          kyoci.RoleType
	Iterations    int
	ToolCallsMade int
	Usage         *kyoci.TokenUsage
}

// StreamChunk is one piece of a streamed execution as seen by the gRPC layer.
// Carries the same data as kyoci.StreamChunk.
type StreamChunk struct {
	Partial string
	Done    bool
	Usage   *kyoci.TokenUsage
}

// Status is the status snapshot Backend.Status returns.
type Status struct {
	Version       string
	UptimeSeconds int64
	ActiveRoles   []string
	Providers     []string
	Started       bool
	MemoryEntries int32
}

// Backend is the execution backend the gRPC server delegates to. It is a subset
// of *orchestrator.Orchestrator's public surface (Execute / ExecuteStream /
// Status), expressed in agentgrpc-local types so the package is unit-testable
// without standing up the full orchestrator.
//
// Implementations must be goroutine-safe; gRPC dispatches concurrent RPCs.
type Backend interface {
	// Execute runs prompt under role ("" or "custom" => auto-detect) and returns
	// the final result or a wrapped error.
	Execute(ctx context.Context, prompt, role string) (*TaskResult, error)
	// ExecuteStream runs prompt and emits chunks until the channel closes; the
	// last chunk carries Done=true. A non-nil error aborts the stream.
	ExecuteStream(ctx context.Context, prompt, role string) (<-chan StreamChunk, error)
	// Status returns the current status snapshot.
	Status() *Status
}

// grpcServer adapts a Backend to the generated AgentServiceServer interface.
type grpcServer struct {
	pb.UnimplementedAgentServiceServer
	backend Backend
	logger  *slog.Logger
	started time.Time
}

var _ pb.AgentServiceServer = (*grpcServer)(nil)

// Execute handles the unary Execute RPC: delegate to the backend, map the
// result to the proto response, and translate Go errors into gRPC status codes.
func (s *grpcServer) Execute(ctx context.Context, req *pb.ExecuteRequest) (*pb.ExecuteResponse, error) {
	prompt := req.GetPrompt()
	if prompt == "" {
		return nil, status.Error(codes.InvalidArgument, "prompt is required")
	}
	if req.GetTimeoutSeconds() > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(req.GetTimeoutSeconds())*time.Second)
		defer cancel()
	}

	start := time.Now()
	res, err := s.backend.Execute(ctx, prompt, req.GetRole())
	elapsed := time.Since(start)

	resp := &pb.ExecuteResponse{
		DurationMs: elapsed.Milliseconds(),
	}
	if err != nil {
		resp.Error = err.Error()
		resp.Role = req.GetRole()
		s.logger.Warn("Execute failed", "error", err, "role", req.GetRole(), "duration", elapsed)
		return resp, toStatusError(err)
	}
	resp.Result = res.Content
	resp.Role = res.Role.String()
	resp.Iterations = int32(res.Iterations)
	resp.ToolCalls = int32(res.ToolCallsMade)
	resp.Usage = toProtoUsage(res.Usage)
	s.logger.Info("Execute ok", "role", resp.Role, "iterations", resp.Iterations, "duration", elapsed)
	return resp, nil
}

// ExecuteStream handles the server-streaming ExecuteStream RPC: open the
// backend channel and forward chunks until done or the client disconnects.
func (s *grpcServer) ExecuteStream(req *pb.ExecuteRequest, stream grpc.ServerStreamingServer[pb.StreamChunk]) error {
	prompt := req.GetPrompt()
	if prompt == "" {
		return status.Error(codes.InvalidArgument, "prompt is required")
	}

	ctx := stream.Context()
	if req.GetTimeoutSeconds() > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(req.GetTimeoutSeconds())*time.Second)
		defer cancel()
	}

	ch, err := s.backend.ExecuteStream(ctx, prompt, req.GetRole())
	if err != nil {
		return toStatusError(err)
	}

	for {
		select {
		case chunk, ok := <-ch:
			if !ok {
				// Channel closed without a terminal done chunk — emit one so the
				// client stream resolves cleanly.
				return stream.Send(&pb.StreamChunk{Done: true})
			}
			if err := stream.Send(&pb.StreamChunk{
				Partial: chunk.Partial,
				Done:    chunk.Done,
				Usage:   toProtoUsage(chunk.Usage),
			}); err != nil {
				return err
			}
			if chunk.Done {
				return nil
			}
		case <-ctx.Done():
			return status.FromContextError(ctx.Err()).Err()
		}
	}
}

// GetStatus handles the unary GetStatus RPC.
func (s *grpcServer) GetStatus(_ context.Context, _ *pb.StatusRequest) (*pb.StatusResponse, error) {
	st := s.backend.Status()
	if st == nil {
		return nil, status.Error(codes.Unavailable, "status unavailable")
	}
	resp := &pb.StatusResponse{
		Version:       st.Version,
		UptimeSeconds: st.UptimeSeconds,
		ActiveRoles:   st.ActiveRoles,
		Providers:     st.Providers,
		Started:       st.Started,
		MemoryEntries: st.MemoryEntries,
	}
	if resp.Version == "" {
		resp.Version = "unknown"
	}
	if resp.UptimeSeconds == 0 {
		resp.UptimeSeconds = int64(time.Since(s.started).Seconds())
	}
	return resp, nil
}

// Server wraps a *grpc.Server with the AgentService registered and provides
// lifecycle helpers for main.go, mirroring internal/hitl.Server. The returned
// server is NOT started — call Serve with a listener (typically in a goroutine).
type Server struct {
	grpc    *grpc.Server
	backend Backend
	logger  *slog.Logger
	started time.Time
}

// NewServer constructs a gRPC server backed by backend. version is reported in
// GetStatus responses when the backend's own Status does not supply one.
func NewServer(backend Backend, version string, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	l := logger.With("component", "agent-grpc")
	now := time.Now()
	srv := &Server{
		backend: backend,
		logger:  l,
		started: now,
	}
	srv.grpc = grpc.NewServer()
	pb.RegisterAgentServiceServer(srv.grpc, &grpcServer{
		backend: backend,
		logger:  l,
		started: now,
	})
	return srv
}

// Serve blocks, serving gRPC on ln. Returns when Stop or GracefulStop is
// called, or the listener returns a fatal error. Panics if called twice.
func (s *Server) Serve(ln net.Listener) error {
	s.logger.Info("agent gRPC server listening", "addr", ln.Addr().String())
	return s.grpc.Serve(ln)
}

// GracefulStop stops the gRPC server, allowing in-flight RPCs to complete.
// Safe to call from a different goroutine than Serve.
func (s *Server) GracefulStop() {
	start := time.Now()
	s.grpc.GracefulStop()
	s.logger.Info("agent gRPC server stopped", "elapsed", time.Since(start))
}

// toProtoUsage converts a kyoci.TokenUsage pointer to the proto form. Returns
// nil for a nil pointer so omitted-usage stays absent on the wire.
func toProtoUsage(u *kyoci.TokenUsage) *pb.TokenUsage {
	if u == nil {
		return nil
	}
	return &pb.TokenUsage{
		PromptTokens:     int32(u.PromptTokens),
		CompletionTokens: int32(u.CompletionTokens),
		TotalTokens:      int32(u.TotalTokens),
	}
}

// toStatusError maps a Go error to a gRPC status error, honoring context
// cancellation/deadline. Unknown errors become Internal.
func toStatusError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return status.Error(codes.Canceled, err.Error())
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return status.Error(codes.DeadlineExceeded, err.Error())
	}
	return status.Error(codes.Internal, err.Error())
}
