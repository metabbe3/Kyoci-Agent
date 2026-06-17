package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/metabbe3/Kyoci-Agent/pkg"
)

// MCPManager manages MCP servers and provides tools as Kyoci Tool interface
type MCPManager struct {
	registry   *MCPRegistry
	clients    []*MCPClient
	serverName string // Name prefix for identifying this manager's tools
}

// NewMCPManager creates a new MCP manager
func NewMCPManager(serverName string) *MCPManager {
	return &MCPManager{
		registry:   NewMCPRegistry(),
		clients:    make([]*MCPClient, 0),
		serverName: serverName,
	}
}

// AddServer adds and initializes an MCP server
func (m *MCPManager) AddServer(ctx context.Context, config MCPServerConfig) error {
	if err := m.registry.AddServer(ctx, config); err != nil {
		return err
	}
	// Get the client from the registry
	client, exists := m.registry.GetClient(config.Name)
	if !exists {
		return fmt.Errorf("client not found after adding: %s", config.Name)
	}
	m.clients = append(m.clients, client)
	slog.Info("MCP manager added server", "manager", m.serverName, "server", config.Name)
	return nil
}

// GetTools returns all MCP tools as Kyoci Tool implementations
func (m *MCPManager) GetTools() []kyoci.Tool {
	tools := make([]kyoci.Tool, 0)
	mcpTools := m.registry.AllTools()

	for _, mcpTool := range mcpTools {
		// Find the client for this tool
		client, exists := m.registry.tools[mcpTool.Name]
		if !exists {
			slog.Warn("tool found without client", "tool", mcpTool.Name)
			continue
		}

		adapter := &MCPToolAdapter{
			client:     client,
			toolName:   mcpTool.Name,
			toolDesc:   mcpTool.Description,
			inputSchema: mcpTool.InputSchema,
			managerName: m.serverName,
		}

		tools = append(tools, adapter)
	}

	return tools
}

// Close closes all MCP server connections
func (m *MCPManager) Close() error {
	m.registry.CloseAll()
	m.clients = nil
	slog.Info("MCP manager closed", "manager", m.serverName)
	return nil
}

// Stats returns statistics about the MCP manager
func (m *MCPManager) Stats() RegistryStats {
	return m.registry.Stats()
}

// MCPToolAdapter implements the kyoci.Tool interface for MCP tools
type MCPToolAdapter struct {
	client      *MCPClient
	toolName    string
	toolDesc    string
	inputSchema json.RawMessage
	managerName string
	// Cached parameters to avoid repeated parsing
	cachedParams []kyoci.ToolParameter
}

// Name returns the unique name of this tool
func (a *MCPToolAdapter) Name() string {
	// Prefix with manager name to avoid conflicts
	if a.managerName != "" {
		return fmt.Sprintf("%s_%s", a.managerName, a.toolName)
	}
	return a.toolName
}

// Description returns a human-readable description
func (a *MCPToolAdapter) Description() string {
	return a.toolDesc
}

// Parameters returns the parameter definition
func (a *MCPToolAdapter) Parameters() []kyoci.ToolParameter {
	// Return cached parameters if available
	if a.cachedParams != nil {
		return a.cachedParams
	}

	// Parse JSON schema from MCP tool
	if len(a.inputSchema) == 0 {
		a.cachedParams = []kyoci.ToolParameter{}
		return a.cachedParams
	}

	var schema map[string]interface{}
	if err := json.Unmarshal(a.inputSchema, &schema); err != nil {
		slog.Warn("failed to parse MCP tool schema", "tool", a.toolName, "error", err)
		a.cachedParams = []kyoci.ToolParameter{}
		return a.cachedParams
	}

	// Convert JSON Schema to Kyoci ToolParameter
	a.cachedParams = a.parseSchemaToParameters(schema)
	return a.cachedParams
}

// Execute executes the tool with the given parameters
func (a *MCPToolAdapter) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	// Call the MCP tool
	result, err := a.client.CallTool(ctx, a.toolName, params)
	if err != nil {
		return "", fmt.Errorf("MCP tool call failed: %w", err)
	}

	// Format the result
	if result.IsError {
		return "", fmt.Errorf("tool returned error")
	}

	// Concatenate all content blocks
	output := ""
	for _, block := range result.Content {
		if block.Type == "text" {
			output += block.Text
		}
	}

	return output, nil
}

// parseSchemaToParameters converts JSON Schema to Kyoci ToolParameter
func (a *MCPToolAdapter) parseSchemaToParameters(schema map[string]interface{}) []kyoci.ToolParameter {
	params := make([]kyoci.ToolParameter, 0)

	// Get properties
	properties, ok := schema["properties"].(map[string]interface{})
	if !ok {
		return params
	}

	// Get required fields
	requiredFields := make(map[string]bool)
	if required, ok := schema["required"].([]interface{}); ok {
		for _, field := range required {
			if name, ok := field.(string); ok {
				requiredFields[name] = true
			}
		}
	}

	// Convert each property
	for name, prop := range properties {
		propMap, ok := prop.(map[string]interface{})
		if !ok {
			continue
		}

		param := kyoci.ToolParameter{
			Name: name,
		}

		// Extract type
		if typeVal, ok := propMap["type"].(string); ok {
			param.Type = typeVal
		} else {
			param.Type = "object" // Default to object for complex types
		}

		// Extract description
		if desc, ok := propMap["description"].(string); ok {
			param.Description = desc
		}

		// Check if required
		param.Required = requiredFields[name]

		// Handle default value
		if defaultVal, ok := propMap["default"]; ok {
			param.Default = defaultVal
		}

		// Handle enum values
		if enumVal, ok := propMap["enum"].([]interface{}); ok {
			enumStrings := make([]string, 0, len(enumVal))
			for _, e := range enumVal {
				if str, ok := e.(string); ok {
					enumStrings = append(enumStrings, str)
				}
			}
			param.EnumValues = enumStrings
		}

		// For complex types, store the schema
		if param.Type == "object" || param.Type == "array" {
			param.Schema = propMap
		}

		params = append(params, param)
	}

	return params
}

// GetMCPToolName returns the original MCP tool name (without prefix)
func (a *MCPToolAdapter) GetMCPToolName() string {
	return a.toolName
}

// GetClientName returns the name of the MCP server providing this tool
func (a *MCPToolAdapter) GetClientName() string {
	return a.client.Name()
}