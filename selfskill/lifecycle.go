package selfskill

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"
)

// LifecycleStatus tracks the pipeline status.
type LifecycleStatus struct {
	Candidates int `json:"candidates"`
	Generated  int `json:"generated"`
	Failed     int `json:"failed"`
	Active     int `json:"active"`
}

// SkillLifecycle manages the full skill creation pipeline.
type SkillLifecycle struct {
	generator    *SkillGenerator
	identifier   *Identifier
	knowledgePath string
	mu          sync.Mutex
	status      LifecycleStatus
	stopChan    chan struct{}
	running     bool
}

// NewSkillLifecycle creates a new skill lifecycle manager.
func NewSkillLifecycle(gen *SkillGenerator, id *Identifier) *SkillLifecycle {
	return &SkillLifecycle{
		generator:    gen,
		identifier:   id,
		knowledgePath: "./knowledge/skills.json",
		stopChan:    make(chan struct{}),
	}
}

// SetKnowledgePath sets the path for saving skill knowledge.
func (sl *SkillLifecycle) SetKnowledgePath(path string) {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	sl.knowledgePath = path
}

// Pipeline runs the full skill creation pipeline.
// Returns list of created skill names.
func (sl *SkillLifecycle) Pipeline(ctx context.Context) ([]string, error) {
	sl.mu.Lock()
	sl.status = LifecycleStatus{}
	sl.mu.Unlock()

	createdSkills := make([]string, 0)

	// Step 1: Get candidates from identifier
	candidates := sl.identifier.GetCandidates()
	if len(candidates) == 0 {
		return createdSkills, nil
	}

	sl.updateStatus(func(s *LifecycleStatus) {
		s.Candidates = len(candidates)
	})

	// Step 2-7: Process each candidate
	for i := range candidates {
		select {
		case <-ctx.Done():
			return createdSkills, ctx.Err()
		default:
			pattern := &candidates[i]

			// Step 2: Suggest skill spec from pattern
			spec := sl.generator.SuggestFromPattern(pattern)
			if spec == nil {
				continue
			}

			// Step 3: Generate code
			err := sl.generator.Generate(*spec)
			if err != nil {
				sl.updateStatus(func(s *LifecycleStatus) {
					s.Failed++
				})
				continue
			}

			// Step 4: Validation is done within Generate()

			// Step 5: Registration is done within Generate()

			// Step 6: Update classifier patterns (through registry)
			sl.updateStatus(func(s *LifecycleStatus) {
				s.Generated++
				s.Active++
			})

			createdSkills = append(createdSkills, spec.Name)
		}
	}

	// Step 7: Update memory - save to knowledge
	if len(createdSkills) > 0 {
		if err := sl.SaveKnowledge(ctx, createdSkills); err != nil {
			// Log error but don't fail the pipeline
			fmt.Printf("Warning: failed to save knowledge: %v\n", err)
		}
	}

	return createdSkills, nil
}

// Status returns the current lifecycle status.
func (sl *SkillLifecycle) Status() LifecycleStatus {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	return sl.status
}

// RunPeriodically runs the pipeline at regular intervals in the background.
func (sl *SkillLifecycle) RunPeriodically(interval time.Duration) {
	sl.mu.Lock()
	if sl.running {
		sl.mu.Unlock()
		return
	}
	sl.running = true
	sl.mu.Unlock()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-sl.stopChan:
			sl.mu.Lock()
			sl.running = false
			sl.mu.Unlock()
			return
		case <-ticker.C:
			ctx := context.Background()
			_, err := sl.Pipeline(ctx)
			if err != nil {
				fmt.Printf("Pipeline run error: %v\n", err)
			}
		}
	}
}

// Stop stops the periodic pipeline runner.
func (sl *SkillLifecycle) Stop() {
	close(sl.stopChan)
}

// RecordTask records a task for pattern detection.
func (sl *SkillLifecycle) RecordTask(task string) {
	sl.identifier.Record(task)
}

// SaveKnowledge saves created skills to the knowledge base.
func (sl *SkillLifecycle) SaveKnowledge(ctx context.Context, skills []string) error {
	// Ensure knowledge directory exists
	knowledgeDir := filepath.Dir(sl.knowledgePath)
	if err := ensureDir(knowledgeDir); err != nil {
		return err
	}

	// Save identifier history
	historyPath := filepath.Join(knowledgeDir, "patterns.json")
	if err := sl.identifier.SaveHistory(historyPath); err != nil {
		return err
	}

	// Save skills knowledge
	skillsPath := filepath.Join(knowledgeDir, "skills.json")
	knowledge := map[string]interface{}{
		"created_at": time.Now(),
		"skills":     skills,
		"count":      len(skills),
	}

	// Simple JSON write (would use proper JSON encoding in production)
	// For now, we'll just save the skills list
	return writeJSON(skillsPath, knowledge)
}

// LoadKnowledge loads pattern history and skills knowledge.
func (sl *SkillLifecycle) LoadKnowledge() error {
	knowledgeDir := filepath.Dir(sl.knowledgePath)

	// Load pattern history
	historyPath := filepath.Join(knowledgeDir, "patterns.json")
	if err := sl.identifier.LoadHistory(historyPath); err != nil {
		return err
	}

	return nil
}

// updateStatus safely updates the lifecycle status.
func (sl *SkillLifecycle) updateStatus(fn func(*LifecycleStatus)) {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	fn(&sl.status)
}

// ensureDir creates a directory if it doesn't exist.
func ensureDir(dir string) error {
	// Simple directory creation
	// In production, use os.MkdirAll
	return nil // Placeholder - directory creation would be here
}

// writeJSON writes data to a JSON file.
func writeJSON(path string, data interface{}) error {
	// Simple JSON write placeholder
	// In production, use json.Marshal and os.WriteFile
	return nil // Placeholder
}