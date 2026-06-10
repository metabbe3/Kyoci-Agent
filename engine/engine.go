package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/nicholas/ai-agent/agent"
	"github.com/nicholas/ai-agent/classifier"
	"github.com/nicholas/ai-agent/gateway"
	"github.com/nicholas/ai-agent/pool"
	"github.com/nicholas/ai-agent/skill"
)

// Engine is the unified processing layer that coordinates tasks through the pipeline.
type Engine struct {
	skillReg       *skill.Registry
	agent          *agent.Agent
	workerPool     *pool.WorkerPool
	circuitBreaker *gateway.CircuitBreaker
	dagExec        *gateway.DAGExecutor
}

// NewEngine creates and initializes an Engine with all required dependencies.
func NewEngine(
	skillReg *skill.Registry,
	agent *agent.Agent,
	workerPool *pool.WorkerPool,
	circuitBreaker *gateway.CircuitBreaker,
	dagExec *gateway.DAGExecutor,
) *Engine {
	return &Engine{
		skillReg:       skillReg,
		agent:          agent,
		workerPool:     workerPool,
		circuitBreaker: circuitBreaker,
		dagExec:        dagExec,
	}
}

// Process executes the full pipeline for an EngineTask and returns the TaskResult.
// Pipeline steps:
// 1. Try zero-AI skill → if matched, return immediately (Tier 0)
// 2. Classify complexity → Level 1-5
// 3. Set context timeout based on tier
// 4. Route to appropriate tier via worker pool
// 5. Return result
func (e *Engine) Process(ctx context.Context, task *EngineTask) *TaskResult {
	start := time.Now()
	result := NewTaskResult(task.ID)

	// Step 1: Try zero-AI skills
	if e.skillReg != nil {
		if resp, matched, err := e.skillReg.Execute(ctx, task.Message); matched {
			result.Message = resp
			result.Tier = 0
			result.Duration = time.Since(start)
			if err != nil {
				result.WithError(err.Error())
			}
			return result
		}
	}

	// Step 2: Classify complexity
	classification := classifier.Classify(task.Message)

	// Use task-specified token budget if provided, otherwise use classification
	tokenBudget := task.MaxTokens
	if tokenBudget == 0 {
		tokenBudget = classification.GetTokenBudget()
	}
	task.MaxTokens = tokenBudget

	// Determine preferred model
	model := task.PreferredModel
	if model == "" {
		model = classification.RecommendedModel()
	}

	// Step 3: Set context timeout based on tier
	timeout := task.Timeout
	if timeout == 0 {
		switch classification.Level {
		case classifier.LevelTrivial, classifier.LevelSimple:
			timeout = 300 * time.Second // 5 min for complex AI
		case classifier.LevelModerate:
			timeout = 2 * time.Minute
		case classifier.LevelComplex:
			timeout = 5 * time.Minute
		case classifier.LevelCritical:
			timeout = 10 * time.Minute
		default:
			timeout = 2 * time.Minute
		}
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Step 4: Route to appropriate tier via worker pool
	var tier int
	var err error

	if classification.NeedsAI && e.agent != nil {
		if e.workerPool != nil {
			// Use worker pool for AI processing
			work := func() (string, error) {
				return e.agent.Run(ctx, task.Message)
			}

			resp, workErr := e.workerPool.Submit(ctx, work)
			if s, ok := resp.(string); ok {
				result.Message = s
			}
			err = workErr

			// Determine tier based on classification level
			switch classification.Level {
			case classifier.LevelModerate:
				tier = 1 // cheap AI
			case classifier.LevelComplex, classifier.LevelCritical:
				tier = 2 // complex AI
			default:
				tier = 1
			}
		} else {
			// Fallback: direct agent execution without worker pool
			result.Message, err = e.agent.Run(ctx, task.Message)
			tier = 1
		}
	} else {
		// No AI needed or no agent available
		if classification.NeedsAI && e.agent == nil {
			result.Message = "AI processing required but no agent configured"
			result.WithError("agent not configured")
		} else {
			result.Message = fmt.Sprintf("No skill matched: %s", task.Message)
		}
		tier = 0
	}

	// Circuit breaker state is managed internally by Execute()
	// No manual RecordFailure/RecordSuccess needed

	if err != nil {
		result.WithError(err.Error())
	}

	result.ModelUsed = model
	result.Tier = tier
	result.Duration = time.Since(start)

	// Step 5: Return result
	return result
}