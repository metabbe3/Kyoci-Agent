package selfimprove

import (
	"context"
	"testing"
	"time"
)

// TestSkillGenerator_DetectPatterns tests pattern detection
func TestSkillGenerator_DetectPatterns(t *testing.T) {
	learner := NewExperienceLearner("")
	
	// Add some test experiences
	testData := []struct {
		task    string
		tool    string
		success bool
	}{
		{"read file config.json", "read_file", true},
		{"read file settings.yaml", "read_file", true},
		{"read file data.txt", "read_file", true},
		{"write file output.json", "write_file", true},
		{"write file result.txt", "write_file", true},
		{"search for pattern regex", "search_files", true},
		{"invalid operation", "unknown_tool", false},
		{"read file error", "read_file", false},
	}

	for _, td := range testData {
		err := learner.Record(td.task, td.tool, td.success, time.Millisecond*100, "")
		if err != nil {
			t.Fatalf("failed to record experience: %v", err)
		}
	}

	// Create skill generator (router will be nil for this test)
	sg := NewSkillGenerator(nil, learner, "")

	// Test pattern detection
	candidates, err := sg.DetectPatterns(context.Background(), 2, 0.5)
	if err != nil {
		t.Fatalf("DetectPatterns failed: %v", err)
	}

	// Verify we found patterns
	if len(candidates) == 0 {
		t.Error("expected to find at least one pattern")
	}

	// Verify "read" pattern exists
	var readPattern *PatternCandidate
	for i := range candidates {
		if candidates[i].TaskPattern == "read" {
			readPattern = &candidates[i]
			break
		}
	}

	if readPattern == nil {
		t.Error("expected to find 'read' pattern")
	} else {
		if readPattern.Tool != "read_file" {
			t.Errorf("expected tool 'read_file', got '%s'", readPattern.Tool)
		}
		if readPattern.UsageCount < 3 {
			t.Errorf("expected usage count >= 3, got %d", readPattern.UsageCount)
		}
	}
}

// TestExtractTaskKeywords tests keyword extraction
func TestExtractTaskKeywords(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"read file config.json", []string{"read"}},
		{"write output to file", []string{"write"}},
		{"search and replace", []string{"search", "replace"}},
		{"create new resource", []string{"create"}},
		{"delete old entries", []string{"delete"}},
		{"update the configuration", []string{"update"}},
		{"install dependencies", []string{"install"}},
		{"analyze the data", []string{"analyze"}},
		{"build the project", []string{"build"}},
		{"deploy to production", []string{"deploy"}},
		{"empty or no action", []string{}},
	}

	for _, tt := range tests {
		result := extractTaskKeywords(tt.input)
		if len(result) != len(tt.expected) {
			t.Errorf("for '%s': expected %d keywords, got %d", tt.input, len(tt.expected), len(result))
			continue
		}
		for i, kw := range tt.expected {
			if i >= len(result) || result[i] != kw {
				t.Errorf("for '%s': expected keyword '%s' at position %d, got '%s'", tt.input, kw, i, result[i])
			}
		}
	}
}

// TestPatternCandidate tests PatternCandidate struct
func TestPatternCandidate(t *testing.T) {
	pattern := PatternCandidate{
		TaskPattern:  "read",
		Tool:         "read_file",
		SuccessRate:  0.9,
		UsageCount:   10,
		LastUsed:     time.Now(),
		Examples:     []string{"read file a", "read file b", "read file c"},
	}

	if pattern.TaskPattern != "read" {
		t.Errorf("expected TaskPattern 'read', got '%s'", pattern.TaskPattern)
	}
	if pattern.Tool != "read_file" {
		t.Errorf("expected Tool 'read_file', got '%s'", pattern.Tool)
	}
	if len(pattern.Examples) != 3 {
		t.Errorf("expected 3 examples, got %d", len(pattern.Examples))
	}
}

// TestGeneratedSkill tests GeneratedSkill struct
func TestGeneratedSkill(t *testing.T) {
	skill := GeneratedSkill{
		Name:        "test_skill",
		Description: "A test skill",
		Pattern:     `test.*pattern`,
		HandlerCode: "func handleTestSkill(ctx context.Context, input string) (string, error) { return \"test\", nil }",
		Confidence:  0.95,
	}

	if skill.Name != "test_skill" {
		t.Errorf("expected Name 'test_skill', got '%s'", skill.Name)
	}
	if skill.Confidence < 0.9 {
		t.Errorf("expected Confidence >= 0.9, got %f", skill.Confidence)
	}
	if skill.HandlerCode == "" {
		t.Error("expected non-empty HandlerCode")
	}
}

