package kyoci

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
)

// ==============================================================================
// Skill Interface and Types
// ==============================================================================

// Skill is the interface that all skills must implement.
// Skills are zero-AI, instant execution capabilities (e.g., search, calculation).
// Unlike tools, skills don't require LLM invocation and execute immediately.
// Goroutine-safe: Implementations MUST be safe for concurrent use from multiple goroutines.
// The interface methods are called concurrently and must be properly synchronized.
type Skill interface {
	// Name returns the unique name of this skill.
	Name() string

	// Description returns a human-readable description of what this skill does.
	Description() string

	// Match checks if this skill can handle the given query.
	// This is used to select the appropriate skill for a user query.
	// The matching logic can be simple (keyword) or complex (regex, pattern).
	//
	// Parameters:
	//   - query: The user query to match against
	//
	// Returns:
	//   - bool: true if this skill can handle the query
	Match(query string) bool

	// Execute executes the skill with the given query.
	// Skills execute immediately without LLM involvement.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout control
	//   - query: The user query to process
	//
	// Returns:
	//   - string: The result of executing the skill
	//   - error: Any error that occurred during execution
	//
	// Errors:
	//   - context.Canceled if ctx was canceled
	//   - ErrSkillExecution if skill execution failed
	Execute(ctx context.Context, query string) (string, error)
}

// SkillInfo represents information about a skill.
// Goroutine-safe: SkillInfo values should be treated as immutable after creation.
type SkillInfo struct {
	// Name is the skill name
	Name string
	// Description describes what the skill does
	Description string
	// Keywords are keywords that trigger this skill (optional)
	Keywords []string
	// Category is the skill category (optional)
	Category string
	// Metadata contains additional skill metadata
	Metadata map[string]string
}

// ==============================================================================
// Skill Registry
// ==============================================================================

// SkillRegistry manages a collection of skills.
// Goroutine-safe: All methods are safe for concurrent use.
// Uses internal synchronization (RWMutex) for thread-safe operations.
type SkillRegistry struct {
	mu       sync.RWMutex
	skills   map[string]Skill
	patterns map[string]*regexp.Regexp
	logger   *slog.Logger
}

// NewSkillRegistry creates a new skill registry.
func NewSkillRegistry() *SkillRegistry {
	return &SkillRegistry{
		skills:   make(map[string]Skill),
		patterns: make(map[string]*regexp.Regexp),
		logger:   slog.Default(),
	}
}

// Register adds a skill to the registry.
// If a skill with the same name already exists, it will be replaced.
//
// Parameters:
//   - skill: The skill to register
//
// Returns:
//   - error: nil on success, error if validation fails
func (r *SkillRegistry) Register(skill Skill) error {
	if skill == nil {
		return NewValidationError("skill", "skill cannot be nil", nil)
	}

	name := skill.Name()
	if name == "" {
		return NewValidationError("name", "skill name cannot be empty", name)
	}

	if skill.Description() == "" {
		return NewValidationError("description", "skill description cannot be empty", "")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.skills[name] = skill
	r.logger.Info("skill registered", "name", name)
	return nil
}

// RegisterPattern adds a regex pattern that maps to a skill name.
// This allows matching skills by regex patterns in addition to Match() method.
//
// Parameters:
//   - pattern: The regex pattern
//   - skillName: The skill name to match
//
// Returns:
//   - error: Error if regex compilation fails
func (r *SkillRegistry) RegisterPattern(pattern, skillName string) error {
	regex, err := regexp.Compile(pattern)
	if err != nil {
		return NewValidationError("pattern", "invalid regex pattern: "+err.Error(), pattern)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.patterns[pattern] = regex
	r.logger.Info("skill pattern registered", "pattern", pattern, "skill", skillName)
	return nil
}

// Match finds a skill that can handle the given query.
// Returns the first matching skill based on registration order and patterns.
//
// Parameters:
//   - query: The user query to match
//
// Returns:
//   - Skill: The matching skill, or nil if none found
//   - bool: true if a match was found
func (r *SkillRegistry) Match(query string) (Skill, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Check patterns first (higher priority)
	for _, regex := range r.patterns {
		if regex.MatchString(query) {
			if skill, ok := r.skills[regex.String()]; ok {
				return skill, true
			}
		}
	}

	// Check skill Match() methods
	for _, skill := range r.skills {
		if skill.Match(query) {
			return skill, true
		}
	}

	return nil, false
}

// MatchAll returns all skills that can handle the given query.
//
// Parameters:
//   - query: The user query to match
//
// Returns:
//   - []Skill: List of matching skills
func (r *SkillRegistry) MatchAll(query string) []Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()

	matches := make([]Skill, 0)

	// Check patterns
	for _, regex := range r.patterns {
		if regex.MatchString(query) {
			if skill, ok := r.skills[regex.String()]; ok {
				matches = append(matches, skill)
			}
		}
	}

	// Check skill Match() methods
	for _, skill := range r.skills {
		if skill.Match(query) {
			// Avoid duplicates
			found := false
			for _, m := range matches {
				if m.Name() == skill.Name() {
					found = true
					break
				}
			}
			if !found {
				matches = append(matches, skill)
			}
		}
	}

	return matches
}

// List returns information about all registered skills.
//
// Returns:
//   - []SkillInfo: List of skill information
func (r *SkillRegistry) List() []SkillInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	infos := make([]SkillInfo, 0, len(r.skills))
	for _, skill := range r.skills {
		infos = append(infos, SkillInfo{
			Name:        skill.Name(),
			Description: skill.Description(),
		})
	}
	return infos
}

