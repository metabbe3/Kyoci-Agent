package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// JobFunc is the callback type for scheduled jobs
type JobFunc func(ctx context.Context, prompt string) (string, error)

// Job represents a scheduled job
type Job struct {
	ID        string
	Interval  time.Duration
	Prompt    string
	Func      JobFunc
	Ticker    *time.Ticker
	StopChan  chan struct{}
	Running   bool
	LastRun   time.Time
	NextRun   time.Time
	RunCount  int
}

// SchedulerTool manages background scheduled tasks
type SchedulerTool struct {
	jobs      sync.Map
	callbacks sync.Map // Store JobFunc callbacks
}

// NewSchedulerTool creates a new scheduler tool
func NewSchedulerTool() *SchedulerTool {
	return &SchedulerTool{}
}

func (t *SchedulerTool) Name() string {
	return "scheduler"
}

func (t *SchedulerTool) Description() string {
	return "Manage background scheduled tasks. Supports creating, listing, canceling, and running jobs at regular intervals."
}

func (t *SchedulerTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"description": "Action to perform: create, list, cancel, run",
				"enum":        []string{"create", "list", "cancel", "run"},
			},
			"id": map[string]interface{}{
				"type":        "string",
				"description": "Job identifier (required for create, cancel, run)",
			},
			"interval": map[string]interface{}{
				"type":        "string",
				"description": "Time interval (e.g., '5m' for 5 minutes, '1h' for 1 hour, '30s' for 30 seconds). Required for create.",
			},
			"prompt": map[string]interface{}{
				"type":        "string",
				"description": "Prompt/task to execute on each run. Required for create.",
			},
		},
	}
}

func (t *SchedulerTool) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var params struct {
		Action   string `json:"action"`
		ID       string `json:"id"`
		Interval string `json:"interval"`
		Prompt   string `json:"prompt"`
	}

	if err := json.Unmarshal(raw, &params); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}

	switch params.Action {
	case "create":
		if params.ID == "" {
			return "", fmt.Errorf("id is required for create action")
		}
		if params.Interval == "" {
			return "", fmt.Errorf("interval is required for create action")
		}
		if params.Prompt == "" {
			return "", fmt.Errorf("prompt is required for create action")
		}
		return t.createJob(ctx, params.ID, params.Interval, params.Prompt)

	case "list":
		return t.listJobs()

	case "cancel":
		if params.ID == "" {
			return "", fmt.Errorf("id is required for cancel action")
		}
		return t.cancelJob(params.ID)

	case "run":
		if params.ID == "" {
			return "", fmt.Errorf("id is required for run action")
		}
		return t.runJobNow(ctx, params.ID)

	default:
		return "", fmt.Errorf("unknown action: %s. Valid actions: create, list, cancel, run", params.Action)
	}
}

// RegisterCallback registers a callback function for jobs
func (t *SchedulerTool) RegisterCallback(jobID string, callback JobFunc) {
	t.callbacks.Store(jobID, callback)
}

// parseInterval parses a duration string like "5m", "1h", "30s"
func (t *SchedulerTool) parseInterval(interval string) (time.Duration, error) {
	if len(interval) < 2 {
		return 0, fmt.Errorf("invalid interval format: %s", interval)
	}

	suffix := interval[len(interval)-1:]
	numStr := interval[:len(interval)-1]

	num, err := strconv.Atoi(numStr)
	if err != nil {
		return 0, fmt.Errorf("invalid interval number: %w", err)
	}

	if num <= 0 {
		return 0, fmt.Errorf("interval must be positive")
	}

	switch suffix {
	case "s":
		return time.Duration(num) * time.Second, nil
	case "m":
		return time.Duration(num) * time.Minute, nil
	case "h":
		return time.Duration(num) * time.Hour, nil
	default:
		return 0, fmt.Errorf("invalid interval suffix: %s (use s, m, or h)", suffix)
	}
}

