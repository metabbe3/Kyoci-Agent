package selfimprove

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// KnowledgeStore stores learned patterns in JSON format
type KnowledgeStore struct {
	data map[string]interface{}
	mu   sync.RWMutex
	path string
}

// NewKnowledgeStore creates a new KnowledgeStore
func NewKnowledgeStore(path string) *KnowledgeStore {
	ks := &KnowledgeStore{
		data: make(map[string]interface{}),
		path: path,
	}
	ks.initialize()
	ks.load()
	return ks
}

// initialize sets up the default knowledge structure
func (ks *KnowledgeStore) initialize() {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	ks.data = map[string]interface{}{
		"tool_patterns":    make(map[string]interface{}),
		"error_patterns":   make(map[string]interface{}),
		"user_preferences": make(map[string]interface{}),
		"workflow_patterns": make(map[string]interface{}),
	}
}

// ToolPattern tracks which tool works best for specific tasks
type ToolPattern struct {
	TaskPattern    string  `json:"task_pattern"`
	RecommendedTool string `json:"recommended_tool"`
	SuccessRate    float64 `json:"success_rate"`
	UsageCount     int     `json:"usage_count"`
}

// ErrorPattern tracks common failures and their fixes
type ErrorPattern struct {
	ErrorMessage string `json:"error_message"`
	Fix          string `json:"fix"`
	OccurrenceCount int `json:"occurrence_count"`
}

// UserPreference tracks learned preferences from corrections
type UserPreference struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	Confidence  float64 `json:"confidence"`
}

// WorkflowPattern tracks successful multi-step patterns
type WorkflowPattern struct {
	Name        string   `json:"name"`
	Steps       []string `json:"steps"`
	SuccessRate float64  `json:"success_rate"`
	UsageCount  int      `json:"usage_count"`
}

// AddToolPattern records a tool recommendation for a task
func (ks *KnowledgeStore) AddToolPattern(pattern ToolPattern) error {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	toolPatterns, ok := ks.data["tool_patterns"].(map[string]interface{})
	if !ok {
		toolPatterns = make(map[string]interface{})
		ks.data["tool_patterns"] = toolPatterns
	}

	key := pattern.TaskPattern
	patterns, ok := toolPatterns[key].([]interface{})
	if !ok {
		patterns = make([]interface{}, 0)
	}

	// Check if this tool already exists for this pattern
	updated := false
	for i, p := range patterns {
		if m, ok := p.(map[string]interface{}); ok {
			if rec, ok := m["recommended_tool"].(string); ok && rec == pattern.RecommendedTool {
				// Update existing entry
				m["success_rate"] = pattern.SuccessRate
				m["usage_count"] = pattern.UsageCount
				patterns[i] = m
				updated = true
				break
			}
		}
	}

	if !updated {
		patterns = append(patterns, map[string]interface{}{
			"task_pattern":     pattern.TaskPattern,
			"recommended_tool": pattern.RecommendedTool,
			"success_rate":     pattern.SuccessRate,
			"usage_count":      pattern.UsageCount,
		})
	}

	toolPatterns[key] = patterns
	return ks.save()
}

// GetToolPatterns returns all tool patterns for a task
func (ks *KnowledgeStore) GetToolPatterns(taskPattern string) []ToolPattern {
	ks.mu.RLock()
	defer ks.mu.RUnlock()

	toolPatterns, ok := ks.data["tool_patterns"].(map[string]interface{})
	if !ok {
		return nil
	}

	patterns, ok := toolPatterns[taskPattern].([]interface{})
	if !ok {
		return nil
	}

	result := make([]ToolPattern, 0)
	for _, p := range patterns {
		if m, ok := p.(map[string]interface{}); ok {
			tp := ToolPattern{
				TaskPattern:    getString(m, "task_pattern"),
				RecommendedTool: getString(m, "recommended_tool"),
				SuccessRate:    getFloat(m, "success_rate"),
				UsageCount:     getInt(m, "usage_count"),
			}
			result = append(result, tp)
		}
	}

	return result
}

// AddErrorPattern records an error and its fix
func (ks *KnowledgeStore) AddErrorPattern(pattern ErrorPattern) error {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	errorPatterns, ok := ks.data["error_patterns"].(map[string]interface{})
	if !ok {
		errorPatterns = make(map[string]interface{})
		ks.data["error_patterns"] = errorPatterns
	}

	key := normalizeKey(pattern.ErrorMessage)
	existing, ok := errorPatterns[key].(map[string]interface{})
	if !ok {
		existing = make(map[string]interface{})
		existing["error_message"] = pattern.ErrorMessage
		existing["fix"] = pattern.Fix
		existing["occurrence_count"] = 1
	} else {
		existing["occurrence_count"] = getInt(existing, "occurrence_count") + 1
		existing["fix"] = pattern.Fix
	}

	errorPatterns[key] = existing
	return ks.save()
}

// GetErrorPattern returns a fix for a known error
func (ks *KnowledgeStore) GetErrorPattern(errorMessage string) (ErrorPattern, bool) {
	ks.mu.RLock()
	defer ks.mu.RUnlock()

	errorPatterns, ok := ks.data["error_patterns"].(map[string]interface{})
	if !ok {
		return ErrorPattern{}, false
	}

	key := normalizeKey(errorMessage)
	m, ok := errorPatterns[key].(map[string]interface{})
	if !ok {
		return ErrorPattern{}, false
	}

	return ErrorPattern{
		ErrorMessage:     getString(m, "error_message"),
		Fix:              getString(m, "fix"),
		OccurrenceCount:  getInt(m, "occurrence_count"),
	}, true
}

