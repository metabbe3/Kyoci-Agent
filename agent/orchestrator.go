package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/nicholas/ai-agent/llm"
	"github.com/nicholas/ai-agent/memory"
)

// routerWrapper wraps a Router to implement the Provider interface
type routerWrapper struct {
	router *llm.Router
}

// Chat implements llm.Provider
func (w *routerWrapper) Chat(ctx context.Context, messages []llm.Message, tools []llm.ToolSchema) (*llm.Response, error) {
	return w.router.Chat(ctx, messages, tools)
}

// Stream implements llm.Provider
func (w *routerWrapper) Stream(ctx context.Context, messages []llm.Message, tools []llm.ToolSchema) (<-chan llm.Chunk, error) {
	return w.router.Stream(ctx, messages, tools)
}

// Name implements llm.Provider
func (w *routerWrapper) Name() string {
	return "router-wrapper"
}

// Orchestrator is the master orchestrator for sub-agent delegation
type Orchestrator struct {
	master      *Agent
	subAgents   map[string]*SubAgent
	maxParallel int    // max concurrent sub-agents (default: 3)
	maxDepth    int    // max nesting (default: 1)
	tokenBudget int    // total budget for all sub-agents
	mu          sync.RWMutex

	// Semaphores for controlling concurrency
	semaphore chan struct{}
}

// OrchestratorOption configures the orchestrator
type OrchestratorOption func(*Orchestrator)

// WithMaxParallel sets the maximum number of concurrent sub-agents
func WithMaxParallel(n int) OrchestratorOption {
	return func(o *Orchestrator) {
		o.maxParallel = n
	}
}

// WithMaxDepth sets the maximum nesting depth for sub-agents
func WithMaxDepth(n int) OrchestratorOption {
	return func(o *Orchestrator) {
		o.maxDepth = n
	}
}

// WithTokenBudget sets the total token budget for all sub-agents
func WithTokenBudget(n int) OrchestratorOption {
	return func(o *Orchestrator) {
		o.tokenBudget = n
	}
}

// NewOrchestrator creates a new orchestrator with the given master agent
func NewOrchestrator(master *Agent, opts ...OrchestratorOption) *Orchestrator {
	o := &Orchestrator{
		master:      master,
		subAgents:   make(map[string]*SubAgent),
		maxParallel: 3,  // default: 3 concurrent sub-agents
		maxDepth:    1,  // default: 1 level of nesting
		tokenBudget: 10000, // default: 10k tokens total
		semaphore:   make(chan struct{}, 3),
	}

	for _, opt := range opts {
		opt(o)
	}

	// Resize semaphore based on maxParallel
	o.semaphore = make(chan struct{}, o.maxParallel)

	return o
}

// ShouldDelegate determines if a task should be delegated to sub-agents
func (o *Orchestrator) ShouldDelegate(task string, level int) bool {
	if level >= o.maxDepth {
		return false
	}

	// Check for multi-step keywords
	multiStepKeywords := []string{
		"multiple", "several", "various", "different", "parallel",
		"at the same time", "simultaneously", "concurrently",
		"each of", "all of", "for each", "for all",
		"research and", "find and", "analyze and", "process and",
	}

	taskLower := strings.ToLower(task)
	for _, keyword := range multiStepKeywords {
		if strings.Contains(taskLower, keyword) {
			return true
		}
	}

	// Check for complex task patterns
	complexPatterns := []string{
		"?", // Multiple questions
		", ", // Comma-separated items
		"; ", // Semicolon-separated items
		" and ", // Multiple tasks joined by "and"
	}

	for _, pattern := range complexPatterns {
		if strings.Count(taskLower, pattern) > 1 {
			return true
		}
	}

	return false
}

