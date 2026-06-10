package thinking

import (
	"fmt"
	"regexp"
	"strings"
)

// ClarificationRequest represents a request for clarification from the user
type ClarificationRequest struct {
	Task               string            `json:"task"`
	UncertainAspects   []string          `json:"uncertain_aspects"`
	Questions          []string          `json:"questions"`
	SuggestedOptions   map[string][]string `json:"suggested_options"`
}

// DetectAmbiguity performs rule-based ambiguity detection on a task
func DetectAmbiguity(task string) *ClarificationRequest {
	request := &ClarificationRequest{
		Task:               task,
		UncertainAspects:   make([]string, 0),
		Questions:          make([]string, 0),
		SuggestedOptions:   make(map[string][]string),
	}
	
	lowerTask := strings.ToLower(task)
	
	// Check 1: Multiple interpretations possible
	ambiguousTerms := []string{"it", "that", "the file", "the code", "the project"}
	for _, term := range ambiguousTerms {
		if strings.Contains(lowerTask, " "+term+" ") || strings.HasPrefix(lowerTask, term+" ") || strings.HasSuffix(lowerTask, " "+term) {
			request.UncertainAspects = append(request.UncertainAspects, fmt.Sprintf("Ambiguous reference: '%s'", term))
			request.Questions = append(request.Questions, fmt.Sprintf("What does '%s' refer to specifically?", term))
			break
		}
	}
	
	// Check 2: Missing context (no tool specified)
	toolKeywords := []string{"run", "execute", "build", "test", "compile", "deploy"}
	hasTool := false
	for _, kw := range toolKeywords {
		if strings.Contains(lowerTask, kw) {
			hasTool = true
			// Check if tool name follows the keyword
			for _, tool := range []string{"docker", "make", "npm", "python", "go", "java", "mvn"} {
				if strings.Contains(lowerTask, kw+" "+tool) || strings.Contains(lowerTask, tool+" "+kw) {
					hasTool = true
					break
				}
			}
			break
		}
	}
	
	if hasTool && !strings.Contains(lowerTask, "using") && !strings.Contains(lowerTask, "with") {
		request.UncertainAspects = append(request.UncertainAspects, "Tool specification missing")
		request.Questions = append(request.Questions, "What tool or method should be used to perform this action?")
		request.SuggestedOptions["tool"] = []string{"Specify the tool explicitly", "Ask AI to recommend a tool", "Use default tool for the task type"}
	}
	
	// Check 3: Missing target
	if strings.Contains(lowerTask, "test") || strings.Contains(lowerTask, "build") {
		if !strings.Contains(lowerTask, ".") && !strings.Contains(lowerTask, "file") && !strings.Contains(lowerTask, "directory") {
			request.UncertainAspects = append(request.UncertainAspects, "Target not specified")
			request.Questions = append(request.Questions, "What files, directories, or components should be targeted?")
		}
	}
	
	// Check 4: Contradictory requirements
	if strings.Contains(lowerTask, "quick") && strings.Contains(lowerTask, "thorough") {
		request.UncertainAspects = append(request.UncertainAspects, "Potentially contradictory requirements")
		request.Questions = append(request.Questions, "Should I prioritize speed or thoroughness?")
		request.SuggestedOptions["priority"] = []string{"Speed first", "Thoroughness first", "Balanced approach"}
	}
	
	if strings.Contains(lowerTask, "simple") && strings.Contains(lowerTask, "comprehensive") {
		request.UncertainAspects = append(request.UncertainAspects, "Potentially contradictory requirements")
		request.Questions = append(request.Questions, "Should I prioritize simplicity or comprehensiveness?")
		request.SuggestedOptions["priority"] = []string{"Keep it simple", "Make it comprehensive", "Balance both"}
	}
	
	// Check 5: Vague quantifiers
	type quantifierInfo struct {
		question string
		options  []string
	}
	
	vagueQuantifiers := map[string]quantifierInfo{
		"some": {"How many? What quantity?", []string{"1-5 items", "5-10 items", "All items", "A subset"}},
		"many": {"How many? What's the upper limit?", []string{"Less than 10", "10-50", "50-100", "All available"}},
		"few": {"How few? What's the minimum?", []string{"1-2 items", "3-5 items", "Under 10 items"}},
		"better": {"Better than what? What's the baseline?", []string{"Improve current state", "Outperform competitor", "Meet specific metrics"}},
		"faster": {"Faster than what? What's the current performance?", []string{"2x improvement", "10% improvement", "Best effort"}},
		"optimize": {"What should be optimized? Speed, memory, cost?", []string{"Speed/performance", "Memory usage", "Code clarity", "All of the above"}},
	}
	
	for quant, info := range vagueQuantifiers {
		if strings.Contains(lowerTask, " "+quant+" ") || strings.HasPrefix(lowerTask, quant+" ") || strings.HasSuffix(lowerTask, " "+quant) {
			request.UncertainAspects = append(request.UncertainAspects, fmt.Sprintf("Vague quantifier: '%s'", quant))
			request.Questions = append(request.Questions, info.question)
			request.SuggestedOptions[quant] = info.options
		}
	}
	
	// If no issues found, return nil
	if len(request.Questions) == 0 {
		return nil
	}
	
	return request
}

