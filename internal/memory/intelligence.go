package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// =============================================================================
// Intelligence Layer — L1/L2/L3 Memory + Experience + Profile + Reflection
// =============================================================================
//
// This file implements the Hermes-inspired intelligence systems:
//
//   L1 (Working)    — current conversation, token-budgeted (existing ShortTermMemory)
//   L2 (Session)    — compacted summaries when L1 fills up (existing Compactor)
//   L3 (Long-Term)  — persistent facts, preferences, lessons, experiences
//
//   ExperienceEngine — records task outcomes, detects patterns, suggests approaches
//   ProfileStore     — persistent user facts (name, preferences, environment)
//   ReflectionEngine — post-task analysis for complex tasks
//
// All systems are designed for gemma4:latest (8B) — the Go code does the
// intelligence work, not the LLM. No extra LLM calls for routine operations.

// =============================================================================
// Memory Category Constants
// =============================================================================

// MemoryCategory classifies what kind of information is stored in L3.
type MemoryCategory string

const (
	CategoryFact        MemoryCategory = "fact"         // e.g., "User's name is Nicholas"
	CategoryPreference  MemoryCategory = "preference"   // e.g., "User prefers concise responses"
	CategoryLesson      MemoryCategory = "lesson"       // e.g., "Telegram needs plain text, not Markdown"
	CategoryExperience  MemoryCategory = "experience"   // task outcome record
	CategorySummary     MemoryCategory = "summary"      // compacted conversation summary
)

// =============================================================================
// Experience Record
// =============================================================================

// ExperienceRecord captures the outcome of a single task execution.
// This is the core data structure for the self-improvement system.
type ExperienceRecord struct {
	ID           string    `json:"id"`
	Task         string    `json:"task"`
	Role         string    `json:"role"`
	ToolsUsed    []string  `json:"tools_used"`
	Iterations   int       `json:"iterations"`
	ToolCalls    int       `json:"tool_calls"`
	Success      bool      `json:"success"`
	DurationMs   int64     `json:"duration_ms"`
	ErrorMsg     string    `json:"error,omitempty"`
	TaskHash     string    `json:"task_hash"`  // for similarity matching
	CreatedAt    time.Time `json:"created_at"`
}

// ExperienceStats holds aggregate statistics about recorded experiences.
type ExperienceStats struct {
	TotalExperiences  int            `json:"total"`
	SuccessfulTasks   int            `json:"successful"`
	FailedTasks       int            `json:"failed"`
	SuccessRate       float64        `json:"success_rate"`
	AvgIterations     float64        `json:"avg_iterations"`
	AvgDurationMs     int64          `json:"avg_duration_ms"`
	ByTool            map[string]int `json:"by_tool"`
}

// =============================================================================
// Experience Engine
// =============================================================================

// ExperienceEngine records task outcomes and learns patterns from them.
// It does NOT make extra LLM calls — all intelligence is rule-based.
// Thread-safe.
type ExperienceEngine struct {
	mu      sync.RWMutex
	storage *LongTermMemory
	logger  *slog.Logger
}

// NewExperienceEngine creates a new experience engine backed by long-term memory.
func NewExperienceEngine(ltm *LongTermMemory, logger *slog.Logger) *ExperienceEngine {
	if logger == nil {
		logger = slog.Default()
	}
	return &ExperienceEngine{
		storage: ltm,
		logger:  logger.With("component", "experience-engine"),
	}
}

// Storage returns the underlying long-term memory store (for PatternDetector).
func (ee *ExperienceEngine) Storage() *LongTermMemory {
	return ee.storage
}

