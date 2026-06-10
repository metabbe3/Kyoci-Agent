package selfimprove

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/nicholas/ai-agent/codegraph"
)

// ImprovementPhase represents the current phase of an improvement job
type ImprovementPhase string

const (
	PhaseSearch   ImprovementPhase = "SEARCH"
	PhasePlan     ImprovementPhase = "PLAN"
	PhaseExecute  ImprovementPhase = "EXECUTE"
	PhaseValidate ImprovementPhase = "VALIDATE"
	PhaseReview   ImprovementPhase = "REVIEW"
	PhasePR       ImprovementPhase = "PR"
	PhaseComplete ImprovementPhase = "COMPLETE"
	PhaseFailed   ImprovementPhase = "FAILED"
)

// ImprovementJob represents a self-improvement task
type ImprovementJob struct {
	ID                  string             `json:"id"`
	Task                string             `json:"task"`
	Phase               ImprovementPhase   `json:"phase"`
	FilesChanged        []string           `json:"filesChanged"`
	ValidationResult    *ValidationResult  `json:"validationResult,omitempty"`
	Plan                string             `json:"plan,omitempty"`
	PRAvailable         bool               `json:"prAvailable"`
	PRURL               string             `json:"prUrl,omitempty"`
	RiskLevel           string             `json:"riskLevel"`
	CreatedAt           time.Time          `json:"createdAt"`
	Error               string             `json:"error,omitempty"`
	ApprovalChannel     chan bool          `json:"-"` // Internal channel for approval
	ApproveCallback     func(*ImprovementJob) `json:"-"`
	RejectCallback      func(*ImprovementJob) `json:"-"`
}

// SelfImprovePipeline manages the safe self-improvement workflow
type SelfImprovePipeline struct {
	knowledge   *codegraph.CodeKnowledge
	impact      *codegraph.ImpactAnalyzer
	validator   *Validator
	lspClient   *codegraph.LSPClient
	projectRoot string
	maxParallel int
	jobs        *sync.Map // jobID -> *ImprovementJob
	notify      func(job *ImprovementJob)
}

// NewSelfImprovePipeline creates a new pipeline instance
func NewSelfImprovePipeline(root string, ck *codegraph.CodeKnowledge, lsp *codegraph.LSPClient) *SelfImprovePipeline {
	var impact *codegraph.ImpactAnalyzer
	if ck != nil && lsp != nil {
		// Import codegraph from internal package: "github.com/nicholas/ai-agent/codegraph"
		impact = codegraph.NewImpactAnalyzer(nil, lsp)
	}

	return &SelfImprovePipeline{
		knowledge:   ck,
		impact:      impact,
		lspClient:   lsp,
		projectRoot: root,
		validator:   NewValidator(root),
		maxParallel: 1, // Start with 1 for safety
		jobs:        &sync.Map{},
		notify:      nil, // Can be set later
	}
}

// SetNotify sets the notification callback
func (p *SelfImprovePipeline) SetNotify(fn func(job *ImprovementJob)) {
	p.notify = fn
}

// Run executes the full self-improvement pipeline
func (p *SelfImprovePipeline) Run(ctx context.Context, task string) (*ImprovementJob, error) {
	jobID := fmt.Sprintf("job-%d", time.Now().UnixNano())

	job := &ImprovementJob{
		ID:          jobID,
		Task:        task,
		Phase:       PhaseSearch,
		RiskLevel:   "unknown",
		CreatedAt:   time.Now(),
		PRAvailable: false,
	}

	p.jobs.Store(jobID, job)

	// Phase 1: SEARCH
	if err := p.phaseSearch(ctx, job); err != nil {
		return p.failJob(job, err)
	}

	// Phase 2: PLAN
	if err := p.phasePlan(ctx, job); err != nil {
		return p.failJob(job, err)
	}

	// Phase 3: EXECUTE
	if err := p.phaseExecute(ctx, job); err != nil {
		return p.failJob(job, err)
	}

	// Phase 4: VALIDATE
	if err := p.phaseValidate(ctx, job); err != nil {
		return p.failJob(job, err)
	}

	// Phase 5: REVIEW
	if err := p.phaseReview(ctx, job); err != nil {
		return p.failJob(job, err)
	}

	// Phase 6: PR (if approved)
	if err := p.phasePR(ctx, job); err != nil {
		return p.failJob(job, err)
	}

	// Phase 7: COMPLETE
	job.Phase = PhaseComplete
	p.jobs.Store(jobID, job)

	return job, nil
}

