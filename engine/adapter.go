package engine

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/nicholas/ai-agent/security"
)

// Adapter converts protocol-specific input into an EngineTask.
type Adapter interface {
	Adapt(r io.Reader) (*EngineTask, error)
}

// HTTPAdapter parses JSON HTTP request bodies into EngineTask.
type HTTPAdapter struct{}

// NewHTTPAdapter returns an adapter for HTTP JSON payloads.
func NewHTTPAdapter() *HTTPAdapter {
	return &HTTPAdapter{}
}

// Adapt parses the JSON body and constructs an EngineTask.
// Expected JSON schema: {session_id, message, mode, model, max_tokens, temperature}
func (a *HTTPAdapter) Adapt(r io.Reader) (*EngineTask, error) {
	var payload struct {
		SessionID  string `json:"session_id"`
		Message    string `json:"message"`
		Mode       string `json:"mode"`
		Model      string `json:"model"`
		MaxTokens  int    `json:"max_tokens"`
		Temperature float64 `json:"temperature"`
	}
	if err := json.NewDecoder(r).Decode(&payload); err != nil {
		return nil, fmt.Errorf("http_adapter: failed to decode JSON: %w", err)
	}
	message := security.SanitizeString(payload.Message)
	task := NewEngineTask(SourceHTTP, message)
	task = task.WithSession(payload.SessionID)
	if payload.Mode != "" {
		task = task.WithMetadata("mode", security.SanitizeString(payload.Mode))
	}
	if payload.Model != "" {
		task = task.WithMetadata("model", security.SanitizeString(payload.Model))
	}
	if payload.MaxTokens > 0 {
		task = task.WithTokenBudget(payload.MaxTokens)
	}
	if payload.Temperature > 0 {
		task = task.WithMetadata("temperature", fmt.Sprintf("%.2f", payload.Temperature))
	}
	return task, nil
}

// GRPCAdapter wraps proto message fields into an EngineTask.
// This is a minimal implementation that adapts from a JSON representation of gRPC request.
type GRPCAdapter struct{}

// NewGRPCAdapter returns a minimal gRPC adapter.
func NewGRPCAdapter() *GRPCAdapter {
	return &GRPCAdapter{}
}

// Adapt adapts from a JSON representation of the gRPC request.
// Expected JSON schema: {session_id, message, user_id}
func (a *GRPCAdapter) Adapt(r io.Reader) (*EngineTask, error) {
	var payload struct {
		SessionID string `json:"session_id"`
		Message   string `json:"message"`
		UserID    string `json:"user_id"`
	}
	if err := json.NewDecoder(r).Decode(&payload); err != nil {
		return nil, fmt.Errorf("grpc_adapter: failed to decode JSON: %w", err)
	}
	message := security.SanitizeString(payload.Message)
	task := NewEngineTask(SourceGRPC, message)
	task = task.WithSession(payload.SessionID)
	if payload.UserID != "" {
		task = task.WithMetadata("user_id", security.SanitizeString(payload.UserID))
	}
	return task, nil
}

// WSAdapter parses JSON WebSocket messages into EngineTask.
type WSAdapter struct{}

// NewWSAdapter returns an adapter for WebSocket JSON payloads.
func NewWSAdapter() *WSAdapter {
	return &WSAdapter{}
}

// Adapt parses the JSON payload and constructs an EngineTask.
// Expected JSON schema: {type, payload: {message, session_id}}
func (a *WSAdapter) Adapt(r io.Reader) (*EngineTask, error) {
	var wrapper struct {
		Type    string `json:"type"`
		Payload struct {
			Message   string `json:"message"`
			SessionID string `json:"session_id"`
		} `json:"payload"`
	}
	if err := json.NewDecoder(r).Decode(&wrapper); err != nil {
		return nil, fmt.Errorf("ws_adapter: failed to decode JSON: %w", err)
	}
	message := security.SanitizeString(wrapper.Payload.Message)
	task := NewEngineTask(SourceWS, message)
	task = task.WithSession(wrapper.Payload.SessionID)
	if wrapper.Type != "" {
		task = task.WithMetadata("type", security.SanitizeString(wrapper.Type))
	}
	return task, nil
}

// REPLAdapter wraps raw string input into an EngineTask.
type REPLAdapter struct{}

// NewREPLAdapter returns an adapter for REPL (raw string) input.
func NewREPLAdapter() *REPLAdapter {
	return &REPLAdapter{}
}

// Adapt reads raw input from the reader and creates an EngineTask.
func (a *REPLAdapter) Adapt(r io.Reader) (*EngineTask, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("repl_adapter: failed to read input: %w", err)
	}
	message := security.SanitizeString(string(data))
	task := NewEngineTask(SourceREPL, message)
	return task, nil
}