// Execute executes a skill by name with the given query.
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - name: The skill name to execute
//   - query: The query to process
//
// Returns:
//   - string: The result of executing the skill
//   - error: Any error that occurred
func (r *SkillRegistry) Execute(ctx context.Context, name, query string) (string, error) {
	r.mu.RLock()
	skill, ok := r.skills[name]
	r.mu.RUnlock()

	if !ok {
		return "", ErrSkillNotFound
	}

	r.logger.Info("executing skill", "name", name, "query", query)
	result, err := skill.Execute(ctx, query)
	if err != nil {
		r.logger.Error("skill execution failed", "name", name, "error", err)
		return "", fmt.Errorf("%w: %v", ErrSkillExecution, err)
	}

	r.logger.Info("skill executed successfully", "name", name)
	return result, nil
}

// Remove removes a skill from the registry.
//
// Parameters:
//   - name: The skill name to remove
//
// Returns:
//   - error: nil on success, error if not found
func (r *SkillRegistry) Remove(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.skills[name]; !ok {
		return ErrSkillNotFound
	}
	delete(r.skills, name)
	r.logger.Info("skill removed", "name", name)
	return nil
}

// Has checks if a skill is registered.
//
// Parameters:
//   - name: The skill name
//
// Returns:
//   - bool: true if the skill exists
func (r *SkillRegistry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.skills[name]
	return ok
}

// Count returns the number of registered skills.
//
// Returns:
//   - int: Number of skills
func (r *SkillRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.skills)
}

// ==============================================================================
// Base Skill Implementation
// ==============================================================================

// BaseSkill provides a default implementation of the Skill interface.
// It can be embedded in custom skill implementations to reduce boilerplate.
// Goroutine-safe: BaseSkill values should be treated as immutable after creation.
type BaseSkill struct {
	name        string
	description string
	keywords    []string
	logger      *slog.Logger
}

// NewBaseSkill creates a new base skill with the given information.
func NewBaseSkill(name, description string, keywords []string) *BaseSkill {
	return &BaseSkill{
		name:        name,
		description: description,
		keywords:    keywords,
		logger:      slog.Default(),
	}
}

// Name returns the skill name.
func (s *BaseSkill) Name() string {
	return s.name
}

// Description returns the skill description.
func (s *BaseSkill) Description() string {
	return s.description
}

// Match checks if any of the skill's keywords are in the query.
// This is a simple implementation that can be overridden.
func (s *BaseSkill) Match(query string) bool {
	queryLower := toLower(query)
	for _, keyword := range s.keywords {
		if contains(queryLower, toLower(keyword)) {
			return true
		}
	}
	return false
}

// Execute returns an error indicating it must be overridden.
func (s *BaseSkill) Execute(ctx context.Context, query string) (string, error) {
	return "", fmt.Errorf("Execute method not implemented for skill %s", s.name)
}

// ==============================================================================
// Built-in Skills
// ==============================================================================

// EchoSkill is a simple skill that echoes back the query.
type EchoSkill struct {
	*BaseSkill
}

// NewEchoSkill creates a new echo skill.
func NewEchoSkill() *EchoSkill {
	return &EchoSkill{
		BaseSkill: NewBaseSkill(
			"echo",
			"Echoes back the input query",
			[]string{"echo", "repeat", "say"},
		),
	}
}

// Match checks if the query contains echo keywords.
func (s *EchoSkill) Match(query string) bool {
	return s.BaseSkill.Match(query)
}

// Execute echoes back the query.
func (s *EchoSkill) Execute(ctx context.Context, query string) (string, error) {
	return query, nil
}

// TimeSkill returns the current time.
type TimeSkill struct {
	*BaseSkill
}

// NewTimeSkill creates a new time skill.
func NewTimeSkill() *TimeSkill {
	return &TimeSkill{
		BaseSkill: NewBaseSkill(
			"time",
			"Returns the current time",
			[]string{"time", "clock", "now", "current time"},
		),
	}
}

// Match checks if the query contains time keywords.
func (s *TimeSkill) Match(query string) bool {
	return s.BaseSkill.Match(query)
}

// Execute returns the current time.
func (s *TimeSkill) Execute(ctx context.Context, query string) (string, error) {
	return ctx.Value("time").(string), nil
}

// HelpSkill lists all available skills.
type HelpSkill struct {
	*BaseSkill
	registry *SkillRegistry
}

// NewHelpSkill creates a new help skill.
func NewHelpSkill(registry *SkillRegistry) *HelpSkill {
	return &HelpSkill{
		BaseSkill: NewBaseSkill(
			"help",
			"Lists all available skills",
			[]string{"help", "list skills", "available skills"},
		),
		registry: registry,
	}
}

// Match checks if the query contains help keywords.
func (s *HelpSkill) Match(query string) bool {
	return s.BaseSkill.Match(query)
}

// Execute returns a list of all skills.
func (s *HelpSkill) Execute(ctx context.Context, query string) (string, error) {
	infos := s.registry.List()
	lines := make([]string, len(infos)+1)
	lines[0] = "Available skills:"
	for i, info := range infos {
		lines[i+1] = fmt.Sprintf("  - %s: %s", info.Name, info.Description)
	}
	return strings.Join(lines, "\n"), nil
}

// ==============================================================================
// Utility Functions
// ==============================================================================

// contains checks if a string contains a substring (case-insensitive).
func contains(s, substr string) bool {
	return findSubstring(toLower(s), toLower(substr)) >= 0
}

// ==============================================================================
// Error Types
// ==============================================================================

// ErrSkillExecution indicates that a skill execution failed.
var ErrSkillExecution = NewValidationError("skill", "skill execution failed", nil)

// ErrSkillNotFound indicates that a skill was not found.
var ErrSkillNotFound = NewValidationError("skill", "skill not found in registry", nil)

// ErrNoMatchingSkill indicates that no skill could handle the query.
var ErrNoMatchingSkill = NewValidationError("query", "no matching skill found for query", nil)