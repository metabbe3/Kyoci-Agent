package hitl

import (
	"context"
	"log/slog"
	"net"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"

	pb "github.com/metabbe3/Kyoci-Agent/internal/hitl/pb"
)

// grpcServer adapts the in-process Hub to the gRPC HITLServiceServer
// interface. It is the network-facing counterpart to the Hub's in-process API.
type grpcServer struct {
	pb.UnimplementedHITLServiceServer
	hub    *Hub
	logger *slog.Logger
}

var _ pb.HITLServiceServer = (*grpcServer)(nil)

// SubscribeHelpRequests streams every new HelpRequest to the operator client.
// Blocks until the client disconnects or the context is cancelled.
func (s *grpcServer) SubscribeHelpRequests(
	_ *pb.SubscribeRequest,
	stream grpc.ServerStreamingServer[pb.HelpRequest],
) error {
	ch, unsub := s.hub.Subscribe()
	defer unsub()

	ctx := stream.Context()
	for {
		select {
		case helpReq := <-ch:
			if helpReq == nil {
				continue
			}
			if err := stream.Send(helpReq); err != nil {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// SubmitHint delivers an operator hint to the waiting agent goroutine.
// Returns accepted=false (no gRPC-level error) if no agent is currently
// waiting on the task ID.
func (s *grpcServer) SubmitHint(_ context.Context, sub *pb.HintSubmission) (*pb.HintResponse, error) {
	ok := s.hub.DeliverHint(sub.GetTaskId(), sub.GetHint())
	s.logger.Info("SubmitHint",
		"task_id", sub.GetTaskId(), "delivered", ok, "hint_len", len(sub.GetHint()))
	return &pb.HintResponse{Accepted: ok}, nil
}

// Server wraps a *grpc.Server with the HITL service registered and provides
// lifecycle helpers for main.go. The underlying Hub is shared between the
// gRPC layer and the in-process HITLHook consumers (the orchestrator).
type Server struct {
	grpc       *grpc.Server
	hub        *Hub
	logger     *slog.Logger
	started    atomic.Bool
}

// NewServer constructs a gRPC server backed by hub. The returned server is
// NOT started — call Serve with a listener (typically in a goroutine).
func NewServer(hub *Hub, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	srv := &Server{
		hub:    hub,
		logger: logger.With("component", "hitl-grpc"),
	}
	srv.grpc = grpc.NewServer()
	pb.RegisterHITLServiceServer(srv.grpc, &grpcServer{
		hub:    hub,
		logger: srv.logger,
	})
	return srv
}

// Serve blocks, serving gRPC on ln. Returns when Stop or GracefulStop is
// called, or the listener returns a fatal error. Panics if called twice.
func (s *Server) Serve(ln net.Listener) error {
	if !s.started.CompareAndSwap(false, true) {
		panic("hitl.Server: Serve called twice")
	}
	s.logger.Info("HITL gRPC server listening", "addr", ln.Addr().String())
	return s.grpc.Serve(ln)
}

// GracefulStop stops the gRPC server, allowing in-flight RPCs to complete.
// Safe to call from a different goroutine than Serve.
func (s *Server) GracefulStop() {
	start := time.Now()
	s.grpc.GracefulStop()
	s.logger.Info("HITL gRPC server stopped", "elapsed", time.Since(start))
}

// Hub returns the underlying in-process hub. Callers that need the HITLHook
// interface (e.g. the orchestrator) should use Hub directly rather than
// going through the network.
func (s *Server) Hub() *Hub { return s.hub }
