# MCP (Model Context Protocol) Package for Kyoci Agent v5

This package provides a complete MCP client implementation with an adapter to expose MCP server tools as native Kyoci tools.

## Overview

The MCP package consists of:

- **Protocol types** (`protocol.go`): JSON-RPC 2.0 types and MCP protocol constants
- **Transport** (`transport.go`): Stdio-based transport for communicating with MCP servers
- **Client** (`client.go`): MCP client for tool, prompt, and resource operations
- **Registry** (`registry.go`): Manages multiple MCP servers and routes tool calls
- **Prompts** (`prompts.go`): Prompt template types
- **Resources** (`resources.go`): Resource access types
- **Adapter** (`adapter.go`): Converts MCP tools to Kyoci Tool interface

## Usage

### Basic Setup with MCPManager

The `MCPManager` is the high-level interface for integrating MCP servers into Kyoci Agent.

```go
import (
    "context"
    "log/slog"
    "github.com/metabbe3/Kyoci-Agent/internal/mcp"
)

func main() {
    // Create an MCP manager
    manager := mcp.NewMCPManager("mcp")

    // Add MCP servers
    ctx := context.Background()

    // Example: Add a filesystem server
    config := mcp.MCPServerConfig{
        Name:    "filesystem",
        Command: "npx",
        Args:    []string{"-y", "@modelcontextprotocol/server-filesystem", "/tmp"},
        Env:     nil,
    }

    if err := manager.AddServer(ctx, config); err != nil {
        slog.Error("failed to add MCP server", "error", err)
        return
    }

    // Get all tools as Kyoci Tool interface
    tools := manager.GetTools()
    slog.Info("MCP tools loaded", "count", len(tools))

    // Register tools with Kyoci's tool registry
    toolRegistry := kyoci.NewToolRegistry()
    for _, tool := range tools {
        if err := toolRegistry.Register(tool); err != nil {
            slog.Error("failed to register tool", "tool", tool.Name(), "error", err)
        }
    }

    // Tools can now be executed by Kyoci Agent
    // Example: Execute a tool
    result, err := toolRegistry.Execute(ctx, "mcp_read_file", map[string]interface{}{
        "path": "/tmp/test.txt",
    })
    if err != nil {
        slog.Error("tool execution failed", "error", err)
        return
    }

    slog.Info("tool result", "result", result)

    // Cleanup
    manager.Close()
}
```

### Direct MCP Client Usage

For more control, you can use the MCP client directly:

```go
import (
    "context"
    "log/slog"
    "github.com/metabbe3/Kyoci-Agent/internal/mcp"
)

func main() {
    ctx := context.Background()

    // Create client configuration
    config := mcp.MCPServerConfig{
        Name:    "my-mcp-server",
        Command: "path/to/mcp-server",
        Args:    []string{"--option", "value"},
        Env: map[string]string{
            "API_KEY": "your-api-key",
        },
    }

    // Create client
    client, err := mcp.NewMCPClient(config)
    if err != nil {
        slog.Error("failed to create client", "error", err)
        return
    }
    defer client.Close()

    // Initialize
    if err := client.Initialize(ctx); err != nil {
        slog.Error("failed to initialize", "error", err)
        return
    }

    // List available tools
    tools, err := client.ListTools(ctx)
    if err != nil {
        slog.Error("failed to list tools", "error", err)
        return
    }

    slog.Info("available tools", "count", len(tools))
    for _, tool := range tools {
        slog.Info("tool", "name", tool.Name, "description", tool.Description)
    }

    // Call a tool
    result, err := client.CallTool(ctx, "tool_name", map[string]interface{}{
        "param1": "value1",
    })
    if err != nil {
        slog.Error("tool call failed", "error", err)
        return
    }

    // Process result
    for _, block := range result.Content {
        if block.Type == "text" {
            slog.Info("tool output", "text", block.Text)
        }
    }
}
```

### Using MCPRegistry

The registry manages multiple MCP servers:

```go
import (
    "context"
    "log/slog"
    "github.com/metabbe3/Kyoci-Agent/internal/mcp"
)

func main() {
    ctx := context.Background()

    // Create registry
    registry := mcp.NewMCPRegistry()

    // Add multiple servers
    servers := []mcp.MCPServerConfig{
        {
            Name:    "filesystem",
            Command: "npx",
            Args:    []string{"-y", "@modelcontextprotocol/server-filesystem", "/tmp"},
        },
        {
            Name:    "brave-search",
            Command: "npx",
            Args:    []string{"-y", "@modelcontextprotocol/server-brave-search"},
        },
    }

    for _, server := range servers {
        registry.AddServerGracefully(ctx, server)
    }

    // Get all tools from all servers
    allTools := registry.AllTools()
    slog.Info("total tools from all servers", "count", len(allTools))

    // Call a tool (routes to correct server automatically)
    result, err := registry.CallTool(ctx, "read_file", map[string]interface{}{
        "path": "/tmp/file.txt",
    })
    if err != nil {
        slog.Error("tool call failed", "error", err)
        return
    }

    // Get statistics
    stats := registry.Stats()
    slog.Info("registry stats",
        "servers", stats.Servers,
        "tools", stats.TotalTools,
        "alive", stats.Alive,
    )

    // Cleanup
    registry.CloseAll()
}
```

## MCPToolAdapter Details

The `MCPToolAdapter` wraps MCP tools to implement the Kyoci Tool interface:

- **Name**: Returns the tool name, optionally prefixed with the manager name
- **Description**: Returns the tool's description
- **Parameters**: Parses the JSON Schema from the MCP tool into Kyoci ToolParameter format
- **Execute**: Calls the MCP server's tool and formats the result

### Parameter Parsing

The adapter converts JSON Schema to Kyoci ToolParameter format:

- `type`: Mapped to `ToolParameter.Type`
- `description`: Mapped to `ToolParameter.Description`
- `required`: Mapped to `ToolParameter.Required`
- `default`: Mapped to `ToolParameter.Default`
- `enum`: Mapped to `ToolParameter.EnumValues`
- Complex schemas (objects/arrays) are stored in `ToolParameter.Schema`

## Configuration

### MCPServerConfig

```go
type MCPServerConfig struct {
    Name    string            // Unique name for this server
    Command string            // Command to start the MCP server
    Args    []string          // Arguments for the command
    Env     map[string]string // Environment variables
    URL     string            // URL for SSE transport (not yet implemented)
}
```

## Protocol Support

This client supports MCP protocol version `2024-11-05`.

Supported features:
- ✅ Tool discovery and invocation
- ✅ Prompt templates
- ✅ Resource access
- ✅ Stdio transport
- ⏳ SSE transport (not yet implemented)

## Error Handling

The package uses Go's standard error interface:

- `ToolNotFoundError`: Returned when a tool is not found in the registry
- `JSONRPCError`: JSON-RPC protocol errors
- Standard Go errors for transport and parsing issues

## Logging

The package uses `log/slog` for logging:
- Info level: Successful operations, tool registration
- Warn level: Recoverable issues, version mismatches
- Error level: Failed operations, connection issues

## Thread Safety

- `MCPClient`: Thread-safe with internal mutex
- `MCPRegistry`: Thread-safe with RWMutex
- `MCPManager`: Thread-safe, delegates to registry
- `MCPToolAdapter`: Thread-safe, immutable after creation

## Examples

See the `_archive_v4/mcp/example/main.go` for usage examples (note: imports need updating for v5).

## Version Information

- Client version: `5.1.0`
- Protocol version: `2024-11-05`
- Package: `github.com/metabbe3/Kyoci-Agent/internal/mcp`