// TestCapitalize tests the capitalize helper function
func TestCapitalize(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"test", "Test"},
		{"Test", "Test"},
		{"", ""},
		{"a", "A"},
		{"helloWorld", "HelloWorld"},
	}

	for _, tt := range tests {
		result := capitalize(tt.input)
		if result != tt.expected {
			t.Errorf("capitalize('%s'): expected '%s', got '%s'", tt.input, tt.expected, result)
		}
	}
}

// TestExperienceLearner tests the ExperienceLearner
func TestExperienceLearner(t *testing.T) {
	learner := NewExperienceLearner("")

	// Test recording
	err := learner.Record("test task", "test_tool", true, time.Millisecond*100, "")
	if err != nil {
		t.Fatalf("failed to record: %v", err)
	}

	// Test getting total count
	count := learner.GetTotalCount()
	if count != 1 {
		t.Errorf("expected count 1, got %d", count)
	}

	// Test getting success rate
	rate, count := learner.GetSuccessRate("test_tool", "")
	if count != 1 {
		t.Errorf("expected count 1, got %d", count)
	}
	if rate != 1.0 {
		t.Errorf("expected success rate 1.0, got %f", rate)
	}

	// Test getting recent experiences
	recent := learner.GetRecentExperiences(10)
	if len(recent) != 1 {
		t.Errorf("expected 1 recent experience, got %d", len(recent))
	}
}

// TestKnowledgeStore tests the KnowledgeStore
func TestKnowledgeStore(t *testing.T) {
	store := NewKnowledgeStore("")

	// Test adding tool pattern
	pattern := ToolPattern{
		TaskPattern:     "read",
		RecommendedTool: "read_file",
		SuccessRate:     1.0,
		UsageCount:      5,
	}
	err := store.AddToolPattern(pattern)
	if err != nil {
		t.Fatalf("failed to add tool pattern: %v", err)
	}

	// Test getting tool patterns
	patterns := store.GetToolPatterns("read")
	if len(patterns) != 1 {
		t.Errorf("expected 1 pattern, got %d", len(patterns))
	}
	if patterns[0].RecommendedTool != "read_file" {
		t.Errorf("expected tool 'read_file', got '%s'", patterns[0].RecommendedTool)
	}

	// Test adding error pattern
	errorPattern := ErrorPattern{
		ErrorMessage: "file not found",
		Fix:          "Check file path",
		OccurrenceCount: 1,
	}
	err = store.AddErrorPattern(errorPattern)
	if err != nil {
		t.Fatalf("failed to add error pattern: %v", err)
	}

	// Test getting error pattern
	fix, ok := store.GetErrorPattern("file not found")
	if !ok {
		t.Error("expected to find error pattern")
	}
	if fix.Fix != "Check file path" {
		t.Errorf("expected fix 'Check file path', got '%s'", fix.Fix)
	}

	// Test adding workflow pattern
	workflow := WorkflowPattern{
		Name:        "read_analyze",
		Steps:       []string{"read_file", "analyze"},
		SuccessRate: 0.9,
		UsageCount:  3,
	}
	err = store.AddWorkflowPattern(workflow)
	if err != nil {
		t.Fatalf("failed to add workflow pattern: %v", err)
	}

	// Test getting workflow pattern
	wf, ok := store.GetWorkflowPattern("read_analyze")
	if !ok {
		t.Error("expected to find workflow pattern")
	}
	if len(wf.Steps) != 2 {
		t.Errorf("expected 2 steps, got %d", len(wf.Steps))
	}

	// Test getting stats
	stats := store.GetAllStats()
	if stats["tool_pattern_count"] != 1 {
		t.Errorf("expected tool_pattern_count 1, got %v", stats["tool_pattern_count"])
	}
}

