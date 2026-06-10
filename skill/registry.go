package skill

import (
	"context"
	"regexp"
	"sync"
)

// SkillHandler is a function that processes input and returns a result.
// It runs without AI - pure Go logic.
type SkillHandler func(ctx context.Context, input string) (string, error)

// Skill represents a zero-AI capability with a pattern matcher.
type Skill struct {
	Name        string
	Pattern     *regexp.Regexp
	Handler     SkillHandler
	Description string
}

// Registry manages zero-AI skill handlers.
type Registry struct {
	skills map[string]*Skill
	mu     sync.RWMutex
}

// NewRegistry creates a new skill registry.
func NewRegistry() *Registry {
	return &Registry{
		skills: make(map[string]*Skill),
	}
}

// Register adds a new skill to the registry.
func (r *Registry) Register(name, pattern string, handler SkillHandler, desc string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	re, err := regexp.Compile(pattern)
	if err != nil {
		return err
	}

	r.skills[name] = &Skill{
		Name:        name,
		Pattern:     re,
		Handler:     handler,
		Description: desc,
	}

	return nil
}

// Match checks if input matches any registered skill pattern.
// Returns the matching skill and true if found, false otherwise.
func (r *Registry) Match(input string) (*Skill, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, skill := range r.skills {
		if skill.Pattern.MatchString(input) {
			return skill, true
		}
	}

	return nil, false
}

// Execute tries to match and execute a skill for the given input.
// Returns the result and true if a skill was matched/ran, false otherwise.
func (r *Registry) Execute(ctx context.Context, input string) (string, bool, error) {
	skill, found := r.Match(input)
	if !found {
		return "", false, nil
	}

	result, err := skill.Handler(ctx, input)
	return result, true, err
}

// List returns all registered skill names.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.skills))
	for name := range r.skills {
		names = append(names, name)
	}
	return names
}

// Get retrieves a skill by name.
func (r *Registry) Get(name string) (*Skill, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	skill, ok := r.skills[name]
	return skill, ok
}