package kyoci

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
)

// ==============================================================================
// Role Interface and Types
// ==============================================================================

// RoleType represents the type of an agent role.
// Goroutine-safe: This is a simple integer type and safe for concurrent use.
type RoleType int

const (
	// RoleDeveloper represents a developer role focused on coding and technical tasks
	RoleDeveloper RoleType = iota
	// RoleSRE represents a site reliability engineer role focused on operations and reliability
	RoleSRE
	// RoleQA represents a quality assurance role focused on testing and validation
	RoleQA
	// RolePM represents a project manager role focused on planning and coordination
	RolePM
	// RoleFrontend represents a frontend developer role focused on UI/UX, HTML/CSS/JS/TS
	RoleFrontend
	// RoleCustom represents a custom role defined by the user
	RoleCustom
)

// String returns a string representation of the RoleType.
func (rt RoleType) String() string {
	switch rt {
	case RoleDeveloper:
		return "developer"
	case RoleSRE:
		return "sre"
	case RoleQA:
		return "qa"
	case RolePM:
		return "pm"
	case RoleFrontend:
		return "frontend"
	case RoleCustom:
		return "custom"
	default:
		return "unknown"
	}
}

// Role is the interface that all agent roles must implement.
// Roles define the behavior, capabilities, and constraints of an AI agent.
// Goroutine-safe: Implementations MUST be safe for concurrent use from multiple goroutines.
// The interface methods are called concurrently and must be properly synchronized.
type Role interface {
	// Type returns the type of this role.
	Type() RoleType

	// SystemPrompt returns the system prompt that defines the role's behavior.
	// This prompt sets the context and personality for the AI.
	SystemPrompt() string

	// Tools returns the list of tool names this role can use.
	// Only tools in this list will be available to the role.
	Tools() []string

	// PreferredProvider returns the name of the preferred LLM provider.
	// Returns empty string if no preference.
	PreferredProvider() string

	// MaxIterations returns the maximum number of reasoning/execution iterations.
	// The role will stop after this many iterations, even if not complete.
	MaxIterations() int

	// Execute executes a task through this role.
	// This is the main entry point for role-based task execution.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout control
	//   - task: The task description to execute
	//   - memory: The memory store for context and recall
	//
	// Returns:
	//   - *TaskResult: The result of task execution
	//   - error: Any error that occurred
	//
	// Errors:
	//   - context.Canceled if ctx was canceled
	//   - context.DeadlineExceeded if timeout exceeded
	//   - ErrMaxIterations if max iterations reached
	//   - ErrTaskFailed if task execution failed
	Execute(ctx context.Context, task string, memory MemoryStore) (*TaskResult, error)
}

// RoleConfig represents the configuration for a role.
// Goroutine-safe: RoleConfig values should be treated as immutable after creation.
type RoleConfig struct {
	// Type is the role type
	Type RoleType
	// SystemPrompt is the system prompt defining the role's behavior
	SystemPrompt string
	// Tools are the tool names this role can use
	Tools []string
	// PreferredProvider is the preferred LLM provider name
	PreferredProvider string
	// MaxIterations is the maximum number of iterations
	MaxIterations int
	// Temperature controls the randomness (0.0 = deterministic, 1.0 = random)
	Temperature float64
	// Model is the specific model to use (overrides provider default)
	Model string
}

// Validate checks if the role config is valid.
func (c RoleConfig) Validate() error {
	if c.SystemPrompt == "" {
		return NewValidationError("system_prompt", "system prompt cannot be empty", c.SystemPrompt)
	}
	if c.MaxIterations < 1 {
		return NewValidationError("max_iterations", "must be at least 1", c.MaxIterations)
	}
	if c.Temperature < 0 || c.Temperature > 2 {
		return NewValidationError("temperature", "must be between 0 and 2", c.Temperature)
	}
	return nil
}

// ==============================================================================
// Role Registry
// ==============================================================================

// RoleRegistry manages a collection of roles.
// Goroutine-safe: All methods are safe for concurrent use.
// Uses internal synchronization (RWMutex) for thread-safe operations.
type RoleRegistry struct {
	mu        sync.RWMutex
	roles     map[RoleType]Role
	providers map[RoleType]Provider
	logger    *slog.Logger
}

// NewRoleRegistry creates a new role registry.
func NewRoleRegistry() *RoleRegistry {
	return &RoleRegistry{
		roles:     make(map[RoleType]Role),
		providers: make(map[RoleType]Provider),
		logger:    slog.Default(),
	}
}

