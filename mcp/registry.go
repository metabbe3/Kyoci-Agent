package mcp

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
)

// MCPRegistry manages multiple MCP clients and their tools
type MCPRegistry struct {
	clients map[string]*MCPClient
	tools   map[string]*MCPClient // tool_name -> client
	mu      sync.RWMutex
}

// NewMCPRegistry creates a new MCP registry
func NewMCPRegistry() *MCPRegistry {
	return &MCPRegistry{
		clients: make(map[string]*MCPClient),
		tools:   make(map[string]*MCPClient),
	}
}

// AddServer adds and initializes an MCP server to the registry
func (r *MCPRegistry) AddServer(ctx context.Context, config MCPServerConfig) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if server already exists
	if _, exists := r.clients[config.Name]; exists {
		return nil // Already added
	}

	// Create client
	client, err := NewMCPClient(config)
	if err != nil {
		slog.Error("failed to create MCP client", "server", config.Name, "error", err)
		return err
	}

	// Initialize client
	if err := client.Initialize(ctx); err != nil {
		slog.Error("failed to initialize MCP server", "server", config.Name, "error", err)
		client.Close()
		return err
	}

	// List tools from the server
	tools, err := client.ListTools(ctx)
	if err != nil {
		slog.Error("failed to list tools", "server", config.Name, "error", err)
		client.Close()
		return err
	}

	// Add client to registry
	r.clients[config.Name] = client

	// Map tool names to this client
	for _, tool := range tools {
		r.tools[tool.Name] = client
		slog.Info("registered tool", "tool", tool.Name, "server", config.Name)
	}

	slog.Info("added MCP server", "server", config.Name, "tool_count", len(tools))
	return nil
}

// AddServerGracefully adds an MCP server, logging warnings but not failing
func (r *MCPRegistry) AddServerGracefully(ctx context.Context, config MCPServerConfig) {
	if err := r.AddServer(ctx, config); err != nil {
		slog.Warn("failed to add MCP server", "server", config.Name, "error", err)
	}
}

// CallTool calls a tool by name, routing to the appropriate client
func (r *MCPRegistry) CallTool(ctx context.Context, toolName string, args map[string]interface{}) (*CallToolResult, error) {
	r.mu.RLock()
	client, exists := r.tools[toolName]
	r.mu.RUnlock()

	if !exists {
		return nil, NewToolNotFoundError(toolName)
	}

	// Try to call the tool
	result, err := client.CallTool(ctx, toolName, args)
	if err != nil {
		// Check if client is still alive
		if !client.IsAlive() {
			// Client died - attempt restart
			slog.Warn("MCP client died, attempting restart", "client", client.Name())
			if restartErr := client.Restart(ctx); restartErr != nil {
				return nil, fmt.Errorf("tool '%s' unavailable: client '%s' failed to restart: %w",
					toolName, client.Name(), restartErr)
			}
			// Retry the call after restart
			result, err = client.CallTool(ctx, toolName, args)
			if err != nil {
				return nil, fmt.Errorf("tool '%s' unavailable after client restart: %w", toolName, err)
			}
		} else {
			return nil, fmt.Errorf("tool '%s' call failed: %w", toolName, err)
		}
	}

	return result, nil
}

// AllTools returns all available tools from all registered servers
func (r *MCPRegistry) AllTools() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	allTools := make([]Tool, 0)
	for _, client := range r.clients {
		tools := client.Tools()
		allTools = append(allTools, tools...)
	}
	return allTools
}

// ToolsByServer returns tools for a specific server
func (r *MCPRegistry) ToolsByServer(serverName string) []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	client, exists := r.clients[serverName]
	if !exists {
		return nil
	}
	return client.Tools()
}

// ListAllPrompts returns all available prompts from all registered servers
func (r *MCPRegistry) ListAllPrompts(ctx context.Context) map[string][]Prompt {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string][]Prompt)
	for name, client := range r.clients {
		prompts, err := client.ListPrompts(ctx)
		if err != nil {
			slog.Error("failed to list prompts", "server", name, "error", err)
			continue
		}
		result[name] = prompts
	}
	return result
}

