package thinking

import (
	"context"
	"fmt"
	"testing"
)

// MockExecutor is a test helper that simulates AI responses
type MockExecutor struct {
	responses []string
	index     int
}

func NewMockExecutor(responses []string) *MockExecutor {
	return &MockExecutor{
		responses: responses,
		index:     0,
	}
}

func (m *MockExecutor) Execute(ctx context.Context, prompt string) (string, error) {
	if m.index >= len(m.responses) {
		return "Mock response", nil
	}
	response := m.responses[m.index]
	m.index++
	return response, nil
}

func TestNewReasoningEngine(t *testing.T) {
	engine := NewReasoningEngine()
	if engine == nil {
		t.Fatal("Expected engine to be created")
	}
	if engine.maxSteps != 4 {
		t.Errorf("Expected maxSteps to be 4, got %d", engine.maxSteps)
	}
	if engine.confidenceThreshold != 0.7 {
		t.Errorf("Expected confidenceThreshold to be 0.7, got %f", engine.confidenceThreshold)
	}
}

func TestReasoningEngine_Think(t *testing.T) {
	responses := []string{
		"Task decomposition:\n1. Research the topic\n2. Write code\n3. Test the code\n4. Deploy",
		"Plan:\n1. Research using web search\n2. Write Go code in main.go\n3. Run tests\n4. Deploy to production",
		"Review: The plan looks good with high confidence. No errors found.",
		"Final synthesis: Complete solution ready for execution.",
	}

	engine := NewReasoningEngine()
	mock := NewMockExecutor(responses)

	result, err := engine.Think(context.Background(), "Build a web app", mock.Execute)
	if err != nil {
		t.Fatalf("Think() failed: %v", err)
	}

	if result == nil {
		t.Fatal("Expected result to not be nil")
	}

	if len(result.Steps) != 4 {
		t.Errorf("Expected 4 steps, got %d", len(result.Steps))
	}

	expectedPhases := []string{"Deconstruct", "Strategic Planning", "Self-Correction", "Synthesis"}
	for i, step := range result.Steps {
		if step.Phase != expectedPhases[i] {
			t.Errorf("Step %d: expected phase %s, got %s", i, expectedPhases[i], step.Phase)
		}
	}

	if result.Conclusion == "" {
		t.Error("Expected conclusion to be non-empty")
	}
}

func TestDetectAmbiguity(t *testing.T) {
	tests := []struct {
		name     string
		task     string
		wantNil  bool
		question string
	}{
		{
			name:     "ambiguous reference",
			task:     "Test the file",
			wantNil:  false,
			question: "What files, directories, or components should be targeted?",
		},
		{
			name:     "vague quantifier - some",
			task:     "Create some tests",
			wantNil:  false,
			question: "How many? What quantity?",
		},
		{
			name:     "vague quantifier - better",
			task:     "Make it better",
			wantNil:  false,
			question: "Better than what? What's the baseline?",
		},
		{
			name:     "no ambiguity",
			task:     "Run go test ./...",
			wantNil:  true,
			question: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := DetectAmbiguity(tt.task)
			if tt.wantNil {
				if request != nil {
					t.Errorf("Expected nil, got %+v", request)
				}
				return
			}

			if request == nil {
				t.Fatal("Expected request to not be nil")
			}

			found := false
			for _, q := range request.Questions {
				if q == tt.question {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected question %q, got %v", tt.question, request.Questions)
			}
		})
	}
}

func TestNewPlan(t *testing.T) {
	steps := []PlanStep{
		{Number: 1, Action: "Research", Tool: "web-search", ExpectedResult: "Documentation"},
		{Number: 2, Action: "Implement", Tool: "go-build", ExpectedResult: "Binary"},
	}

	plan := NewPlan("Build app", steps)

	if plan.Task != "Build app" {
		t.Errorf("Expected task 'Build app', got %s", plan.Task)
	}

	if plan.Status != "pending" {
		t.Errorf("Expected status 'pending', got %s", plan.Status)
	}

	if len(plan.Steps) != 2 {
		t.Errorf("Expected 2 steps, got %d", len(plan.Steps))
	}

	if plan.ID == "" {
		t.Error("Expected ID to be generated")
	}
}

func TestPlan_Format(t *testing.T) {
	steps := []PlanStep{
		{Number: 1, Action: "Step one", Tool: "tool1", ExpectedResult: "result1"},
	}
	plan := NewPlan("Test task", steps)

	formatted := plan.Format()
	if formatted == "" {
		t.Error("Expected formatted string to be non-empty")
	}

	expectedSubstrings := []string{"Test task", "Step one", "tool1", "result1"}
	for _, substr := range expectedSubstrings {
		if !contains(formatted, substr) {
			t.Errorf("Expected formatted output to contain %q", substr)
		}
	}
}

func TestPlan_ToJSON(t *testing.T) {
	steps := []PlanStep{
		{Number: 1, Action: "Test action"},
	}
	plan := NewPlan("Test task", steps)

	jsonStr, err := plan.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON() failed: %v", err)
	}

	if jsonStr == "" {
		t.Error("Expected JSON to be non-empty")
	}

	expectedFields := []string{"id", "task", "steps", "status", "created_at"}
	for _, field := range expectedFields {
		if !contains(jsonStr, field) {
			t.Errorf("Expected JSON to contain field %q", field)
		}
	}
}

