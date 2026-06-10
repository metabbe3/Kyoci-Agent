package classifier

import (
	"regexp"
	"strings"
	"unicode"
)

// ClassificationLevel represents the complexity level (1-5)
type ClassificationLevel int

const (
	LevelTrivial  ClassificationLevel = 1
	LevelSimple   ClassificationLevel = 2
	LevelModerate ClassificationLevel = 3
	LevelComplex  ClassificationLevel = 4
	LevelCritical ClassificationLevel = 5
)

// ClassificationResult contains the analysis result
type ClassificationResult struct {
	Level            ClassificationLevel
	Category         string
	NeedsAI          bool
	TokenBudget      int
	SuggestedSkill   string
	Reasoning        []string
	MatchedPatterns  []string
	InputLength      int
	WordCount        int
}

// Classify analyzes input and returns classification result
func Classify(input string) *ClassificationResult {
	input = strings.TrimSpace(input)
	if input == "" {
		return &ClassificationResult{
			Level:       LevelTrivial,
			Category:    "empty",
			NeedsAI:     false,
			TokenBudget: 0,
			Reasoning:   []string{"Empty input"},
			WordCount:   0,
		}
	}

	result := &ClassificationResult{
		InputLength:     len(input),
		WordCount:       countWords(input),
		MatchedPatterns: []string{},
	}

	// Step 1: Check for skill matches (exact or near-exact)
	if skill := MatchSkill(input); skill != "" {
		result.Level = LevelSimple
		result.Category = "skill_invocation"
		result.SuggestedSkill = skill
		result.NeedsAI = false
		result.TokenBudget = 0
		result.Reasoning = append(result.Reasoning, "Exact skill match detected")
		return result
	}

	// Step 2: Evaluate each level (use the package-level helper from patterns.go)
	if patterns, category := MatchesPatterns(input, Level1Patterns); len(patterns) > 0 {
		result.Level = LevelTrivial
		result.Category = category
		result.MatchedPatterns = patterns
		result.NeedsAI = false
		result.TokenBudget = 0
		result.Reasoning = append(result.Reasoning, "Level 1 patterns matched")
		return result
	}

	if patterns, category := MatchesPatterns(input, Level2Patterns); len(patterns) > 0 {
		result.Level = LevelSimple
		result.Category = category
		result.MatchedPatterns = patterns
		result.NeedsAI = false
		result.TokenBudget = 0
		result.Reasoning = append(result.Reasoning, "Level 2 patterns matched")
		return result
	}

	if patterns, category := MatchesPatterns(input, Level3Patterns); len(patterns) > 0 {
		result.Level = LevelModerate
		result.Category = category
		result.MatchedPatterns = patterns
		result.NeedsAI = true
		result.TokenBudget = 1024
		result.Reasoning = append(result.Reasoning, "Level 3 patterns matched")
		return result
	}

	if patterns, category := MatchesPatterns(input, Level4Patterns); len(patterns) > 0 {
		result.Level = LevelComplex
		result.Category = category
		result.MatchedPatterns = patterns
		result.NeedsAI = true
		result.TokenBudget = 4096
		result.Reasoning = append(result.Reasoning, "Level 4 patterns matched")
		return result
	}

	if patterns, category := MatchesPatterns(input, Level5Patterns); len(patterns) > 0 {
		result.Level = LevelCritical
		result.Category = category
		result.MatchedPatterns = patterns
		result.NeedsAI = true
		result.TokenBudget = 8192
		result.Reasoning = append(result.Reasoning, "Level 5 patterns matched")
		return result
	}

	// Step 3: Apply heuristics if no explicit pattern matches
	result = applyHeuristics(result, input)
	return result
}

// NeedsAICheck returns true if the classification requires AI processing
func (c *ClassificationResult) NeedsAICheck() bool {
	return c.NeedsAI
}

// IsAIRequired returns true if the classification requires AI processing
func (c *ClassificationResult) IsAIRequired() bool {
	return c.NeedsAI
}

// RecommendedModel returns the recommended model based on level
func (c *ClassificationResult) RecommendedModel() string {
	switch c.Level {
	case LevelTrivial, LevelSimple:
		return "" // No AI needed
	case LevelModerate:
		return "local/small"
	case LevelComplex:
		return "mid-tier"
	case LevelCritical:
		return "best"
	default:
		return ""
	}
}

// GetTokenBudget returns the token budget based on level
func (c *ClassificationResult) GetTokenBudget() int {
	return c.TokenBudget
}

// Budget returns the token budget based on level
func (c *ClassificationResult) Budget() int {
	return c.TokenBudget
}

