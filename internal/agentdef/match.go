package agentdef

import (
	"regexp"
	"strings"
)

// MatchScore reports how strongly an agent's triggers fire against a task.
// Returns 0 when nothing matches.
//
// Scoring mirrors the original internal/orchestrator/classifier.go weights:
//   - keyword match: +1 each
//   - anchor match:  +3 each (strong domain signals like ".tsx" or " k8s ")
//   - regex match:   +1 each
//
// Keyword and anchor matching is case-insensitive substring (matching the
// pre-refactor classifier exactly). Regex is compiled and matched as-is.
func MatchScore(def AgentDef, task string) int {
	if len(def.Triggers.Keywords) == 0 && len(def.Triggers.Anchors) == 0 && len(def.Triggers.Regex) == 0 {
		return 0
	}
	taskLower := strings.ToLower(task)
	score := 0
	for _, kw := range def.Triggers.Keywords {
		if kw != "" && strings.Contains(taskLower, strings.ToLower(kw)) {
			score++
		}
	}
	for _, a := range def.Triggers.Anchors {
		if a != "" && strings.Contains(taskLower, strings.ToLower(a)) {
			score += 3
		}
	}
	for _, pat := range def.Triggers.Regex {
		if pat == "" {
			continue
		}
		re, err := regexp.Compile(pat)
		if err != nil {
			continue // malformed regex silently ignored; loader warns at load time
		}
		if re.MatchString(task) {
			score++
		}
	}
	return score
}

// PriorityRank converts the priority string to a sortable int. Higher wins.
// Unknown values default to normal.
func PriorityRank(p string) int {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case "high":
		return 2
	case "low":
		return 0
	default: // "normal", ""
		return 1
	}
}

// MinSpecialistScore is the threshold a specialist agent must clear to win
// dispatch. Single accidental substring hits (e.g. "ui " inside "quit") are
// not enough. Preserved from the pre-refactor classifier for behavioral
// parity.
const MinSpecialistScore = 2

// BestMatch scans defs and returns the name of the highest-scoring agent.
// Specialists must clear MinSpecialistScore; otherwise the agent named
// "generalist" is returned as the fallback. If no generalist is loaded,
// returns "" (the caller decides what to do — typically route to whatever
// agent is registered under the RoleGeneralist constant).
//
// Ties are broken first by PriorityRank (high > normal > low) then by the
// order defs appears in the slice (callers should pass a slice sorted by
// SourcePath for deterministic load-order tiebreaks, matching the loader).
func BestMatch(task string, defs []AgentDef) string {
	if len(defs) == 0 {
		return ""
	}
	type cand struct {
		def           AgentDef
		score         int
		priorityRank  int
		loadOrder     int
	}
	var cands []cand
	for i, d := range defs {
		cands = append(cands, cand{def: d, score: MatchScore(d, task), priorityRank: PriorityRank(d.Priority), loadOrder: i})
	}

	// Find best specialist (>= MinSpecialistScore).
	bestIdx := -1
	for i, c := range cands {
		if c.score < MinSpecialistScore {
			continue
		}
		if bestIdx == -1 {
			bestIdx = i
			continue
		}
		cur := cands[bestIdx]
		switch {
		case c.score != cur.score:
			if c.score > cur.score {
				bestIdx = i
			}
		case c.priorityRank != cur.priorityRank:
			if c.priorityRank > cur.priorityRank {
				bestIdx = i
			}
		case c.loadOrder < cur.loadOrder:
			bestIdx = i
		}
	}
	if bestIdx != -1 {
		return cands[bestIdx].def.Name
	}

	// Fallback: find an agent literally named "generalist".
	for _, d := range defs {
		if d.Name == "generalist" {
			return d.Name
		}
	}

	// Last resort: the first loaded agent. Better than returning "" — the
	// caller would otherwise have no agent to dispatch to.
	return defs[0].Name
}
