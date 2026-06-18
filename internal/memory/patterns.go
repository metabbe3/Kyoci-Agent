package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// =============================================================================
// Pattern Detection + Auto-Skill Generation
// =============================================================================
//
// After each successful task, check if similar task patterns have been seen
// before. When a pattern reaches the threshold (2+ similar successful tasks),
// automatically generate a skill file so future tasks of the same type can
// use the zero-AI fast path.
//
// This is entirely rule-based — no extra LLM calls. Designed for gemma4 8B.

// SkillGeneratorConfig controls auto-skill generation behavior.
type SkillGeneratorConfig struct {
	// MinPatternCount is how many similar successful tasks before auto-generating
	MinPatternCount int
	// SkillsDir is where generated skill files are saved
	SkillsDir string
}

// DefaultSkillGeneratorConfig returns sensible defaults.
func DefaultSkillGeneratorConfig() SkillGeneratorConfig {
	return SkillGeneratorConfig{
		MinPatternCount: 2,
		SkillsDir:       "data/skills",
	}
}

// PatternDetector analyzes experiences and auto-generates skills.
type PatternDetector struct {
	config  SkillGeneratorConfig
	storage *LongTermMemory
	logger  *slog.Logger
	mu      sync.Mutex

	// Track which skills we've already generated to avoid duplicates
	generated map[string]bool
}

// NewPatternDetector creates a new pattern detector.
func NewPatternDetector(ltm *LongTermMemory, cfg SkillGeneratorConfig, logger *slog.Logger) *PatternDetector {
	if logger == nil {
		logger = slog.Default()
	}
	return &PatternDetector{
		config:    cfg,
		storage:   ltm,
		logger:    logger.With("component", "pattern-detector"),
		generated: make(map[string]bool),
	}
}

// CheckAndGenerate analyzes recent experiences and auto-creates skills
// when patterns are detected. Called after each successful task.
// Returns the name of the skill generated (if any), or empty string.
func (pd *PatternDetector) CheckAndGenerate(ctx context.Context, rec ExperienceRecord) string {
	if !rec.Success {
		return ""
	}

	pd.mu.Lock()
	defer pd.mu.Unlock()

	// Extract task signature: primary tool + top keywords
	keywords := extractKeywords(rec.Task)
	signature := taskSignature(keywords, rec.ToolsUsed)

	// Skip if we already generated a skill for this pattern
	if pd.generated[signature] {
		return ""
	}

	// Find similar successful experiences
	similar := pd.findSimilarExperiences(ctx, keywords, rec.ToolsUsed, 10)

	// Check if we have enough similar tasks to warrant a skill
	if len(similar) < pd.config.MinPatternCount {
		return ""
	}

	// Generate skill
	skillName := generateSkillName(keywords, rec.ToolsUsed)
	skillContent := pd.generateSkillContent(skillName, rec, similar)

	// Save to disk
	skillPath := filepath.Join(pd.config.SkillsDir, skillName+".md")
	if err := os.MkdirAll(pd.config.SkillsDir, 0755); err != nil {
		pd.logger.Warn("failed to create skills dir", "error", err)
		return ""
	}

	if _, err := os.Stat(skillPath); err == nil {
		// Skill already exists — don't overwrite
		pd.generated[signature] = true
		return ""
	}

	if err := os.WriteFile(skillPath, []byte(skillContent), 0644); err != nil {
		pd.logger.Warn("failed to write skill file", "error", err)
		return ""
	}

	pd.generated[signature] = true
	pd.logger.Info("auto-skill generated",
		"skill", skillName,
		"pattern_count", len(similar),
		"path", skillPath)

	// Also store in L3 memory as a lesson
	pd.storeLesson(ctx, skillName, rec, similar)

	return skillName
}

