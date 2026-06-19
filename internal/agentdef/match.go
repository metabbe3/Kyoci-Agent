package agentdef

import (
	"regexp"
	"strings"
)

// negationRe matches a negation token that ends immediately before a trigger
// term, bounded on the left by a non-alphanumeric (or start of text) so words
// like "piano" or "info" are not mistaken for "no". An optional run of common
// filler words (a/an/the/plain/use/…) may sit between the negation and the
// term, so "no html", "not plain html", "don't use tailwind", "without any
// css" are all recognized as negated. Case-insensitive (callers pre-lowercase).
var negationRe = regexp.MustCompile(`(?:^|[^a-z0-9])(?:no|not|without|avoid|skip|never|non|n't|don't|dont|cannot|can't|won't|shouldn't)(?:[\s-]+(?:a|an|the|this|that|plain|raw|vanilla|any|just|only|more|use|using|used|write|writing|written|build|building|built|add|adding|added|include|including|included|generate|generating|generated|produce|producing|produced|create|creating|created|make|making))*[\s-]*$`)

// isNegated reports whether the text immediately preceding position idx in
// taskLower ends with a negation token (e.g. in "no html", the "html" at idx
// is negated). Inspects a small window (32 chars) before idx.
func isNegated(taskLower string, idx int) bool {
	start := idx - 32
	if start < 0 {
		start = 0
	}
	return negationRe.MatchString(taskLower[start:idx])
}

// containsNonNegated reports whether needle occurs in haystack at least once at
// a position NOT preceded by a negation token. A negated occurrence ("no html")
// is skipped; a separate positive occurrence still counts. haystack must be
// already lowercased; needle is lowercased here.
func containsNonNegated(haystack, needle string) bool {
	needle = strings.ToLower(needle)
	if needle == "" {
		return false
	}
	from := 0
	for {
		i := strings.Index(haystack[from:], needle)
		if i < 0 {
			return false
		}
		pos := from + i
		if !isNegated(haystack, pos) {
			return true
		}
		from = pos + len(needle)
	}
}

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
		if kw != "" && containsNonNegated(taskLower, kw) {
			score++
		}
	}
	for _, a := range def.Triggers.Anchors {
		if a != "" && containsNonNegated(taskLower, a) {
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
