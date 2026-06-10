package delegation

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/nicholas/ai-agent/agent"
	"github.com/nicholas/ai-agent/classifier"
	"github.com/nicholas/ai-agent/pool"
)

// CavemanLevel represents the optimization level (1-5)
// Lower values = more token optimization (less detailed prompts)
// Higher values = full capabilities (more detailed prompts)
type CavemanLevel int

const (
	CavemanFull CavemanLevel = 1 // Full system prompt + tool guide + all context
	CavemanRich CavemanLevel = 2 // System prompt + essential context only
	CavemanCompact CavemanLevel = 3 // Compacted system + telegraphic context
	CavemanMinimal CavemanLevel = 4 // No system prompt, just goal + minimal params
	CavemanBare CavemanLevel = 5 // Single line goal only, no context
)

// Task represents a delegation task
type Task struct {
	Goal    string        // The main goal/objective
	Context string        // Additional context or parameters
	Level   CavemanLevel  // Optimization level (1-5)
	Timeout time.Duration // Timeout for this task
}

// Result represents the result of a delegated task
type Result struct {
	TaskID    string
	Goal      string
	Output    string
	Error     error
	Duration  time.Duration
	Level     CavemanLevel
}

// Delegator manages parallel task delegation with Caveman token optimization
type Delegator struct {
	agent       *agent.Agent
	pool        *pool.BufferPool
	active      sync.Map // taskID -> context.CancelFunc
	maxParallel int
	sem         chan struct{}
	taskCounter int64
	mu          sync.Mutex
	nextID      int
}

// NewDelegator creates a new delegator with parallel task limit
func NewDelegator(ag *agent.Agent, maxParallel int) *Delegator {
	if maxParallel <= 0 {
		maxParallel = 1
	}
	return &Delegator{
		agent:       ag,
		pool:        pool.NewBufferPool(),
		maxParallel: maxParallel,
		sem:         make(chan struct{}, maxParallel),
		nextID:      1,
	}
}

// Delegate executes a single task with Caveman token optimization
func (d *Delegator) Delegate(ctx context.Context, task Task) (string, error) {
	if task.Level == 0 {
		task.Level = d.GetLevel(task.Goal)
	}

	if task.Timeout == 0 {
		task.Timeout = 5 * time.Minute
	}

	taskCtx, cancel := context.WithTimeout(ctx, task.Timeout)
	defer cancel()

	// Acquire semaphore slot
	select {
	case d.sem <- struct{}{}:
		defer func() { <-d.sem }()
	case <-taskCtx.Done():
		return "", taskCtx.Err()
	}

	// Track active task
	d.mu.Lock()
	taskID := fmt.Sprintf("task-%d", d.nextID)
	d.nextID++
	d.mu.Unlock()

	d.active.Store(taskID, cancel)

	// Prepare prompt with Caveman optimization
	prompt := d.preparePrompt(task)

	start := time.Now()

	// Execute via agent
	output, err := d.agent.Run(taskCtx, prompt)

	d.active.Delete(taskID)

	duration := time.Since(start)
	if d.agent != nil {
		// Update stats if available
	}

	if err != nil {
		return "", fmt.Errorf("task %s failed after %s: %w", taskID, duration, err)
	}

	return output, nil
}

// DelegateBatch executes multiple tasks in parallel
func (d *Delegator) DelegateBatch(ctx context.Context, tasks []Task) []Result {
	results := make([]Result, len(tasks))
	var wg sync.WaitGroup

	for i := range tasks {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			task := tasks[idx]
			if task.Level == 0 {
				task.Level = d.GetLevel(task.Goal)
			}

			d.mu.Lock()
			taskID := fmt.Sprintf("batch-%d-%d", time.Now().Unix(), idx)
			d.mu.Unlock()

			start := time.Now()
			output, err := d.Delegate(ctx, task)
			duration := time.Since(start)

			results[idx] = Result{
				TaskID:   taskID,
				Goal:     task.Goal,
				Output:   output,
				Error:    err,
				Duration: duration,
				Level:    task.Level,
			}
		}(i)
	}

	wg.Wait()
	return results
}

