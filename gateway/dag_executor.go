package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

// DAGTask represents a single task in a DAG plan
type DAGTask struct {
	Step          int             `json:"step"`
	ServiceTarget string          `json:"service_target"`
	RPCMethod     string          `json:"rpc_method"`
	Payload       json.RawMessage `json:"payload"`
	TierFallback  int             `json:"tier_fallback"`
	Dependencies  []int           `json:"dependencies"` // step IDs this depends on
}

// DAGPlan represents a plan of tasks to execute
type DAGPlan struct {
	PlanID        string    `json:"plan_id"`
	ExecutionMode string    `json:"execution_mode"` // PARALLEL or SEQUENTIAL
	Tasks         []DAGTask `json:"tasks"`
	CreatedAt     time.Time `json:"created_at,omitempty"`
}

// TaskResult represents the result of executing a task
type TaskResult struct {
	Step     int         `json:"step"`
	Success  bool        `json:"success"`
	Error    error       `json:"error,omitempty"`
	Result   interface{} `json:"result,omitempty"`
	Duration time.Duration `json:"duration,omitempty"`
}

func (tr TaskResult) MarshalJSON() ([]byte, error) {
	type Alias TaskResult
	aux := &struct {
		ErrorMsg string `json:"error,omitempty"`
		*Alias
	}{
		ErrorMsg: "",
		Alias:    (*Alias)(&tr),
	}
	if tr.Error != nil {
		aux.ErrorMsg = tr.Error.Error()
	}
	return json.Marshal(aux)
}

// WorkerPool is a simple worker pool for concurrent execution
type WorkerPool struct {
	workers   chan struct{}
	taskQueue chan func()
	wg        sync.WaitGroup
}

// NewWorkerPool creates a new worker pool with maxParallel workers
func NewWorkerPool(maxParallel int) *WorkerPool {
	if maxParallel <= 0 {
		maxParallel = 1
	}

	pool := &WorkerPool{
		workers:   make(chan struct{}, maxParallel),
		taskQueue: make(chan func(), maxParallel*2),
	}

	// Fill worker semaphore
	for i := 0; i < maxParallel; i++ {
		pool.workers <- struct{}{}
	}

	// Start dispatcher
	pool.wg.Add(1)
	go pool.dispatcher()

	return pool
}

// dispatcher processes tasks from the queue
func (p *WorkerPool) dispatcher() {
	defer p.wg.Done()
	for task := range p.taskQueue {
		<-p.workers // Acquire worker slot
		p.wg.Add(1)
		go func(fn func()) {
			defer func() {
				p.workers <- struct{}{} // Release worker slot
				p.wg.Done()
			}()
			fn()
		}(task)
	}
}

// Submit submits a task to the worker pool
func (p *WorkerPool) Submit(fn func()) {
	p.taskQueue <- fn
}

// Shutdown waits for all tasks to complete
func (p *WorkerPool) Shutdown() {
	close(p.taskQueue)
	p.wg.Wait()
}

// DAGExecutor executes tasks as DAG or parallel list
type DAGExecutor struct {
	maxParallel int
	timeout     time.Duration
	pool        *WorkerPool
	wal         *WAL
}

// NewDAGExecutor creates a new DAG executor
func NewDAGExecutor(maxParallel int, timeout time.Duration, wal *WAL) *DAGExecutor {
	return &DAGExecutor{
		maxParallel: maxParallel,
		timeout:     timeout,
		pool:        NewWorkerPool(maxParallel),
		wal:         wal,
	}
}

// Execute executes the given DAG plan
func (d *DAGExecutor) Execute(ctx context.Context, plan DAGPlan) []TaskResult {
	if len(plan.Tasks) == 0 {
		return []TaskResult{}
	}

	switch plan.ExecutionMode {
	case "PARALLEL":
		return d.executeParallel(ctx, plan)
	case "SEQUENTIAL", "DAG":
		return d.executeSequential(ctx, plan)
	default:
		// Default to sequential for safety
		return d.executeSequential(ctx, plan)
	}
}

