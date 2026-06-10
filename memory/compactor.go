package memory

import (
	"fmt"
	"sync"
	"time"
)

// ConversationCompactor implements AutoCompactor using LLM summarization
type ConversationCompactor struct {
	buffer          *ConversationBuffer
	threshold       float64
	summarizeFunc   SummarizeFunc
	stats           CompactionStats
	mu              sync.RWMutex
	maxMessagesToSummarize int
}

// NewConversationCompactor creates a new auto-compactor
func NewConversationCompactor(buffer *ConversationBuffer, threshold float64) *ConversationCompactor {
	return &ConversationCompactor{
		buffer:                buffer,
		threshold:             threshold,
		summarizeFunc:         nil,
		maxMessagesToSummarize: 10,
	}
}

// SetThreshold sets the token usage threshold (0.0-1.0) for triggering compaction
func (cc *ConversationCompactor) SetThreshold(threshold float64) {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	if threshold < 0.0 {
		threshold = 0.0
	}
	if threshold > 1.0 {
		threshold = 1.0
	}

	cc.threshold = threshold
}

// GetThreshold returns the current threshold
func (cc *ConversationCompactor) GetThreshold() float64 {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	return cc.threshold
}

// SetSummarizeFunc sets the function used to summarize messages
func (cc *ConversationCompactor) SetSummarizeFunc(fn SummarizeFunc) {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	cc.summarizeFunc = fn
}

// ShouldCompact determines if compaction is needed
func (cc *ConversationCompactor) ShouldCompact() bool {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	if cc.summarizeFunc == nil {
		return false
	}

	// Check if token usage exceeds threshold
	tokenUsage := cc.buffer.GetTokenUsage()
	return tokenUsage > cc.threshold
}

// Compact performs the compaction
func (cc *ConversationCompactor) Compact() error {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	if cc.summarizeFunc == nil {
		return &CompactionError{
			Op:  "compact",
			Err: fmt.Errorf("summarize function not configured"),
		}
	}

	// Get current messages
	messages := cc.buffer.GetMessages()

	// Check if we have enough messages to compact
	if len(messages) < 2 {
		return nil
	}

	// Determine how many messages to summarize
	messagesToSummarize := cc.maxMessagesToSummarize
	if messagesToSummarize > len(messages) {
		messagesToSummarize = len(messages)
	}

	// Take the oldest messages
	oldMessages := messages[:messagesToSummarize]

	// Calculate tokens before compaction
	tokensBefore := cc.buffer.TokenCount()

	// Call the summarize function
	summary, err := cc.summarizeFunc(oldMessages)
	if err != nil {
		return &CompactionError{
			Op:  "summarize",
			Err: err,
		}
	}

	// Replace old messages with summary
	newMessages := messages[messagesToSummarize:]

	// Add summary as a system message
	cc.buffer.Clear()
	cc.buffer.AddSystemMessage(summary)

	// Re-add remaining messages
	for _, msg := range newMessages {
		cc.buffer.Add(msg.Role, msg.Content)
	}

	// Calculate tokens after compaction
	tokensAfter := cc.buffer.TokenCount()

	// Update statistics
	cc.stats.TotalCompactions++
	cc.stats.MessagesSummarized += messagesToSummarize
	cc.stats.LastCompaction = time.Now()
	cc.stats.TokensSaved += tokensBefore - tokensAfter

	return nil
}

// GetStats returns compaction statistics
func (cc *ConversationCompactor) GetStats() CompactionStats {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	return CompactionStats{
		TotalCompactions:   cc.stats.TotalCompactions,
		MessagesSummarized: cc.stats.MessagesSummarized,
		LastCompaction:     cc.stats.LastCompaction,
		TokensSaved:        cc.stats.TokensSaved,
	}
}

// ResetStats resets compaction statistics
func (cc *ConversationCompactor) ResetStats() {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	cc.stats = CompactionStats{
		TotalCompactions:   0,
		MessagesSummarized: 0,
		TokensSaved:        0,
		LastCompaction:     time.Time{},
	}
}

// SetMaxMessagesToSummarize sets the maximum number of messages to summarize in one compaction
func (cc *ConversationCompactor) SetMaxMessagesToSummarize(max int) {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	if max < 1 {
		max = 1
	}

	cc.maxMessagesToSummarize = max
}

// GetMaxMessagesToSummarize returns the maximum number of messages to summarize
func (cc *ConversationCompactor) GetMaxMessagesToSummarize() int {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	return cc.maxMessagesToSummarize
}

// GetCompactionEfficiency returns the average tokens saved per compaction
func (cc *ConversationCompactor) GetCompactionEfficiency() float64 {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	if cc.stats.TotalCompactions == 0 {
		return 0.0
	}

	return float64(cc.stats.TokensSaved) / float64(cc.stats.TotalCompactions)
}

// EstimateCompactionBenefit estimates how many tokens would be saved by compacting now
func (cc *ConversationCompactor) EstimateCompactionBenefit() (int, error) {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	if cc.summarizeFunc == nil {
		return 0, &CompactionError{
			Op:  "estimate",
			Err: fmt.Errorf("summarize function not configured"),
		}
	}

	messages := cc.buffer.GetMessages()

	if len(messages) < 2 {
		return 0, nil
	}

	messagesToSummarize := cc.maxMessagesToSummarize
	if messagesToSummarize > len(messages) {
		messagesToSummarize = len(messages)
	}

	// Estimate tokens in old messages
	oldTokens := 0
	for i := 0; i < messagesToSummarize; i++ {
		oldTokens += cc.buffer.EstimateMessageSize(messages[i].Role, messages[i].Content)
	}

	// Estimate summary would be about 30% of original size
	estimatedSummaryTokens := int(float64(oldTokens) * 0.3)

	return oldTokens - estimatedSummaryTokens, nil
}