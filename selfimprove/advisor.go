package selfimprove

import (
	"fmt"
	"sort"
	"strings"
)

// ToolAdvisor recommends tools based on past success rates
type ToolAdvisor struct {
	learner *ExperienceLearner
	store   *KnowledgeStore
}

// NewToolAdvisor creates a new ToolAdvisor
func NewToolAdvisor(learner *ExperienceLearner, store *KnowledgeStore) *ToolAdvisor {
	return &ToolAdvisor{
		learner: learner,
		store:   store,
	}
}

// Recommendation represents a tool recommendation with confidence
type Recommendation struct {
	Tool      string  `json:"tool"`
	Confidence float64 `json:"confidence"`
	Reason    string  `json:"reason"`
}

// GetAdvice returns a recommendation for which tool(s) to use for a task
func (ta *ToolAdvisor) GetAdvice(task string) string {
	taskWords := extractKeywords(task)

	// Check for existing workflow patterns first
	if wf, ok := ta.store.GetWorkflowPattern(task); ok {
		return fmt.Sprintf("Use workflow '%s': %s (success rate: %.0f%%)", 
			wf.Name, strings.Join(wf.Steps, " → "), wf.SuccessRate*100)
	}

	// Check knowledge store for tool patterns
	var recommendations []Recommendation

	for _, word := range taskWords {
		patterns := ta.store.GetToolPatterns(word)
		for _, p := range patterns {
			confidence := p.SuccessRate
			if p.UsageCount > 0 {
				confidence = confidence * (1.0 + float64(p.UsageCount)*0.1)
				if confidence > 1.0 {
					confidence = 1.0
				}
			}
			recommendations = append(recommendations, Recommendation{
				Tool:      p.RecommendedTool,
				Confidence: confidence,
				Reason:    fmt.Sprintf("matched '%s' pattern (%.0f%% success, %d uses)", 
					p.TaskPattern, p.SuccessRate*100, p.UsageCount),
			})
		}
	}

	// Fall back to experience data from learner
	if len(recommendations) == 0 {
		tools := []string{"patch", "read_file", "write_file", "search_files"} // Common tools
		for _, tool := range tools {
			rate, count := ta.learner.GetSuccessRate(tool, task)
			if count > 0 {
				recommendations = append(recommendations, Recommendation{
					Tool:      tool,
					Confidence: rate,
					Reason:    fmt.Sprintf("historical success rate %.0f%% (%d uses)", rate*100, count),
				})
			}
		}
	}

	// Sort by confidence and deduplicate
	if len(recommendations) > 0 {
		sort.Slice(recommendations, func(i, j int) bool {
			return recommendations[i].Confidence > recommendations[j].Confidence
		})

		// Remove duplicate tool recommendations
		seen := make(map[string]bool)
		unique := make([]Recommendation, 0)
		for _, r := range recommendations {
			if !seen[r.Tool] {
				seen[r.Tool] = true
				unique = append(unique, r)
			}
		}
		recommendations = unique
	}

	// Generate brief 1-2 line response
	if len(recommendations) > 0 {
		top := recommendations[0]
		if len(recommendations) > 1 {
			return fmt.Sprintf("Try %s (%s). Alternative: %s", 
				top.Tool, top.Reason, recommendations[1].Tool)
		}
		return fmt.Sprintf("Try %s (%s)", top.Tool, top.Reason)
	}

	// Default fallback advice based on task keywords
	return getDefaultAdvice(task)
}

// GetDetailedAdvice returns a more detailed recommendation
func (ta *ToolAdvisor) GetDetailedAdvice(task string) []Recommendation {
	taskWords := extractKeywords(task)
	recommendations := make([]Recommendation, 0)
	seen := make(map[string]bool)

	// Check knowledge store
	for _, word := range taskWords {
		patterns := ta.store.GetToolPatterns(word)
		for _, p := range patterns {
			if !seen[p.RecommendedTool] {
				seen[p.RecommendedTool] = true
				confidence := p.SuccessRate
				if p.UsageCount > 0 {
					confidence = confidence * (1.0 + float64(p.UsageCount)*0.1)
					if confidence > 1.0 {
						confidence = 1.0
					}
				}
				recommendations = append(recommendations, Recommendation{
					Tool:      p.RecommendedTool,
					Confidence: confidence,
					Reason:    fmt.Sprintf("pattern match: %s", p.TaskPattern),
				})
			}
		}
	}

	// Sort by confidence
	sort.Slice(recommendations, func(i, j int) bool {
		return recommendations[i].Confidence > recommendations[j].Confidence
	})

	return recommendations
}