// ExecuteWithDAGID executes the given DAG plan with a specific DAG ID
func (d *DAGExecutor) ExecuteWithDAGID(ctx context.Context, plan DAGPlan, dagID string) []TaskResult {
	if len(plan.Tasks) == 0 {
		return []TaskResult{}
	}

	switch plan.ExecutionMode {
	case "PARALLEL":
		return d.executeParallelWithWAL(ctx, plan, dagID)
	case "SEQUENTIAL", "DAG":
		return d.executeSequentialWithWAL(ctx, plan, dagID)
	default:
		// Default to sequential for safety
		return d.executeSequentialWithWAL(ctx, plan, dagID)
	}
}

// executeParallel executes independent tasks concurrently
func (d *DAGExecutor) executeParallel(ctx context.Context, plan DAGPlan) []TaskResult {
	// For backward compatibility, generate a DAG ID from PlanID
	dagID := plan.PlanID
	if dagID == "" {
		dagID = fmt.Sprintf("dag-%d", time.Now().UnixNano())
	}
	return d.executeParallelWithWAL(ctx, plan, dagID)
}

// executeParallelWithWAL executes independent tasks concurrently with WAL support
func (d *DAGExecutor) executeParallelWithWAL(ctx context.Context, plan DAGPlan, dagID string) []TaskResult {
	// Find max step to properly size results array
	maxStep := 0
	for _, task := range plan.Tasks {
		if task.Step > maxStep {
			maxStep = task.Step
		}
	}
	results := make([]TaskResult, maxStep+1)
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Group tasks by dependency level
	levels := d.buildDependencyLevels(plan.Tasks)

	// Check WAL for completed steps
	completedSteps := d.loadCompletedSteps(dagID)

	for _, levelTasks := range levels {
		for _, step := range levelTasks {
			task := plan.GetTask(step)
			if task == nil {
				continue
			}

			// Skip if already completed
			if result, ok := completedSteps[fmt.Sprintf("%d", task.Step)]; ok {
				mu.Lock()
				results[task.Step] = TaskResult{
					Step:    task.Step,
					Success: true,
					Result:  result,
				}
				mu.Unlock()
				continue
			}

			stepID := fmt.Sprintf("%d", task.Step)

			wg.Add(1)
			d.pool.Submit(func() {
				defer wg.Done()

				// Checkpoint before execution
				if d.wal != nil {
					_ = d.wal.Checkpoint(dagID, stepID, "RUNNING", "")
				}

				result := d.executeSingleTask(ctx, task)
				
				// Checkpoint after execution
				if d.wal != nil {
					if result.Success {
						resultStr := fmt.Sprintf("%v", result.Result)
						_ = d.wal.Checkpoint(dagID, stepID, "COMPLETED", resultStr)
					} else {
						errMsg := ""
						if result.Error != nil {
							errMsg = result.Error.Error()
						}
						_ = d.wal.CheckpointWithError(dagID, stepID, "FAILED", errMsg)
					}
				}

				mu.Lock()
				results[task.Step] = result
				mu.Unlock()
			})
		}

		// Wait for this level to complete before starting next level
		wg.Wait()
	}

	// Mark DAG as complete
	if d.wal != nil {
		_ = d.wal.Complete(dagID)
	}

	return results
}

// executeSequential executes tasks in order, respecting dependencies
func (d *DAGExecutor) executeSequential(ctx context.Context, plan DAGPlan) []TaskResult {
	// For backward compatibility, generate a DAG ID from PlanID
	dagID := plan.PlanID
	if dagID == "" {
		dagID = fmt.Sprintf("dag-%d", time.Now().UnixNano())
	}
	return d.executeSequentialWithWAL(ctx, plan, dagID)
}

