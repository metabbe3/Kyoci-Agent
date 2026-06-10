package grpc

import (
	"context"
	"fmt"
	"io"
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
		Message:    message,
		SessionId:  generateSessionID(),
		Mode:       "default",
		MaxTokens:  4096,
		Temperature: 0.7,
	}

	for _, opt := range opts {
		opt(req)
	}

	return c.client.Chat(ctx, req)
}

// ChatStream sends a message and returns a streaming response
func (c *Client) ChatStream(ctx context.Context, message string, opts ...ChatOption) (chan string, error) {
	req := &proto.ChatRequest{
		Message:    message,
		SessionId:  generateSessionID(),
		Mode:       "default",
		MaxTokens:  4096,
		Temperature: 0.7,
	}

	for _, opt := range opts {
		opt(req)
	}

	stream, err := c.client.ChatStream(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("stream failed: %w", err)
	}

	ch := make(chan string, 10)

	go func() {
		defer close(ch)
		for {
			chunk, err := stream.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				slog.Error("gRPC stream error", "error", err)
				return
			}
			if chunk.Done {
				return
			}
			if chunk.Content != "" {
				ch <- chunk.Content
			}
		}
	}()

	return ch, nil
}

// ExecuteTool executes a tool directly
func (c *Client) ExecuteTool(ctx context.Context, toolName string, paramsJSON string) (*proto.ToolResponse, error) {
	req := &proto.ToolRequest{
		ToolName:       toolName,
		ParametersJson: paramsJSON,
	}

	return c.client.ExecuteTool(ctx, req)
}

// GetStatus retrieves system status
func (c *Client) GetStatus(ctx context.Context) (*proto.StatusResponse, error) {
	return c.client.GetStatus(ctx, &proto.StatusRequest{})
}

// ChatOption configures a ChatRequest
type ChatOption func(*proto.ChatRequest)

// WithSessionID sets the session ID
func WithSessionID(id string) ChatOption {
	return func(r *proto.ChatRequest) {
		r.SessionId = id
	}
}

// WithMode sets the agent mode
func WithMode(mode string) ChatOption {
	return func(r *proto.ChatRequest) {
		r.Mode = mode
	}
}

// WithModel sets the preferred model
func WithModel(model string) ChatOption {
	return func(r *proto.ChatRequest) {
		r.PreferredModel = model
	}
}

// WithMaxTokens sets the max tokens
func WithMaxTokens(tokens int32) ChatOption {
	return func(r *proto.ChatRequest) {
		r.MaxTokens = tokens
	}
}

// WithTemperature sets the temperature
func WithTemperature(temp float64) ChatOption {
	return func(r *proto.ChatRequest) {
		r.Temperature = temp
	}
}

func generateSessionID() string {
	return fmt.Sprintf("session-%d", time.Now().UnixNano())
}