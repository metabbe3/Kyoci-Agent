package selfimprove

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// ValidationResult represents the result of code validation
type ValidationResult struct {
	Valid       bool          `json:"valid"`
	Errors      []string      `json:"errors"`
	Warnings    []string      `json:"warnings"`
	TestsPassed int           `json:"testsPassed"`
	TestsFailed int           `json:"testsFailed"`
	Coverage    float64       `json:"coverage"`
	Duration    time.Duration `json:"duration"`
	LintScore   int           `json:"lintScore"` // 0-100
}

// Validator provides deterministic code validation using pure Go tooling
type Validator struct {
	projectRoot string
	lintPath    string // path to golangci-lint binary
}

// NewValidator creates a new Validator instance
func NewValidator(projectRoot string) *Validator {
	v := &Validator{
		projectRoot: projectRoot,
	}

	// Try to find golangci-lint
	if path, err := exec.LookPath("golangci-lint"); err == nil {
		v.lintPath = path
	}

	return v
}

// Validate runs comprehensive validation on changed files
func (v *Validator) Validate(ctx context.Context, changedFiles []string) (*ValidationResult, error) {
	start := time.Now()
	result := &ValidationResult{
		Errors:      []string{},
		Warnings:    []string{},
		TestsPassed: 0,
		TestsFailed: 0,
		Coverage:    0.0,
		LintScore:   100,
	}

	// Step 1: go vet
	if issues, err := v.GoVet(ctx); err != nil {
		return nil, fmt.Errorf("go vet failed: %w", err)
	} else {
		result.Errors = append(result.Errors, issues...)
	}

	// Step 2: go build
	if buildErrors, err := v.GoBuild(ctx); err != nil {
		return nil, fmt.Errorf("go build failed: %w", err)
	} else {
		result.Errors = append(result.Errors, buildErrors...)
	}

	// Step 3: golangci-lint (if available)
	if v.lintPath != "" {
		if lintIssues, err := v.Lint(ctx); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Lint check failed: %v", err))
		} else {
			result.Warnings = append(result.Warnings, lintIssues...)
			// Compute lint score (0-100 based on issues found)
			result.LintScore = v.computeLintScore(lintIssues)
		}
	}

	// Step 4: go test
	passed, failed, testOutput, err := v.GoTest(ctx)
	if err != nil && failed == 0 {
		// Only error if no tests ran or some other failure
		result.Warnings = append(result.Warnings, fmt.Sprintf("Test execution issue: %v", err))
	}
	result.TestsPassed = passed
	result.TestsFailed = failed

	// Try to extract coverage from test output
	result.Coverage = v.extractCoverage(testOutput)

	result.Duration = time.Since(start)
	result.Valid = len(result.Errors) == 0 && result.TestsFailed == 0

	return result, nil
}

// ValidateFile validates a single file
func (v *Validator) ValidateFile(ctx context.Context, filePath string) (*ValidationResult, error) {
	return v.Validate(ctx, []string{filePath})
}

// GoVet runs go vet on all packages
func (v *Validator) GoVet(ctx context.Context) ([]string, error) {
	cmd := exec.CommandContext(ctx, "go", "vet", "./...")
	cmd.Dir = v.projectRoot

	output, err := cmd.CombinedOutput()
	if err != nil {
		// go vet returns non-zero if issues found, parse output
		return v.parseGoVetOutput(string(output)), nil
	}
	return nil, nil
}

// GoBuild ensures compilation succeeds
func (v *Validator) GoBuild(ctx context.Context) ([]string, error) {
	cmd := exec.CommandContext(ctx, "go", "build", "./...")
	cmd.Dir = v.projectRoot

	output, err := cmd.CombinedOutput()
	if err != nil {
		return v.parseBuildOutput(string(output)), fmt.Errorf("build failed")
	}
	return nil, nil
}

// GoTest runs all tests and returns pass/fail counts
func (v *Validator) GoTest(ctx context.Context) (passed, failed int, output string, err error) {
	cmd := exec.CommandContext(ctx, "go", "test", "./...", "-v")
	cmd.Dir = v.projectRoot

	rawOutput, err := cmd.CombinedOutput()
	output = string(rawOutput)

	if err != nil {
		// Tests may have failed but we still get output
		passed, failed = v.parseTestOutput(output)
		return passed, failed, output, err
	}

	passed, failed = v.parseTestOutput(output)
	return passed, failed, output, nil
}