// Record stores a new experience in L3 memory.
func (ee *ExperienceEngine) Record(rec ExperienceRecord) error {
	ee.mu.Lock()
	defer ee.mu.Unlock()

	if rec.ID == "" {
		rec.ID = fmt.Sprintf("exp_%d", time.Now().UnixNano())
	}
	rec.CreatedAt = time.Now()
	rec.TaskHash = hashTask(rec.Task)

	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("failed to marshal experience: %w", err)
	}

	metadata := map[string]string{
		"category":   string(CategoryExperience),
		"task_hash":  rec.TaskHash,
		"success":    fmt.Sprintf("%v", rec.Success),
		"role":       rec.Role,
	}

	_, err = ee.storage.Store(string(data), kyoci.MemoryLongTerm, metadata)
	if err != nil {
		return fmt.Errorf("failed to store experience: %w", err)
	}

	ee.logger.Debug("experience recorded",
		"task", truncateForLog(rec.Task, 60),
		"success", rec.Success,
		"iterations", rec.Iterations,
		"duration_ms", rec.DurationMs)

	return nil
}

// FindSimilar retrieves past experiences with similar tasks.
// Uses FTS5 full-text search on the task content.
func (ee *ExperienceEngine) FindSimilar(task string, limit int) ([]ExperienceRecord, error) {
	ee.mu.RLock()
	defer ee.mu.RUnlock()

	if limit <= 0 {
		limit = 5
	}

	// Search L3 for experiences matching this task
	entries, err := ee.storage.Recall(task, limit, kyoci.MemoryLongTerm)
	if err != nil {
		return nil, fmt.Errorf("failed to search experiences: %w", err)
	}

	records := make([]ExperienceRecord, 0, len(entries))
	for _, entry := range entries {
		// Only parse entries that are experiences
		if entry.Metadata["category"] != string(CategoryExperience) {
			continue
		}
		var rec ExperienceRecord
		if err := json.Unmarshal([]byte(entry.Content), &rec); err == nil {
			records = append(records, rec)
		}
	}

	return records, nil
}

// GetStats computes aggregate statistics from all recorded experiences.
func (ee *ExperienceEngine) GetStats() ExperienceStats {
	ee.mu.RLock()
	defer ee.mu.RUnlock()

	stats := ExperienceStats{
		ByTool: make(map[string]int),
	}

	// Query all experiences (broad search)
	entries, err := ee.storage.Recall("", 500, kyoci.MemoryLongTerm)
	if err != nil {
		ee.logger.Warn("failed to get experience stats", "error", err)
		return stats
	}

	totalDuration := int64(0)
	totalIterations := 0
	count := 0

	for _, entry := range entries {
		if entry.Metadata["category"] != string(CategoryExperience) {
			continue
		}
		var rec ExperienceRecord
		if err := json.Unmarshal([]byte(entry.Content), &rec); err != nil {
			continue
		}
		count++
		stats.TotalExperiences++
		if rec.Success {
			stats.SuccessfulTasks++
		} else {
			stats.FailedTasks++
		}
		totalIterations += rec.Iterations
		totalDuration += rec.DurationMs
		for _, tool := range rec.ToolsUsed {
			stats.ByTool[tool]++
		}
	}

	if count > 0 {
		stats.SuccessRate = float64(stats.SuccessfulTasks) / float64(count)
		stats.AvgIterations = float64(totalIterations) / float64(count)
		stats.AvgDurationMs = totalDuration / int64(count)
	}

	return stats
}

// SuggestApproach generates a context string from past experiences for similar tasks.
// This is injected into the system prompt to help the agent learn from history.
// Returns empty string if no relevant experiences exist.
func (ee *ExperienceEngine) SuggestApproach(task string) string {
	records, err := ee.FindSimilar(task, 3)
	if err != nil || len(records) == 0 {
		return ""
	}

	// Find successful experiences only
	var successful []ExperienceRecord
	for _, rec := range records {
		if rec.Success {
			successful = append(successful, rec)
		}
	}

	if len(successful) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("Past experience with similar tasks:\n")
	for i, rec := range successful {
		if i >= 2 { // Limit to top 2
			break
		}
		sb.WriteString(fmt.Sprintf("- Task: %s → Used: %s (iterations: %d)\n",
			truncateForLog(rec.Task, 50),
			strings.Join(rec.ToolsUsed, ", "),
			rec.Iterations))
	}

	return sb.String()
}

