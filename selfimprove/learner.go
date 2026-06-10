package selfimprove

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Experience represents a single tool execution outcome
type Experience struct {
	Timestamp time.Time `json:"timestamp"`
	Task      string    `json:"task"`
	Tool      string    `json:"tool"`
	Success   bool      `json:"success"`
	Duration  float64   `json:"duration"` // in seconds
	Details   string    `json:"details,omitempty"`
}

// ExperienceLearner captures tool outcomes and tracks patterns
type ExperienceLearner struct {
	experiences []Experience
	mu          sync.RWMutex
	storePath   string
}

// NewExperienceLearner creates a new ExperienceLearner
func NewExperienceLearner(storePath string) *ExperienceLearner {
	el := &ExperienceLearner{
		experiences: make([]Experience, 0),
		storePath:   storePath,
	}
	el.load()
	return el
}

// Record captures a tool execution outcome
func (el *ExperienceLearner) Record(task, tool string, success bool, duration time.Duration, details string) error {
	el.mu.Lock()
	defer el.mu.Unlock()

	exp := Experience{
		Timestamp: time.Now(),
		Task:      task,
		Tool:      tool,
		Success:   success,
		Duration:  duration.Seconds(),
		Details:   details,
	}

	el.experiences = append(el.experiences, exp)
	return el.save()
}

// GetSuccessRate returns the success rate for a given tool and task pattern
func (el *ExperienceLearner) GetSuccessRate(tool, taskPattern string) (float64, int) {
	el.mu.RLock()
	defer el.mu.RUnlock()

	if len(el.experiences) == 0 {
		return 0.0, 0
	}

	var matches, successes int
	for _, exp := range el.experiences {
		if tool == "" || exp.Tool == tool {
			if taskPattern == "" || containsTask(exp.Task, taskPattern) {
				matches++
				if exp.Success {
					successes++
				}
			}
		}
	}

	if matches == 0 {
		return 0.0, 0
	}
	return float64(successes) / float64(matches), matches
}

// GetAverageDuration returns the average duration for a given tool and task
func (el *ExperienceLearner) GetAverageDuration(tool, taskPattern string) float64 {
	el.mu.RLock()
	defer el.mu.RUnlock()

	var total float64
	var count int

	for _, exp := range el.experiences {
		if (tool == "" || exp.Tool == tool) && (taskPattern == "" || containsTask(exp.Task, taskPattern)) {
			total += exp.Duration
			count++
		}
	}

	if count == 0 {
		return 0
	}
	return total / float64(count)
}

// GetRecentExperiences returns the last n experiences for a tool
func (el *ExperienceLearner) GetRecentExperiences(n int) []Experience {
	el.mu.RLock()
	defer el.mu.RUnlock()

	if n <= 0 || n > len(el.experiences) {
		n = len(el.experiences)
	}

	start := len(el.experiences) - n
	return el.experiences[start:]
}

// GetTotalCount returns the total number of recorded experiences
func (el *ExperienceLearner) GetTotalCount() int {
	el.mu.RLock()
	defer el.mu.RUnlock()
	return len(el.experiences)
}

// save writes experiences to disk
func (el *ExperienceLearner) save() error {
	if el.storePath == "" {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(el.storePath), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(el.experiences, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(el.storePath, data, 0644)
}

// load reads experiences from disk
func (el *ExperienceLearner) load() error {
	if el.storePath == "" {
		return nil
	}

	data, err := os.ReadFile(el.storePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil // First run, no data yet
	}
	if err != nil {
		return err
	}

	return json.Unmarshal(data, &el.experiences)
}

// Clear removes all experiences
func (el *ExperienceLearner) Clear() error {
	el.mu.Lock()
	defer el.mu.Unlock()
	el.experiences = make([]Experience, 0)
	return el.save()
}

// containsTask checks if task contains pattern keywords
func containsTask(task, pattern string) bool {
	return len(pattern) == 0 || 
		   len(task) >= len(pattern) && 
		   (task == pattern || 
		    containsSubsequence(task, pattern))
}

// containsSubsequence checks if pattern keywords appear in task
func containsSubsequence(task, pattern string) bool {
	words := splitWords(pattern)
	for _, word := range words {
		if !containsWord(task, word) {
			return false
		}
	}
	return true
}

// containsWord checks if a word is present in text
func containsWord(text, word string) bool {
	return len(word) > 0 && 
	       (text == word || 
	        len(text) > len(word) && 
	        (text[:len(word)] == word || 
	         text[len(text)-len(word):] == word || 
	         containsSubstring(text, word)))
}

// containsSubstring checks for substring presence
func containsSubstring(text, substr string) bool {
	for i := 0; i <= len(text)-len(substr); i++ {
		if text[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// splitWords splits text into words
func splitWords(text string) []string {
	words := make([]string, 0)
	current := ""
	
	for _, ch := range text {
		if ch == ' ' || ch == ',' || ch == '.' || ch == '\t' || ch == '\n' {
			if len(current) > 0 {
				words = append(words, current)
				current = ""
			}
		} else {
			current += string(ch)
		}
	}
	if len(current) > 0 {
		words = append(words, current)
	}
	return words
}