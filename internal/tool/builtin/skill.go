package builtin

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// SkillTool implements the kyoci.Tool interface for saving and loading reusable procedures.
type SkillTool struct {
	logger *slog.Logger
}

// NewSkillTool creates a new skill tool instance.
func NewSkillTool() *SkillTool {
	return &SkillTool{
		logger: slog.Default(),
	}
}

// Name returns the tool name.
func (s *SkillTool) Name() string {
	return "skill"
}

// Description returns the tool description.
func (s *SkillTool) Description() string {
	return "Save, load, list, or delete reusable procedures (NOT the zero-AI skill registry). action=\"save\" name=\"deploy-checklist\" content=\"...\"; action=\"load\" name=\"deploy-checklist\"; action=\"list\"; action=\"delete\" name=\"...\"."
}

// Parameters returns the tool parameter definition.
func (s *SkillTool) Parameters() []kyoci.ToolParameter {
	return []kyoci.ToolParameter{
		{
			Name:        "action",
			Type:        "string",
			Description: "Action to perform: save, load, list, delete",
			Required:    true,
			EnumValues:  []string{"save", "load", "list", "delete"},
		},
		{
			Name:        "name",
			Type:        "string",
			Description: "The skill name (lowercase, hyphens)",
			Required:    false,
		},
		{
			Name:        "content",
			Type:        "string",
			Description: "The procedure text/steps to store (required for save action)",
			Required:    false,
		},
	}
}

// Execute performs skill operations.
//
// Parameters:
//   - ctx: Context for cancellation
//   - params: Map containing "action", "name", and optionally "content"
//
// Returns:
//   - string: Result of the operation
//   - error: Error if operation fails
func (s *SkillTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	// Extract action
	action, ok := params["action"].(string)
	if !ok || action == "" {
		return "", fmt.Errorf("action parameter is required and must be a string")
	}

	s.logger.Info("executing skill action", "action", action)

	// Execute based on action type
	switch action {
	case "save":
		return s.saveSkill(params)
	case "load":
		return s.loadSkill(params)
	case "list":
		return s.listSkills()
	case "delete":
		return s.deleteSkill(params)
	default:
		return "", fmt.Errorf("unknown action: %s", action)
	}
}

// getSkillsDir returns the skills directory path, creating it if needed.
func (s *SkillTool) getSkillsDir() (string, error) {
	skillsDir := "data/skills"
	
	// Create directory if it doesn't exist
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create skills directory: %w", err)
	}
	
	// Resolve to absolute path
	absPath, err := filepath.Abs(skillsDir)
	if err != nil {
		return "", fmt.Errorf("failed to resolve skills directory path: %w", err)
	}
	
	return absPath, nil
}

// saveSkill saves a skill to a markdown file.
func (s *SkillTool) saveSkill(params map[string]interface{}) (string, error) {
	// Extract name
	name, ok := params["name"].(string)
	if !ok || name == "" {
		return "", fmt.Errorf("name parameter is required for save action")
	}

	// Validate name format (lowercase, hyphens only)
	if !isValidSkillName(name) {
		return "", fmt.Errorf("invalid skill name: must use lowercase letters and hyphens only")
	}

	// Extract content
	content, ok := params["content"].(string)
	if !ok {
		return "", fmt.Errorf("content parameter is required for save action")
	}

	skillsDir, err := s.getSkillsDir()
	if err != nil {
		return "", err
	}

	// Create file path
	filePath := filepath.Join(skillsDir, name+".md")

	s.logger.Info("saving skill", "name", name, "path", filePath)

	// Write content to file
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		if os.IsPermission(err) {
			return "", fmt.Errorf("permission denied: %s", filePath)
		}
		return "", fmt.Errorf("failed to write skill file: %w", err)
	}

	return fmt.Sprintf("Skill '%s' saved successfully to %s", name, filePath), nil
}

// loadSkill loads a skill from a markdown file.
func (s *SkillTool) loadSkill(params map[string]interface{}) (string, error) {
	// Extract name
	name, ok := params["name"].(string)
	if !ok || name == "" {
		return "", fmt.Errorf("name parameter is required for load action")
	}

	skillsDir, err := s.getSkillsDir()
	if err != nil {
		return "", err
	}

	// Create file path
	filePath := filepath.Join(skillsDir, name+".md")

	s.logger.Info("loading skill", "name", name, "path", filePath)

	// Read content from file
	content, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("skill not found: %s", name)
		}
		if os.IsPermission(err) {
			return "", fmt.Errorf("permission denied: %s", filePath)
		}
		return "", fmt.Errorf("failed to read skill file: %w", err)
	}

	return string(content), nil
}

// listSkills lists all saved skills.
func (s *SkillTool) listSkills() (string, error) {
	skillsDir, err := s.getSkillsDir()
	if err != nil {
		return "", err
	}

	s.logger.Info("listing skills", "directory", skillsDir)

	// Read directory entries
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return "", fmt.Errorf("failed to read skills directory: %w", err)
	}

	// Filter for .md files
	var skills []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			// Remove .md extension
			name := strings.TrimSuffix(entry.Name(), ".md")
			skills = append(skills, name)
		}
	}

	if len(skills) == 0 {
		return "No skills found. Use the 'save' action to create new skills.", nil
	}

	// Format result
	var result strings.Builder
	result.WriteString(fmt.Sprintf("Found %d saved skills:\n", len(skills)))
	for _, skill := range skills {
		result.WriteString(fmt.Sprintf("  - %s\n", skill))
	}

	return result.String(), nil
}

// deleteSkill deletes a skill file.
func (s *SkillTool) deleteSkill(params map[string]interface{}) (string, error) {
	// Extract name
	name, ok := params["name"].(string)
	if !ok || name == "" {
		return "", fmt.Errorf("name parameter is required for delete action")
	}

	skillsDir, err := s.getSkillsDir()
	if err != nil {
		return "", err
	}

	// Create file path
	filePath := filepath.Join(skillsDir, name+".md")

	s.logger.Info("deleting skill", "name", name, "path", filePath)

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return "", fmt.Errorf("skill not found: %s", name)
	}

	// Delete file
	if err := os.Remove(filePath); err != nil {
		if os.IsPermission(err) {
			return "", fmt.Errorf("permission denied: %s", filePath)
		}
		return "", fmt.Errorf("failed to delete skill file: %w", err)
	}

	return fmt.Sprintf("Skill '%s' deleted successfully", name), nil
}

// isValidSkillName checks if a skill name is valid (lowercase, hyphens only).
func isValidSkillName(name string) bool {
	if name == "" {
		return false
	}

	for _, ch := range name {
		if ch >= 'a' && ch <= 'z' {
			continue
		}
		if ch == '-' {
			continue
		}
		return false
	}

	return true
}