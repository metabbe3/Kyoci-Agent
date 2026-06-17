package memory

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// =============================================================================
// LLM Summarizer — LLM-backed implementation of SummarizerFunc
//
// Asks an LLM to extract durable preferences, facts, and lessons from a batch
// of short-term entries. When the LLM decides nothing is worth remembering, it
// returns the sentinel "NOTHING_TO_REMEMBER" and the summarizer returns
// ("", nil) so the compactor skips long-term storage.
// =============================================================================

// LLMClient is the minimal interface the summarizer needs from an LLM
// provider. Defined locally to avoid importing internal/llm from the memory
// package (layering: memory depends on pkg, not internal/llm).
type LLMClient interface {
	Complete(ctx context.Context, req kyoci.CompletionRequest) (*kyoci.CompletionResponse, error)
}

// LLMSummarizerConfig configures the LLM-backed summarizer.
type LLMSummarizerConfig struct {
	// Model is the model ID to use for summarization (e.g., "gemma4:12b").
	// If empty, the provider's default model is used.
	Model string
	// MaxEntries limits how many entries are included in the prompt.
	// 0 means no limit (use with caution for very large batches).
	MaxEntries int
	// MaxTokens is the max response length. 0 = provider default.
	MaxTokens int
}

// nothingToRememberSentinel is the exact string the LLM must return when it
// determines nothing in the conversation is worth persisting to long-term memory.
const nothingToRememberSentinel = "NOTHING_TO_REMEMBER"

const summarizerSystemPrompt = `You are a memory compaction assistant. Your job is to read a batch of conversation messages and extract durable, reusable knowledge that will help in future tasks.

Extract:
- **Preferences**: coding style, language choices, tool preferences, formatting rules (e.g., "prefers Go with net/http", "uses snake_case JSON tags")
- **Permanent facts**: things that won't change soon (e.g., "project uses SQLite", "deploys to AWS")
- **Lessons**: insights from mistakes or successes (e.g., "macOS sed does not support \s in regex")

Write a 3-6 sentence prose summary. Be specific and actionable — avoid generic statements.

If nothing in these messages is worth remembering long-term (e.g., it is all trivial back-and-forth), output exactly: NOTHING_TO_REMEMBER
Do not output anything else when there is nothing to remember.`

// NewLLMSummarizer creates a SummarizerFunc backed by an LLM client.
// The returned function extracts preferences/facts/lessons from conversation
// entries and returns prose suitable for long-term storage. When the LLM
// decides nothing is memorable, it returns ("", nil).
func NewLLMSummarizer(cli LLMClient, cfg LLMSummarizerConfig, logger *slog.Logger) SummarizerFunc {
	return func(ctx context.Context, entries []*kyoci.MemoryEntry) (string, error) {
		if len(entries) == 0 {
			return "", nil
		}

		// Limit entries if configured.
		if cfg.MaxEntries > 0 && len(entries) > cfg.MaxEntries {
			entries = entries[:cfg.MaxEntries]
		}

		// Build the user message from conversation entries.
		var userMsg strings.Builder
		userMsg.WriteString("Conversation messages to summarize:\n\n")
		for i, entry := range entries {
			userMsg.WriteString(fmt.Sprintf("[msg %d] %s\n", i+1, entry.Content))
		}

		req := kyoci.CompletionRequest{
			Messages: []kyoci.Message{
				{Role: kyoci.RoleSystem, Content: summarizerSystemPrompt},
				{Role: kyoci.RoleUser, Content: userMsg.String()},
			},
			Model:       cfg.Model,
			MaxTokens:   cfg.MaxTokens,
			Temperature: 0.3, // low temperature for factual extraction
			ToolChoice:  "none",
		}

		resp, err := cli.Complete(ctx, req)
		if err != nil {
			logger.Warn("llm summarizer request failed", "error", err, "entries", len(entries))
			return "", fmt.Errorf("llm summarizer: %w", err)
		}

		content := strings.TrimSpace(resp.Content)
		if content == "" || content == nothingToRememberSentinel {
			logger.Info("llm summarizer: nothing worth remembering", "entries", len(entries))
			return "", nil
		}

		logger.Info("llm summarizer produced summary", "entries", len(entries), "summary_len", len(content))
		return content, nil
	}
}
