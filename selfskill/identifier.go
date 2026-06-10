package selfskill

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

// TaskPattern represents a recurring task pattern.
type TaskPattern struct {
	Pattern    string    `json:"pattern"`
	Frequency  int       `json:"frequency"`
	LastSeen   time.Time `json:"last_seen"`
	Examples   []string  `json:"examples"`
	Complexity int       `json:"complexity"`
}

// Identifier detects recurring task patterns that could become skills.
type Identifier struct {
	patterns  map[string]*TaskPattern
	mu        sync.RWMutex
	threshold int
}

// NewIdentifier creates a new pattern identifier.
func NewIdentifier(detectionThreshold int) *Identifier {
	return &Identifier{
		patterns:  make(map[string]*TaskPattern),
		threshold: detectionThreshold,
	}
}

// Record records a task and updates pattern frequencies.
func (id *Identifier) Record(task string) {
	id.mu.Lock()
	defer id.mu.Unlock()

	pattern := id.DetectPattern(task)
	if pattern == "" {
		return
	}

	tp, exists := id.patterns[pattern]
	if !exists {
		tp = &TaskPattern{
			Pattern:    pattern,
			Frequency:  0,
			LastSeen:   time.Now(),
			Examples:   make([]string, 0, 5),
			Complexity: id.calculateComplexity(task),
		}
		id.patterns[pattern] = tp
	}

	tp.Frequency++
	tp.LastSeen = time.Now()

	// Keep up to 5 examples
	if len(tp.Examples) < 5 {
		tp.Examples = append(tp.Examples, task)
	} else {
		// Rotate examples
		tp.Examples = append(tp.Examples[1:], task)
	}
}

// DetectPattern classifies a task into a pattern key.
// Uses rule-based detection: normalize task, hash first 3 words.
func (id *Identifier) DetectPattern(task string) string {
	// Normalize: lowercase, remove numbers, collapse whitespace
	normalized := strings.ToLower(task)
	re := regexp.MustCompile(`\d+`)
	normalized = re.ReplaceAllString(normalized, " ")
	re = regexp.MustCompile(`\s+`)
	normalized = re.ReplaceAllString(normalized, " ")
	normalized = strings.TrimSpace(normalized)

	if normalized == "" {
		return ""
	}

	// Get first 3 words
	words := strings.Fields(normalized)
	if len(words) == 0 {
		return ""
	}

	var patternWords []string
	for i := 0; i < len(words) && i < 3; i++ {
		patternWords = append(patternWords, words[i])
	}

	return strings.Join(patternWords, " ")
}

// GetCandidates returns patterns that have reached the threshold.
func (id *Identifier) GetCandidates() []TaskPattern {
	id.mu.RLock()
	defer id.mu.RUnlock()

	var candidates []TaskPattern
	for _, tp := range id.patterns {
		if tp.Frequency >= id.threshold {
			candidates = append(candidates, *tp)
		}
	}
	return candidates
}

// LoadHistory loads pattern history from a JSON file.
func (id *Identifier) LoadHistory(path string) error {
	id.mu.Lock()
	defer id.mu.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File not found is OK
		}
		return err
	}

	var patterns map[string]*TaskPattern
	if err := json.Unmarshal(data, &patterns); err != nil {
		return err
	}

	id.patterns = patterns
	return nil
}

// SaveHistory saves pattern history to a JSON file.
func (id *Identifier) SaveHistory(path string) error {
	id.mu.RLock()
	defer id.mu.RUnlock()

	data, err := json.MarshalIndent(id.patterns, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// calculateComplexity estimates pattern complexity based on task length and structure.
func (id *Identifier) calculateComplexity(task string) int {
	words := strings.Fields(task)
	if len(words) <= 3 {
		return 1
	}
	if len(words) <= 7 {
		return 2
	}
	if len(words) <= 12 {
		return 3
	}
	return 4
}