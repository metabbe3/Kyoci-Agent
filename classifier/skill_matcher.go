package classifier

import (
	"strings"
	"sync"
)

// SkillMetadata contains metadata about a registered skill
type SkillMetadata struct {
	Name        string
	Aliases     []string
	Description string
	Level       int // Suggested level for this skill (1-5)
}

// RegisteredSkills maintains a registry of available skills
type RegisteredSkills struct {
	skills map[string]SkillMetadata
	mu     sync.RWMutex
}

// Global skills registry
var skillRegistry = &RegisteredSkills{
	skills: make(map[string]SkillMetadata),
}

// registerSkill adds a skill to the registry
func registerSkill(name string, aliases []string, description string, level int) {
	skillRegistry.mu.Lock()
	defer skillRegistry.mu.Unlock()

	skillRegistry.skills[strings.ToLower(name)] = SkillMetadata{
		Name:        name,
		Aliases:     aliases,
		Description: description,
		Level:       level,
	}

	// Register aliases
	for _, alias := range aliases {
		skillRegistry.skills[strings.ToLower(alias)] = SkillMetadata{
			Name:        name,
			Aliases:     aliases,
			Description: description,
			Level:       level,
		}
	}
}

// unregisterSkill removes a skill from the registry
func unregisterSkill(name string) {
	skillRegistry.mu.Lock()
	defer skillRegistry.mu.Unlock()

	nameLower := strings.ToLower(name)
	if skill, exists := skillRegistry.skills[nameLower]; exists {
		delete(skillRegistry.skills, nameLower)
		// Remove aliases too
		for _, alias := range skill.Aliases {
			delete(skillRegistry.skills, strings.ToLower(alias))
		}
	}
}

// MatchSkill attempts to find an exact skill match in the input
// Returns the skill name if found, empty string otherwise
func MatchSkill(input string) string {
	skillRegistry.mu.RLock()
	defer skillRegistry.mu.RUnlock()

	inputLower := strings.ToLower(input)
	inputLower = strings.TrimSpace(inputLower)

	// First, try exact match for skill name
	if skill, exists := skillRegistry.skills[inputLower]; exists {
		return skill.Name
	}

	// Check if input starts with a skill name
	for skillName, skill := range skillRegistry.skills {
		if strings.HasPrefix(inputLower, skillName+" ") ||
			strings.HasPrefix(inputLower, skillName+":") ||
			strings.HasPrefix(inputLower, skillName+"/") {
			return skill.Name
		}
	}

	// Check for skill invocation patterns
	prefixes := []string{"use ", "run ", "execute ", "invoke ", "call ", "apply "}
	for _, prefix := range prefixes {
		if strings.HasPrefix(inputLower, prefix) {
			potentialSkill := strings.TrimSpace(strings.TrimPrefix(inputLower, prefix))
			if skill, exists := skillRegistry.skills[potentialSkill]; exists {
				return skill.Name
			}
			// Check for first word after prefix
			words := strings.Fields(potentialSkill)
			if len(words) > 0 {
				if skill, exists := skillRegistry.skills[words[0]]; exists {
					return skill.Name
				}
			}
		}
	}

	return ""
}

// getSkill returns metadata for a registered skill
func getSkill(name string) (SkillMetadata, bool) {
	skillRegistry.mu.RLock()
	defer skillRegistry.mu.RUnlock()

	if skill, exists := skillRegistry.skills[strings.ToLower(name)]; exists {
		return skill, true
	}
	return SkillMetadata{}, false
}

// ListSkills returns all registered skill names
func ListSkills() []string {
	skillRegistry.mu.RLock()
	defer skillRegistry.mu.RUnlock()

	names := make([]string, 0, len(skillRegistry.skills))
	seen := make(map[string]bool)

	for _, skill := range skillRegistry.skills {
		if !seen[skill.Name] {
			names = append(names, skill.Name)
			seen[skill.Name] = true
		}
	}

	return names
}

// InitializeDefaultSkills registers common skills
func InitializeDefaultSkills() {
	// Level 1-2 skills (simple, code-only)
	registerSkill("calculator", []string{"calc", "math", "compute"}, "Perform basic and complex calculations", 1)
	registerSkill("file", []string{"file_handler", "files", "file_ops"}, "Handle file operations", 2)
	registerSkill("terminal", []string{"cmd", "shell", "bash", "command"}, "Execute terminal commands", 2)
	registerSkill("schedule", []string{"scheduler", "cron", "timer"}, "Schedule tasks and reminders", 2)
	registerSkill("config", []string{"settings", "configuration", "preferences"}, "Manage configuration settings", 2)

	// Level 2-3 skills (code preferred, AI optional)
	registerSkill("browser", []string{"web", "browse", "navigate"}, "Browse the web", 2)
	registerSkill("email", []string{"mail", "send_email", "compose_email"}, "Send and manage emails", 2)
	registerSkill("database", []string{"db", "sql", "query"}, "Query databases", 2)
	registerSkill("http_client", []string{"http", "request", "api_call"}, "Make HTTP requests", 2)
	registerSkill("vision", []string{"image", "analyze_image", "ocr"}, "Analyze images and text", 2)
	registerSkill("code_exec", []string{"execute_code", "run_code", "eval"}, "Execute code snippets", 2)

	// Level 3-4 skills (AI needed)
	registerSkill("web_search", []string{"search", "google", "find"}, "Search the web", 2)
	registerSkill("web_scraper", []string{"scrape", "extract_web", "crawl"}, "Scrape web content", 2)
	registerSkill("image_gen", []string{"generate_image", "create_image", "dalle"}, "Generate images", 4)
	registerSkill("delegation", []string{"delegate", "assign", "subtask"}, "Delegate tasks to sub-agents", 4)

	// Level 4-5 skills (AI required)
	registerSkill("memory", []string{"recall", "remember", "store_memory"}, "Access and manage memories", 3)
	registerSkill("self_improve", []string{"improve", "learn", "adapt"}, "Improve own capabilities", 4)
	registerSkill("research", []string{"deep_research", "study", "investigate"}, "Conduct deep research", 5)
}