// GetLevel determines the Caveman level based on prompt classification
func (d *Delegator) GetLevel(prompt string) CavemanLevel {
	if prompt == "" {
		return CavemanBare
	}

	// Use classifier to determine level
	result := classifier.Classify(prompt)

	// Map classifier levels to Caveman levels
	// Note: Caveman 1 = full details, Caveman 5 = minimal
	// Classifier: 1=Trivial, 5=Critical
	// Invert the mapping for Caveman: trivial tasks need less detail (higher Caveman level)
	switch result.Level {
	case classifier.LevelTrivial:
		return CavemanBare      // Level 5: single line only
	case classifier.LevelSimple:
		return CavemanMinimal   // Level 4: goal + minimal params
	case classifier.LevelModerate:
		return CavemanCompact   // Level 3: compacted system + telegraphic
	case classifier.LevelComplex:
		return CavemanRich      // Level 2: system + essential context
	case classifier.LevelCritical:
		return CavemanFull      // Level 1: full system prompt + all context
	default:
		return CavemanCompact
	}
}

// preparePrompt creates the prompt with Caveman token optimization
func (d *Delegator) preparePrompt(task Task) string {
	var buf *strings.Builder

	// Use pooled buffer for large prompts
	if len(task.Goal)+len(task.Context) > 4096 {
		// For large prompts, we could use the pool, but for simplicity
		// we'll use strings.Builder which is still efficient
		buf = &strings.Builder{}
		buf.Grow(8192)
	} else {
		buf = &strings.Builder{}
	}

	switch task.Level {
	case CavemanFull:
		// Level 1: Full system prompt + tool guide + all context
		if task.Context != "" {
			buf.WriteString(task.Context)
			buf.WriteString("\n\n")
		}
		buf.WriteString("Goal: ")
		buf.WriteString(task.Goal)

	case CavemanRich:
		// Level 2: System prompt + essential context only
		// Extract essential keywords from context
		if task.Context != "" {
			essential := extractEssential(task.Context)
			buf.WriteString("Context: ")
			buf.WriteString(essential)
			buf.WriteString("\n\n")
		}
		buf.WriteString("Goal: ")
		buf.WriteString(task.Goal)

	case CavemanCompact:
		// Level 3: Compacted system + telegraphic context
		if task.Context != "" {
			telegraphic := makeTelegraphic(task.Context)
			buf.WriteString(telegraphic)
			buf.WriteString("\n")
		}
		buf.WriteString("Do: ")
		buf.WriteString(task.Goal)

	case CavemanMinimal:
		// Level 4: No system prompt, just goal + minimal params
		buf.WriteString(task.Goal)
		if task.Context != "" {
			params := extractParams(task.Context)
			if params != "" {
				buf.WriteString(". Params: ")
				buf.WriteString(params)
			}
		}

	case CavemanBare:
		// Level 5: Single line goal only, no context
		buf.WriteString(task.Goal)

	default:
		buf.WriteString(task.Goal)
	}

	return buf.String()
}

// ActiveCount returns the number of currently active tasks
func (d *Delegator) ActiveCount() int {
	count := 0
	d.active.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	return count
}

// CancelAll cancels all active tasks
func (d *Delegator) CancelAll() {
	d.active.Range(func(key, value interface{}) bool {
		if cancel, ok := value.(context.CancelFunc); ok {
			cancel()
		}
		return true
	})
}

// Helper: extract essential keywords from context
func extractEssential(context string) string {
	if len(context) <= 200 {
		return context
	}
	// Simple extraction: first and last sentences
	words := strings.Fields(context)
	if len(words) <= 40 {
		return context
	}
	// Take first 15 words and last 15 words
	first := strings.Join(words[:15], " ")
	last := strings.Join(words[len(words)-15:], " ")
	return first + " ... " + last
}

// Helper: make telegraphic context (drop articles, short forms)
func makeTelegraphic(context string) string {
	words := strings.Fields(context)
	var tele []string

	// Skip common stop words
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true,
		"but": true, "for": true, "with": true, "on": true, "at": true,
		"to": true, "of": true, "in": true, "is": true, "are": true,
	}

	for _, word := range words {
		if !stopWords[strings.ToLower(word)] {
			tele = append(tele, word)
		}
	}

	if len(tele) > 30 {
		tele = tele[:30]
	}
	return strings.Join(tele, " ")
}

// Helper: extract parameters from context (key: value pairs)
func extractParams(context string) string {
	words := strings.Fields(context)
	var params []string

	for i, word := range words {
		if strings.HasSuffix(word, ":") || strings.HasSuffix(word, "=") {
			if i+1 < len(words) {
				params = append(params, word+" "+words[i+1])
			}
		}
	}

	if len(params) == 0 {
		if len(words) > 0 && len(words) <= 5 {
			return strings.Join(words, " ")
		}
		return ""
	}

	return strings.Join(params, ", ")
}