// Format returns a pretty-printed representation of the clarification request
func (cr *ClarificationRequest) Format() string {
	if cr == nil {
		return ""
	}
	
	var sb strings.Builder
	
	sb.WriteString("❓ Clarification Needed\n\n")
	sb.WriteString(fmt.Sprintf("Task: %s\n\n", cr.Task))
	
	if len(cr.UncertainAspects) > 0 {
		sb.WriteString("Uncertain Aspects:\n")
		for _, aspect := range cr.UncertainAspects {
			sb.WriteString(fmt.Sprintf("  • %s\n", aspect))
		}
		sb.WriteString("\n")
	}
	
	if len(cr.Questions) > 0 {
		sb.WriteString("Questions:\n")
		for i, q := range cr.Questions {
			sb.WriteString(fmt.Sprintf("  %d. %s\n", i+1, q))
		}
		sb.WriteString("\n")
	}
	
	if len(cr.SuggestedOptions) > 0 {
		sb.WriteString("Suggested Options:\n")
		for key, options := range cr.SuggestedOptions {
			sb.WriteString(fmt.Sprintf("  For '%s':\n", key))
			for _, opt := range options {
				sb.WriteString(fmt.Sprintf("    - %s\n", opt))
			}
		}
		sb.WriteString("\n")
	}
	
	sb.WriteString("Please provide clarification so I can proceed effectively.")
	
	return sb.String()
}

// GenerateQuestions uses AI to generate clarification questions for a task
func GenerateQuestions(task string, executor func(string) (string, error)) ([]string, error) {
	prompt := fmt.Sprintf(`Analyze this task and identify what information is missing or unclear:

Task: %s

Please provide a list of clarifying questions that would help complete this task accurately. 
Focus on:
1. Ambiguous terms or references
2. Missing context or specifications
3. Unclear requirements or constraints
4. Missing targets or scope

Return only the questions, one per line.`, task)
	
	response, err := executor(prompt)
	if err != nil {
		return nil, fmt.Errorf("failed to generate questions: %w", err)
	}
	
	// Parse the response into individual questions
	questions := make([]string, 0)
	lines := strings.Split(response, "\n")
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		
		// Remove numbering if present (e.g., "1. " or "• ")
		line = regexp.MustCompile(`^\d+[\.\)]\s+`).ReplaceAllString(line, "")
		line = regexp.MustCompile(`^[•\-\*]\s+`).ReplaceAllString(line, "")
		
		line = strings.TrimSpace(line)
		if line != "" {
			questions = append(questions, line)
		}
	}
	
	if len(questions) == 0 {
		// If parsing failed, return the whole response as one question
		questions = append(questions, response)
	}
	
	return questions, nil
}

// NeedsClarification checks if any clarification requests exist
func (cr *ClarificationRequest) NeedsClarification() bool {
	if cr == nil {
		return false
	}
	return len(cr.Questions) > 0
}

// AddQuestion adds a question to the request
func (cr *ClarificationRequest) AddQuestion(question string) {
	cr.Questions = append(cr.Questions, question)
}

// AddSuggestedOptions adds suggested options for a question
func (cr *ClarificationRequest) AddSuggestedOptions(key string, options []string) {
	if cr.SuggestedOptions == nil {
		cr.SuggestedOptions = make(map[string][]string)
	}
	cr.SuggestedOptions[key] = options
}