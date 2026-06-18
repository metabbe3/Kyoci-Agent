package agentgrpc

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	pb "github.com/metabbe3/Kyoci-Agent/internal/agentpb"
	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// stubBackend is a controllable Backend for unit tests. It records the last
// call and returns whatever the test configured.
type stubBackend struct {
	mu       sync.Mutex
	execIn   string
	execRole string
	execErr  error
	// streamChunks is emitted in order by ExecuteStream; streamErr is returned
	// from the open call when non-nil.
	streamChunks []StreamChunk
	streamErr    error
	stat         *Status
}

func (s *stubBackend) Execute(ctx context.Context, prompt, role string) (*TaskResult, error) {
	s.mu.Lock()
	s.execIn = prompt
	s.execRole = role
	err := s.execErr
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	u := kyoci.TokenUsage{PromptTokens: 10, CompletionTokens: 20, TotalTokens: 30}
	return &TaskResult{
		Content:       "stubbed:" + prompt,
		Role:          kyoci.RoleDeveloper,
		Iterations:    2,
		ToolCallsMade: 1,
		Usage:         &u,
	}, nil
}

func (s *stubBackend) ExecuteStream(ctx context.Context, prompt, role string) (<-chan StreamChunk, error) {
	if s.streamErr != nil {
		return nil, s.streamErr
	}
	ch := make(chan StreamChunk, len(s.streamChunks))
	for _, c := range s.streamChunks {
		ch <- c
	}
	close(ch)
	return ch, nil
}

func (s *stubBackend) Status() *Status {
	if s.stat != nil {
		return s.stat
	}
	return &Status{
		Version:       "test",
		UptimeSeconds: 5,
		ActiveRoles:   []string{"developer", "sre"},
		Providers:     []string{"ollama"},
		Started:       true,
		MemoryEntries: 7,
	}
}

// newTestServer wires a stubBackend to a gRPC server backed by an in-process
// bufconn listener, returning a started client. The cleanup func stops the
// server and closes the client.
func newTestServer(t *testing.T, backend Backend) (pb.AgentServiceClient, func()) {
	t.Helper()
	const bufSize = 1024 * 1024
	ln := bufconn.Listen(bufSize)
	srv := NewServer(backend, "test", nil)
	go func() { _ = srv.Serve(ln) }()

	conn, err := grpc.NewClient("passthrough://bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return ln.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("grpc.NewClient: %v", err)
	}
	client := pb.NewAgentServiceClient(conn)
	return client, func() {
		_ = conn.Close()
		srv.GracefulStop()
	}
}

func TestExecute_Ok(t *testing.T) {
	backend := &stubBackend{}
	client, cleanup := newTestServer(t, backend)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := client.Execute(ctx, &pb.ExecuteRequest{Prompt: "hello", Role: "developer"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if resp.GetResult() != "stubbed:hello" {
		t.Errorf("result = %q, want %q", resp.GetResult(), "stubbed:hello")
	}
	if resp.GetRole() != "developer" {
		t.Errorf("role = %q, want developer", resp.GetRole())
	}
	if resp.GetIterations() != 2 || resp.GetToolCalls() != 1 {
		t.Errorf("iterations=%d tool_calls=%d", resp.GetIterations(), resp.GetToolCalls())
	}
	if u := resp.GetUsage(); u == nil || u.GetTotalTokens() != 30 {
		t.Errorf("usage = %+v, want total=30", u)
	}
	if resp.GetDurationMs() < 0 {
		t.Errorf("duration_ms = %d, want >= 0", resp.GetDurationMs())
	}
	if backend.execIn != "hello" || backend.execRole != "developer" {
		t.Errorf("backend saw prompt=%q role=%q", backend.execIn, backend.execRole)
	}
}

func TestExecute_EmptyPromptRejected(t *testing.T) {
	client, cleanup := newTestServer(t, &stubBackend{})
	defer cleanup()

	_, err := client.Execute(context.Background(), &pb.ExecuteRequest{Prompt: ""})
	if err == nil {
		t.Fatal("expected error for empty prompt, got nil")
	}
	if st, ok := status.FromError(err); !ok || st.Code() != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", err)
	}
}

func TestExecute_BackendErrorMapsToInternal(t *testing.T) {
	backend := &stubBackend{execErr: errors.New("boom")}
	client, cleanup := newTestServer(t, backend)
	defer cleanup()

	_, err := client.Execute(context.Background(), &pb.ExecuteRequest{Prompt: "x"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if st, ok := status.FromError(err); !ok || st.Code() != codes.Internal {
		t.Errorf("code = %v, want Internal", err)
	}
}

func TestExecuteStream_ForwardsChunks(t *testing.T) {
	backend := &stubBackend{
		streamChunks: []StreamChunk{
			{Partial: "alpha"},
			{Partial: "beta", Done: true},
		},
	}
	client, cleanup := newTestServer(t, backend)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := client.ExecuteStream(ctx, &pb.ExecuteRequest{Prompt: "go"})
	if err != nil {
		t.Fatalf("ExecuteStream open: %v", err)
	}

	var partials []string
	var sawDone bool
	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Recv: %v", err)
		}
		partials = append(partials, chunk.GetPartial())
		if chunk.GetDone() {
			sawDone = true
		}
	}
	if len(partials) != 2 || partials[0] != "alpha" || partials[1] != "beta" {
		t.Errorf("partials = %v", partials)
	}
	if !sawDone {
		t.Error("stream never observed a terminal done chunk")
	}
}

func TestGetStatus_ReturnsBackendSnapshot(t *testing.T) {
	client, cleanup := newTestServer(t, &stubBackend{})
	defer cleanup()

	resp, err := client.GetStatus(context.Background(), &pb.StatusRequest{})
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if resp.GetVersion() != "test" {
		t.Errorf("version = %q, want test", resp.GetVersion())
	}
	if !resp.GetStarted() {
		t.Error("started = false, want true")
	}
	if resp.GetMemoryEntries() != 7 {
		t.Errorf("memory_entries = %d, want 7", resp.GetMemoryEntries())
	}
	if len(resp.GetActiveRoles()) != 2 || len(resp.GetProviders()) != 1 {
		t.Errorf("roles=%v providers=%v", resp.GetActiveRoles(), resp.GetProviders())
	}
}

func TestParseRoleName(t *testing.T) {
	cases := []struct {
		in   string
		want kyoci.RoleType
	}{
		{"developer", kyoci.RoleDeveloper},
		{"dev", kyoci.RoleDeveloper},
		{"sre", kyoci.RoleSRE},
		{"qa", kyoci.RoleQA},
		{"pm", kyoci.RolePM},
		{"frontend", kyoci.RoleFrontend},
		{"", kyoci.RoleCustom}, // auto-detect
		{"unknown", kyoci.RoleCustom},
	}
	for _, c := range cases {
		if got := ParseRoleName(c.in); got != c.want {
			t.Errorf("ParseRoleName(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// Compile-time assertion that stubBackend and OrchestratorBackend satisfy the
// Backend contract. OrchestratorBackend lives in adapter.go.
var (
	_ Backend = (*stubBackend)(nil)
	_ Backend = (*OrchestratorBackend)(nil)
)