// GetErrorFix returns a suggested fix for a known error
func (ta *ToolAdvisor) GetErrorFix(errorMessage string) (string, bool) {
	if pattern, ok := ta.store.GetErrorPattern(errorMessage); ok {
		return pattern.Fix, true
	}
	return "", false
}

// UpdateFromOutcome updates recommendations based on a tool outcome
func (ta *ToolAdvisor) UpdateFromOutcome(task, tool string, success bool) error {
	taskWords := extractKeywords(task)

	for _, word := range taskWords {
		// Get existing patterns
		patterns := ta.store.GetToolPatterns(word)
		found := false
		
		for i, p := range patterns {
			if p.RecommendedTool == tool {
				// Update existing pattern
				newRate := (p.SuccessRate*float64(p.UsageCount) + boolToFloat(success)) / float64(p.UsageCount+1)
				patterns[i] = ToolPattern{
					TaskPattern:     word,
					RecommendedTool: tool,
					SuccessRate:     newRate,
					UsageCount:      p.UsageCount + 1,
				}
				found = true
				break
			}
		}

		if !found {
			// Add new pattern
			patterns = append(patterns, ToolPattern{
				TaskPattern:     word,
				RecommendedTool: tool,
				SuccessRate:     boolToFloat(success),
				UsageCount:      1,
			})
		}

		// Save the highest rated pattern for this keyword
		if len(patterns) > 0 {
			bestIdx := 0
			for i := 1; i < len(patterns); i++ {
				if patterns[i].SuccessRate > patterns[bestIdx].SuccessRate {
					bestIdx = i
				}
			}
			if err := ta.store.AddToolPattern(patterns[bestIdx]); err != nil {
				return err
			}
		}
	}

	return nil
}

// extractKeywords extracts meaningful keywords from a task description
func extractKeywords(task string) []string {
	words := make([]string, 0)
	current := ""
	
	keywords := map[string]bool{
		"read": true, "write": true, "patch": true, "search": true,
		"file": true, "code": true, "test": true, "build": true,
		"find": true, "replace": true, "create": true, "delete": true,
		"copy": true, "move": true, "list": true, "run": true,
		"install": true, "update": true, "git": true, "json": true,
		"yaml": true, "config": true, "setup": true, "deploy": true,
	}

	for _, ch := range task {
		if ch == ' ' || ch == ',' || ch == '.' || ch == '\t' || ch == '\n' {
			if len(current) > 2 {
				if keywords[current] {
					words = append(words, current)
				}
			}
			current = ""
		} else if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') {
			if ch >= 'A' && ch <= 'Z' {
				current += string(ch + 32)
			} else {
				current += string(ch)
			}
		}
	}
	if len(current) > 2 {
		if keywords[current] {
			words = append(words, current)
		}
	}

	return words
}

// getDefaultAdvice provides fallback advice when no data is available
func getDefaultAdvice(task string) string {
	taskLower := strings.ToLower(task)
	
	switch {
	case strings.Contains(taskLower, "read") || strings.Contains(taskLower, "view"):
		return "Try read_file for viewing file contents"
	case strings.Contains(taskLower, "write") || strings.Contains(taskLower, "create"):
		return "Try write_file for creating new files"
	case strings.Contains(taskLower, "edit") || strings.Contains(taskLower, "modify") || strings.Contains(taskLower, "replace"):
		return "Try patch for editing existing files"
	case strings.Contains(taskLower, "search") || strings.Contains(taskLower, "find"):
		return "Try search_files for finding content"
	case strings.Contains(taskLower, "test"):
		return "Try running tests with appropriate test command"
	default:
		return "No historical data available - proceed with best judgment"
	}
}

// boolToFloat converts bool to 1.0 or 0.0
func boolToFloat(b bool) float64 {
	if b {
		return 1.0
	}
	return 0.0
}