package main

import (
	"fmt"
	"log"

	"github.com/nicholas/ai-agent/mcp"
)

func main() {
	// Create a new MCP registry
	registry := mcp.NewMCPRegistry()

	// Example: Add an MCP server (note: this requires an actual MCP server)
	// config := mcp.MCPServerConfig{
	// 	Name:    "filesystem",
	// 	Command: "npx",
	// 	Args:    []string{"-y", "@modelcontextprotocol/server-filesystem", "/tmp"},
	// }
	//
	// ctx := context.Background()
	// if err := registry.AddServer(ctx, config); err != nil {
	// 	log.Fatalf("Failed to add server: %v", err)
	// }

	// Get registry statistics
	stats := registry.Stats()
	fmt.Printf("Registry Stats:\n")
	fmt.Printf("  Servers: %d\n", stats.Servers)
	fmt.Printf("  Total Tools: %d\n", stats.TotalTools)
	fmt.Printf("  Alive: %d\n", stats.Alive)

	// Get all available tools
	tools := registry.AllTools()
	fmt.Printf("\nAvailable Tools (%d):\n", len(tools))
	for i, tool := range tools {
		fmt.Printf("  %d. %s: %s\n", i+1, tool.Name, tool.Description)
	}

	// Example: Call a tool
	// result, err := registry.CallTool(ctx, "read_file", map[string]interface{}{
	// 	"path": "/tmp/test.txt",
	// })
	// if err != nil {
	// 	log.Fatalf("Failed to call tool: %v", err)
	// }
	//
	// fmt.Printf("Tool Result: %+v\n", result)

	// Graceful shutdown
	fmt.Println("\nShutting down...")
	registry.CloseAll()
	log.Println("Registry closed successfully")
}