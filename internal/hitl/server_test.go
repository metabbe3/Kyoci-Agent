package hitl

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/metabbe3/Kyoci-Agent/internal/hitl/pb"
)

// TestGRPC_RoundTrip exercises the full network path:
//   in-process agent → Hub → gRPC server → gRPC client (operator) → SubmitHint → Hub → agent
//
// Uses bufconn so no real port is bound. This is the closest unit-level analog
// to the real cmd/hitlctl ↔ cmd/server round-trip the L4 benchmark drives.
func TestGRPC_RoundTrip(t *testing.T) {
	hub := NewHub(2*time.Second, nil)
	srv := NewServer(hub, nil)

	// bufconn in-process listener — no real port.
	ln := bufconnListen(t)
	srvDone := make(chan struct{})
	go func() {
		_ = srv.Serve(ln)
		close(srvDone)
	}()
	defer func() {
		srv.GracefulStop()
		<-srvDone
	}()

	// gRPC client — the operator side.
	conn := dialBufconn(t, ln)
	defer conn.Close()
	client := pb.NewHITLServiceClient(conn)

	// Open the subscription stream — simulates cmd/hitlctl.
	stream, err := client.SubscribeHelpRequests(context.Background(), &pb.SubscribeRequest{})
	if err != nil {
		t.Fatalf("SubscribeHelpRequests: %v", err)
	}

	// Give the subscriber a moment to register with the Hub. The stream open
	// call returns immediately but registration happens async in the handler.
	if !waitForSubscriber(hub, 500*time.Millisecond) {
		t.Fatal("subscriber did not register with hub within 500ms")
	}

	// Agent emits a HelpRequest (in goroutine; RequestHelp blocks).
	type res struct {
		hint string
		err  error
	}
	resCh := make(chan res, 1)
	go func() {
		hint, err := hub.RequestHelp(context.Background(), HelpRequest{
			TaskID:   "task-grpc-1",
			Role:     "developer",
			Attempt:  2,
			Question: "stuck on calculator.go",
		})
		resCh <- res{hint, err}
	}()

	// Operator reads the HelpRequest off the stream. stream.Recv blocks, so
	// we wrap it in a goroutine + select to enforce a deadline.
	type recvResult struct {
		req *pb.HelpRequest
		err error
	}
	recvCh := make(chan recvResult, 1)
	go func() {
		req, err := stream.Recv()
		recvCh <- recvResult{req, err}
	}()
	var gotReq *pb.HelpRequest
	select {
	case r := <-recvCh:
		if r.err != nil {
			t.Fatalf("stream.Recv: %v", r.err)
		}
		gotReq = r.req
	case <-time.After(500 * time.Millisecond):
		t.Fatal("did not receive HelpRequest on stream within 500ms")
	}
	if gotReq.GetTaskId() != "task-grpc-1" {
		t.Fatalf("expected task-grpc-1, got %q", gotReq.GetTaskId())
	}
	if gotReq.GetQuestion() != "stuck on calculator.go" {
		t.Fatalf("unexpected question: %q", gotReq.GetQuestion())
	}

	// Operator submits the hint via gRPC.
	resp, err := client.SubmitHint(context.Background(), &pb.HintSubmission{
		TaskId: "task-grpc-1",
		Hint:   "use + not *",
	})
	if err != nil {
		t.Fatalf("SubmitHint: %v", err)
	}
	if !resp.GetAccepted() {
		t.Fatal("SubmitHint returned accepted=false; expected true")
	}

	// Agent should now have the hint.
	select {
	case r := <-resCh:
		if r.err != nil {
			t.Fatalf("RequestHelp returned error: %v", r.err)
		}
		if r.hint != "use + not *" {
			t.Fatalf("expected hint 'use + not *', got %q", r.hint)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("RequestHelp did not return within 500ms after SubmitHint")
	}
}

// TestGRPC_SubmitHint_NoWaiter verifies that SubmitHint returns accepted=false
// (not a gRPC error) when no agent is currently waiting on the task ID.
func TestGRPC_SubmitHint_NoWaiter(t *testing.T) {
	hub := NewHub(1*time.Second, nil)
	srv := NewServer(hub, nil)
	ln := bufconnListen(t)
	srvDone := make(chan struct{})
	go func() {
		_ = srv.Serve(ln)
		close(srvDone)
	}()
	defer func() {
		srv.GracefulStop()
		<-srvDone
	}()

	conn := dialBufconn(t, ln)
	defer conn.Close()
	client := pb.NewHITLServiceClient(conn)

	resp, err := client.SubmitHint(context.Background(), &pb.HintSubmission{
		TaskId: "nonexistent",
		Hint:   "irrelevant",
	})
	if err != nil {
		t.Fatalf("SubmitHint returned transport error: %v", err)
	}
	if resp.GetAccepted() {
		t.Fatal("expected accepted=false for nonexistent waiter")
	}
}

// TestGRPC_NoOperator verifies that the Hub-level ErrNoSubscriber path still
// surfaces through the orchestrator-facing API even when the gRPC server is
// running but no operator has subscribed yet.
func TestGRPC_NoOperator(t *testing.T) {
	hub := NewHub(500*time.Millisecond, nil)
	srv := NewServer(hub, nil)
	ln := bufconnListen(t)
	srvDone := make(chan struct{})
	go func() {
		_ = srv.Serve(ln)
		close(srvDone)
	}()
	defer func() {
		srv.GracefulStop()
		<-srvDone
	}()

	// No client subscribed yet — fast fail.
	hint, err := hub.RequestHelp(context.Background(), HelpRequest{
		TaskID: "task-noop",
	})
	if !errors.Is(err, ErrNoSubscriber) {
		t.Fatalf("expected ErrNoSubscriber, got %v (hint=%q)", err, hint)
	}
}

// -----------------------------------------------------------------------------
// bufconn helpers — in-process gRPC listener, no real port
// -----------------------------------------------------------------------------

const bufconnSize = 1024 * 1024

func bufconnListen(t *testing.T) net.Listener {
	t.Helper()
	// Defer import to the test so the non-test package doesn't pull bufconn.
	return newBufconn(bufconnSize)
}

func dialBufconn(t *testing.T, ln net.Listener) *grpc.ClientConn {
	t.Helper()
	// We dial the same listener the server is serving on. The listener is a
	// pipe, so we just need any dialer that connects to it. Plain net.Dial
	// works because both ends are in-process.
	conn, err := grpc.NewClient(
		"passthrough://bufnet",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return ln.(*bufconnListener).Dial()
		}),
	)
	if err != nil {
		t.Fatalf("grpc.NewClient: %v", err)
	}
	return conn
}

// waitForSubscriber polls HasSubscriber up to the deadline. Returns false if
// the deadline passes without a subscriber registering.
func waitForSubscriber(hub *Hub, deadline time.Duration) bool {
	stop := time.Now().Add(deadline)
	for time.Now().Before(stop) {
		if hub.HasSubscriber() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return hub.HasSubscriber()
}
