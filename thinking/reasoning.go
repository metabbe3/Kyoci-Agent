package thinking

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Step represents a single step in the reasoning process
type Step struct {
	Phase      string    `json:"phase"`
	Content    string    `json:"content"`
	Confidence float64   `json:"confidence"`
	Timestamp  time.Time `json:"timestamp"`
}

// ReasoningResult contains the complete reasoning output
type ReasoningResult struct {
	Steps              []Step `json:"steps"`
	Conclusion         string `json:"conclusion"`
	NeedsClarification bool   `json:"needs_clarification"`
	Questions          []string `json:"questions,omitempty"`
	Plan               *Plan  `json:"plan,omitempty"`
}

// ReasoningEngine orchestrates step-by-step reasoning
type ReasoningEngine struct {
	maxSteps            int
	confidenceThreshold float64
}

// NewReasoningEngine creates a new reasoning engine with default settings
func NewReasoningEngine() *ReasoningEngine {
	return &ReasoningEngine{
		maxSteps:            4, // Deconstruct, Plan, Review, Synthesize
		confidenceThreshold: 0.7,
	}
}

// SetMaxSteps sets the maximum number of reasoning steps
func (r *ReasoningEngine) SetMaxSteps(maxSteps int) {
	r.maxSteps = maxSteps
}

// SetConfidenceThreshold sets the confidence threshold for requiring clarification
func (r *ReasoningEngine) SetConfidenceThreshold(threshold float64) {
	r.confidenceThreshold = threshold
}

// Think executes a step-by-step reasoning loop
func (r *ReasoningEngine) Think(ctx context.Context, task string, executor func(ctx context.Context, prompt string) (string, error)) (*ReasoningResult, error) {
	result := &ReasoningResult{
		Steps: make([]Step, 0, r.maxSteps),
	}

	// Phase 1: Deconstruct
	deconstructPrompt := fmt.Sprintf(`Break down this task into sub-tasks:
Task: %s

Please provide:
1. A clear decomposition of the task into logical sub-tasks
2. Dependencies between sub-tasks
3. Estimated complexity of each sub-task
4. Any assumptions you're making

Format your response clearly and concisely.`, task)

	response, err := executor(ctx, deconstructPrompt)
	if err != nil {
		return nil, fmt.Errorf("deconstruction phase failed: %w", err)
	}

	result.Steps = append(result.Steps, Step{
		Phase:      "Deconstruct",
		Content:    response,
		Confidence: r.extractConfidence(response),
		Timestamp:  time.Now(),
	})

	// Phase 2: Strategic Planning
	planningPrompt := fmt.Sprintf(`Given these sub-tasks, create a detailed execution plan considering available tools and resources.

Original Task: %s

Decomposition:
%s

Please provide:
1. An ordered list of execution steps
2. Tools or methods to use for each step
3. Expected results for each step
4. How to handle potential failures

Format as a structured, actionable plan.`, task, response)

	response, err = executor(ctx, planningPrompt)
	if err != nil {
		return nil, fmt.Errorf("planning phase failed: %w", err)
	}

	result.Steps = append(result.Steps, Step{
		Phase:      "Strategic Planning",
		Content:    response,
		Confidence: r.extractConfidence(response),
		Timestamp:  time.Now(),
	})

	// Phase 3: Self-Correction
	reviewPrompt := fmt.Sprintf(`Review this plan for errors, gaps, or better approaches. Fix if needed.

Original Task: %s

Current Plan:
%s

Please:
1. Identify any logical errors or gaps
2. Suggest improvements or alternative approaches
3. Highlight potential risks
4. Provide a corrected, optimized plan if changes are needed

Be critical and thorough.`, task, response)

	response, err = executor(ctx, reviewPrompt)
	if err != nil {
		return nil, fmt.Errorf("review phase failed: %w", err)
	}

	reviewStep := Step{
		Phase:      "Self-Correction",
		Content:    response,
		Confidence: r.extractConfidence(response),
		Timestamp:  time.Now(),
	}
	result.Steps = append(result.Steps, reviewStep)

	// Check if clarification is needed based on confidence
	if reviewStep.Confidence < r.confidenceThreshold {
		result.NeedsClarification = true
		questions, err := GenerateQuestions(task, func(prompt string) (string, error) {
			return executor(ctx, prompt)
		})
		if err != nil {
			// Log error but continue
			fmt.Printf("Warning: failed to generate questions: %v\n", err)
		} else {
			result.Questions = questions
		}
	}

	// Phase 4: Synthesis
	synthesisPrompt := fmt.Sprintf(`Compile the final structured result from the validated plan.

Original Task: %s

Review and Optimized Plan:
%s

Please provide:
1. A comprehensive, executable conclusion
2. Clear steps to follow
3. Final verification checklist
4. Any remaining notes or considerations

Format as a final, actionable result.`, task, response)

	response, err = executor(ctx, synthesisPrompt)
	if err != nil {
		return nil, fmt.Errorf("synthesis phase failed: %w", err)
	}

	result.Steps = append(result.Steps, Step{
		Phase:      "Synthesis",
		Content:    response,
		Confidence: r.extractConfidence(response),
		Timestamp:  time.Now(),
	})
	result.Conclusion = response

	// Try to parse a Plan from the reasoning
	plan, err := ParsePlanFromAI(response)
	if err == nil && plan != nil {
		result.Plan = plan
	}

	return result, nil
}

// extractConfidence attempts to extract a confidence score from AI response
// Returns a default value if none found
func (r *ReasoningEngine) extractConfidence(response string) float64 {
	// Look for explicit confidence mentions in the response
	response = strings.ToLower(response)
	
	// Simple heuristic - in production, this would be more sophisticated
	if strings.Contains(response, "high confidence") || strings.Contains(response, "very confident") {
		return 0.9
	}
	if strings.Contains(response, "moderate confidence") || strings.Contains(response, "somewhat confident") {
		return 0.7
	}
	if strings.Contains(response, "low confidence") || strings.Contains(response, "uncertain") {
		return 0.5
	}
	
	// Default to moderate confidence
	return 0.75
}