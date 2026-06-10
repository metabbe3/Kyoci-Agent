package grpc

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/nicholas/ai-agent/proto"
)

// Client is a gRPC client for Kyoci Agent
type Client struct {
	conn   *grpc.ClientConn
	client proto.AgentServiceClient
}

// NewClient creates a new gRPC client
func NewClient(addr string) (*Client, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}

	return &Client{
		conn:   conn,
		client: proto.NewAgentServiceClient(conn),
	}, nil
}

// Close closes the client connection
func (c *Client) Close() error {
	return c.conn.Close()
}

// Chat sends a single message and returns the response
func (c *Client) Chat(ctx context.Context, message string, opts ...ChatOption) (*proto.ChatResponse, error) {
	req := &proto.ChatRequest{
		Message:   message,
		SessionId: generateSessionID(),
		Provider:  "",
	}

	for _, opt := range opts {
		opt(req)
	}

	return c.client.Chat(ctx, req)
}

// StreamChat sends a message and returns a streaming response
func (c *Client) StreamChat(ctx context.Context, message string, opts ...ChatOption) (chan string, error) {
	req := &proto.ChatRequest{
		Message:   message,
		SessionId: generateSessionID(),
		Provider:  "",
	}

	for _, opt := range opts {
		opt(req)
	}

	stream, err := c.client.StreamChat(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("stream failed: %w", err)
	}

	ch := make(chan string, 10)

	go func() {
		defer close(ch)
		for {
			resp, err := stream.Recv()
			if err != nil {
				slog.Error("gRPC stream error", "error", err)
				return
			}
			if resp.Message != "" {
				ch <- resp.Message
			}
		}
	}()

	return ch, nil
}

// Status retrieves system status
func (c *Client) Status(ctx context.Context) (*proto.StatusResponse, error) {
	return c.client.Status(ctx, &proto.StatusRequest{})
}

// ChatOption configures a ChatRequest
type ChatOption func(*proto.ChatRequest)

// WithSessionID sets the session ID
func WithSessionID(id string) ChatOption {
	return func(r *proto.ChatRequest) {
		r.SessionId = id
	}
}

// WithProvider sets the provider
func WithProvider(provider string) ChatOption {
	return func(r *proto.ChatRequest) {
		r.Provider = provider
	}
}

func generateSessionID() string {
	return fmt.Sprintf("session-%d", time.Now().UnixNano())
}