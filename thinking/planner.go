package thinking

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// PlanStep represents a single step in an execution plan
type PlanStep struct {
	Number        int      `json:"number"`
	Action        string   `json:"action"`
	Tool          string   `json:"tool,omitempty"`
	ExpectedResult string  `json:"expected_result,omitempty"`
	Dependencies   []int    `json:"dependencies,omitempty"`
}

// Plan represents a complete execution plan
type Plan struct {
	ID          string     `json:"id"`
	Task        string     `json:"task"`
	Steps       []PlanStep `json:"steps"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	ApprovedBy  string     `json:"approved_by,omitempty"`
}

// NewPlan creates a new Plan instance
func NewPlan(task string, steps []PlanStep) *Plan {
	return &Plan{
		ID:        uuid.New().String(),
		Task:      task,
		Steps:     steps,
		Status:    "pending",
		CreatedAt: time.Now(),
	}
}

// Format returns a pretty-printed representation of the plan
func (p *Plan) Format() string {
	var sb strings.Builder
	
	sb.WriteString(fmt.Sprintf("📋 Plan: %s\n", p.Task))
	sb.WriteString(fmt.Sprintf("   ID: %s\n", p.ID))
	sb.WriteString(fmt.Sprintf("   Status: %s\n", p.Status))
	sb.WriteString(fmt.Sprintf("   Created: %s\n\n", p.CreatedAt.Format("2006-01-02 15:04:05")))
	
	sb.WriteString("Steps:\n")
	for _, step := range p.Steps {
		sb.WriteString(fmt.Sprintf("  %d. %s\n", step.Number, step.Action))
		if step.Tool != "" {
			sb.WriteString(fmt.Sprintf("     Tool: %s\n", step.Tool))
		}
		if step.ExpectedResult != "" {
			sb.WriteString(fmt.Sprintf("     Expected: %s\n", step.ExpectedResult))
		}
		if len(step.Dependencies) > 0 {
			sb.WriteString(fmt.Sprintf("     Depends on: %v\n", step.Dependencies))
		}
		sb.WriteString("\n")
	}
	
	if p.ApprovedBy != "" {
		sb.WriteString(fmt.Sprintf("✓ Approved by: %s\n", p.ApprovedBy))
	}
	
	return sb.String()
}

// ToJSON serializes the plan to JSON format
func (p *Plan) ToJSON() (string, error) {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal plan: %w", err)
	}
	return string(data), nil
}

// FromJSON deserializes a plan from JSON format
func FromJSON(jsonStr string) (*Plan, error) {
	var plan Plan
	err := json.Unmarshal([]byte(jsonStr), &plan)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal plan: %w", err)
	}
	return &plan, nil
}

// ParsePlanFromAI attempts to parse a plan from AI-generated text
func ParsePlanFromAI(response string) (*Plan, error) {
	steps := make([]PlanStep, 0)
	
	// Try to parse numbered steps with structure
	// Pattern: "1. Action [tool: X] [expected: Y]"
	lines := strings.Split(response, "\n")
	stepPattern := regexp.MustCompile(`^\s*(\d+)\.\s+(.+?)(?:\[tool:\s*(.+?)\])?(?:\[expected:\s*(.+?)\])?\s*$`)
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		
		matches := stepPattern.FindStringSubmatch(line)
		if len(matches) > 2 {
			num, err := strconv.Atoi(matches[1])
			if err != nil {
				continue
			}
			
			step := PlanStep{
				Number: num,
				Action: strings.TrimSpace(matches[2]),
			}
			
			if len(matches) > 3 && matches[3] != "" {
				step.Tool = strings.TrimSpace(matches[3])
			}
			
			if len(matches) > 4 && matches[4] != "" {
				step.ExpectedResult = strings.TrimSpace(matches[4])
			}
			
			steps = append(steps, step)
		}
	}
	
	// Also try bullet point format
	if len(steps) == 0 {
		bulletPattern := regexp.MustCompile(`^\s*[-*]\s+(.+?)(?:\|\s*tool:\s*(.+?))?(?:\|\s*expected:\s*(.+?))?\s*$`)
		num := 1
		for _, line := range lines {
			matches := bulletPattern.FindStringSubmatch(line)
			if len(matches) > 1 {
				step := PlanStep{
					Number: num,
					Action: strings.TrimSpace(matches[1]),
				}
				
				if len(matches) > 2 && matches[2] != "" {
					step.Tool = strings.TrimSpace(matches[2])
				}
				
				if len(matches) > 3 && matches[3] != "" {
					step.ExpectedResult = strings.TrimSpace(matches[3])
				}
				
				steps = append(steps, step)
				num++
			}
		}
	}
	
	// If we found steps, create a plan
	if len(steps) > 0 {
		plan := &Plan{
			ID:        uuid.New().String(),
			Steps:     steps,
			Status:    "pending",
			CreatedAt: time.Now(),
		}
		return plan, nil
	}
	
	// Return nil if no plan could be parsed
	return nil, fmt.Errorf("no plan structure found in response")
}

// SetStatus updates the plan status
func (p *Plan) SetStatus(status string) {
	p.Status = status
}

// Approve marks the plan as approved
func (p *Plan) Approve(approver string) {
	p.Status = "approved"
	p.ApprovedBy = approver
}

// Reject marks the plan as rejected
func (p *Plan) Reject() {
	p.Status = "rejected"
}

// IsApproved checks if the plan has been approved
func (p *Plan) IsApproved() bool {
	return p.Status == "approved"
}

// IsPending checks if the plan is pending approval
func (p *Plan) IsPending() bool {
	return p.Status == "pending"
}