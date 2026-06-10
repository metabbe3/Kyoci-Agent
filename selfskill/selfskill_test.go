package selfskill

import (
	"testing"
	"time"

	"github.com/nicholas/ai-agent/skill"
)

func TestNewIdentifier(t *testing.T) {
	id := NewIdentifier(5)
	if id == nil {
		t.Fatal("NewIdentifier returned nil")
	}
	if id.threshold != 5 {
		t.Errorf("Expected threshold 5, got %d", id.threshold)
	}
	if id.patterns == nil {
		t.Error("patterns map not initialized")
	}
}

func TestDetectPattern(t *testing.T) {
	id := NewIdentifier(3)

	tests := []struct {
		input    string
		expected string
	}{
		{"create file test", "create file test"},
		{"Create File Test", "create file test"},
		{"Create 123 Files Test", "create files test"},
		{"  create   multiple   spaces  ", "create multiple spaces"},
		{"short", "short"},
		{"", ""},
	}

	for _, tt := range tests {
		result := id.DetectPattern(tt.input)
		if result != tt.expected {
			t.Errorf("DetectPattern(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestRecordAndGetCandidates(t *testing.T) {
	id := NewIdentifier(3)

	// Record tasks below threshold
	id.Record("create file test1")
	id.Record("create file test2")

	candidates := id.GetCandidates()
	if len(candidates) != 0 {
		t.Errorf("Expected 0 candidates, got %d", len(candidates))
	}

	// Record one more to reach threshold
	id.Record("create file test3")

	candidates = id.GetCandidates()
	if len(candidates) != 1 {
		t.Errorf("Expected 1 candidate, got %d", len(candidates))
	}
	if candidates[0].Pattern != "create file test" {
		t.Errorf("Expected pattern 'create file test', got %q", candidates[0].Pattern)
	}
	if candidates[0].Frequency != 3 {
		t.Errorf("Expected frequency 3, got %d", candidates[0].Frequency)
	}
}

func TestNewSkillGenerator(t *testing.T) {
	id := NewIdentifier(3)
	reg := skill.NewRegistry()
	gen := NewSkillGenerator(id, "./output", reg)

	if gen == nil {
		t.Fatal("NewSkillGenerator returned nil")
	}
	if gen.identifier != id {
		t.Error("identifier not set correctly")
	}
	if gen.outputDir != "./output" {
		t.Errorf("Expected outputDir './output', got %q", gen.outputDir)
	}
	if gen.registry != reg {
		t.Error("registry not set correctly")
	}
}

func TestDeriveSkillName(t *testing.T) {
	id := NewIdentifier(3)
	reg := skill.NewRegistry()
	gen := NewSkillGenerator(id, "./output", reg)

	tests := []struct {
		pattern string
		name    string
	}{
		{"create file", "create_file"},
		{"get weather", "get_weather"},
		{"send message", "send_message"},
	}

	for _, tt := range tests {
		result := gen.deriveSkillName(tt.pattern)
		if result != tt.name {
			t.Errorf("deriveSkillName(%q) = %q, want %q", tt.pattern, result, tt.name)
		}
	}
}

func TestNewSkillLifecycle(t *testing.T) {
	id := NewIdentifier(3)
	reg := skill.NewRegistry()
	gen := NewSkillGenerator(id, "./output", reg)
	lc := NewSkillLifecycle(gen, id)

	if lc == nil {
		t.Fatal("NewSkillLifecycle returned nil")
	}
	if lc.generator != gen {
		t.Error("generator not set correctly")
	}
	if lc.identifier != id {
		t.Error("identifier not set correctly")
	}
}

func TestLifecycleStatus(t *testing.T) {
	id := NewIdentifier(3)
	reg := skill.NewRegistry()
	gen := NewSkillGenerator(id, "./output", reg)
	lc := NewSkillLifecycle(gen, id)

	status := lc.Status()
	if status.Candidates != 0 || status.Generated != 0 || status.Failed != 0 || status.Active != 0 {
		t.Error("New lifecycle should have zero status")
	}

	// Record some tasks
	id.Record("test pattern one")
	id.Record("test pattern two")
	id.Record("test pattern three")

	lc.updateStatus(func(s *LifecycleStatus) {
		s.Candidates = 3
	})

	status = lc.Status()
	if status.Candidates != 3 {
		t.Errorf("Expected Candidates=3, got %d", status.Candidates)
	}
}

func TestCalculateComplexity(t *testing.T) {
	id := NewIdentifier(3)

	tests := []struct {
		task       string
		complexity int
	}{
		{"short task", 1},
		{"a medium length task with words", 2},
		{"this is a longer task that has even more words than before", 3},
		{"this is a very long task that contains many words and exceeds the normal length significantly", 4},
	}

	for _, tt := range tests {
		result := id.calculateComplexity(tt.task)
		if result != tt.complexity {
			t.Errorf("calculateComplexity(%q) = %d, want %d", tt.task, result, tt.complexity)
		}
	}
}

func TestSuggestFromPattern(t *testing.T) {
	id := NewIdentifier(3)
	reg := skill.NewRegistry()
	gen := NewSkillGenerator(id, "./output", reg)

	pattern := &TaskPattern{
		Pattern:    "create file",
		Frequency:  5,
		LastSeen:   time.Now(),
		Examples:   []string{"create file test", "create file readme", "create file config"},
		Complexity: 2,
	}

	spec := gen.SuggestFromPattern(pattern)
	if spec == nil {
		t.Fatal("SuggestFromPattern returned nil")
	}

	if spec.Name == "" {
		t.Error("spec.Name should not be empty")
	}
	if spec.Pattern == "" {
		t.Error("spec.Pattern should not be empty")
	}
	if spec.PackageName != "skill" {
		t.Errorf("Expected PackageName 'skill', got %q", spec.PackageName)
	}
	if spec.ReturnType != "string" {
		t.Errorf("Expected ReturnType 'string', got %q", spec.ReturnType)
	}
}