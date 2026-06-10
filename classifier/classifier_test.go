package classifier

import (
	"fmt"
	"testing"
)

func TestClassifyLevel1(t *testing.T) {
	// Initialize default skills for testing
	InitializeDefaultSkills()

	tests := []struct {
		name           string
		input          string
		expectedLevel  ClassificationLevel
		expectedNeedsAI bool
	}{
		{"Greeting", "hello", LevelTrivial, false},
		{"Basic math", "what is 2+2", LevelTrivial, false},
		{"Time query", "what time is it", LevelTrivial, false},
		{"Acknowledgment", "thank you", LevelTrivial, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Classify(tt.input)
			if result.Level != tt.expectedLevel {
				t.Errorf("Expected level %d, got %d", tt.expectedLevel, result.Level)
			}
			if result.NeedsAI != tt.expectedNeedsAI {
				t.Errorf("Expected NeedsAI %v, got %v", tt.expectedNeedsAI, result.NeedsAI)
			}
		})
	}
}

func TestClassifyLevel2(t *testing.T) {
	InitializeDefaultSkills()

	tests := []struct {
		name           string
		input          string
		expectedLevel  ClassificationLevel
		expectedNeedsAI bool
	}{
		{"Format request", "format this as json", LevelSimple, false},
		{"Parse request", "parse this data", LevelSimple, false},
		{"Lookup request", "define algorithm", LevelSimple, false},
		{"Skill invocation", "use calculator to add 5 and 3", LevelSimple, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Classify(tt.input)
			if result.Level != tt.expectedLevel {
				t.Errorf("Expected level %d, got %d", tt.expectedLevel, result.Level)
			}
			if result.NeedsAI != tt.expectedNeedsAI {
				t.Errorf("Expected NeedsAI %v, got %v", tt.expectedNeedsAI, result.NeedsAI)
			}
		})
	}
}

func TestClassifyLevel3(t *testing.T) {
	InitializeDefaultSkills()

	tests := []struct {
		name           string
		input          string
		expectedLevel  ClassificationLevel
		expectedNeedsAI bool
	}{
		{"Summarize", "summarize this text about machine learning", LevelModerate, true},
		{"Explain", "explain how neural networks work", LevelModerate, true},
		{"Compare", "compare python and javascript", LevelModerate, true},
		{"Translate", "translate this to spanish", LevelModerate, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Classify(tt.input)
			if result.Level != tt.expectedLevel {
				t.Errorf("Expected level %d, got %d", tt.expectedLevel, result.Level)
			}
			if result.NeedsAI != tt.expectedNeedsAI {
				t.Errorf("Expected NeedsAI %v, got %v", tt.expectedNeedsAI, result.NeedsAI)
			}
		})
	}
}

func TestClassifyLevel4(t *testing.T) {
	InitializeDefaultSkills()

	tests := []struct {
		name           string
		input          string
		expectedLevel  ClassificationLevel
		expectedNeedsAI bool
	}{
		{"Debug", "debug this code that's not working", LevelComplex, true},
		{"Architecture", "design a scalable microservice architecture", LevelComplex, true},
		{"Analyze", "analyze this dataset and provide insights", LevelComplex, true},
		{"Optimize", "optimize this algorithm for better performance", LevelComplex, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Classify(tt.input)
			if result.Level != tt.expectedLevel {
				t.Errorf("Expected level %d, got %d", tt.expectedLevel, result.Level)
			}
			if result.NeedsAI != tt.expectedNeedsAI {
				t.Errorf("Expected NeedsAI %v, got %v", tt.expectedNeedsAI, result.NeedsAI)
			}
		})
	}
}

func TestClassifyLevel5(t *testing.T) {
	InitializeDefaultSkills()

	tests := []struct {
		name           string
		input          string
		expectedLevel  ClassificationLevel
		expectedNeedsAI bool
	}{
		{"Creative writing", "write a science fiction story about AI", LevelCritical, true},
		{"Research", "conduct deep research on quantum computing applications", LevelCritical, true},
		{"Multi-system", "design an end-to-end enterprise platform", LevelCritical, true},
		{"Strategic", "create a long-term AI integration roadmap", LevelCritical, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Classify(tt.input)
			if result.Level != tt.expectedLevel {
				t.Errorf("Expected level %d, got %d", tt.expectedLevel, result.Level)
			}
			if result.NeedsAI != tt.expectedNeedsAI {
				t.Errorf("Expected NeedsAI %v, got %v", tt.expectedNeedsAI, result.NeedsAI)
			}
		})
	}
}

func TestTokenBudget(t *testing.T) {
	tests := []struct {
		name             string
		level            ClassificationLevel
		expectedBudget   int
	}{
		{"Level 1", LevelTrivial, 0},
		{"Level 2", LevelSimple, 0},
		{"Level 3", LevelModerate, 1024},
		{"Level 4", LevelComplex, 4096},
		{"Level 5", LevelCritical, 8192},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &ClassificationResult{
				Level:       tt.level,
				TokenBudget: tt.expectedBudget,
			}
			if result.Budget() != tt.expectedBudget {
				t.Errorf("Expected budget %d, got %d", tt.expectedBudget, result.Budget())
			}
		})
	}
}

func TestRecommendedModel(t *testing.T) {
	tests := []struct {
		name               string
		level              ClassificationLevel
		expectedModel      string
	}{
		{"Level 1", LevelTrivial, ""},
		{"Level 2", LevelSimple, ""},
		{"Level 3", LevelModerate, "local/small"},
		{"Level 4", LevelComplex, "mid-tier"},
		{"Level 5", LevelCritical, "best"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &ClassificationResult{Level: tt.level}
			if result.RecommendedModel() != tt.expectedModel {
				t.Errorf("Expected model %q, got %q", tt.expectedModel, result.RecommendedModel())
			}
		})
	}
}

func TestSkillMatcher(t *testing.T) {
	registerSkill("test_skill", []string{"test", "ts"}, "A test skill", 1)
	defer unregisterSkill("test_skill")

	tests := []struct {
		name         string
		input        string
		expectedSkill string
	}{
		{"Exact match", "test_skill", "test_skill"},
		{"Prefix match", "test_skill with args", "test_skill"},
		{"Alias match", "test", "test_skill"},
		{"With use prefix", "use test", "test_skill"},
		{"No match", "random command", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skill := MatchSkill(tt.input)
			if skill != tt.expectedSkill {
				t.Errorf("Expected skill %q, got %q", tt.expectedSkill, skill)
			}
		})
	}
}

func ExampleClassify() {
	InitializeDefaultSkills()

	result := Classify("hello")
	fmt.Printf("Level: %d, NeedsAI: %v, Budget: %d\n",
		result.Level, result.NeedsAI, result.TokenBudget)

	result = Classify("write a function to sort an array")
	fmt.Printf("Level: %d, NeedsAI: %v, Budget: %d\n",
		result.Level, result.NeedsAI, result.TokenBudget)
}