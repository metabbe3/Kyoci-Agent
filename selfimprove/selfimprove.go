package selfimprove

import (
	"path/filepath"
	"time"
)

// SelfImprover ties together all components of the self-improvement system
type SelfImprover struct {
	learner *ExperienceLearner
	store   *KnowledgeStore
	advisor *ToolAdvisor
	baseDir string
}

// NewSelfImprover creates a new SelfImprover instance
func NewSelfImprover(baseDir string) *SelfImprover {
	si := &SelfImprover{
		baseDir: baseDir,
	}

	// Initialize components with paths
	si.learner = NewExperienceLearner(filepath.Join(baseDir, "experiences.json"))
	si.store = NewKnowledgeStore(filepath.Join(baseDir, "knowledge.json"))
	si.advisor = NewToolAdvisor(si.learner, si.store)

	return si
}

// GetLearner returns the underlying ExperienceLearner (for pipeline sharing)
func (si *SelfImprover) GetLearner() *ExperienceLearner {
	return si.learner
}

// RecordOutcome captures a tool execution outcome
func (si *SelfImprover) RecordOutcome(task, tool string, success bool, duration time.Duration) error {
	// Record in learner
	if err := si.learner.Record(task, tool, success, duration, ""); err != nil {
		return err
	}

	// Update advisor recommendations
	if err := si.advisor.UpdateFromOutcome(task, tool, success); err != nil {
		return err
	}

	return nil
}

// GetAdvice returns a recommendation for which tools to use for a task
func (si *SelfImprover) GetAdvice(task string) string {
	return si.advisor.GetAdvice(task)
}

// LearnFromCorrection learns from user corrections to wrong tool choices
func (si *SelfImprover) LearnFromCorrection(wrong, correct string) error {
	// Store as a user preference
	pref := UserPreference{
		Key:        "correction:" + normalizeKey(wrong),
		Value:      correct,
		Confidence: 0.8,
	}
	
	return si.store.SetUserPreference(pref)
}

// LearnFromError learns from errors and their fixes
func (si *SelfImprover) LearnFromError(errorMessage, fix string) error {
	pattern := ErrorPattern{
		ErrorMessage:    errorMessage,
		Fix:             fix,
		OccurrenceCount: 1,
	}
	
	return si.store.AddErrorPattern(pattern)
}

// GetStats returns statistics about the system
func (si *SelfImprover) GetStats() map[string]interface{} {
	stats := make(map[string]interface{})

	// Learner stats
	stats["total_experiences"] = si.learner.GetTotalCount()

	// Knowledge store stats
	knowledgeStats := si.store.GetAllStats()
	for k, v := range knowledgeStats {
		stats[k] = v
	}

	// Recent activity
	recent := si.learner.GetRecentExperiences(10)
	recentCount := make(map[string]int)
	for _, exp := range recent {
		recentCount[exp.Tool]++
	}
	stats["recent_tool_usage"] = recentCount

	return stats
}

// GetErrorFix returns a suggested fix for a known error
func (si *SelfImprover) GetErrorFix(errorMessage string) (string, bool) {
	return si.advisor.GetErrorFix(errorMessage)
}

// LearnWorkflow learns a successful multi-step pattern
func (si *SelfImprover) LearnWorkflow(name string, steps []string, success bool) error {
	// Calculate success rate based on existing pattern
	var rate float64 = 1.0
	usageCount := 1
	if existing, ok := si.store.GetWorkflowPattern(name); ok {
		total := existing.UsageCount + 1
		rate = (existing.SuccessRate*float64(existing.UsageCount) + boolToFloat(success)) / float64(total)
		usageCount = total
	}

	pattern := WorkflowPattern{
		Name:        name,
		Steps:       steps,
		SuccessRate: rate,
		UsageCount:  usageCount,
	}

	return si.store.AddWorkflowPattern(pattern)
}

// GetDetailedStats returns detailed statistics including per-tool success rates
func (si *SelfImprover) GetDetailedStats() map[string]interface{} {
	stats := si.GetStats()

	// Add per-tool success rates for common tools
	tools := []string{"patch", "read_file", "write_file", "search_files"}
	toolStats := make(map[string]interface{})

	for _, tool := range tools {
		rate, count := si.learner.GetSuccessRate(tool, "")
		toolStats[tool] = map[string]interface{}{
			"success_rate": rate,
			"usage_count":  count,
			"avg_duration": si.learner.GetAverageDuration(tool, ""),
		}
	}

	stats["tool_statistics"] = toolStats

	return stats
}

// ClearHistory clears all learned data (use with caution)
func (si *SelfImprover) ClearHistory() error {
	if err := si.learner.Clear(); err != nil {
		return err
	}

	si.store.initialize()
	return si.store.save()
}

// GetRecentExperiences returns recent tool execution history
func (si *SelfImprover) GetRecentExperiences(count int) []Experience {
	return si.learner.GetRecentExperiences(count)
}

// SuggestWorkflow suggests a workflow for a complex task
func (si *SelfImprover) SuggestWorkflow(task string) (WorkflowPattern, bool) {
	taskWords := extractKeywords(task)
	
	for _, word := range taskWords {
		if wf, ok := si.store.GetWorkflowPattern(word); ok {
			return wf, true
		}
	}

	return WorkflowPattern{}, false
}