// TestToolAdvisor tests the ToolAdvisor
func TestToolAdvisor(t *testing.T) {
	learner := NewExperienceLearner("")
	store := NewKnowledgeStore("")

	// Record some experiences
	learner.Record("read file", "read_file", true, time.Millisecond*100, "")
	learner.Record("read file", "read_file", true, time.Millisecond*100, "")
	learner.Record("read file", "read_file", false, time.Millisecond*100, "")

	advisor := NewToolAdvisor(learner, store)

	// Get advice
	advice := advisor.GetAdvice("read file config.json")
	if advice == "" {
		t.Error("expected non-empty advice")
	}

	// Test detailed advice
	detailed := advisor.GetDetailedAdvice("read file")
	if detailed == nil {
		t.Error("expected non-nil detailed advice")
	}

	// Test error fix
	err := advisor.UpdateFromOutcome("read file", "read_file", true)
	if err != nil {
		t.Errorf("failed to update from outcome: %v", err)
	}
}

// TestSelfImprover tests the SelfImprover
func TestSelfImprover(t *testing.T) {
	si := NewSelfImprover(t.TempDir())

	// Test recording outcome
	err := si.RecordOutcome("test task", "test_tool", true, time.Millisecond*100)
	if err != nil {
		t.Fatalf("failed to record outcome: %v", err)
	}

	// Test getting stats
	stats := si.GetStats()
	if stats["total_experiences"] != 1 {
		t.Errorf("expected total_experiences 1, got %v", stats["total_experiences"])
	}

	// Test getting detailed stats
	detailedStats := si.GetDetailedStats()
	if detailedStats["total_experiences"] != 1 {
		t.Errorf("expected total_experiences 1 in detailed stats, got %v", detailedStats["total_experiences"])
	}

	// Test learning from correction
	err = si.LearnFromCorrection("wrong_tool", "correct_tool")
	if err != nil {
		t.Errorf("failed to learn from correction: %v", err)
	}

	// Test learning from error
	err = si.LearnFromError("test error", "test fix")
	if err != nil {
		t.Errorf("failed to learn from error: %v", err)
	}

	// Test learning workflow
	err = si.LearnWorkflow("test_workflow", []string{"step1", "step2"}, true)
	if err != nil {
		t.Errorf("failed to learn workflow: %v", err)
	}

	// Test getting recent experiences
	recent := si.GetRecentExperiences(10)
	if len(recent) != 1 {
		t.Errorf("expected 1 recent experience, got %d", len(recent))
	}
}

// TestValidator tests the Validator
func TestValidator(t *testing.T) {
	_ = NewValidator(".")
	// Test ValidationResult structure
	result := &ValidationResult{
		Valid:       true,
		Errors:      []string{},
		Warnings:    []string{},
		TestsPassed: 10,
		TestsFailed: 0,
		Coverage:    80.5,
		LintScore:   95,
	}

	if !result.Valid {
		t.Error("expected Valid to be true")
	}
	if result.TestsPassed != 10 {
		t.Errorf("expected TestsPassed 10, got %d", result.TestsPassed)
	}
	if result.LintScore != 95 {
		t.Errorf("expected LintScore 95, got %d", result.LintScore)
	}
}

// TestImprovementJob tests the ImprovementJob struct
func TestImprovementJob(t *testing.T) {
	job := &ImprovementJob{
		ID:           "test-job-1",
		Task:         "Test improvement",
		Phase:        PhaseComplete,
		RiskLevel:    "low",
		FilesChanged: []string{"test.go"},
		PRAvailable:  true,
		PRURL:        "http://example.com/pr/1",
		CreatedAt:    time.Now(),
	}

	if job.ID != "test-job-1" {
		t.Errorf("expected ID 'test-job-1', got '%s'", job.ID)
	}
	if job.Phase != PhaseComplete {
		t.Errorf("expected Phase %s, got %s", PhaseComplete, job.Phase)
	}
	if len(job.FilesChanged) != 1 {
		t.Errorf("expected 1 changed file, got %d", len(job.FilesChanged))
	}
}

// TestImprovementPhase constants
func TestImprovementPhase(t *testing.T) {
	phases := []ImprovementPhase{
		PhaseSearch,
		PhasePlan,
		PhaseExecute,
		PhaseValidate,
		PhaseReview,
		PhasePR,
		PhaseComplete,
		PhaseFailed,
	}

	expected := []string{"SEARCH", "PLAN", "EXECUTE", "VALIDATE", "REVIEW", "PR", "COMPLETE", "FAILED"}

	for i, phase := range phases {
		if string(phase) != expected[i] {
			t.Errorf("expected phase '%s', got '%s'", expected[i], phase)
		}
	}
}