// =============================================================================
// Profile Store — Persistent User Facts
// =============================================================================

// ProfileEntry represents a single fact about the user.
type ProfileEntry struct {
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	Category  string    `json:"category"` // fact, preference, environment
	Source    string    `json:"source"`   // "conversation", "config", "inferred"
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ProfileStore manages persistent facts about the user.
// Stored in L3 memory with category=preference or category=fact.
// Thread-safe.
type ProfileStore struct {
	mu      sync.RWMutex
	storage *LongTermMemory
	cache   map[string]ProfileEntry
	logger  *slog.Logger
}

// NewProfileStore creates a new profile store.
func NewProfileStore(ltm *LongTermMemory, logger *slog.Logger) *ProfileStore {
	if logger == nil {
		logger = slog.Default()
	}
	ps := &ProfileStore{
		storage: ltm,
		cache:   make(map[string]ProfileEntry),
		logger:  logger.With("component", "profile-store"),
	}
	ps.loadCache()
	return ps
}

// loadCache loads all profile entries from L3 into memory.
// Called WITHOUT the write lock — used during construction.
func (ps *ProfileStore) loadCache() {
	ps.loadCacheLocked(false)
}

// Reload refreshes the in-memory cache from SQLite. Use this to pick up
// entries written by other paths (e.g. the `remember` tool, which writes
// via MemoryStore.Store and bypasses the ProfileStore cache). Thread-safe.
func (ps *ProfileStore) Reload() error {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.loadCacheLocked(true)
	return nil
}

// loadCacheLocked rebuilds ps.cache from SQLite. If holdLock is true, the
// caller already holds ps.mu (Reload path); if false, no lock is held
// (construction path).
func (ps *ProfileStore) loadCacheLocked(holdLock bool) {
	_ = holdLock // lock acquisition is the caller's responsibility
	entries, err := ps.storage.Recall("", 200, kyoci.MemoryLongTerm)
	if err != nil {
		ps.logger.Warn("failed to load profile cache", "error", err)
		return
	}

	// Rebuild cache from scratch so deletions in SQLite are reflected.
	ps.cache = make(map[string]ProfileEntry)


	for _, entry := range entries {
		cat := entry.Metadata["category"]
		if cat != string(CategoryFact) && cat != string(CategoryPreference) {
			continue
		}

		var pe ProfileEntry
		if err := json.Unmarshal([]byte(entry.Content), &pe); err == nil {
			ps.cache[pe.Key] = pe
		}
	}

	ps.logger.Info("profile loaded", "entries", len(ps.cache))
}

// Set stores or updates a profile entry.
func (ps *ProfileStore) Set(key, value, category, source string) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	now := time.Now()
	pe := ProfileEntry{
		Key:       key,
		Value:     value,
		Category:  category,
		Source:    source,
		UpdatedAt: now,
	}

	// Preserve original creation time if updating
	if existing, ok := ps.cache[key]; ok {
		pe.CreatedAt = existing.CreatedAt
	} else {
		pe.CreatedAt = now
	}

	data, err := json.Marshal(pe)
	if err != nil {
		return fmt.Errorf("failed to marshal profile entry: %w", err)
	}

	metadata := map[string]string{
		"category": category,
		"key":      key,
	}

	_, err = ps.storage.Store(string(data), kyoci.MemoryLongTerm, metadata)
	if err != nil {
		return fmt.Errorf("failed to store profile entry: %w", err)
	}

	ps.cache[key] = pe
	ps.logger.Debug("profile entry stored", "key", key, "value", value, "category", category)
	return nil
}

// Get retrieves a profile entry by key.
func (ps *ProfileStore) Get(key string) (ProfileEntry, bool) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	pe, ok := ps.cache[key]
	return pe, ok
}

