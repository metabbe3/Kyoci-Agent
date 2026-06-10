package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
)

// MCPServerConfig contains configuration for an MCP server
type MCPServerConfig struct {
	Name    string            `json:"name"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
	URL     string            `json:"url,omitempty"` // for SSE transport
}

// MCPClient is a client for communicating with an MCP server
type MCPClient struct {
	name      string
	config    MCPServerConfig
	transport Transport
	tools     []Tool
	mu        sync.RWMutex
	ready     bool
}

// Transport defines the interface for MCP transport implementations
type Transport interface {
	Send(request JSONRPCRequest) (*JSONRPCResponse, error)
	SendWithContext(ctx context.Context, request JSONRPCRequest) (*JSONRPCResponse, error)
	Close() error
	IsAlive() bool
}

// NewMCPClient creates a new MCP client for the given server configuration
func NewMCPClient(config MCPServerConfig) (*MCPClient, error) {
	client := &MCPClient{
		name:   config.Name,
		config: config,
		tools:  make([]Tool, 0),
	}

	// Create transport based on config
	var err error
	if config.Command != "" {
		// Use stdio transport
		client.transport, err = NewStdioTransport(config.Command, config.Args, config.Env)
		if err != nil {
			return nil, fmt.Errorf("failed to create stdio transport: %w", err)
		}
	} else if config.URL != "" {
		// SSE transport would go here (not implemented yet)
		return nil, errors.New("SSE transport not yet implemented")
	} else {
		return nil, errors.New("either command or url must be specified")
	}

	return client, nil
}

// Initialize initializes the connection to the MCP server
func (c *MCPClient) Initialize(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.transport.IsAlive() {
		return errors.New("transport is not alive")
	}

	// Send initialize request
	params := InitializeParams{
		ProtocolVersion: ProtocolVersion,
		Capabilities: map[string]interface{}{
			"roots": map[string]interface{}{
				"listChanged": false,
			},
			"sampling": map[string]interface{}{},
		},
		ClientInfo: ClientInfo{
			Name:    "kyoci-agent",
			Version: "4.3",
		},
	}

	request := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  MethodInitialize,
		Params:  params,
	}

	var response *JSONRPCResponse
	var err error

	if ctx != nil {
		response, err = c.transport.SendWithContext(ctx, request)
	} else {
		response, err = c.transport.Send(request)
	}

	if err != nil {
		return fmt.Errorf("initialize request failed: %w", err)
	}

	// Parse the result
	var result InitializeResult
	if err := json.Unmarshal(response.Result, &result); err != nil {
		return fmt.Errorf("failed to unmarshal initialize result: %w", err)
	}

	// Verify protocol version compatibility
	if result.ProtocolVersion != ProtocolVersion {
		slog.Warn("protocol version mismatch", "server_version", result.ProtocolVersion, "client_version", ProtocolVersion)
	}

	slog.Info("initialized MCP server", "name", result.ServerInfo.Name, "version", result.ServerInfo.Version)

	c.ready = true
	return nil
}

// ListTools retrieves the list of available tools from the MCP server
func (c *MCPClient) ListTools(ctx context.Context) ([]Tool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.ready {
		if err := c.initializeLocked(ctx); err != nil {
			return nil, err
		}
	}

	// Send tools/list request
	request := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  MethodToolsList,
		Params:  map[string]interface{}{},
	}

	var response *JSONRPCResponse
	var err error

	if ctx != nil {
		response, err = c.transport.SendWithContext(ctx, request)
	} else {
		response, err = c.transport.Send(request)
	}

	if err != nil {
		return nil, fmt.Errorf("tools/list request failed: %w", err)
	}

	// Parse the result
	var result ListToolsResult
	if err := json.Unmarshal(response.Result, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tools/list result: %w", err)
	}

	c.tools = result.Tools
	return result.Tools, nil
}

// CallTool calls a tool on the MCP server with the given arguments
func (c *MCPClient) CallTool(ctx context.Context, name string, args map[string]interface{}) (*CallToolResult, error) {
	c.mu.RLock()

	if !c.ready {
		c.mu.RUnlock()
		c.mu.Lock()
		if !c.ready {
			if err := c.initializeLocked(ctx); err != nil {
				c.mu.Unlock()
				return nil, err
			}
		}
		c.mu.RUnlock()
	} else {
		c.mu.RUnlock()
	}

	// Send tools/call request
	params := CallToolParams{
		Name:      name,
		Arguments: args,
	}

	request := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  MethodToolsCall,
		Params:  params,
	}

	var response *JSONRPCResponse
	var err error

	if ctx != nil {
		response, err = c.transport.SendWithContext(ctx, request)
	} else {
		response, err = c.transport.Send(request)
	}

	if err != nil {
		return nil, fmt.Errorf("tools/call request failed: %w", err)
	}

	// Parse the result
	var result CallToolResult
	if err := json.Unmarshal(response.Result, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tools/call result: %w", err)
	}

	if result.IsError {
		return &result, fmt.Errorf("tool call returned error")
	}

	return &result, nil
}

// ListPrompts retrieves the list of available prompts from the MCP server
func (c *MCPClient) ListPrompts(ctx context.Context) ([]Prompt, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.ready {
		if err := c.initializeLocked(ctx); err != nil {
			return nil, err
		}
	}

	// Send prompts/list request
	request := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  MethodPromptsList,
		Params:  map[string]interface{}{},
	}

	var response *JSONRPCResponse
	var err error

	if ctx != nil {
		response, err = c.transport.SendWithContext(ctx, request)
	} else {
		response, err = c.transport.Send(request)
	}

	if err != nil {
		return nil, fmt.Errorf("prompts/list request failed: %w", err)
	}

	// Parse the result
	var result ListPromptsResult
	if err := json.Unmarshal(response.Result, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal prompts/list result: %w", err)
	}

	return result.Prompts, nil
}

// GetPrompt retrieves a specific prompt template with the given arguments
func (c *MCPClient) GetPrompt(ctx context.Context, name string, args map[string]string) (*GetPromptResult, error) {
	c.mu.RLock()

	if !c.ready {
		c.mu.RUnlock()
		c.mu.Lock()
		if !c.ready {
			if err := c.initializeLocked(ctx); err != nil {
				c.mu.Unlock()
				return nil, err
			}
		}
		c.mu.RUnlock()
	} else {
		c.mu.RUnlock()
	}

	// Send prompts/get request
	params := GetPromptParams{
		Name:      name,
		Arguments: args,
	}

	request := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  MethodPromptsGet,
		Params:  params,
	}

	var response *JSONRPCResponse
	var err error

	if ctx != nil {
		response, err = c.transport.SendWithContext(ctx, request)
	} else {
		response, err = c.transport.Send(request)
	}

	if err != nil {
		return nil, fmt.Errorf("prompts/get request failed: %w", err)
	}

	// Parse the result
	var result GetPromptResult
	if err := json.Unmarshal(response.Result, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal prompts/get result: %w", err)
	}

	return &result, nil
}

// ListResources retrieves the list of available resources from the MCP server
func (c *MCPClient) ListResources(ctx context.Context) ([]Resource, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.ready {
		if err := c.initializeLocked(ctx); err != nil {
			return nil, err
		}
	}

	// Send resources/list request
	request := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  MethodResourcesList,
		Params:  map[string]interface{}{},
	}

	var response *JSONRPCResponse
	var err error

	if ctx != nil {
		response, err = c.transport.SendWithContext(ctx, request)
	} else {
		response, err = c.transport.Send(request)
	}

	if err != nil {
		return nil, fmt.Errorf("resources/list request failed: %w", err)
	}

	// Parse the result
	var result ListResourcesResult
	if err := json.Unmarshal(response.Result, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal resources/list result: %w", err)
	}

	return result.Resources, nil
}

// ReadResource reads the contents of a specific resource by URI
func (c *MCPClient) ReadResource(ctx context.Context, uri string) (*ReadResourceResult, error) {
	c.mu.RLock()

	if !c.ready {
		c.mu.RUnlock()
		c.mu.Lock()
		if !c.ready {
			if err := c.initializeLocked(ctx); err != nil {
				c.mu.Unlock()
				return nil, err
			}
		}
		c.mu.RUnlock()
	} else {
		c.mu.RUnlock()
	}

	// Send resources/read request
	params := ReadResourceParams{
		URI: uri,
	}

	request := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  MethodResourcesRead,
		Params:  params,
	}

	var response *JSONRPCResponse
	var err error

	if ctx != nil {
		response, err = c.transport.SendWithContext(ctx, request)
	} else {
		response, err = c.transport.Send(request)
	}

	if err != nil {
		return nil, fmt.Errorf("resources/read request failed: %w", err)
	}

	// Parse the result
	var result ReadResourceResult
	if err := json.Unmarshal(response.Result, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal resources/read result: %w", err)
	}

	return &result, nil
}

// Tools returns the cached list of tools
func (c *MCPClient) Tools() []Tool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	tools := make([]Tool, len(c.tools))
	copy(tools, c.tools)
	return tools
}

// Name returns the name of this MCP client
func (c *MCPClient) Name() string {
	return c.name
}

// Close closes the connection to the MCP server
func (c *MCPClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.ready {
		return nil
	}

	// Send shutdown notification if possible
	if c.transport.IsAlive() {
		request := JSONRPCRequest{
			JSONRPC: "2.0",
			Method:  MethodShutdown,
			Params:  map[string]interface{}{},
		}
		// Best effort - ignore errors
		_, _ = c.transport.Send(request)
	}

	err := c.transport.Close()
	c.ready = false
	c.tools = nil

	return err
}

// IsAlive returns true if the transport is still connected
func (c *MCPClient) IsAlive() bool {
	return c.transport.IsAlive()
}

// Restart restarts the MCP client connection
func (c *MCPClient) Restart(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Close existing connection
	if c.ready {
		_ = c.transport.Close()
		c.ready = false
	}

	// Re-create transport
	var err error
	if c.config.Command != "" {
		c.transport, err = NewStdioTransport(c.config.Command, c.config.Args, c.config.Env)
	} else if c.config.URL != "" {
		return errors.New("SSE transport not yet implemented")
	} else {
		return errors.New("either command or url must be specified")
	}

	if err != nil {
		return fmt.Errorf("failed to recreate transport: %w", err)
	}

	// Re-initialize
	return c.initializeLocked(ctx)
}

// initializeLocked is a helper that assumes lock is held
func (c *MCPClient) initializeLocked(ctx context.Context) error {
	if !c.transport.IsAlive() {
		return errors.New("transport is not alive")
	}

	params := InitializeParams{
		ProtocolVersion: ProtocolVersion,
		Capabilities: map[string]interface{}{
			"roots": map[string]interface{}{
				"listChanged": false,
			},
			"sampling": map[string]interface{}{},
		},
		ClientInfo: ClientInfo{
			Name:    "kyoci-agent",
			Version: "4.3",
		},
	}

	request := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  MethodInitialize,
		Params:  params,
	}

	var response *JSONRPCResponse
	var err error

	if ctx != nil {
		response, err = c.transport.SendWithContext(ctx, request)
	} else {
		response, err = c.transport.Send(request)
	}

	if err != nil {
		return fmt.Errorf("initialize request failed: %w", err)
	}

	var result InitializeResult
	if err := json.Unmarshal(response.Result, &result); err != nil {
		return fmt.Errorf("failed to unmarshal initialize result: %w", err)
	}

	slog.Info("initialized MCP server", "name", result.ServerInfo.Name, "version", result.ServerInfo.Version)

	c.ready = true
	return nil
}

// Ready returns true if the client is ready for use
func (c *MCPClient) Ready() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ready
}