// SetUserPreference stores a user preference learned from corrections
func (ks *KnowledgeStore) SetUserPreference(pref UserPreference) error {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	userPrefs, ok := ks.data["user_preferences"].(map[string]interface{})
	if !ok {
		userPrefs = make(map[string]interface{})
		ks.data["user_preferences"] = userPrefs
	}

	userPrefs[pref.Key] = map[string]interface{}{
		"value":     pref.Value,
		"confidence": pref.Confidence,
	}

	return ks.save()
}

// GetUserPreference retrieves a user preference
func (ks *KnowledgeStore) GetUserPreference(key string) (UserPreference, bool) {
	ks.mu.RLock()
	defer ks.mu.RUnlock()

	userPrefs, ok := ks.data["user_preferences"].(map[string]interface{})
	if !ok {
		return UserPreference{}, false
	}

	m, ok := userPrefs[key].(map[string]interface{})
	if !ok {
		return UserPreference{}, false
	}

	return UserPreference{
		Key:       key,
		Value:     getString(m, "value"),
		Confidence: getFloat(m, "confidence"),
	}, true
}

// AddWorkflowPattern records a successful multi-step workflow
func (ks *KnowledgeStore) AddWorkflowPattern(pattern WorkflowPattern) error {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	workflowPatterns, ok := ks.data["workflow_patterns"].(map[string]interface{})
	if !ok {
		workflowPatterns = make(map[string]interface{})
		ks.data["workflow_patterns"] = workflowPatterns
	}

	key := normalizeKey(pattern.Name)
	existing, ok := workflowPatterns[key].(map[string]interface{})
	if !ok {
		existing = make(map[string]interface{})
		existing["name"] = pattern.Name
		existing["steps"] = pattern.Steps
		existing["success_rate"] = pattern.SuccessRate
		existing["usage_count"] = 1
	} else {
		existing["success_rate"] = pattern.SuccessRate
		existing["usage_count"] = getInt(existing, "usage_count") + 1
	}

	workflowPatterns[key] = existing
	return ks.save()
}

// GetWorkflowPattern retrieves a workflow by name
func (ks *KnowledgeStore) GetWorkflowPattern(name string) (WorkflowPattern, bool) {
	ks.mu.RLock()
	defer ks.mu.RUnlock()

	workflowPatterns, ok := ks.data["workflow_patterns"].(map[string]interface{})
	if !ok {
		return WorkflowPattern{}, false
	}

	key := normalizeKey(name)
	m, ok := workflowPatterns[key].(map[string]interface{})
	if !ok {
		return WorkflowPattern{}, false
	}

	steps := make([]string, 0)
	if stepsData, ok := m["steps"].([]interface{}); ok {
		for _, s := range stepsData {
			if str, ok := s.(string); ok {
				steps = append(steps, str)
			}
		}
	}

	return WorkflowPattern{
		Name:        getString(m, "name"),
		Steps:       steps,
		SuccessRate: getFloat(m, "success_rate"),
		UsageCount:  getInt(m, "usage_count"),
	}, true
}

// GetAllStats returns statistics about the knowledge store
func (ks *KnowledgeStore) GetAllStats() map[string]interface{} {
	ks.mu.RLock()
	defer ks.mu.RUnlock()

	stats := make(map[string]interface{})

	if toolPatterns, ok := ks.data["tool_patterns"].(map[string]interface{}); ok {
		stats["tool_pattern_count"] = len(toolPatterns)
	}

	if errorPatterns, ok := ks.data["error_patterns"].(map[string]interface{}); ok {
		stats["error_pattern_count"] = len(errorPatterns)
	}

	if userPrefs, ok := ks.data["user_preferences"].(map[string]interface{}); ok {
		stats["user_preference_count"] = len(userPrefs)
	}

	if workflowPatterns, ok := ks.data["workflow_patterns"].(map[string]interface{}); ok {
		stats["workflow_pattern_count"] = len(workflowPatterns)
	}

	return stats
}

// save writes knowledge to disk
func (ks *KnowledgeStore) save() error {
	if ks.path == "" {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(ks.path), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(ks.data, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(ks.path, data, 0644)
}

// load reads knowledge from disk
func (ks *KnowledgeStore) load() error {
	if ks.path == "" {
		return nil
	}

	data, err := os.ReadFile(ks.path)
	if errors.Is(err, os.ErrNotExist) {
		ks.initialize()
		return ks.save()
	}
	if err != nil {
		return err
	}

	loaded := make(map[string]interface{})
	if err := json.Unmarshal(data, &loaded); err == nil {
		ks.data = loaded
	}

	// Ensure all categories exist
	categories := []string{"tool_patterns", "error_patterns", "user_preferences", "workflow_patterns"}
	for _, cat := range categories {
		if _, ok := ks.data[cat]; !ok {
			ks.data[cat] = make(map[string]interface{})
		}
	}

	return nil
}

// normalizeKey creates a normalized key for storage
func normalizeKey(s string) string {
	key := ""
	for _, ch := range s {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') {
			key += string(ch)
		} else if ch >= 'A' && ch <= 'Z' {
			key += string(ch + 32)
		}
	}
	return key
}

// Helper functions for safe type extraction
func getString(m map[string]interface{}, key string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return ""
}

func getFloat(m map[string]interface{}, key string) float64 {
	if val, ok := m[key].(float64); ok {
		return val
	}
	return 0.0
}

func getInt(m map[string]interface{}, key string) int {
	if val, ok := m[key].(float64); ok {
		return int(val)
	}
	return 0
}