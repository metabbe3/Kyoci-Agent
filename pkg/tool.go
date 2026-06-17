package kyoci

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
)

// ==============================================================================
// Tool Interface and Types
// ==============================================================================

// Tool is the interface that all tools must implement.
// Tools represent callable functions that the AI can invoke to perform actions.
// Goroutine-safe: Implementations MUST be safe for concurrent use from multiple goroutines.
// The interface methods are called concurrently and must be properly synchronized.
type Tool interface {
	// Name returns the unique name of this tool.
	// This name is used to identify the tool when calling Execute.
	// Must be a valid identifier (no spaces, special characters).
	Name() string

	// Description returns a human-readable description of what this tool does.
	// This is shown to the AI to help it understand when and how to use the tool.
	Description() string

	// Parameters returns the parameter definition for this tool.
	// Defines what parameters the tool accepts, their types, and requirements.
	Parameters() []ToolParameter

	// Execute executes the tool with the given parameters.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout control
	//   - params: Map of parameter names to values (validated against Parameters())
	//
	// Returns:
	//   - string: The result of executing the tool
	//   - error: Any error that occurred during execution
	//
	// Errors:
	//   - context.Canceled if ctx was canceled
	//   - context.DeadlineExceeded if timeout exceeded
	//   - ErrToolExecution if the tool execution failed
	//   - Any other implementation-specific error
	Execute(ctx context.Context, params map[string]interface{}) (string, error)
}

// ToolParameter represents a single parameter for a tool.
// Goroutine-safe: ToolParameter values should be treated as immutable after creation.
type ToolParameter struct {
	// Name is the parameter name (must match key in params map)
	Name string
	// Type is the parameter type (e.g., "string", "integer", "boolean", "array", "object")
	Type string
	// Description is a human-readable description of this parameter
	Description string
	// Required indicates whether this parameter must be provided
	Required bool
	// EnumValues are the allowed values (if Type is enum)
	EnumValues []string
	// Default is the default value if not provided (optional)
	Default interface{}
	// Schema is the JSON schema for complex types (objects/arrays)
	Schema map[string]interface{}
}

// ToolDefinition represents a tool's complete definition for schema generation.
// Goroutine-safe: ToolDefinition values should be treated as immutable after creation.
type ToolDefinition struct {
	// Name is the unique tool name
	Name string
	// Description describes what the tool does
	Description string
	// Parameters are the tool's parameters
	Parameters []ToolParameter
	// Metadata contains additional tool metadata
	Metadata map[string]string
}

// ToJSONSchema converts the tool definition to a JSON Schema format.
// This is useful for sending to LLM APIs that expect JSON Schema.
//
// Returns:
//   - map[string]interface{}: JSON Schema representation
func (d ToolDefinition) ToJSONSchema() map[string]interface{} {
	properties := make(map[string]interface{})
	required := make([]string, 0)

	for _, param := range d.Parameters {
		prop := map[string]interface{}{
			"type":        param.Type,
			"description": param.Description,
		}

		if param.Default != nil {
			prop["default"] = param.Default
		}

		if len(param.EnumValues) > 0 {
			prop["enum"] = param.EnumValues
		}

		if param.Schema != nil {
			prop = mergeMaps(prop, param.Schema)
		}

		properties[param.Name] = prop

		if param.Required {
			required = append(required, param.Name)
		}
	}

	schema := map[string]interface{}{
		"type":       "object",
		"properties": properties,
	}

	if len(required) > 0 {
		schema["required"] = required
	}

	return schema
}

// ToOpenAISchema converts the tool definition to OpenAI function-calling format.
//
// Returns:
//   - map[string]interface{}: OpenAI function format
func (d ToolDefinition) ToOpenAISchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "function",
		"function": map[string]interface{}{
			"name":        d.Name,
			"description": d.Description,
			"parameters":  d.ToJSONSchema(),
		},
	}
}

// ValidateParams validates parameters against this tool's definition.
//
// Parameters:
//   - params: Map of parameter names to values
//
// Returns:
//   - error: ValidationError if validation fails, nil otherwise
func (d ToolDefinition) ValidateParams(params map[string]interface{}) error {
	for _, param := range d.Parameters {
		value, exists := params[param.Name]

		if param.Required && !exists {
			return NewValidationError(
				param.Name,
				"required parameter not provided",
				nil,
			)
		}

		if exists {
			if err := validateType(param.Name, value, param.Type); err != nil {
				return err
			}

			if len(param.EnumValues) > 0 {
				if strValue, ok := value.(string); ok {
					valid := false
					for _, enum := range param.EnumValues {
						if strValue == enum {
							valid = true
							break
						}
					}
					if !valid {
						return NewValidationError(
							param.Name,
							fmt.Sprintf("value must be one of %v", param.EnumValues),
							value,
						)
					}
				}
			}
		}
	}
	return nil
}

// ==============================================================================
// Tool Registry
// ==============================================================================

// ToolRegistry manages a collection of tools.
// Goroutine-safe: All methods are safe for concurrent use.
// Uses internal synchronization (RWMutex) for thread-safe operations.
type ToolRegistry struct {
	mu     sync.RWMutex
	tools  map[string]Tool
	logger *slog.Logger
}

// NewToolRegistry creates a new tool registry.
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools:  make(map[string]Tool),
		logger: slog.Default(),
	}
}

