package kyoci

import (
	"errors"
	"time"
)

// ==============================================================================
// Core Types and Constants
// ==============================================================================

// MessageRole represents the role of a message in a conversation.
// Goroutine-safe: This is a simple integer type and safe for concurrent use.
type MessageRole int

const (
	// RoleSystem represents a system-level message (instructions, prompts, etc.)
	RoleSystem MessageRole = iota
	// RoleUser represents a user message
	RoleUser
	// RoleAssistant represents an assistant message (LLM response)
	RoleAssistant
	// RoleTool represents a tool result message
	RoleTool
)

// String returns a string representation of the MessageRole.
func (r MessageRole) String() string {
	switch r {
	case RoleSystem:
		return "system"
	case RoleUser:
		return "user"
	case RoleAssistant:
		return "assistant"
	case RoleTool:
		return "tool"
	default:
		return "unknown"
	}
}

// Message represents a single message in a conversation.
// Goroutine-safe: Message values should be treated as immutable after creation.
// If sharing across goroutines, make a copy or use proper synchronization.
type Message struct {
	// Role is the message role (system, user, assistant, tool)
	Role MessageRole
	// Content is the message content
	Content string
	// Name is an optional name for the message sender (e.g., for role tool)
	Name string
	// ToolCallID is the ID of the tool call this message is responding to (for tool role)
	ToolCallID string
	// ToolCalls are the tool calls made by the assistant
	ToolCalls []ToolCall
}

// ToolCall represents a single tool call invocation.
// Goroutine-safe: ToolCall values should be treated as immutable after creation.
type ToolCall struct {
	// ID is the unique identifier for this tool call
	ID string
	// Name is the name of the tool to call
	Name string
	// Arguments is the JSON-encoded arguments for the tool call
	Arguments string
}

// TokenUsage represents token usage statistics for a completion.
// Goroutine-safe: TokenUsage values should be treated as immutable after creation.
type TokenUsage struct {
	// PromptTokens is the number of tokens in the prompt
	PromptTokens int
	// CompletionTokens is the number of tokens in the completion
	CompletionTokens int
	// TotalTokens is the total number of tokens (prompt + completion)
	TotalTokens int
}

// TaskResult represents the result of executing a task through a role.
// Goroutine-safe: TaskResult values should be treated as immutable after creation.
type TaskResult struct {
	// Content is the final response content
	Content string
	// Role is the actual role type used (may differ from request if auto-detected)
	Role RoleType
	// ToolCallLog records each tool call made during execution
	ToolCallLog []ToolCallEntry
	// ToolCallsMade is the number of tool calls made during task execution
	ToolCallsMade int
	// Iterations is the number of reasoning/execution iterations
	Iterations int
	// Usage contains token usage statistics
	Usage TokenUsage
	// Error is any error that occurred during task execution
	Error error
}

// ToolCallEntry records a single tool call during task execution.
// Goroutine-safe: ToolCallEntry values should be treated as immutable after creation.
type ToolCallEntry struct {
	Tool       string `json:"tool"`
	Args       string `json:"args,omitempty"`       // JSON string of tool arguments
	Success    bool   `json:"success"`
	DurationMs int64  `json:"duration_ms"`
}

// ErrTaskFailed indicates that a task execution failed.
var ErrTaskFailed = errors.New("task execution failed")

// ErrMaxIterations indicates that the maximum number of iterations was reached.
var ErrMaxIterations = errors.New("maximum iterations exceeded")

// ErrToolExecution indicates that a tool execution failed.
var ErrToolExecution = errors.New("tool execution failed")

// ErrToolNotFound indicates that a requested tool was not found.
var ErrToolNotFound = errors.New("tool not found")

// ErrMemoryCompact indicates that memory compaction failed.
var ErrMemoryCompact = errors.New("memory compaction failed")

// FinishReason represents the reason why a completion finished.
// Goroutine-safe: String type, safe for concurrent use.
type FinishReason string

const (
	// FinishStop indicates the completion stopped normally
	FinishStop FinishReason = "stop"
	// FinishToolCall indicates the completion stopped to make tool calls
	FinishToolCall FinishReason = "tool_calls"
	// FinishMaxTokens indicates the completion stopped due to max tokens limit
	FinishMaxTokens FinishReason = "max_tokens"
	// FinishLength indicates the completion stopped due to token length
	FinishLength FinishReason = "length"
	// FinishContentFilter indicates the completion was filtered
	FinishContentFilter FinishReason = "content_filter"
	// FinishError indicates the completion stopped due to an error
	FinishError FinishReason = "error"
)