// findSimilarExperiences searches L3 for tasks with overlapping keywords + tools.
func (pd *PatternDetector) findSimilarExperiences(ctx context.Context, keywords []string, tools []string, limit int) []ExperienceRecord {
	// Search using top keyword
	query := strings.Join(keywords[:min(3, len(keywords))], " ")
	if query == "" {
		return nil
	}

	entries, err := pd.storage.Recall(ctx, query, limit, kyoci.MemoryLongTerm)
	if err != nil {
		return nil
	}

	var records []ExperienceRecord
	for _, entry := range entries {
		if entry.Metadata["category"] != string(CategoryExperience) {
			continue
		}
		if entry.Metadata["success"] != "true" {
			continue
		}
		var rec ExperienceRecord
		if err := json.Unmarshal([]byte(entry.Content), &rec); err != nil {
			continue
		}
		// Check tool overlap
		if hasToolOverlap(rec.ToolsUsed, tools) {
			records = append(records, rec)
		}
	}

	return records
}

// generateSkillContent creates a markdown skill file from the pattern data.
func (pd *PatternDetector) generateSkillContent(name string, latest ExperienceRecord, similar []ExperienceRecord) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# %s\n\n", name))
	sb.WriteString(fmt.Sprintf("> Auto-generated from %d similar successful tasks.\n\n", len(similar)))

	// Trigger keywords
	keywords := extractKeywords(latest.Task)
	sb.WriteString("## Triggers\n")
	sb.WriteString(fmt.Sprintf("Keywords: %s\n\n", strings.Join(keywords, ", ")))

	// Tools used (sorted by frequency across all similar tasks)
	toolFreq := make(map[string]int)
	for _, s := range similar {
		for _, t := range s.ToolsUsed {
			toolFreq[t]++
		}
	}
	sb.WriteString("## Recommended Tools\n")
	type toolEntry struct {
		name string
		freq int
	}
	var tools []toolEntry
	for name, freq := range toolFreq {
		tools = append(tools, toolEntry{name, freq})
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].freq > tools[j].freq })
	for i, t := range tools {
		emoji := toolEmoji(t.name)
		sb.WriteString(fmt.Sprintf("%d. %s %s (used %d/%d times)\n", i+1, emoji, t.name, t.freq, len(similar)))
	}

	// Performance stats
	totalDur := int64(0)
	totalIter := 0
	for _, s := range similar {
		totalDur += s.DurationMs
		totalIter += s.Iterations
	}
	avgDur := totalDur / int64(len(similar))
	avgIter := totalIter / len(similar)

	sb.WriteString("\n## Performance\n")
	sb.WriteString(fmt.Sprintf("- Average duration: %s\n", formatMs(avgDur)))
	sb.WriteString(fmt.Sprintf("- Average iterations: %d\n", avgIter))
	sb.WriteString(fmt.Sprintf("- Success rate: 100%% (sampled from %d successful tasks)\n", len(similar)))

	// Best practices derived from patterns
	sb.WriteString("\n## Approach\n")
	sb.WriteString(fmt.Sprintf("1. Identify the task type from user message\n"))
	sb.WriteString(fmt.Sprintf("2. Use %s tool to execute\n", tools[0].name))
	if len(tools) > 1 {
		sb.WriteString(fmt.Sprintf("3. Use %s if needed for follow-up\n", tools[1].name))
	}
	sb.WriteString("4. Report results concisely\n")

	sb.WriteString("\n---\n")
	sb.WriteString(fmt.Sprintf("Generated: %s\n", time.Now().Format("2006-01-02 15:04:05")))

	return sb.String()
}

// storeLesson stores the skill generation event as a lesson in L3.
func (pd *PatternDetector) storeLesson(ctx context.Context, skillName string, rec ExperienceRecord, similar []ExperienceRecord) {
	lesson := fmt.Sprintf("Skill '%s' auto-generated: %d similar tasks detected. Tools: %s",
		skillName, len(similar), strings.Join(rec.ToolsUsed, ", "))

	metadata := map[string]string{
		"category":   string(CategoryLesson),
		"skill_name": skillName,
	}

	_, err := pd.storage.Store(ctx, lesson, kyoci.MemoryLongTerm, metadata)
	if err != nil {
		pd.logger.Warn("failed to store skill lesson", "error", err)
	}
}