// createJob creates and starts a new scheduled job
func (t *SchedulerTool) createJob(ctx context.Context, id, interval, prompt string) (string, error) {
	// Check if job already exists
	if _, exists := t.jobs.Load(id); exists {
		return "", fmt.Errorf("job with id '%s' already exists", id)
	}

	duration, err := t.parseInterval(interval)
	if err != nil {
		return "", err
	}

	job := &Job{
		ID:       id,
		Interval: duration,
		Prompt:   prompt,
		Ticker:   time.NewTicker(duration),
		StopChan: make(chan struct{}),
		Running:  true,
		NextRun:  time.Now().Add(duration),
	}

	// Get the callback if registered
	if callback, ok := t.callbacks.Load(id); ok {
		job.Func = callback.(JobFunc)
	} else {
		// Default no-op callback
		job.Func = func(c context.Context, p string) (string, error) {
			return fmt.Sprintf("Job '%s' executed: %s (no callback configured)", id, p), nil
		}
	}

	t.jobs.Store(id, job)

	// Start the job in a goroutine
	go t.runJob(job, ctx)

	return fmt.Sprintf("Job '%s' created successfully with interval %s. Next run: %s",
		id, interval, job.NextRun.Format(time.RFC3339)), nil
}

// runJob executes the job loop
func (t *SchedulerTool) runJob(job *Job, ctx context.Context) {
	for {
		select {
		case <-job.Ticker.C:
			job.LastRun = time.Now()
			job.RunCount++
			job.NextRun = time.Now().Add(job.Interval)

			// Execute the job callback
			if job.Func != nil {
				result, err := job.Func(ctx, job.Prompt)
				if err != nil {
					fmt.Printf("[Scheduler] Job '%s' error: %v\n", job.ID, err)
				} else {
					fmt.Printf("[Scheduler] Job '%s' completed: %s\n", job.ID, result)
				}
			}

		case <-job.StopChan:
			job.Running = false
			fmt.Printf("[Scheduler] Job '%s' stopped\n", job.ID)
			return

		case <-ctx.Done():
			job.Running = false
			fmt.Printf("[Scheduler] Job '%s' context cancelled\n", job.ID)
			return
		}
	}
}

// listJobs returns information about all jobs
func (t *SchedulerTool) listJobs() (string, error) {
	var jobs []string

	t.jobs.Range(func(key, value interface{}) bool {
		job := value.(*Job)
		status := "running"
		if !job.Running {
			status = "stopped"
		}

		lastRun := "never"
		if !job.LastRun.IsZero() {
			lastRun = job.LastRun.Format(time.RFC3339)
		}

		jobs = append(jobs, fmt.Sprintf(
			"- ID: %s\n  Status: %s\n  Interval: %v\n  Prompt: %s\n  Run count: %d\n  Last run: %s\n  Next run: %s",
			job.ID, status, job.Interval, job.Prompt, job.RunCount, lastRun, job.NextRun.Format(time.RFC3339),
		))
		return true
	})

	if len(jobs) == 0 {
		return "No scheduled jobs found", nil
	}

	return fmt.Sprintf("Scheduled Jobs (%d):\n%s", len(jobs), strings.Join(jobs, "\n\n")), nil
}

// cancelJob stops and removes a job
func (t *SchedulerTool) cancelJob(id string) (string, error) {
	value, exists := t.jobs.Load(id)
	if !exists {
		return "", fmt.Errorf("job not found: %s", id)
	}

	job := value.(*Job)

	// Stop the job
	close(job.StopChan)
	job.Ticker.Stop()
	job.Running = false

	// Remove from the map
	t.jobs.Delete(id)

	return fmt.Sprintf("Job '%s' cancelled successfully. Total runs: %d", id, job.RunCount), nil
}

// runJobNow immediately executes a job
func (t *SchedulerTool) runJobNow(ctx context.Context, id string) (string, error) {
	value, exists := t.jobs.Load(id)
	if !exists {
		return "", fmt.Errorf("job not found: %s", id)
	}

	job := value.(*Job)

	if job.Func == nil {
		return "", fmt.Errorf("no callback configured for job: %s", id)
	}

	result, err := job.Func(ctx, job.Prompt)
	if err != nil {
		return "", fmt.Errorf("job execution failed: %w", err)
	}

	job.LastRun = time.Now()
	job.RunCount++
	job.NextRun = time.Now().Add(job.Interval)

	return fmt.Sprintf("Job '%s' executed manually:\nResult: %s\nTotal runs: %d", id, result, job.RunCount), nil
}

// StopAll stops all running jobs
func (t *SchedulerTool) StopAll() {
	t.jobs.Range(func(key, value interface{}) bool {
		job := value.(*Job)
		if job.Running {
			close(job.StopChan)
			job.Ticker.Stop()
			job.Running = false
		}
		return true
	})
}