// StreamChunk represents a chunk of data from a streaming completion.
// Goroutine-safe: StreamChunk values should be treated as immutable after creation.
type StreamChunk struct {
	// Content is the partial content in this chunk
	Content string
	// ToolCall is the tool call being built (if any)
	ToolCall *ToolCall
	// Done indicates whether this is the final chunk
	Done bool
	// Usage contains token usage statistics (only populated on final chunk)
	Usage *TokenUsage
	// FinishReason is the reason the stream finished (only populated on final chunk)
	FinishReason FinishReason
	// Error is any error that occurred during streaming
	Error error
}

// StreamError creates a StreamChunk representing an error.
func StreamError(err error) StreamChunk {
	return StreamChunk{
		Error: err,
		Done:  true,
	}
}

// FinalChunk creates a StreamChunk representing the final chunk with usage info.
func FinalChunk(content string, usage TokenUsage, reason FinishReason) StreamChunk {
	return StreamChunk{
		Content:     content,
		Done:        true,
		Usage:       &usage,
		FinishReason: reason,
	}
}

// ContentChunk creates a StreamChunk with content only.
func ContentChunk(content string) StreamChunk {
	return StreamChunk{
		Content: content,
	}
}

// ToolCallChunk creates a StreamChunk with a tool call.
func ToolCallChunk(toolCall ToolCall) StreamChunk {
	return StreamChunk{
		ToolCall: &toolCall,
	}
}

// Timestamp represents a point in time with millisecond precision.
// Goroutine-safe: int64 type, safe for concurrent use.
type Timestamp int64

// Now returns the current timestamp.
func Now() Timestamp {
	return Timestamp(time.Now().UnixMilli())
}

// ToTime converts the timestamp to a time.Time.
func (t Timestamp) ToTime() time.Time {
	return time.UnixMilli(int64(t))
}

// FromTime creates a timestamp from a time.Time.
func FromTime(tm time.Time) Timestamp {
	return Timestamp(tm.UnixMilli())
}

// ==============================================================================
// Error Types
// ==============================================================================

// APIError represents an error returned from an API.
// Goroutine-safe: APIError values should be treated as immutable after creation.
type APIError struct {
	// Type is the error type
	Type string
	// Message is the error message
	Message string
	// Code is the error code (if applicable)
	Code string
	// StatusCode is the HTTP status code (if applicable)
	StatusCode int
	// Err is the underlying error
	Err error
}

// Error implements the error interface.
func (e *APIError) Error() string {
	if e.Code != "" {
		return e.Type + " [" + e.Code + "]: " + e.Message
	}
	return e.Type + ": " + e.Message
}

// Unwrap returns the underlying error.
func (e *APIError) Unwrap() error {
	return e.Err
}

// NewAPIError creates a new APIError.
func NewAPIError(errType, message, code string, statusCode int, err error) *APIError {
	return &APIError{
		Type:       errType,
		Message:    message,
		Code:       code,
		StatusCode: statusCode,
		Err:        err,
	}
}

// ConfigError represents a configuration error.
// Goroutine-safe: ConfigError values should be treated as immutable after creation.
type ConfigError struct {
	// Field is the configuration field that caused the error
	Field string
	// Message is the error message
	Message string
	// Err is the underlying error
	Err error
}

// Error implements the error interface.
func (e *ConfigError) Error() string {
	if e.Field != "" {
		return "config error in field '" + e.Field + "': " + e.Message
	}
	return "config error: " + e.Message
}

// Unwrap returns the underlying error.
func (e *ConfigError) Unwrap() error {
	return e.Err
}

// NewConfigError creates a new ConfigError.
func NewConfigError(field, message string, err error) *ConfigError {
	return &ConfigError{
		Field:   field,
		Message: message,
		Err:     err,
	}
}

// ValidationError represents a validation error.
// Goroutine-safe: ValidationError values should be treated as immutable after creation.
type ValidationError struct {
	// Field is the field that failed validation
	Field string
	// Message is the validation error message
	Message string
	// Value is the value that failed validation
	Value interface{}
}

// Error implements the error interface.
func (e *ValidationError) Error() string {
	if e.Field != "" {
		return "validation failed for field '" + e.Field + "': " + e.Message
	}
	return "validation failed: " + e.Message
}

// NewValidationError creates a new ValidationError.
func NewValidationError(field, message string, value interface{}) *ValidationError {
	return &ValidationError{
		Field:   field,
		Message: message,
		Value:   value,
	}
}