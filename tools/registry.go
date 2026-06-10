package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// Tool is the interface all tools must implement
type Tool interface {
	// Name returns the tool identifier
	Name() string

	// Description explains what the tool does (shown to LLM)
	Description() string

	// Parameters returns the JSON Schema for the tool's input
	Parameters() map[string]interface{}

	// Execute runs the tool with given parameters
	Execute(ctx context.Context, params json.RawMessage) (string, error)
}

// Registry holds all available tools
type Registry struct {
	tools map[string]Tool
}

// NewRegistry creates an empty tool registry
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register adds a tool to the registry
func (r *Registry) Register(tool Tool) error {
	if _, exists := r.tools[tool.Name()]; exists {
		return fmt.Errorf("tool already registered: %s", tool.Name())
	}
	r.tools[tool.Name()] = tool
	return nil
}

// Get retrieves a tool by name
func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// List returns all registered tools
func (r *Registry) List() []Tool {
	result := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		result = append(result, t)
	}
	return result
}

// Schemas returns tool schemas in LLM-compatible format
func (r *Registry) Schemas() []map[string]interface{} {
	schemas := make([]map[string]interface{}, 0, len(r.tools))
	for _, t := range r.tools {
		schemas = append(schemas, map[string]interface{}{
			"name":        t.Name(),
			"description": t.Description(),
			"parameters":  t.Parameters(),
		})
	}
	return schemas
}

// ExecuteTool runs a tool by name with JSON params
func (r *Registry) ExecuteTool(ctx context.Context, name string, rawArgs json.RawMessage) (string, error) {
	tool, ok := r.tools[name]
	if !ok {
		return "", fmt.Errorf("tool not found: %s", name)
	}
	return tool.Execute(ctx, rawArgs)
}

// MustRegister registers a tool, returns error on duplicate
func (r *Registry) MustRegister(tool Tool) error {
	if _, exists := r.tools[tool.Name()]; exists {
		return fmt.Errorf("tool already registered: %s", tool.Name())
	}
	return r.Register(tool)
}