// GetAll returns all profile entries.
func (ps *ProfileStore) GetAll() []ProfileEntry {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	entries := make([]ProfileEntry, 0, len(ps.cache))
	for _, pe := range ps.cache {
		entries = append(entries, pe)
	}
	return entries
}

// FormatForPrompt formats all profile entries for injection into the system prompt.
// Returns a human-readable string summarizing what the agent knows about the user.
func (ps *ProfileStore) FormatForPrompt() string {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	if len(ps.cache) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("What you know about the user:\n")

	for _, pe := range ps.cache {
		sb.WriteString(fmt.Sprintf("- %s: %s\n", pe.Key, pe.Value))
	}

	return sb.String()
}

// =============================================================================
// Reflection Engine — Post-Task Analysis
// =============================================================================

// ReflectionResult represents the output of reflecting on a completed task.
type ReflectionResult struct {
	ShouldRemember bool   `json:"should_remember"`
	Insight        string `json:"insight"`
	Category       string `json:"category"` // lesson, fact, preference
}

// ReflectionEngine analyzes completed tasks to extract lessons.
// Only activates for complex tasks (3+ iterations) to avoid overhead.
// Uses rule-based analysis — no extra LLM calls.
type ReflectionEngine struct {
	mu      sync.RWMutex
	storage *LongTermMemory
	profile *ProfileStore
	logger  *slog.Logger
}

// NewReflectionEngine creates a new reflection engine.
func NewReflectionEngine(ltm *LongTermMemory, profile *ProfileStore, logger *slog.Logger) *ReflectionEngine {
	if logger == nil {
		logger = slog.Default()
	}
	return &ReflectionEngine{
		storage: ltm,
		profile: profile,
		logger:  logger.With("component", "reflection-engine"),
	}
}

// Reflect analyzes a completed task and extracts any lessons worth remembering.
// This is called AFTER the task is complete and does NOT block the response.
func (re *ReflectionEngine) Reflect(ctx context.Context, rec ExperienceRecord) ([]ReflectionResult, error) {
	re.mu.Lock()
	defer re.mu.Unlock()

	results := make([]ReflectionResult, 0)

	// Rule 1: If task failed after many iterations, record as lesson
	if !rec.Success && rec.Iterations >= 5 {
		results = append(results, ReflectionResult{
			ShouldRemember: true,
			Insight: fmt.Sprintf("Task '%s' failed after %d iterations. Error: %s",
				truncateForLog(rec.Task, 80), rec.Iterations, rec.ErrorMsg),
			Category: string(CategoryLesson),
		})
	}

	// Rule 2: If task succeeded with many iterations, there may be a lesson
	if rec.Success && rec.Iterations >= 4 {
		results = append(results, ReflectionResult{
			ShouldRemember: true,
			Insight: fmt.Sprintf("Task '%s' required %d iterations. Tools: %s. Consider optimizing approach.",
				truncateForLog(rec.Task, 80), rec.Iterations, strings.Join(rec.ToolsUsed, ", ")),
			Category: string(CategoryLesson),
		})
	}

	// Rule 3: If task took very long, record as a performance data point
	if rec.DurationMs > 60000 { // >60 seconds
		results = append(results, ReflectionResult{
			ShouldRemember: true,
			Insight: fmt.Sprintf("Task '%s' took %dms. Long-running task pattern detected.",
				truncateForLog(rec.Task, 80), rec.DurationMs),
			Category: string(CategoryLesson),
		})
	}

	// Store lessons in L3
	for _, r := range results {
		if r.ShouldRemember {
			metadata := map[string]string{
				"category": r.Category,
				"task":     rec.Task,
			}
			_, err := re.storage.Store(r.Insight, kyoci.MemoryLongTerm, metadata)
			if err != nil {
				re.logger.Warn("failed to store reflection lesson", "error", err)
			}
		}
	}

	if len(results) > 0 {
		re.logger.Info("reflection completed",
			"insights", len(results),
			"task", truncateForLog(rec.Task, 60))
	}

	return results, nil
}