// Register adds a tool to the registry.
// If a tool with the same name already exists, it will be replaced.
//
// Parameters:
//   - tool: The tool to register
//
// Returns:
//   - error: nil on success, error if validation fails
func (r *ToolRegistry) Register(tool Tool) error {
	if tool == nil {
		return NewValidationError("tool", "tool cannot be nil", nil)
	}

	name := tool.Name()
	if name == "" {
		return NewValidationError("name", "tool name cannot be empty", name)
	}

	// Validate tool definition
	for _, param := range tool.Parameters() {
		if param.Name == "" {
			return NewValidationError("parameter", "parameter name cannot be empty", nil)
		}
		if param.Type == "" {
			return NewValidationError("parameter", "parameter type cannot be empty", param.Name)
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[name] = tool
	r.logger.Info("tool registered", "name", name)
	return nil
}

// Get retrieves a tool by name.
//
// Parameters:
//   - name: The tool name
//
// Returns:
//   - Tool: The tool if found
//   - error: ErrToolNotFound if not found
func (r *ToolRegistry) Get(name string) (Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tool, ok := r.tools[name]
	if !ok {
		return nil, ErrToolNotFound
	}
	return tool, nil
}

// List returns all registered tool definitions.
//
// Returns:
//   - []ToolDefinition: List of tool definitions
func (r *ToolRegistry) List() []ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	definitions := make([]ToolDefinition, 0, len(r.tools))
	for _, tool := range r.tools {
		definitions = append(definitions, ToolDefinition{
			Name:        tool.Name(),
			Description: tool.Description(),
			Parameters:  tool.Parameters(),
		})
	}
	return definitions
}

// Execute executes a tool by name with the given parameters.
// This method validates the parameters and calls the tool's Execute method.
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - name: The tool name to execute
//   - params: Map of parameter names to values
//
// Returns:
//   - string: The result of executing the tool
//   - error: Any error that occurred
func (r *ToolRegistry) Execute(ctx context.Context, name string, params map[string]interface{}) (string, error) {
	tool, err := r.Get(name)
	if err != nil {
		return "", err
	}

	definition := ToolDefinition{
		Name:        tool.Name(),
		Description: tool.Description(),
		Parameters:  tool.Parameters(),
	}

	if err := definition.ValidateParams(params); err != nil {
		return "", fmt.Errorf("parameter validation failed: %w", err)
	}

	r.logger.Info("executing tool", "name", name, "params", params)
	result, err := tool.Execute(ctx, params)
	if err != nil {
		r.logger.Error("tool execution failed", "name", name, "error", err)
		return "", fmt.Errorf("%w: %v", ErrToolExecution, err)
	}

	r.logger.Info("tool executed successfully", "name", name)
	return result, nil
}

// Remove removes a tool from the registry.
//
// Parameters:
//   - name: The tool name to remove
//
// Returns:
//   - error: nil on success, error if not found
func (r *ToolRegistry) Remove(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.tools[name]; !ok {
		return ErrToolNotFound
	}
	delete(r.tools, name)
	r.logger.Info("tool removed", "name", name)
	return nil
}

// Has checks if a tool is registered.
//
// Parameters:
//   - name: The tool name
//
// Returns:
//   - bool: true if the tool exists
func (r *ToolRegistry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.tools[name]
	return ok
}

// Count returns the number of registered tools.
//
// Returns:
//   - int: Number of tools
func (r *ToolRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.tools)
}

// ==============================================================================
// Utility Functions
// ==============================================================================

// validateType validates that a value matches the expected type.
func validateType(name string, value interface{}, expectedType string) error {
	if value == nil {
		return NewValidationError(name, "value cannot be nil", value)
	}

	switch expectedType {
	case "string":
		if _, ok := value.(string); !ok {
			return NewValidationError(name, "expected string", value)
		}
	case "integer", "int":
		if _, ok := value.(int); !ok {
			if _, ok := value.(float64); ok {
				// JSON numbers come as float64, check if it's actually an integer
				return nil
			}
			return NewValidationError(name, "expected integer", value)
		}
	case "number", "float":
		if _, ok := value.(float64); !ok {
			return NewValidationError(name, "expected number", value)
		}
	case "boolean", "bool":
		if _, ok := value.(bool); !ok {
			return NewValidationError(name, "expected boolean", value)
		}
	case "array":
		if _, ok := value.([]interface{}); !ok {
			return NewValidationError(name, "expected array", value)
		}
	case "object":
		if _, ok := value.(map[string]interface{}); !ok {
			return NewValidationError(name, "expected object", value)
		}
	default:
		// Unknown type, accept any value
	}

	return nil
}

// mergeMaps merges two maps, with b taking precedence over a.
func mergeMaps(a, b map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range a {
		result[k] = v
	}
	for k, v := range b {
		result[k] = v
	}
	return result
}

// ParseToolCallArguments parses JSON-encoded tool call arguments.
//
// Parameters:
//   - argsJSON: JSON string containing arguments
//
// Returns:
//   - map[string]interface{}: Parsed arguments
//   - error: JSON parse error if invalid
func ParseToolCallArguments(argsJSON string) (map[string]interface{}, error) {
	if argsJSON == "" {
		return make(map[string]interface{}), nil
	}

	var args map[string]interface{}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return nil, fmt.Errorf("failed to parse tool arguments: %w", err)
	}

	return args, nil
}

// FormatToolResult formats a tool result for inclusion in a message.
//
// Parameters:
//   - toolCallID: The tool call ID this result is for
//   - result: The tool execution result
//   - err: Any error that occurred
//
// Returns:
//   - Message: A tool message with the result
func FormatToolResult(toolCallID, result string, err error) Message {
	content := result
	if err != nil {
		content = fmt.Sprintf("Error: %v", err)
	}

	return Message{
		Role:       RoleTool,
		Content:    content,
		ToolCallID: toolCallID,
	}
}