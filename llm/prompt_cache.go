package llm

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// CacheMarker marks content blocks as cacheable for providers that support it
type CacheMarker struct {
	Type       string // "ephemeral" (Anthropic) or "auto" (OpenAI)
	BlockIndex int    // which message block to mark
}

// ApplyAnthropicCaching applies cache_control markers to Anthropic API requests
// Anthropic supports prompt caching with "cache_control": {"type": "ephemeral"}
// The system prompt and large static blocks can be cached
func ApplyAnthropicCaching(system string, messages []Message) (string, []Message, []CacheMarker) {
	// For Anthropic, we'll mark the system prompt and first user message as cacheable
	// This is effective for code analysis where system + AST context is static

	var markers []CacheMarker

	// The system prompt is always cacheable for Anthropic
	if system != "" {
		// System prompt gets cache control marker
		markers = append(markers, CacheMarker{
			Type:       "ephemeral",
			BlockIndex: -1, // -1 denotes system prompt
		})
	}

	// Also mark the first large user message (usually contains code context)
	for i, msg := range messages {
		if msg.Role == "user" && len(msg.Content) > 1000 {
			markers = append(markers, CacheMarker{
				Type:       "ephemeral",
				BlockIndex: i,
			})
			// Only cache the first large user message
			break
		}
	}

	return system, messages, markers
}

// BuildStaticPrefix builds a cacheable prefix from system + AST context
// This prefix is stable across multiple requests within the same session
func BuildStaticPrefix(systemPrompt string, codeContext string) string {
	var builder strings.Builder

	if systemPrompt != "" {
		builder.WriteString(systemPrompt)
		builder.WriteString("\n\n")
	}

	if codeContext != "" {
		builder.WriteString("=== Code Context ===\n")
		builder.WriteString(codeContext)
		builder.WriteString("\n=== End Context ===\n\n")
	}

	return builder.String()
}

// SplitPrompt separates static (cacheable) from dynamic parts
// Returns the static prefix and the remaining dynamic messages
func SplitPrompt(system string, messages []Message) (static string, dynamic []Message) {
	// The static part includes the system prompt
	static = system

	// For messages, we need to identify what's static vs dynamic
	// A simple heuristic: first user message with large content is static
	for i, msg := range messages {
		if msg.Role == "user" && len(msg.Content) > 1000 {
			// Everything before and including this is static
			static += "\n\n" + msg.Content
			// Everything after is dynamic
			dynamic = messages[i+1:]
			return
		}
	}

	// If no large user message, everything is dynamic
	dynamic = messages
	return
}

// ComputeCacheKey creates a cache key for the static parts of a prompt
// This is used for provider-level caching (Anthropic prompt caching)
func ComputeCacheKey(system string, messages []Message) string {
	h := sha256.New()

	// Hash system prompt
	h.Write([]byte(system))
	h.Write([]byte("|||"))

	// Hash first few messages (typically the static context)
	limit := 3
	if len(messages) < limit {
		limit = len(messages)
	}

	for i := 0; i < limit; i++ {
		msg := messages[i]
		h.Write([]byte(msg.Role))
		h.Write([]byte(":"))
		h.Write([]byte(msg.Content))
		h.Write([]byte("|||"))
	}

	return hex.EncodeToString(h.Sum(nil))[:16]
}

// IsCacheableContext determines if a given system prompt/context should be cached
// Large system prompts (like AST context) benefit from caching
func IsCacheableContext(system string, threshold int) bool {
	return len(system) > threshold
}

// GetDefaultTTL returns the appropriate TTL based on context type
// AST context: 1 hour (stable until code changes)
// Conversation: 5 minutes (more dynamic)
func GetDefaultTTL(isASTContext bool) int {
	if isASTContext {
		return 3600 // 1 hour
	}
	return 300 // 5 minutes
}

// Anthropic-specific helpers for building cache control structures

// AnthropicCacheControl represents the cache control header for Anthropic API
type AnthropicCacheControl struct {
	Type string `json:"type"`
}

// WithCacheControl wraps content with cache control for Anthropic
// This is used when constructing the request body
func WithCacheControl(content string, cacheType string) map[string]interface{} {
	return map[string]interface{}{
		"type":  "text",
		"text":  content,
		"cache_control": map[string]string{
			"type": cacheType,
		},
	}
}

// OpenAI caching helpers

// OpenAI handles caching automatically for consistent prompts
// The key is to ensure stable ordering of messages
func NormalizeForOpenAICache(system string, messages []Message) (string, []Message) {
	// Ensure system prompt is first
	if system == "" {
		system = "You are a helpful assistant."
	}

	// Ensure message order is stable (role-based sorting not needed as order matters)
	// But we can ensure content normalization
	normalized := make([]Message, len(messages))
	for i, msg := range messages {
		normalized[i] = Message{
			Role:    msg.Role,
			Content: strings.TrimSpace(msg.Content),
		}
		if len(msg.ToolCalls) > 0 {
			normalized[i].ToolCalls = msg.ToolCalls
		}
		if msg.ToolCallID != "" {
			normalized[i].ToolCallID = msg.ToolCallID
		}
		if msg.Name != "" {
			normalized[i].Name = msg.Name
		}
	}

	return system, normalized
}

// BuildCachePrefixForOllama creates a prefix for Ollama's simple caching
func BuildCachePrefixForOllama(system string, codeContext string) string {
	prefix := BuildStaticPrefix(system, codeContext)
	return prefix
}