// Register adds a role to the registry.
// If a role of the same type already exists, it will be replaced.
//
// Parameters:
//   - role: The role to register
//
// Returns:
//   - error: nil on success, error if validation fails
func (r *RoleRegistry) Register(role Role) error {
	if role == nil {
		return NewValidationError("role", "role cannot be nil", nil)
	}

	roleType := role.Type()
	if roleType == RoleCustom && role.SystemPrompt() == "" {
		return NewValidationError("system_prompt", "system prompt required for custom roles", "")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.roles[roleType] = role
	r.logger.Info("role registered", "type", roleType.String())
	return nil
}

// Get retrieves a role by type.
//
// Parameters:
//   - roleType: The role type
//
// Returns:
//   - Role: The role if found
//   - error: ErrRoleNotFound if not found
func (r *RoleRegistry) Get(roleType RoleType) (Role, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	role, ok := r.roles[roleType]
	if !ok {
		return nil, ErrRoleNotFound
	}
	return role, nil
}

// List returns all registered role configurations.
//
// Returns:
//   - []RoleConfig: List of role configurations
func (r *RoleRegistry) List() []RoleConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	configs := make([]RoleConfig, 0, len(r.roles))
	for _, role := range r.roles {
		configs = append(configs, RoleConfig{
			Type:              role.Type(),
			SystemPrompt:      role.SystemPrompt(),
			Tools:             role.Tools(),
			PreferredProvider: role.PreferredProvider(),
			MaxIterations:     role.MaxIterations(),
		})
	}
	return configs
}

// AssignProvider assigns a provider to a specific role type.
// The assigned provider will be preferred when executing tasks for this role.
//
// Parameters:
//   - roleType: The role type
//   - provider: The provider to assign
//
// Returns:
//   - error: nil on success, error if validation fails
func (r *RoleRegistry) AssignProvider(roleType RoleType, provider Provider) error {
	if provider == nil {
		return NewValidationError("provider", "provider cannot be nil", nil)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[roleType] = provider
	r.logger.Info("provider assigned to role", "role", roleType.String(), "provider", provider.Name())
	return nil
}

// GetProvider retrieves the provider assigned to a role type.
//
// Parameters:
//   - roleType: The role type
//
// Returns:
//   - Provider: The assigned provider, or nil if none assigned
func (r *RoleRegistry) GetProvider(roleType RoleType) Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.providers[roleType]
}

// Has checks if a role is registered.
//
// Parameters:
//   - roleType: The role type
//
// Returns:
//   - bool: true if the role exists
func (r *RoleRegistry) Has(roleType RoleType) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.roles[roleType]
	return ok
}

// Count returns the number of registered roles.
//
// Returns:
//   - int: Number of roles
func (r *RoleRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.roles)
}

// ==============================================================================
// Base Role Implementation
// ==============================================================================

// BaseRole provides a default implementation of the Role interface.
// It can be embedded in custom role implementations to reduce boilerplate.
// Goroutine-safe: BaseRole values should be treated as immutable after creation.
type BaseRole struct {
	config           RoleConfig
	logger           *slog.Logger
	roleRegistry     *RoleRegistry
	providerRegistry *ProviderRegistry
	toolRegistry     *ToolRegistry
}

// NewBaseRole creates a new base role with the given configuration.
func NewBaseRole(config RoleConfig) *BaseRole {
	return &BaseRole{
		config: config,
		logger: slog.Default(),
	}
}

// Type returns the role type.
func (r *BaseRole) Type() RoleType {
	return r.config.Type
}

// SystemPrompt returns the system prompt.
func (r *BaseRole) SystemPrompt() string {
	return r.config.SystemPrompt
}

// Tools returns the list of available tool names.
func (r *BaseRole) Tools() []string {
	return r.config.Tools
}

// PreferredProvider returns the preferred provider name.
func (r *BaseRole) PreferredProvider() string {
	return r.config.PreferredProvider
}

// MaxIterations returns the maximum iterations.
func (r *BaseRole) MaxIterations() int {
	return r.config.MaxIterations
}

// SetProviderRegistry sets the provider registry for this role.
func (r *BaseRole) SetProviderRegistry(registry *ProviderRegistry) {
	r.providerRegistry = registry
}

// SetRoleRegistry sets the role registry for this role.
func (r *BaseRole) SetRoleRegistry(registry *RoleRegistry) {
	r.roleRegistry = registry
}

// SetToolRegistry sets the tool registry for this role.
func (r *BaseRole) SetToolRegistry(registry *ToolRegistry) {
	r.toolRegistry = registry
}