// GetRelevantLessons retrieves lessons that might be relevant to a given task.
func (re *ReflectionEngine) GetRelevantLessons(task string, limit int) string {
	re.mu.RLock()
	defer re.mu.RUnlock()

	if limit <= 0 {
		limit = 3
	}

	entries, err := re.storage.Recall(task, limit, kyoci.MemoryLongTerm)
	if err != nil {
		return ""
	}

	var lessons []string
	for _, entry := range entries {
		if entry.Metadata["category"] == string(CategoryLesson) {
			lessons = append(lessons, entry.Content)
		}
	}

	if len(lessons) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("Relevant lessons from past tasks:\n")
	for _, lesson := range lessons {
		sb.WriteString(fmt.Sprintf("- %s\n", lesson))
	}

	return sb.String()
}

// =============================================================================
// Context Injector — The Glue Between Memory and Agent
// =============================================================================

// ContextInjector enriches the agent's system prompt with relevant memories,
// experiences, lessons, and user profile data BEFORE the LLM is called.
// This is the key component that makes Kyoci "remember" like Hermes.
type ContextInjector struct {
	experience *ExperienceEngine
	profile    *ProfileStore
	reflection *ReflectionEngine
	logger     *slog.Logger
}

// NewContextInjector creates a new context injector with all subsystems.
func NewContextInjector(
	experience *ExperienceEngine,
	profile *ProfileStore,
	reflection *ReflectionEngine,
	logger *slog.Logger,
) *ContextInjector {
	if logger == nil {
		logger = slog.Default()
	}
	return &ContextInjector{
		experience: experience,
		profile:    profile,
		reflection: reflection,
		logger:     logger.With("component", "context-injector"),
	}
}

// Inject generates additional context to prepend to the system prompt.
// Returns a string with relevant memories, or empty string if nothing relevant.
// This does NOT make LLM calls — it searches L3 and formats results.
func (ci *ContextInjector) Inject(task string) string {
	var sb strings.Builder
	hasContent := false

	// 1. Inject user profile (reload first so entries written by prior
	//    sessions via the `remember` tool are visible mid-process).
	if ci.profile != nil {
		_ = ci.profile.Reload()
		profileCtx := ci.profile.FormatForPrompt()
		if profileCtx != "" {
			sb.WriteString(profileCtx)
			hasContent = true
		}
	}

	// 2. Inject relevant past experiences (successful approaches)
	if ci.experience != nil {
		expCtx := ci.experience.SuggestApproach(task)
		if expCtx != "" {
			if hasContent {
				sb.WriteString("\n")
			}
			sb.WriteString(expCtx)
			hasContent = true
		}
	}

	// 3. Inject relevant lessons
	if ci.reflection != nil {
		lessonCtx := ci.reflection.GetRelevantLessons(task, 3)
		if lessonCtx != "" {
			if hasContent {
				sb.WriteString("\n")
			}
			sb.WriteString(lessonCtx)
			hasContent = true
		}
	}

	if !hasContent {
		return ""
	}

	ci.logger.Debug("context injected",
		"task", truncateForLog(task, 60),
		"context_length", sb.Len())

	return sb.String()
}

// =============================================================================
// Utility Functions
// =============================================================================

// hashTask creates a simple hash of a task string for similarity matching.
func hashTask(task string) string {
	// Simple hash: take first 3 words + length
	words := strings.Fields(strings.ToLower(task))
	prefix := ""
	for i, w := range words {
		if i >= 3 {
			break
		}
		prefix += w + "_"
	}
	return fmt.Sprintf("%s%d", prefix, len(task))
}

// truncateForLog shortens a string for logging purposes.
func truncateForLog(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
