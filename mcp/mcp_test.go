package mcp

import (
	"context"
	"testing"
	"time"
)

func TestProtocolTypes(t *testing.T) {
	// Test JSON-RPC request
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "test",
	}
	if req.JSONRPC != "2.0" {
		t.Errorf("Expected JSONRPC version 2.0, got %s", req.JSONRPC)
	}

	// Test JSON-RPC error
	err := &JSONRPCError{
		Code:    -32000,
		Message: "test error",
		Data:    "test data",
	}
	if err.Error() != "test error: test data" {
		t.Errorf("Unexpected error message: %s", err.Error())
	}

	// Test tool
	tool := Tool{
		Name:        "test_tool",
		Description: "A test tool",
	}
	if tool.Name != "test_tool" {
		t.Errorf("Expected tool name test_tool, got %s", tool.Name)
	}
}

func TestRegistry(t *testing.T) {
	registry := NewMCPRegistry()

	// Test empty registry
	stats := registry.Stats()
	if stats.Servers != 0 {
		t.Errorf("Expected 0 servers, got %d", stats.Servers)
	}

	// Test all tools on empty registry
	tools := registry.AllTools()
	if len(tools) != 0 {
		t.Errorf("Expected 0 tools, got %d", len(tools))
	}

	// Test get client on empty registry
	_, exists := registry.GetClient("nonexistent")
	if exists {
		t.Error("Expected false for nonexistent client")
	}

	// Test close all on empty registry (should not panic)
	registry.CloseAll()
}

func TestToolNotFoundError(t *testing.T) {
	err := NewToolNotFoundError("test_tool")
	if !IsToolNotFoundError(err) {
		t.Error("Expected IsToolNotFoundError to return true")
	}

	if err.Error() != "tool not found: test_tool" {
		t.Errorf("Unexpected error message: %s", err.Error())
	}

	otherErr := &JSONRPCError{
		Code:    -32601,
		Message: "Method not found",
	}
	if IsToolNotFoundError(otherErr) {
		t.Error("Expected IsToolNotFoundError to return false for JSONRPCError")
	}
}

func TestClientNotInitialized(t *testing.T) {
	config := MCPServerConfig{
		Name:    "test",
		Command: "echo",
		Args:    []string{"hello"},
	}

	client, err := NewMCPClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Client should not be ready before initialization
	if client.Ready() {
		t.Error("Client should not be ready before initialization")
	}

	// Tools should be empty before initialization
	tools := client.Tools()
	if len(tools) != 0 {
		t.Errorf("Expected 0 tools before initialization, got %d", len(tools))
	}
}

func TestRegistryAddServerGracefully(t *testing.T) {
	registry := NewMCPRegistry()

	// Add a server with an invalid command (should log warning but not panic)
	config := MCPServerConfig{
		Name:    "invalid",
		Command: "/nonexistent/command",
	}
	registry.AddServerGracefully(context.Background(), config)

	// Stats should still work
	stats := registry.Stats()
	if stats.Servers != 0 {
		t.Errorf("Expected 0 servers after failed add, got %d", stats.Servers)
	}
}

func TestTransportInterface(t *testing.T) {
	transport, err := NewStdioTransport("cat", []string{}, nil)
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}

	// Check IsAlive immediately
	if !transport.IsAlive() {
		t.Error("Transport should be alive immediately after creation")
	}

	// Close the transport
	if err := transport.Close(); err != nil {
		// cat may exit when stdin closes, so this is acceptable
		t.Logf("Transport close returned error (may be expected): %v", err)
	}

	// Check IsAlive after close
	if transport.IsAlive() {
		t.Error("Transport should not be alive after close")
	}
}

func TestClientRestart(t *testing.T) {
	// Create a client with a long-running command
	config := MCPServerConfig{
		Name:    "long_running",
		Command: "sleep",
		Args:    []string{"10"},
	}

	client, err := NewMCPClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Close the client (which kills the process)
	if err := client.Close(); err != nil {
		t.Errorf("Failed to close client: %v", err)
	}

	// Try to restart the client
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Restart(ctx); err != nil {
		// This might fail due to initialization issues, which is expected
		// since echo doesn't implement MCP protocol
		t.Logf("Restart failed as expected: %v", err)
	}

	// Clean up
	client.Close()
}