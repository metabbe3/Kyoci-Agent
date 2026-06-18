package builtin

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/metabbe3/Kyoci-Agent/pkg"
)

// ==============================================================================================
// Memory Recall Tool — Lets the LLM search past experiences, lessons, and facts
// ==============================================================================================

// MemoryRecallTool allows the agent to search its long-term memory for past
// experiences, lessons learned, and stored facts. This makes the agent proactive
// about recalling relevant information instead of only relying on auto-injection.
type MemoryRecallTool struct {
	store  kyoci.MemoryStore
	logger *slog.Logger
}

// NewMemoryRecallTool creates a new memory recall tool.
func NewMemoryRecallTool(store kyoci.MemoryStore) *MemoryRecallTool {
	return &MemoryRecallTool{
		store:  store,
		logger: slog.Default().With("component", "memory-recall-tool"),
	}
}

func (m *MemoryRecallTool) Name() string {
	return "memory_recall"
}

func (m *MemoryRecallTool) Description() string {
	return "Search your long-term memory for past experiences, lessons learned, and stored facts. Use this when the user asks about something you may have done before, or when you need to recall information from previous tasks. Returns matching memories with timestamps."
}

func (m *MemoryRecallTool) Parameters() []kyoci.ToolParameter {
	return []kyoci.ToolParameter{
		{
			Name:        "query",
			Type:        "string",
			Description: "What to search for in memory (e.g., 'portfolio website', 'user preferences', 'past errors')",
			Required:    true,
		},
		{
			Name:        "limit",
			Type:        "integer",
			Description: "Maximum number of memories to return (default: 5)",
			Required:    false,
			Default:     5,
		},
	}
}

func (m *MemoryRecallTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	if m.store == nil {
		return "Memory system not available.", nil
	}

	query, ok := params["query"].(string)
	if !ok || query == "" {
		return "", fmt.Errorf("query parameter is required")
	}

	limit := 5
	if l, ok := params["limit"]; ok {
		switch v := l.(type) {
		case int:
			limit = v
		case float64:
			limit = int(v)
		}
	}

	entries, err := m.store.Recall(ctx, query, limit, kyoci.MemoryLongTerm)
	if err != nil {
		return "", fmt.Errorf("memory search failed: %w", err)
	}

	if len(entries) == 0 {
		return fmt.Sprintf("No memories found for: %s", query), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d memory entry/entries:\n\n", len(entries)))

	for i, entry := range entries {
		cat := entry.Metadata["category"]
		if cat == "" {
			cat = "general"
		}

		content := entry.Content
		if len(content) > 300 {
			content = content[:300] + "..."
		}

		sb.WriteString(fmt.Sprintf("%d. [%s] %s\n", i+1, cat, content))
		if !entry.CreatedAt.IsZero() {
			sb.WriteString(fmt.Sprintf("   Time: %s\n", entry.CreatedAt.Format("2006-01-02 15:04")))
		}
		sb.WriteString("\n")
	}

	return sb.String(), nil
}

// ==============================================================================================
// Profile Set Tool — Lets the LLM store facts about the user
// ==============================================================================================

// ProfileEntryData represents a fact to store.
type ProfileEntryData struct {
	Key      string `json:"key"`
	Value    string `json:"value"`
	Category string `json:"category"`
}

// ProfileSetTool allows the agent to remember facts about the user permanently.
// This is how the agent learns — it stores information like the user's name,
// preferences, project details, etc. for future conversations.
type ProfileSetTool struct {
	store  kyoci.MemoryStore
	logger *slog.Logger
}

// NewProfileSetTool creates a new profile set tool.
func NewProfileSetTool(store kyoci.MemoryStore) *ProfileSetTool {
	return &ProfileSetTool{
		store:  store,
		logger: slog.Default().With("component", "profile-set-tool"),
	}
}

func (p *ProfileSetTool) Name() string {
	return "remember"
}

func (p *ProfileSetTool) Description() string {
	return "Permanently remember a fact about the user or environment. Use this when the user tells you their name, preferences, project details, or any information worth remembering for future conversations. Categories: 'fact', 'preference', 'environment', 'lesson'."
}

func (p *ProfileSetTool) Parameters() []kyoci.ToolParameter {
	return []kyoci.ToolParameter{
		{
			Name:        "key",
			Type:        "string",
			Description: "Short label for this fact (e.g., 'user_name', 'preferred_language', 'project_dir')",
			Required:    true,
		},
		{
			Name:        "value",
			Type:        "string",
			Description: "The fact to remember (e.g., 'Nicholas', 'Python', '/home/user/project')",
			Required:    true,
		},
		{
			Name:        "category",
			Type:        "string",
			Description: "Type of fact",
			Required:    false,
			Default:     "fact",
			EnumValues:  []string{"fact", "preference", "environment", "lesson"},
		},
	}
}

func (p *ProfileSetTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	if p.store == nil {
		return "Memory system not available.", nil
	}

	key, ok := params["key"].(string)
	if !ok || key == "" {
		return "", fmt.Errorf("key parameter is required")
	}

	value, ok := params["value"].(string)
	if !ok || value == "" {
		return "", fmt.Errorf("value parameter is required")
	}

	category := "fact"
	if c, ok := params["category"].(string); ok && c != "" {
		category = c
	}

	// Store as a profile entry in L3 memory
	data := fmt.Sprintf(`{"key":%q,"value":%q,"category":%q}`, key, value, category)

	metadata := map[string]string{
		"category": category,
		"key":      key,
	}

	_, err := p.store.Store(ctx, data, kyoci.MemoryLongTerm, metadata)
	if err != nil {
		return "", fmt.Errorf("failed to store profile entry: %w", err)
	}

	p.logger.Info("profile entry stored via tool", "key", key, "category", category)

	return fmt.Sprintf("Remembered: %s = %s (category: %s)", key, value, category), nil
}