// phaseSearch queries code knowledge and analyzes impact
func (p *SelfImprovePipeline) phaseSearch(ctx context.Context, job *ImprovementJob) error {
	job.Phase = PhaseSearch
	p.jobs.Store(job.ID, job)

	// Query code knowledge for relevant code
	if p.knowledge != nil {
		result, err := p.knowledge.Query(job.Task)
		if err != nil {
			return fmt.Errorf("code knowledge query failed: %w", err)
		}
		
		// Extract relevant files from search result
		// In a real implementation, this would parse the SearchContext
		// For now, we'll track this metadata in Plan
		job.Plan = fmt.Sprintf("Found %d relevant code items", len(result.Results))
	}

	return nil
}

// phasePlan creates a structured plan for changes
func (p *SelfImprovePipeline) phasePlan(ctx context.Context, job *ImprovementJob) error {
	job.Phase = PhasePlan
	p.jobs.Store(job.ID, job)

	// Determine files to change based on search results and task
	// This would use AI in a full implementation
	// For deterministic version, we do basic analysis
	
	files := []string{}
	
	// Search for Go files in project
	cmd := exec.CommandContext(ctx, "find", p.projectRoot, "-name", "*.go", "-type", "f")
	output, err := cmd.CombinedOutput()
	if err == nil {
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" {
				relPath, err := filepath.Rel(p.projectRoot, line)
				if err == nil {
					files = append(files, relPath)
				}
			}
		}
	}

	job.FilesChanged = files
	
	// Estimate risk level based on number of files
	if len(files) > 20 {
		job.RiskLevel = "high"
	} else if len(files) > 5 {
		job.RiskLevel = "medium"
	} else {
		job.RiskLevel = "low"
	}

	job.Plan = fmt.Sprintf("Plan to modify %d files (risk: %s)", len(files), job.RiskLevel)

	return nil
}

// phaseExecute creates sandbox and applies changes
func (p *SelfImprovePipeline) phaseExecute(ctx context.Context, job *ImprovementJob) error {
	job.Phase = PhaseExecute
	p.jobs.Store(job.ID, job)

	// Create sandbox
	sandbox, err := NewSandbox(p.projectRoot)
	if err != nil {
		return fmt.Errorf("failed to create sandbox: %w", err)
	}
	defer sandbox.Cleanup()

	// In a full implementation, this would apply actual changes
	// For now, we just note the sandbox was created
	job.Plan += fmt.Sprintf(" | Sandbox created at %s", sandbox.sandboxDir)

	return nil
}

// phaseValidate runs the validator in the sandbox
func (p *SelfImprovePipeline) phaseValidate(ctx context.Context, job *ImprovementJob) error {
	job.Phase = PhaseValidate
	p.jobs.Store(job.ID, job)

	// Create sandbox for validation
	sandbox, err := NewSandbox(p.projectRoot)
	if err != nil {
		return fmt.Errorf("failed to create sandbox for validation: %w", err)
	}
	defer sandbox.Cleanup()

	// Run validation
	result, err := sandbox.Validate(ctx)
	if err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	job.ValidationResult = result

	if !result.Valid {
		return fmt.Errorf("validation failed: %d errors, %d tests failed", 
			len(result.Errors), result.TestsFailed)
	}

	return nil
}

// phaseReview generates diff and waits for approval
func (p *SelfImprovePipeline) phaseReview(ctx context.Context, job *ImprovementJob) error {
	job.Phase = PhaseReview
	p.jobs.Store(job.ID, job)

	// Create sandbox for diff
	sandbox, err := NewSandbox(p.projectRoot)
	if err != nil {
		return fmt.Errorf("failed to create sandbox for review: %w", err)
	}
	defer sandbox.Cleanup()

	// Generate diff
	diff, err := sandbox.Diff()
	if err != nil {
		return fmt.Errorf("failed to generate diff: %w", err)
	}

	job.Plan += fmt.Sprintf("\n\n=== DIFF ===\n%s", diff)

	// Notify callback
	if p.notify != nil {
		p.notify(job)
	}

	// Wait for approval (blocking with timeout)
	job.ApprovalChannel = make(chan bool)
	
	select {
	case approved := <-job.ApprovalChannel:
		if !approved {
			return fmt.Errorf("improvement rejected by human reviewer")
		}
	case <-time.After(30 * time.Minute):
		return fmt.Errorf("approval timeout")
	case <-ctx.Done():
		return fmt.Errorf("context cancelled")
	}

	return nil
}

