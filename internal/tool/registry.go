package tool

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/metabbe3/Kyoci-Agent/internal/tool/builtin"
	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// Registry wraps the kyoci.ToolRegistry with additional functionality.
// It provides thread-safe tool registration, retrieval, and execution.
type Registry struct {
	registry *kyoci.ToolRegistry
	mu       sync.RWMutex
	logger   *slog.Logger
}

// NewRegistry creates a new tool registry instance.
func NewRegistry() *Registry {
	return &Registry{
		registry: kyoci.NewToolRegistry(),
		logger:   slog.Default(),
	}
}

// Kyoci returns the underlying kyoci.ToolRegistry for use by the agent layer.
func (r *Registry) Kyoci() *kyoci.ToolRegistry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.registry
}

// Register adds a tool to the registry.
func (r *Registry) Register(tool kyoci.Tool) error {
	if tool == nil {
		return fmt.Errorf("tool cannot be nil")
	}
	name := tool.Name()
	if name == "" {
		return fmt.Errorf("tool name cannot be empty")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.registry.Has(name) {
		return fmt.Errorf("tool '%s' already registered", name)
	}
	r.logger.Info("registering tool", "name", name)
	return r.registry.Register(tool)
}

// Get retrieves a tool by name.
func (r *Registry) Get(name string) (kyoci.Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tool, err := r.registry.Get(name)
	if err != nil {
		r.logger.Warn("tool not found", "name", name)
		return nil, err
	}
	return tool, nil
}

// List returns all registered tool definitions.
func (r *Registry) List() []kyoci.ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.registry.List()
}

// Execute executes a tool by name with the given parameters.
func (r *Registry) Execute(ctx context.Context, name string, params map[string]interface{}) (string, error) {
	r.logger.Info("executing tool", "name", name)
	result, err := r.registry.Execute(ctx, name, params)
	if err != nil {
		r.logger.Error("tool execution failed", "name", name, "error", err)
		return "", err
	}
	r.logger.Info("tool executed successfully", "name", name)
	return result, nil
}

// RegisterBuiltin registers all built-in tools.
func (r *Registry) RegisterBuiltin() error {
	r.logger.Info("registering built-in tools")
	tools := []kyoci.Tool{
		builtin.NewTerminalTool(),
		builtin.NewFileTool(),
		builtin.NewHTTPTool(),
		builtin.NewSearchTool(),
		builtin.NewCalculatorTool(),
		builtin.NewBrowserTool(),
		builtin.NewDocsTool(),
		builtin.NewTodoTool(),
		builtin.NewSkillTool(),
		builtin.NewProcessTool(),
	}
	for _, t := range tools {
		if err := r.Register(t); err != nil {
			r.logger.Error("failed to register built-in tool", "name", t.Name(), "error", err)
			return fmt.Errorf("failed to register %s: %w", t.Name(), err)
		}
	}
	r.logger.Info("all built-in tools registered", "count", len(tools))
	return nil
}

// Has checks if a tool is registered.
func (r *Registry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.registry.Has(name)
}

// Count returns the number of registered tools.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.registry.Count()
}

// Remove removes a tool from the registry.
func (r *Registry) Remove(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.registry.Has(name) {
		return fmt.Errorf("tool '%s' not found", name)
	}
	return r.registry.Remove(name)
}
