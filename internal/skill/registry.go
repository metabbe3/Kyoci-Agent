package skill

import (
	"context"
	"log/slog"
	"sync"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
	"github.com/metabbe3/Kyoci-Agent/internal/skill/builtin"
)

// Registry wraps the kyoci.SkillRegistry to provide additional functionality
// for registering and managing built-in skills.
type Registry struct {
	registry *kyoci.SkillRegistry
	mu       sync.RWMutex
	logger   *slog.Logger
}

// NewRegistry creates a new skill registry.
func NewRegistry() *Registry {
	return &Registry{
		registry: kyoci.NewSkillRegistry(),
		logger:   slog.Default(),
	}
}

// Kyoci returns the underlying kyoci.SkillRegistry for use by the agent layer.
func (r *Registry) Kyoci() *kyoci.SkillRegistry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.registry
}

// Register adds a skill to the registry.
// Thread-safe: uses mutex to protect concurrent access.
func (r *Registry) Register(skill kyoci.Skill) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.logger.Debug("registering skill", "name", skill.Name())
	return r.registry.Register(skill)
}

// Match finds a skill that can handle the given query.
// Thread-safe: uses mutex to protect concurrent access.
func (r *Registry) Match(query string) (kyoci.Skill, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.registry.Match(query)
}

// List returns information about all registered skills.
// Thread-safe: uses mutex to protect concurrent access.
func (r *Registry) List() []kyoci.SkillInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.registry.List()
}

// RegisterBuiltin registers all built-in skills.
// This should be called during application initialization.
func (r *Registry) RegisterBuiltin() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.logger.Info("registering built-in skills")

	// Register all built-in skills
	builtinSkills := []kyoci.Skill{
		builtin.NewMathSkill(),
		builtin.NewTimeSkill(),
		builtin.NewHashSkill(),
		builtin.NewUUIDSkill(),
		builtin.NewEncodeSkill(),
		builtin.NewConvertSkill(),
	}

	for _, skill := range builtinSkills {
		if err := r.registry.Register(skill); err != nil {
			r.logger.Error("failed to register built-in skill", "name", skill.Name(), "error", err)
			return err
		}
		r.logger.Info("built-in skill registered", "name", skill.Name())
	}

	return nil
}

// Execute executes a skill by name with the given query.
// Thread-safe: delegates to the underlying registry.
func (r *Registry) Execute(ctx context.Context, name, query string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.registry.Execute(ctx, name, query)
}