// phasePR creates branch, commits, and creates PR
func (p *SelfImprovePipeline) phasePR(ctx context.Context, job *ImprovementJob) error {
	job.Phase = PhasePR
	p.jobs.Store(job.ID, job)

	// Create PR creator
	prCreator := NewPRCreator(p.projectRoot)

	// Create branch
	branchName := fmt.Sprintf("improvement-%s", job.ID)
	if err := prCreator.CreateBranch(ctx, branchName); err != nil {
		return fmt.Errorf("failed to create branch: %w", err)
	}

	// In full implementation, apply actual changes here
	// For now, we just create an empty commit
	
	// Commit changes
	commitMsg := fmt.Sprintf("Self-improvement: %s\n\nJob ID: %s\nRisk Level: %s", 
		job.Task, job.ID, job.RiskLevel)
	
	if len(job.FilesChanged) > 0 {
		if err := prCreator.Commit(ctx, job.FilesChanged, commitMsg); err != nil {
			return fmt.Errorf("failed to commit changes: %w", err)
		}
	}

	// Push branch
	if err := prCreator.Push(ctx, branchName); err != nil {
		return fmt.Errorf("failed to push branch: %w", err)
	}

	// Create PR
	if prCreator.IsGhAvailable() {
		prTitle := fmt.Sprintf("[Self-Improvement] %s", job.Task)
		prBody := fmt.Sprintf("Automated improvement generated by Kyoci Agent.\n\nJob ID: %s\n\n## Changes\n%s\n\n## Validation\nValid: %v\nLint Score: %d\nTests: %d passed, %d failed\nCoverage: %.1f%%",
			job.ID,
			job.Plan,
			job.ValidationResult.Valid,
			job.ValidationResult.LintScore,
			job.ValidationResult.TestsPassed,
			job.ValidationResult.TestsFailed,
			job.ValidationResult.Coverage)

		prURL, err := prCreator.CreatePR(ctx, prTitle, prBody, branchName)
		if err != nil {
			return fmt.Errorf("failed to create PR: %w", err)
		}

		job.PRAvailable = true
		job.PRURL = prURL

		// Add labels
		labels := []string{"self-improvement", "automated"}
		if job.RiskLevel != "" {
			labels = append(labels, fmt.Sprintf("risk-%s", job.RiskLevel))
		}
		prCreator.AddLabels(prURL, labels)
	}

	return nil
}

// failJob marks a job as failed
func (p *SelfImprovePipeline) failJob(job *ImprovementJob, err error) (*ImprovementJob, error) {
	job.Phase = PhaseFailed
	job.Error = err.Error()
	p.jobs.Store(job.ID, job)
	return job, err
}

// Approve approves a job for continuation
func (p *SelfImprovePipeline) Approve(jobID string) error {
	val, ok := p.jobs.Load(jobID)
	if !ok {
		return fmt.Errorf("job not found: %s", jobID)
	}

	job := val.(*ImprovementJob)
	
	if job.ApprovalChannel == nil {
		return fmt.Errorf("job not waiting for approval: %s", jobID)
	}

	job.ApprovalChannel <- true
	return nil
}

// Reject rejects a job
func (p *SelfImprovePipeline) Reject(jobID string) error {
	val, ok := p.jobs.Load(jobID)
	if !ok {
		return fmt.Errorf("job not found: %s", jobID)
	}

	job := val.(*ImprovementJob)
	
	if job.ApprovalChannel == nil {
		return fmt.Errorf("job not waiting for approval: %s", jobID)
	}

	job.ApprovalChannel <- false
	return nil
}

// Status returns the current status of a job
func (p *SelfImprovePipeline) Status(jobID string) (*ImprovementJob, error) {
	val, ok := p.jobs.Load(jobID)
	if !ok {
		return nil, fmt.Errorf("job not found: %s", jobID)
	}

	return val.(*ImprovementJob), nil
}

// ListJobs returns all jobs
func (p *SelfImprovePipeline) ListJobs() []*ImprovementJob {
	var jobs []*ImprovementJob

	p.jobs.Range(func(key, value interface{}) bool {
		jobs = append(jobs, value.(*ImprovementJob))
		return true
	})

	return jobs
}