// Lint runs golangci-lint if available
func (v *Validator) Lint(ctx context.Context) ([]string, error) {
	if v.lintPath == "" {
		return nil, nil // Skip gracefully if not available
	}

	cmd := exec.CommandContext(ctx, v.lintPath, "run", "--out-format=line")
	cmd.Dir = v.projectRoot

	output, err := cmd.CombinedOutput()
	if err != nil {
		// golangci-lint returns non-zero if issues found
		return v.parseLintOutput(string(output)), nil
	}
	return nil, nil
}

// Format returns a pretty-printed validation result
func (v *Validator) Format(vr *ValidationResult) string {
	var sb strings.Builder

	sb.WriteString("=== Validation Result ===\n")
	sb.WriteString(fmt.Sprintf("Valid: %v\n", vr.Valid))
	sb.WriteString(fmt.Sprintf("Duration: %v\n", vr.Duration))
	sb.WriteString(fmt.Sprintf("Lint Score: %d/100\n", vr.LintScore))
	sb.WriteString(fmt.Sprintf("Tests: %d passed, %d failed\n", vr.TestsPassed, vr.TestsFailed))
	sb.WriteString(fmt.Sprintf("Coverage: %.1f%%\n", vr.Coverage))

	if len(vr.Errors) > 0 {
		sb.WriteString(fmt.Sprintf("\nErrors (%d):\n", len(vr.Errors)))
		for _, e := range vr.Errors {
			sb.WriteString(fmt.Sprintf("  - %s\n", e))
		}
	}

	if len(vr.Warnings) > 0 {
		sb.WriteString(fmt.Sprintf("\nWarnings (%d):\n", len(vr.Warnings)))
		for _, w := range vr.Warnings {
			sb.WriteString(fmt.Sprintf("  - %s\n", w))
		}
	}

	return sb.String()
}

// parseGoVetOutput parses go vet output into individual issues
func (v *Validator) parseGoVetOutput(output string) []string {
	lines := strings.Split(output, "\n")
	issues := []string{}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			issues = append(issues, line)
		}
	}

	return issues
}

// parseBuildOutput parses build errors
func (v *Validator) parseBuildOutput(output string) []string {
	lines := strings.Split(output, "\n")
	errors := []string{}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && strings.Contains(line, ":") {
			errors = append(errors, line)
		}
	}

	return errors
}

// parseTestOutput parses go test output for pass/fail counts
func (v *Validator) parseTestOutput(output string) (passed, failed int) {
	lines := strings.Split(output, "\n")
	
	// Look for PASS and FAIL lines
	for _, line := range lines {
		line = strings.TrimSpace(line)
		
		if strings.HasPrefix(line, "PASS") || strings.Contains(line, "--- PASS:") {
			passed++
		}
		
		if strings.HasPrefix(line, "FAIL") || strings.Contains(line, "--- FAIL:") {
			failed++
		}
	}
	
	// Also try to parse summary lines like "ok      package    0.123s"
	okRegex := regexp.MustCompile(`^ok\s+\S+`)
	for _, line := range lines {
		if okRegex.MatchString(line) && !strings.Contains(line, "no test files") {
			passed++
		}
	}

	return passed, failed
}

// parseLintOutput parses golangci-lint output
func (v *Validator) parseLintOutput(output string) []string {
	lines := strings.Split(output, "\n")
	issues := []string{}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "level=") {
			issues = append(issues, line)
		}
	}

	return issues
}

// extractCoverage extracts coverage percentage from test output
func (v *Validator) extractCoverage(output string) float64 {
	// Look for coverage pattern like "coverage: 75.0% of statements"
	re := regexp.MustCompile(`coverage:\s+([\d.]+)%`)
	matches := re.FindStringSubmatch(output)

	if len(matches) > 1 {
		var cov float64
		_, err := fmt.Sscanf(matches[1], "%f", &cov)
		if err == nil {
			return cov
		}
	}

	return 0.0
}

// computeLintScore computes a lint score (0-100) based on issues
func (v *Validator) computeLintScore(issues []string) int {
	if len(issues) == 0 {
		return 100
	}

	// Simple scoring: subtract points for each issue
	// Critical issues (error, bug) cost more
	score := 100
	for _, issue := range issues {
		lower := strings.ToLower(issue)
		if strings.Contains(lower, "error") || strings.Contains(lower, "bug") {
			score -= 10
		} else if strings.Contains(lower, "warning") || strings.Contains(lower, "suspicious") {
			score -= 5
		} else {
			score -= 2
		}
	}

	if score < 0 {
		score = 0
	}

	return score
}