// Execute provides a default implementation of role-based task execution.
// This can be overridden by custom role implementations for specialized behavior.
//
// The default implementation:
// 1. Retrieves conversation context from memory
// 2. Gets the assigned or preferred provider
// 3. Makes a completion request with available tools
// 4. Executes tool calls if any
// 5. Iterates until complete or max iterations reached
// 6. Stores the result in memory
func (r *BaseRole) Execute(ctx context.Context, task string, memory MemoryStore) (*TaskResult, error) {
	if err := r.config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid role config: %w", err)
	}

	r.logger.Info("executing task", "role", r.Type().String(), "task", task)

	// Get provider
	var provider Provider
	if r.roleRegistry != nil {
		if assigned := r.roleRegistry.GetProvider(r.config.Type); assigned != nil {
			provider = assigned
		} else if r.providerRegistry != nil && r.config.PreferredProvider != "" {
			if p, err := r.providerRegistry.Get(r.config.PreferredProvider); err == nil {
				provider = p
			}
		}
	}

	if provider == nil {
		return nil, fmt.Errorf("no provider available for role %s", r.Type().String())
	}

	// Retrieve context from memory
	relevantMemory, err := memory.Recall(ctx, task, 10, MemoryShortTerm)
	if err != nil {
		r.logger.Warn("failed to recall memory", "error", err)
	}

	// Build messages
	messages := r.buildMessages(task, relevantMemory)

	// Execute with iteration loop
	var result TaskResult
	var totalUsage TokenUsage

	for iteration := 1; iteration <= r.config.MaxIterations; iteration++ {
		result.Iterations = iteration

		// Get available tools
		var tools []ToolDefinition
		if r.toolRegistry != nil {
			tools = r.getToolDefinitions()
		}

		// Build completion request
		req := CompletionRequest{
			Messages:      messages,
			Model:         r.config.Model,
			Temperature:   r.config.Temperature,
			MaxTokens:     0, // Use provider default
			Tools:         tools,
			Stream:        false,
			Metadata:      map[string]string{"role": r.Type().String()},
		}

		if req.Model == "" {
			req.Model = provider.Models()[0].ID
		}

		// Make completion request
		resp, err := provider.Complete(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("completion failed: %w", err)
		}

		// Update usage
		totalUsage.PromptTokens += resp.Usage.PromptTokens
		totalUsage.CompletionTokens += resp.Usage.CompletionTokens
		totalUsage.TotalTokens += resp.Usage.TotalTokens
		result.Usage = totalUsage

		// Check if complete
		if resp.FinishReason == FinishStop {
			result.Content = resp.Content
			result.ToolCallsMade = 0
			break
		}

		// Execute tool calls
		if len(resp.ToolCalls) > 0 {
			if r.toolRegistry == nil {
				return nil, errors.New("tool calls requested but no tool registry available")
			}

			toolResults := make([]Message, 0, len(resp.ToolCalls))
			for _, toolCall := range resp.ToolCalls {
				args, err := ParseToolCallArguments(toolCall.Arguments)
				if err != nil {
					toolResults = append(toolResults, FormatToolResult(
						toolCall.ID,
						"",
						fmt.Errorf("invalid arguments: %w", err),
					))
					continue
				}

				result.ToolCallsMade++
				toolResult, err := r.toolRegistry.Execute(ctx, toolCall.Name, args)
				toolResults = append(toolResults, FormatToolResult(
					toolCall.ID,
					toolResult,
					err,
				))
			}

			// Add assistant message with tool calls
			messages = append(messages, Message{
				Role:       RoleAssistant,
				Content:    resp.Content,
				ToolCalls:  resp.ToolCalls,
			})

			// Add tool result messages
			messages = append(messages, toolResults...)
		}

		if iteration == r.config.MaxIterations {
			result.Error = ErrMaxIterations
			result.Content = resp.Content
			break
		}
	}

	// Store result in memory
	if _, err := memory.Store(ctx, result.Content, MemoryShortTerm, map[string]string{
		"role":      r.Type().String(),
		"iterations": string(rune(result.Iterations)),
	}); err != nil {
		r.logger.Warn("failed to store result in memory", "error", err)
	}

	r.logger.Info("task executed", "iterations", result.Iterations, "tool_calls", result.ToolCallsMade)
	return &result, nil
}

// buildMessages constructs the message list for a completion request.
func (r *BaseRole) buildMessages(task string, memory []MemoryEntry) []Message {
	messages := []Message{
		{
			Role:    RoleSystem,
			Content: r.config.SystemPrompt,
		},
	}

	// Add relevant memory context
	for _, entry := range memory {
		messages = append(messages, Message{
			Role:    RoleUser,
			Content: fmt.Sprintf("[Context from memory]: %s", entry.Content),
		})
	}

	// Add current task
	messages = append(messages, Message{
		Role:    RoleUser,
		Content: task,
	})

	return messages
}

// getToolDefinitions retrieves tool definitions for this role's tools.
func (r *BaseRole) getToolDefinitions() []ToolDefinition {
	if r.toolRegistry == nil || len(r.config.Tools) == 0 {
		return nil
	}

	allTools := r.toolRegistry.List()
	toolsMap := make(map[string]ToolDefinition)
	for _, tool := range allTools {
		toolsMap[tool.Name] = tool
	}

	definitions := make([]ToolDefinition, 0, len(r.config.Tools))
	for _, toolName := range r.config.Tools {
		if tool, ok := toolsMap[toolName]; ok {
			definitions = append(definitions, tool)
		}
	}

	return definitions
}

// ErrRoleNotFound indicates that a role was not found in the registry.
var ErrRoleNotFound = NewValidationError("role", "role not found in registry", nil)