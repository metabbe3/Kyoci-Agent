package selfskill

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/nicholas/ai-agent/skill"
)

// SkillSpec defines a generated skill.
type SkillSpec struct {
	Name         string   `json:"name"`
	Pattern      string   `json:"pattern"`
	HandlerCode  string   `json:"handler_code"`
	Description  string   `json:"description"`
	Parameters   []string `json:"parameters"`
	ReturnType   string   `json:"return_type"`
	PackageName  string   `json:"package_name"`
}

// SkillGenerator creates and validates skill files.
type SkillGenerator struct {
	identifier *Identifier
	outputDir  string
	registry   *skill.Registry
}

// NewSkillGenerator creates a new skill generator.
func NewSkillGenerator(id *Identifier, outputDir string, reg *skill.Registry) *SkillGenerator {
	return &SkillGenerator{
		identifier: id,
		outputDir:  outputDir,
		registry:   reg,
	}
}

// Generate writes a skill file and registers the handler.
func (g *SkillGenerator) Generate(spec SkillSpec) error {
	// Ensure output directory exists
	if err := os.MkdirAll(g.outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Generate file content
	content := g.generateFileContent(spec)

	// Write file
	fileName := fmt.Sprintf("%s.go", spec.Name)
	filePath := filepath.Join(g.outputDir, fileName)
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write skill file: %w", err)
	}

	// Validate the generated file
	if err := g.Validate(filePath); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Register the skill in the registry
	handler := g.createHandler(spec)
	err := g.registry.Register(spec.Name, spec.Pattern, handler, spec.Description)
	if err != nil {
		return fmt.Errorf("failed to register skill: %w", err)
	}

	return nil
}

// SuggestFromPattern analyzes a pattern and suggests a handler.
func (g *SkillGenerator) SuggestFromPattern(pattern *TaskPattern) *SkillSpec {
	// Derive skill name from pattern
	name := g.deriveSkillName(pattern.Pattern)

	// Generate a regex pattern based on the task pattern
	regexPattern := g.generateRegexPattern(pattern.Pattern)

	// Suggest handler code
	handlerCode := g.suggestHandlerCode(pattern)

	// Determine description from examples
	description := g.generateDescription(pattern)

	spec := &SkillSpec{
		Name:        name,
		Pattern:     regexPattern,
		HandlerCode: handlerCode,
		Description: description,
		Parameters:  g.extractParameters(pattern),
		ReturnType:  "string",
		PackageName: "skill",
	}

	return spec
}

// Validate runs go vet and go build on the generated file.
func (g *SkillGenerator) Validate(filePath string) error {
	// Run go vet
	vetCmd := exec.Command("go", "vet", filePath)
	if output, err := vetCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go vet failed: %w\n%s", err, string(output))
	}

	// Run go build (compile check)
	buildCmd := exec.Command("go", "build", "-o", "/dev/null", filePath)
	if output, err := buildCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go build failed: %w\n%s", err, string(output))
	}

	return nil
}

// generateFileContent generates the Go file content for a skill.
func (g *SkillGenerator) generateFileContent(spec SkillSpec) string {
	return fmt.Sprintf(`package %s

import (
	"context"
	"fmt"
	"regexp"
)

var %sPattern = regexp.MustCompile("%s")

// %sHandler handles tasks matching pattern: %s
func %sHandler(ctx context.Context, input string) (string, error) {
	// Extract data from input using regex
	matches := %sPattern.FindStringSubmatch(input)
	if matches == nil {
		return "", fmt.Errorf("input does not match expected pattern")
	}

	// Process the matched data
	%s

	return fmt.Sprintf("Processed: %%s", input), nil
}

func init() {
	// This skill is registered by the SkillGenerator
}
`,
		spec.PackageName,
		camelCase(spec.Name),
		spec.Pattern,
		camelCase(spec.Name),
		spec.Pattern,
		camelCase(spec.Name),
		camelCase(spec.Name),
		spec.HandlerCode,
	)
}

// createHandler creates a skill handler from a spec.
func (g *SkillGenerator) createHandler(spec SkillSpec) skill.SkillHandler {
	return func(ctx context.Context, input string) (string, error) {
		re := regexp.MustCompile(spec.Pattern)
		matches := re.FindStringSubmatch(input)
		if matches == nil {
			return "", fmt.Errorf("input does not match pattern: %s", spec.Pattern)
		}

		// Return a formatted result
		return fmt.Sprintf("%s executed successfully with input: %s", spec.Name, input), nil
	}
}

// deriveSkillName creates a skill name from a pattern string.
func (g *SkillGenerator) deriveSkillName(pattern string) string {
	words := strings.Fields(pattern)
	if len(words) == 0 {
		return "unnamed"
	}

	// Join words with underscores and remove special chars
	name := strings.Join(words, "_")
	re := regexp.MustCompile(`[^a-zA-Z0-9_]`)
	name = re.ReplaceAllString(name, "")

	if name == "" {
		return "unnamed"
	}

	return name
}

// generateRegexPattern creates a regex pattern from a task pattern.
func (g *SkillGenerator) generateRegexPattern(pattern string) string {
	// Escape regex special chars and make it flexible
	escaped := regexp.QuoteMeta(pattern)

	// Replace spaces with flexible whitespace matching
	re := regexp.MustCompile(`\s+`)
	escaped = re.ReplaceAllString(escaped, `\s+`)

	// Make it case-insensitive by prefixing with (?i)
	return "(?i)" + escaped
}

// suggestHandlerCode suggests handler implementation based on pattern complexity.
func (g *SkillGenerator) suggestHandlerCode(pattern *TaskPattern) string {
	switch pattern.Complexity {
	case 1:
		return "// Simple task handling\nresult := input\nreturn result, nil"
	case 2:
		return "// Medium complexity: parse and process\n// Add your processing logic here\nprocessed := input\nreturn processed, nil"
	case 3:
		return "// Complex task: multi-step processing\n// Step 1: Analyze input\n// Step 2: Apply transformations\n// Step 3: Return result\nreturn \"processed: \" + input, nil"
	default:
		return "// Advanced processing logic\n// Implement custom handling for this pattern\nreturn input, nil"
	}
}

// generateDescription creates a description from pattern examples.
func (g *SkillGenerator) generateDescription(pattern *TaskPattern) string {
	if len(pattern.Examples) == 0 {
		return fmt.Sprintf("Handles tasks matching pattern: %s", pattern.Pattern)
	}
	return fmt.Sprintf("Handles tasks like: %s (pattern: %s, seen %d times)",
		strings.Join(pattern.Examples[:min(2, len(pattern.Examples))], ", "),
		pattern.Pattern,
		pattern.Frequency)
}

// extractParameters infers potential parameters from examples.
func (g *SkillGenerator) extractParameters(pattern *TaskPattern) []string {
	// Look for common parameter patterns in examples
	params := []string{"input"}

	// Check for numeric patterns
	re := regexp.MustCompile(`\d+`)
	for _, ex := range pattern.Examples {
		if re.MatchString(ex) {
			params = append(params, "number")
			break
		}
	}

	return params
}

// camelCase converts snake_case to CamelCase.
func camelCase(s string) string {
	words := strings.Split(s, "_")
	for i, word := range words {
		if len(word) > 0 {
			words[i] = strings.ToUpper(word[:1]) + strings.ToLower(word[1:])
		}
	}
	return strings.Join(words, "")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}