// executeSequentialWithWAL executes tasks in order, respecting dependencies with WAL support
func (d *DAGExecutor) executeSequentialWithWAL(ctx context.Context, plan DAGPlan, dagID string) []TaskResult {
	// Find max step to properly size results array
	maxStep := 0
	for _, task := range plan.Tasks {
		if task.Step > maxStep {
			maxStep = task.Step
		}
	}
	results := make([]TaskResult, maxStep+1)
	completed := make(map[int]bool)

	// Build task lookup map
	taskMap := make(map[int]*DAGTask)
	for i := range plan.Tasks {
		taskMap[plan.Tasks[i].Step] = &plan.Tasks[i]
	}

	// Check WAL for completed steps
	completedSteps := d.loadCompletedSteps(dagID)

	// Execute in order, skipping until dependencies are satisfied
	for i := 0; i < len(plan.Tasks); i++ {
		task := &plan.Tasks[i]

		// Check if dependencies are satisfied
		if !d.dependenciesSatisfied(task.Dependencies, completed) {
			// Find next task that can execute
			nextTask := d.findNextExecutableTask(plan.Tasks, completed)
			if nextTask == nil {
				// No more tasks can execute - break
				break
			}
			task = nextTask
		}

		stepID := fmt.Sprintf("%d", task.Step)

		// Skip if already completed
		if result, ok := completedSteps[stepID]; ok {
			results[task.Step] = TaskResult{
				Step:    task.Step,
				Success: true,
				Result:  result,
			}
			completed[task.Step] = true
			continue
		}

		// Checkpoint before execution
		if d.wal != nil {
			_ = d.wal.Checkpoint(dagID, stepID, "RUNNING", "")
		}

		result := d.executeSingleTask(ctx, task)
		
		// Checkpoint after execution
		if d.wal != nil {
			if result.Success {
				resultStr := fmt.Sprintf("%v", result.Result)
				_ = d.wal.Checkpoint(dagID, stepID, "COMPLETED", resultStr)
			} else {
				errMsg := ""
				if result.Error != nil {
					errMsg = result.Error.Error()
				}
				_ = d.wal.CheckpointWithError(dagID, stepID, "FAILED", errMsg)
			}
		}

		results[task.Step] = result
		completed[task.Step] = result.Success
	}

	// Mark DAG as complete
	if d.wal != nil {
		_ = d.wal.Complete(dagID)
	}

	return results
}

// executeSingleTask executes a single task with timeout and fallback
func (d *DAGExecutor) executeSingleTask(ctx context.Context, task *DAGTask) TaskResult {
	start := time.Now()
	result := TaskResult{
		Step:    task.Step,
		Success: false,
	}

	// Create context with timeout
	taskCtx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()

	// In a real implementation, this would call the actual service
	// For now, we simulate execution
	err := d.simulateExecution(taskCtx, task)

	result.Duration = time.Since(start)

	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			// Try fallback tier if configured
			if task.TierFallback >= 0 {
				fallbackResult := d.executeWithFallback(taskCtx, task)
				if fallbackResult.Success {
					return fallbackResult
				}
			}
		}
		result.Error = err
		return result
	}

	result.Success = true
	result.Result = fmt.Sprintf("Task %d completed", task.Step)
	return result
}

// executeWithFallback tries execution with a fallback tier
func (d *DAGExecutor) executeWithFallback(ctx context.Context, task *DAGTask) TaskResult {
	result := TaskResult{
		Step:    task.Step,
		Success: false,
	}

	// Simulate fallback execution
	err := d.simulateExecution(ctx, task)

	if err != nil {
		result.Error = fmt.Errorf("fallback failed: %w", err)
		return result
	}

	result.Success = true
	result.Result = fmt.Sprintf("Task %d completed via fallback tier %d", task.Step, task.TierFallback)
	return result
}