// Spawn creates a new sub-agent with a fair share of token budget
func (o *Orchestrator) Spawn(ctx context.Context, goal string) (*SubAgent, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	// Check concurrency limit
	select {
	case o.semaphore <- struct{}{}:
		// Acquired semaphore
	default:
		return nil, fmt.Errorf("maximum concurrent sub-agents (%d) reached", o.maxParallel)
	}

	// Generate unique ID
	id := generateSubAgentID()

	// Calculate fair share of token budget (we already have the write lock)
	activeCount := len(o.subAgents)

	fairShare := o.tokenBudget / (activeCount + 1)
	if fairShare < 1000 {
		fairShare = 1000 // Minimum budget per sub-agent
	}

	// Create isolated memory for sub-agent
	mem := memory.NewConversationBuffer(fairShare)

	// Get tools from master's registry (using the master's tool registry)
	toolList := o.master.tools.List()

	// Create sub-agent with the router (Router implements Provider via Chat method)
	// We need to wrap the router as it doesn't have Name() method
	sa := NewSubAgent(id, goal, &routerWrapper{o.master.router}, mem, toolList, fairShare)

	// Store sub-agent
	o.subAgents[id] = sa

	// Start the sub-agent
	if err := sa.Start(ctx); err != nil {
		delete(o.subAgents, id)
		<-o.semaphore // Release semaphore
		return nil, err
	}

	return sa, nil
}

// SpawnBatch creates multiple sub-agents in parallel
func (o *Orchestrator) SpawnBatch(ctx context.Context, goals []string) ([]*SubAgent, error) {
	if len(goals) > o.maxParallel {
		return nil, fmt.Errorf("cannot spawn %d sub-agents, max parallel is %d", len(goals), o.maxParallel)
	}

	sa := make([]*SubAgent, 0, len(goals))
	errs := make([]error, len(goals))

	// Spawn all sub-agents
	for i, goal := range goals {
		subAgent, err := o.Spawn(ctx, goal)
		if err != nil {
			errs[i] = err
			continue
		}
		sa = append(sa, subAgent)
	}

	if len(sa) == 0 {
		return nil, fmt.Errorf("failed to spawn any sub-agents: %v", errs)
	}

	return sa, nil
}

// CollectResults gathers results from all completed sub-agents
func (o *Orchestrator) CollectResults() []SubAgentResult {
	o.mu.RLock()
	defer o.mu.RUnlock()

	results := make([]SubAgentResult, 0, len(o.subAgents))
	for _, sa := range o.subAgents {
		results = append(results, *sa.GetResult())
	}
	return results
}

// CollectResultsFiltered collects results from sub-agents matching the status filter
func (o *Orchestrator) CollectResultsFiltered(status string) []SubAgentResult {
	o.mu.RLock()
	defer o.mu.RUnlock()

	results := make([]SubAgentResult, 0)
	for _, sa := range o.subAgents {
		if sa.GetStatus() == status {
			results = append(results, *sa.GetResult())
		}
	}
	return results
}

