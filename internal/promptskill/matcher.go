package promptskill

import (
	"regexp"
	"sort"
	"strings"
)

// MatchOptions controls how the matcher caps and ranks results. Zero values
// mean "no limit" — callers should set sensible caps to avoid prompt bloat.
type MatchOptions struct {
	// MaxSkills caps the number of skills returned. 0 = unlimited.
	MaxSkills int
	// MaxTotalChars caps the summed body length of returned skills. 0 = unlimited.
	// Skills are added in ranked order; once the cap would be exceeded, no more
	// are added (so the result stays under the cap).
	MaxTotalChars int
}

// matchResult tracks per-skill scoring for ranking.
type matchResult struct {
	skill PromptSkill
	score int // number of distinct keyword hits + regex hits
}

// Match returns all skills whose triggers fire against the task, ranked by
// priority (high first) then by specificity (more hits first). Results are
// capped per opts to keep the injected prompt bounded.
func (r *Registry) Match(task string, opts MatchOptions) []PromptSkill {
	if r == nil || len(r.skills) == 0 {
		return nil
	}
	taskLower := strings.ToLower(task)

	// Pre-compile regexes once per skill (cached on the skill via a sync.Map
	// would be ideal, but compiling per-match is acceptable for the skill
	// counts we expect — tens, not thousands).
	var results []matchResult
	for _, s := range r.skills {
		score := scoreSkill(s, task, taskLower)
		if score > 0 {
			results = append(results, matchResult{skill: s, score: score})
		}
	}
	if len(results) == 0 {
		return nil
	}

	sort.SliceStable(results, func(i, j int) bool {
		pi := priorityRank(results[i].skill.Priority)
		pj := priorityRank(results[j].skill.Priority)
		if pi != pj {
			return pi > pj // higher priority first
		}
		return results[i].score > results[j].score // more hits first
	})

	out := make([]PromptSkill, 0, len(results))
	total := 0
	for _, mr := range results {
		if opts.MaxSkills > 0 && len(out) >= opts.MaxSkills {
			break
		}
		bodyLen := len(mr.skill.Body)
		if opts.MaxTotalChars > 0 && total+bodyLen > opts.MaxTotalChars && len(out) > 0 {
			// Adding this would exceed the cap; stop. We always keep at least
			// the first (highest-ranked) match even if it alone exceeds the cap.
			break
		}
		out = append(out, mr.skill)
		total += bodyLen
	}
	return out
}

// scoreSkill returns >0 if the skill matches the task; the magnitude reflects
// how many distinct triggers fired (a proxy for relevance).
//
// Keyword matching uses a LEFT word-boundary anchor (\b) so short keywords like
// "ss" don't false-match substrings inside larger words (e.g. "ss" in
// "processes"). The keyword only fires when it starts at a word boundary,
// which matches standalone commands ("run ss -tlnp") and prefixes
// ("network" matches "networking") without matching embedded letter pairs.
func scoreSkill(s PromptSkill, task, taskLower string) int {
	score := 0
	for _, kw := range s.Triggers.Keywords {
		if kw == "" {
			continue
		}
		kwLower := strings.ToLower(kw)
		// Require a word boundary before the keyword. This prevents "ss" from
		// matching "processes" while still matching "ss -tlnp" and "network"
		// matching "networking".
		pat := `\b` + regexp.QuoteMeta(kwLower)
		re, err := regexp.Compile(pat)
		if err != nil {
			// Fallback: substring match (shouldn't happen with QuoteMeta)
			if strings.Contains(taskLower, kwLower) {
				score++
			}
			continue
		}
		if re.MatchString(taskLower) {
			score++
		}
	}
	for _, pat := range s.Triggers.Regex {
		if pat == "" {
			continue
		}
		re, err := regexp.Compile(pat)
		if err != nil {
			continue // malformed regex silently ignored — loader should have warned
		}
		if re.MatchString(task) {
			score++
		}
	}
	return score
}

// priorityRank converts the priority string to a sortable int. Unknown values
// default to normal.
func priorityRank(p string) int {
	switch strings.ToLower(p) {
	case "high":
		return 2
	case "normal", "":
		return 1
	}
	return 1
}