func TestPlan_Approve(t *testing.T) {
	steps := []PlanStep{{Number: 1, Action: "Test"}}
	plan := NewPlan("Test", steps)

	if !plan.IsPending() {
		t.Error("Expected plan to be pending initially")
	}

	plan.Approve("user123")

	if !plan.IsApproved() {
		t.Error("Expected plan to be approved after Approve()")
	}

	if plan.ApprovedBy != "user123" {
		t.Errorf("Expected ApprovedBy to be 'user123', got %s", plan.ApprovedBy)
	}
}

func TestParsePlanFromAI(t *testing.T) {
	tests := []struct {
		name     string
		response string
		wantNil  bool
	}{
		{
			name: "numbered steps",
			response: `1. Research the topic [tool: search] [expected: documentation]
2. Write code [tool: editor] [expected: source files]
3. Test [tool: test-runner] [expected: test results]`,
			wantNil: false,
		},
		{
			name: "bullet points",
			response: `- Setup project | tool: git | expected: initialized repo
- Write code | tool: editor | expected: main.go
- Run tests | tool: go test | expected: pass`,
			wantNil: false,
		},
		{
			name:     "no plan structure",
			response: "This is just some text without plan structure",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := ParsePlanFromAI(tt.response)
			if tt.wantNil {
				if plan != nil {
					t.Errorf("Expected nil, got plan with %d steps", len(plan.Steps))
				}
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("ParsePlanFromAI() failed: %v", err)
			}

			if len(plan.Steps) == 0 {
				t.Error("Expected at least one step")
			}
		})
	}
}

func TestGenerateQuestions(t *testing.T) {
	responses := []string{
		"What specific file should be tested?\nWhat type of tests are needed?\nWhat coverage threshold should be met?",
	}

	mock := &MockExecutor{
		responses: responses,
	}

	questions, err := GenerateQuestions("Test the code", func(prompt string) (string, error) {
		return mock.Execute(context.Background(), prompt)
	})

	if err != nil {
		t.Fatalf("GenerateQuestions() failed: %v", err)
	}

	if len(questions) < 3 {
		t.Errorf("Expected at least 3 questions, got %d", len(questions))
	}

	expected := "What specific file should be tested?"
	found := false
	for _, q := range questions {
		if q == expected {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected question %q not found in %v", expected, questions)
	}
}

func TestClarificationRequest_Format(t *testing.T) {
	request := &ClarificationRequest{
		Task: "Test the file",
		UncertainAspects: []string{"Target not specified"},
		Questions: []string{"What file should be tested?"},
		SuggestedOptions: map[string][]string{
			"file": {"main.go", "all.go files"},
		},
	}

	formatted := request.Format()
	if formatted == "" {
		t.Error("Expected formatted string to be non-empty")
	}

	expectedSubstrings := []string{"Clarification Needed", "Test the file", "What file should be tested?", "Suggested Options"}
	for _, substr := range expectedSubstrings {
		if !contains(formatted, substr) {
			t.Errorf("Expected formatted output to contain %q", substr)
		}
	}
}

func TestClarificationRequest_NeedsClarification(t *testing.T) {
	tests := []struct {
		name string
		cr   *ClarificationRequest
		want bool
	}{
		{
			name: "nil request",
			cr:   nil,
			want: false,
		},
		{
			name: "request with questions",
			cr: &ClarificationRequest{
				Task:      "Test",
				Questions: []string{"What file?"},
			},
			want: true,
		},
		{
			name: "request without questions",
			cr: &ClarificationRequest{
				Task: "Test",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cr.NeedsClarification(); got != tt.want {
				t.Errorf("NeedsClarification() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClarificationRequest_AddQuestion(t *testing.T) {
	cr := &ClarificationRequest{
		Task: "Test",
	}

	if len(cr.Questions) != 0 {
		t.Errorf("Expected 0 questions initially, got %d", len(cr.Questions))
	}

	cr.AddQuestion("Question 1")
	cr.AddQuestion("Question 2")

	if len(cr.Questions) != 2 {
		t.Errorf("Expected 2 questions, got %d", len(cr.Questions))
	}

	if cr.Questions[0] != "Question 1" {
		t.Errorf("Expected first question to be 'Question 1', got %s", cr.Questions[0])
	}
}

func TestClarificationRequest_AddSuggestedOptions(t *testing.T) {
	cr := &ClarificationRequest{
		Task: "Test",
	}

	options := []string{"Option A", "Option B"}
	cr.AddSuggestedOptions("choice", options)

	if cr.SuggestedOptions["choice"] == nil {
		t.Fatal("Expected options to be set")
	}

	if len(cr.SuggestedOptions["choice"]) != 2 {
		t.Errorf("Expected 2 options, got %d", len(cr.SuggestedOptions["choice"]))
	}

	if cr.SuggestedOptions["choice"][0] != "Option A" {
		t.Errorf("Expected first option to be 'Option A', got %s", cr.SuggestedOptions["choice"][0])
	}
}

func ExamplePlan_Format() {
	steps := []PlanStep{
		{Number: 1, Action: "Research", Tool: "web-search", ExpectedResult: "Documentation"},
		{Number: 2, Action: "Implement", Tool: "go-build", ExpectedResult: "Binary"},
	}
	plan := NewPlan("Build app", steps)
	fmt.Println(plan.Format())
}

func ExampleClarificationRequest_Format() {
	request := &ClarificationRequest{
		Task: "Test the file",
		Questions: []string{
			"What file should be tested?",
			"What test framework should be used?",
		},
	}
	fmt.Println(request.Format())
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}