// WaitForAll waits for all running sub-agents to complete
func (o *Orchestrator) WaitForAll(ctx context.Context) ([]SubAgentResult, error) {
	o.mu.RLock()
	running := make([]*SubAgent, 0)
	for _, sa := range o.subAgents {
		if sa.GetStatus() == "running" {
			running = append(running, sa)
		}
	}
	o.mu.RUnlock()

	if len(running) == 0 {
		return o.CollectResults(), nil
	}

	// Wait for all with context
	done := make(chan *SubAgentResult, len(running))
	errCh := make(chan error, len(running))

	for _, sa := range running {
		go func(s *SubAgent) {
			result, err := s.Wait()
			if err != nil {
				errCh <- err
				return
			}
			done <- result
		}(sa)
	}

	// Collect results
	results := make([]SubAgentResult, 0, len(running))
	for i := 0; i < len(running); i++ {
		select {
		case result := <-done:
			results = append(results, *result)
		case err := <-errCh:
			return nil, err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	// Add any completed results
	for _, sa := range o.subAgents {
		if sa.GetStatus() != "running" {
			results = append(results, *sa.GetResult())
		}
	}

	return results, nil
}

// Synthesize formats all results into a coherent summary
func (o *Orchestrator) Synthesize(results []SubAgentResult) string {
	if len(results) == 0 {
		return "No results to synthesize."
	}

	var sb strings.Builder

	sb.WriteString("## Sub-Agent Results Summary\n\n")

	// Individual results
	for i, result := range results {
		sb.WriteString(fmt.Sprintf("### Sub-Agent %d: %s\n", i+1, result.ID))
		sb.WriteString(fmt.Sprintf("**Goal:** %s\n", result.Goal))
		sb.WriteString(fmt.Sprintf("**Status:** %s\n", result.Status))
		sb.WriteString(fmt.Sprintf("**Tokens Used:** %d\n", result.TokensUsed))
		sb.WriteString(fmt.Sprintf("**Duration:** %v\n", result.Duration))

		if result.Error != "" {
			sb.WriteString(fmt.Sprintf("**Error:** %s\n", result.Error))
		}

		sb.WriteString("**Result:**\n")
		sb.WriteString(result.Result)
		sb.WriteString("\n\n")
	}

	// Synthesis section
	sb.WriteString("---\n\n")
	sb.WriteString("## Synthesis\n\n")

	// Count successful/failed
	successCount := 0
	failedCount := 0
	for _, r := range results {
		if r.Status == "done" {
			successCount++
		} else {
			failedCount++
		}
	}

	sb.WriteString(fmt.Sprintf("Completed %d sub-agents (%d successful, %d failed).\n\n",
		len(results), successCount, failedCount))

	// Create summary of all results
	if successCount > 0 {
		sb.WriteString("### Key Findings:\n")
		for i, result := range results {
			if result.Status == "done" {
				sb.WriteString(fmt.Sprintf("- **Sub-Agent %d:** %s\n", i+1, summarizeResult(result.Result)))
			}
		}
		sb.WriteString("\n")
	}

	if failedCount > 0 {
		sb.WriteString("### Issues Encountered:\n")
		for i, result := range results {
			if result.Status != "done" {
				sb.WriteString(fmt.Sprintf("- **Sub-Agent %d:** %s\n", i+1, result.Error))
			}
		}
		sb.WriteString("\n")
	}

	// Total tokens
	totalTokens := 0
	for _, r := range results {
		totalTokens += r.TokensUsed
	}
	sb.WriteString(fmt.Sprintf("Total tokens used across all sub-agents: %d\n", totalTokens))

	return sb.String()
}

// GetSubAgent retrieves a sub-agent by ID
func (o *Orchestrator) GetSubAgent(id string) (*SubAgent, bool) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	sa, ok := o.subAgents[id]
	return sa, ok
}

// GetActiveCount returns the number of currently running sub-agents
func (o *Orchestrator) GetActiveCount() int {
	o.mu.RLock()
	defer o.mu.RUnlock()
	count := 0
	for _, sa := range o.subAgents {
		if sa.GetStatus() == "running" {
			count++
		}
	}
	return count
}

// GetAllSubAgents returns all sub-agents
func (o *Orchestrator) GetAllSubAgents() []*SubAgent {
	o.mu.RLock()
	defer o.mu.RUnlock()
	result := make([]*SubAgent, 0, len(o.subAgents))
	for _, sa := range o.subAgents {
		result = append(result, sa)
	}
	return result
}

// Shutdown cancels all running sub-agents and cleans up
func (o *Orchestrator) Shutdown() {
	o.mu.Lock()
	defer o.mu.Unlock()

	for _, sa := range o.subAgents {
		if sa.GetStatus() == "running" {
			sa.Cancel()
		}
	}

	// Wait a bit for graceful shutdown
	time.Sleep(100 * time.Millisecond)

	// Clear sub-agents
	o.subAgents = make(map[string]*SubAgent)
}

// Cleanup removes completed sub-agents from tracking
func (o *Orchestrator) Cleanup() {
	o.mu.Lock()
	defer o.mu.Unlock()

	for id, sa := range o.subAgents {
		if sa.IsDone() {
			delete(o.subAgents, id)
			<-o.semaphore // Release semaphore
		}
	}
}

// GetStats returns statistics about the orchestrator
func (o *Orchestrator) GetStats() map[string]interface{} {
	o.mu.RLock()
	defer o.mu.RUnlock()

	stats := map[string]interface{}{
		"total_sub_agents": len(o.subAgents),
		"active_count":     o.GetActiveCount(),
		"max_parallel":     o.maxParallel,
		"max_depth":        o.maxDepth,
		"token_budget":     o.tokenBudget,
	}

	// Count by status
	statusCounts := make(map[string]int)
	for _, sa := range o.subAgents {
		statusCounts[sa.GetStatus()]++
	}
	stats["status_counts"] = statusCounts

	return stats
}

// generateSubAgentID generates a unique sub-agent ID
func generateSubAgentID() string {
	return fmt.Sprintf("subagent-%d", time.Now().UnixNano())
}

// summarizeResult creates a brief summary of a result
func summarizeResult(result string) string {
	// Take first 100 characters
	if len(result) <= 100 {
		return result
	}
	return result[:100] + "..."
}