// Helper: count words in input
func countWords(input string) int {
	count := 0
	inWord := false
	for _, r := range input {
		if unicode.IsSpace(r) {
			inWord = false
		} else if !inWord {
			inWord = true
			count++
		}
	}
	return count
}

// Helper: check if input contains code
func containsCode(input string) bool {
	codeIndicators := []string{
		"func ", "func(", "def ", "class ", "function ", "return ",
		"var ", "let ", "const ", "import ", "package ",
		"{", "}", "if (", "for (", "while (", "=>",
		";", ":", "=", "+", "-", "*", "/",
	}
	inputLower := strings.ToLower(input)
	for _, indicator := range codeIndicators {
		if strings.Contains(input, indicator) || strings.Contains(inputLower, strings.ToLower(indicator)) {
			return true
		}
	}
	return false
}

// Helper: check if input requires tools
func requiresTools(input string) bool {
	toolKeywords := []string{
		"search", "file", "read", "write", "open", "close",
		"execute", "run", "command", "terminal", "browser",
		"scrape", "fetch", "http", "api", "database", "sql",
		"email", "schedule", "calendar", "image", "vision",
	}
	inputLower := strings.ToLower(input)
	for _, keyword := range toolKeywords {
		if strings.Contains(inputLower, keyword) {
			return true
		}
	}
	return false
}

// Helper: detect question type
func detectQuestionType(input string) string {
	questionPatterns := map[string]string{
		`(?i)^(what|who|where|when|why|how)\s+\w+`:       "informational",
		`(?i)^(can|could|would|should|will|do|does|did)\s+`: "capability",
		`(?i)^(is|are|was|were)\s+`:                      "factual",
		`(?i)^(explain|describe|tell|summarize)\s+`:       "explanatory",
		`(?i)^(create|write|generate|build|make)\s+`:      "generative",
		`(?i)^(fix|debug|solve|resolve)\s+`:               "problem-solving",
	}
	for pattern, qType := range questionPatterns {
		if matched, _ := regexp.MatchString(pattern, input); matched {
			return qType
		}
	}
	return "unknown"
}

// Helper: apply heuristics
func applyHeuristics(result *ClassificationResult, input string) *ClassificationResult {
	wordCount := result.WordCount
	_ = wordCount // used below
	hasCode := containsCode(input)
	needsTools := requiresTools(input)
	questionType := detectQuestionType(input)

	// Heuristic rules
	if hasCode && needsTools {
		result.Level = LevelComplex
		result.Category = "code_with_tools"
		result.NeedsAI = true
		result.TokenBudget = 4096
		result.Reasoning = append(result.Reasoning, "Code requiring tools detected")
	} else if hasCode {
		result.Level = LevelModerate
		result.Category = "code_only"
		result.NeedsAI = true
		result.TokenBudget = 1024
		result.Reasoning = append(result.Reasoning, "Code detected")
	} else if needsTools {
		result.Level = LevelSimple
		result.Category = "tool_invocation"
		result.NeedsAI = false
		result.TokenBudget = 0
		result.Reasoning = append(result.Reasoning, "Tool usage detected")
	} else if wordCount > 100 {
		result.Level = LevelComplex
		result.Category = "long_input"
		result.NeedsAI = true
		result.TokenBudget = 4096
		result.Reasoning = append(result.Reasoning, "Long input (>100 words)")
	} else if wordCount > 30 {
		result.Level = LevelModerate
		result.Category = "moderate_input"
		result.NeedsAI = true
		result.TokenBudget = 1024
		result.Reasoning = append(result.Reasoning, "Moderate input (>30 words)")
	} else if questionType == "informational" || questionType == "capability" {
		result.Level = LevelSimple
		result.Category = "simple_question"
		result.NeedsAI = false
		result.TokenBudget = 0
		result.Reasoning = append(result.Reasoning, "Simple question type")
	} else if questionType == "problem-solving" || questionType == "explanatory" {
		result.Level = LevelModerate
		result.Category = "complex_question"
		result.NeedsAI = true
		result.TokenBudget = 1024
		result.Reasoning = append(result.Reasoning, "Complex question type")
	} else if questionType == "generative" {
		result.Level = LevelCritical
		result.Category = "creative_generation"
		result.NeedsAI = true
		result.TokenBudget = 8192
		result.Reasoning = append(result.Reasoning, "Generative request")
	} else {
		// Default to moderate for unknown inputs
		result.Level = LevelModerate
		result.Category = "general"
		result.NeedsAI = true
		result.TokenBudget = 1024
		result.Reasoning = append(result.Reasoning, "Default classification")
	}

	return result
}