// GetPrompt retrieves a specific prompt by name, searching all clients
func (r *MCPRegistry) GetPrompt(ctx context.Context, name string, args map[string]string) (*GetPromptResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, client := range r.clients {
		// First check if this client has the prompt
		prompts, err := client.ListPrompts(ctx)
		if err != nil {
			slog.Error("failed to list prompts", "client", client.Name(), "error", err)
			continue
		}

		// Check if this prompt exists on this client
		found := false
		for _, p := range prompts {
			if p.Name == name {
				found = true
				break
			}
		}

		if found {
			// Get the prompt from this client
			return client.GetPrompt(ctx, name, args)
		}
	}

	return nil, fmt.Errorf("prompt not found: %s", name)
}

// ListAllResources returns all available resources from all registered servers
func (r *MCPRegistry) ListAllResources(ctx context.Context) map[string][]Resource {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string][]Resource)
	for name, client := range r.clients {
		resources, err := client.ListResources(ctx)
		if err != nil {
			slog.Error("failed to list resources", "server", name, "error", err)
			continue
		}
		result[name] = resources
	}
	return result
}

// ReadResource reads a specific resource by URI, searching all clients
func (r *MCPRegistry) ReadResource(ctx context.Context, uri string) (*ReadResourceResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, client := range r.clients {
		// First check if this client has the resource
		resources, err := client.ListResources(ctx)
		if err != nil {
			slog.Error("failed to list resources", "client", client.Name(), "error", err)
			continue
		}

		// Check if this resource exists on this client
		found := false
		for _, res := range resources {
			if res.URI == uri {
				found = true
				break
			}
		}

		if found {
			// Read the resource from this client
			return client.ReadResource(ctx, uri)
		}
	}

	return nil, fmt.Errorf("resource not found: %s", uri)
}

// GetClient returns a client by name
func (r *MCPRegistry) GetClient(name string) (*MCPClient, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	client, exists := r.clients[name]
	return client, exists
}

// RemoveServer removes a server from the registry
func (r *MCPRegistry) RemoveServer(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	client, exists := r.clients[name]
	if !exists {
		return nil
	}

	// Remove tool mappings
	for _, tool := range client.Tools() {
		delete(r.tools, tool.Name)
	}

	// Close the client
	if err := client.Close(); err != nil {
		slog.Warn("failed to close MCP client", "server", name, "error", err)
	}

	delete(r.clients, name)
	slog.Info("removed MCP server", "server", name)
	return nil
}

// CloseAll closes all MCP clients in the registry
func (r *MCPRegistry) CloseAll() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for name, client := range r.clients {
		if err := client.Close(); err != nil {
			slog.Warn("failed to close MCP client", "server", name, "error", err)
		}
		delete(r.tools, name)
	}
	r.clients = make(map[string]*MCPClient)
	r.tools = make(map[string]*MCPClient)
}

// Stats returns statistics about the registry
func (r *MCPRegistry) Stats() RegistryStats {
	r.mu.RLock()
	defer r.mu.RUnlock()

	alive := 0
	for _, client := range r.clients {
		if client.IsAlive() {
			alive++
		}
	}

	return RegistryStats{
		Servers:    len(r.clients),
		TotalTools: len(r.tools),
		Alive:      alive,
	}
}

// RegistryStats contains statistics about the MCP registry
type RegistryStats struct {
	Servers    int `json:"servers"`
	TotalTools int `json:"totalTools"`
	Alive      int `json:"alive"`
}

// ToolNotFoundError is returned when a tool is not found in the registry
type ToolNotFoundError struct {
	ToolName string
}

func NewToolNotFoundError(name string) *ToolNotFoundError {
	return &ToolNotFoundError{ToolName: name}
}

func (e *ToolNotFoundError) Error() string {
	return "tool not found: " + e.ToolName
}

// IsToolNotFoundError checks if an error is a ToolNotFoundError
func IsToolNotFoundError(err error) bool {
	_, ok := err.(*ToolNotFoundError)
	return ok
}