// simulateExecution simulates task execution (placeholder)
func (d *DAGExecutor) simulateExecution(ctx context.Context, task *DAGTask) error {
	// In real implementation, this would:
	// 1. Route to appropriate provider using TieredRouter
	// 2. Execute RPC call
	// 3. Handle errors and retry
	select {
	case <-time.After(10 * time.Millisecond): // Simulate work
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// buildDependencyLevels groups tasks by dependency level
func (d *DAGExecutor) buildDependencyLevels(tasks []DAGTask) [][]int {
	completed := make(map[int]bool)
	levels := [][]int{}

	for {
		// Find all tasks whose dependencies are satisfied
		var ready []int
		for _, task := range tasks {
			if !completed[task.Step] && d.dependenciesSatisfied(task.Dependencies, completed) {
				ready = append(ready, task.Step)
			}
		}

		if len(ready) == 0 {
			break
		}

		// Mark as completed for next level
		for _, step := range ready {
			completed[step] = true
		}

		levels = append(levels, ready)
	}

	return levels
}

// dependenciesSatisfied checks if all dependencies are completed
func (d *DAGExecutor) dependenciesSatisfied(deps []int, completed map[int]bool) bool {
	for _, dep := range deps {
		if !completed[dep] {
			return false
		}
	}
	return true
}

// findNextExecutableTask finds the next task whose dependencies are satisfied
func (d *DAGExecutor) findNextExecutableTask(tasks []DAGTask, completed map[int]bool) *DAGTask {
	for i := range tasks {
		task := &tasks[i]
		if !completed[task.Step] && d.dependenciesSatisfied(task.Dependencies, completed) {
			return task
		}
	}
	return nil
}

// GetTask retrieves a task by step number
func (p *DAGPlan) GetTask(step int) *DAGTask {
	for i := range p.Tasks {
		if p.Tasks[i].Step == step {
			return &p.Tasks[i]
		}
	}
	return nil
}

// ParseDAGPlan parses a DAG plan from JSON
func ParseDAGPlan(jsonData []byte) (DAGPlan, error) {
	var plan DAGPlan
	err := json.Unmarshal(jsonData, &plan)
	if err != nil {
		return DAGPlan{}, fmt.Errorf("failed to parse DAG plan: %w", err)
	}

	// Validate
	if plan.PlanID == "" {
		return DAGPlan{}, errors.New("plan_id is required")
	}

	if plan.ExecutionMode != "PARALLEL" && plan.ExecutionMode != "SEQUENTIAL" && plan.ExecutionMode != "DAG" {
		plan.ExecutionMode = "SEQUENTIAL"
	}

	// Set created timestamp if not present
	if plan.CreatedAt.IsZero() {
		plan.CreatedAt = time.Now()
	}

	return plan, nil
}

// Validate validates the DAG plan
func (p *DAGPlan) Validate() error {
	if p.PlanID == "" {
		return errors.New("plan_id is required")
	}

	taskMap := make(map[int]bool)
	for _, task := range p.Tasks {
		// Check for duplicate steps
		if taskMap[task.Step] {
			return fmt.Errorf("duplicate task step: %d", task.Step)
		}
		taskMap[task.Step] = true

		// Validate dependencies exist
		for _, dep := range task.Dependencies {
			if !taskMap[dep] && dep < task.Step {
				// Dependency refers to a task that should come before
				// This is valid - it just hasn't been seen yet in the array
			} else if !taskMap[dep] && dep > task.Step {
				// Forward dependency is suspicious
				return fmt.Errorf("task %d has forward dependency on %d", task.Step, dep)
			}
		}
	}

	return nil
}

// Shutdown shuts down the DAG executor
func (d *DAGExecutor) Shutdown() {
	if d.pool != nil {
		d.pool.Shutdown()
	}
	if d.wal != nil {
		_ = d.wal.Close()
	}
}

// loadCompletedSteps loads completed steps from WAL for a given DAG ID
func (d *DAGExecutor) loadCompletedSteps(dagID string) map[string]string {
	if d.wal == nil {
		return make(map[string]string)
	}

	tasks, err := d.wal.Recover()
	if err != nil {
		return make(map[string]string)
	}

	// Find the task with matching DAG ID
	for _, task := range tasks {
		if task.DAGID == dagID {
			return task.CompletedSteps
		}
	}

	return make(map[string]string)
}

// RecoverAndResume recovers incomplete DAGs from WAL and resumes execution
func (d *DAGExecutor) RecoverAndResume(ctx context.Context) []TaskResult {
	if d.wal == nil {
		return []TaskResult{}
	}

	tasks, err := d.wal.Recover()
	if err != nil {
		return []TaskResult{}
	}

	var allResults []TaskResult

	// Resume each incomplete DAG
	for _, task := range tasks {
		if task.Plan != nil {
			plan, ok := task.Plan.(*DAGPlan)
			if !ok {
				// Plan is not a DAGPlan, skip
				continue
			}
			results := d.ExecuteWithDAGID(ctx, *plan, task.DAGID)
			allResults = append(allResults, results...)
		}
	}

	return allResults
}