// ── Helper Functions ──────────────────────────────────────────────

var stopWords = map[string]bool{
	"the": true, "a": true, "an": true, "is": true, "are": true, "was": true,
	"to": true, "of": true, "in": true, "on": true, "at": true, "by": true,
	"for": true, "with": true, "from": true, "and": true, "or": true, "not": true,
	"this": true, "that": true, "it": true, "be": true, "have": true, "has": true,
	"do": true, "does": true, "did": true, "will": true, "would": true, "can": true,
	"could": true, "should": true, "may": true, "might": true, "must": true,
	"i": true, "you": true, "he": true, "she": true, "we": true, "they": true,
	"me": true, "him": true, "her": true, "us": true, "them": true,
	"my": true, "your": true, "his": true, "its": true, "our": true, "their": true,
	"use": true, "using": true, "what": true, "how": true, "when": true, "where": true,
	"please": true, "just": true, "like": true, "about": true, "into": true,
	"all": true, "any": true, "some": true, "no": true, "yes": true,
	"check": true, "show": true, "get": true, "make": true, "tell": true,
	"give": true, "now": true, "then": true, "also": true,
}

// extractKeywords pulls meaningful keywords from a task string.
func extractKeywords(task string) []string {
	words := strings.Fields(strings.ToLower(task))
	var keywords []string
	for _, w := range words {
		w = strings.Trim(w, ".,!?;:\"'()[]{}")
		if len(w) < 3 {
			continue
		}
		if stopWords[w] {
			continue
		}
		keywords = append(keywords, w)
	}
	return keywords
}

// taskSignature creates a unique signature for a task pattern.
func taskSignature(keywords []string, tools []string) string {
	// Use top 2 keywords + primary tool
	kwPart := ""
	if len(keywords) > 0 {
		kwPart = keywords[0]
	}
	if len(keywords) > 1 {
		kwPart += "_" + keywords[1]
	}
	toolPart := ""
	if len(tools) > 0 {
		toolPart = tools[0]
	}
	return kwPart + "_" + toolPart
}

// generateSkillName creates a clean filename from keywords + tool.
func generateSkillName(keywords []string, tools []string) string {
	var parts []string
	if len(keywords) > 0 {
		parts = append(parts, keywords[0])
	}
	if len(keywords) > 1 {
		parts = append(parts, keywords[1])
	}
	if len(tools) > 0 {
		parts = append(parts, "via")
		parts = append(parts, tools[0])
	}
	if len(parts) == 0 {
		parts = []string{"auto", "task"}
	}

	// Clean for filename
	name := strings.Join(parts, "-")
	name = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		return '-'
	}, name)
	return "auto-" + name
}

// hasToolOverlap checks if two tool lists share any tools.
func hasToolOverlap(a, b []string) bool {
	set := make(map[string]bool)
	for _, t := range b {
		set[t] = true
	}
	for _, t := range a {
		if set[t] {
			return true
		}
	}
	return false
}

// toolEmoji returns an emoji for display.
func toolEmoji(tool string) string {
	emojis := map[string]string{
		"terminal":      "⚙️",
		"file":          "📄",
		"browser":       "🌐",
		"docs":          "📚",
		"http_client":   "🔌",
		"web_search":    "🔍",
		"calculator":    "🔢",
		"todo":          "✅",
		"skill":         "🧠",
		"process":       "⚡",
		"memory_recall": "💭",
		"remember":      "💾",
		"delegation":    "🤖",
	}
	if e, ok := emojis[tool]; ok {
		return e
	}
	return "🔧"
}

// formatMs converts milliseconds to human-readable duration.
func formatMs(ms int64) string {
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	return fmt.Sprintf("%.1fs", float64(ms)/1000)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
