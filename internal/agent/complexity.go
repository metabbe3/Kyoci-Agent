package agent

import (
	"regexp"
	"strings"
)

// ComplexityReport is the output of assessComplexity. Each field is a separate
// signal; Complex is the OR of all of them. The Assess state uses Complex to
// decide between the fast path (simple → Execute) and the full multi-pass
// loop (complex → Plan).
type ComplexityReport struct {
	MultiFile    bool
	Vague        bool
	ExplicitPlan bool
	AnyFailure   bool
	Complex      bool
}

// Pre-compiled regexes for performance and clarity.

// multiFileVerbRe matches verbs that typically imply multi-file work.
var multiFileVerbRe = regexp.MustCompile(`(?i)\b(create|update|modify|refactor|rename|move|delete|migrate)\b`)

// multiFileConnectorRe matches connectors that link multiple targets.
var multiFileConnectorRe = regexp.MustCompile(`(?i)\b(and|across|multiple|each|every|all)\b`)

// filePathRe matches common file-path shapes (at least one path segment + extension).
var filePathRe = regexp.MustCompile(`(?:[\w./-]+/)*[\w-]+\.\w+`)

// explicitPlanRe matches user phrasings that explicitly ask for a plan.
var explicitPlanRe = regexp.MustCompile(`(?i)\b(plan|planning|steps|break down|how would you|decompose|outline)\b`)

// concreteReferenceRe matches file paths, commands, code snippets, or URLs —
// signals that the task has enough context not to be vague.
var concreteReferenceRe = regexp.MustCompile(`(?i)(\.\w{1,5}\b|/[\w.-]+|` + "`" + `|https?://)`)

// assessComplexity classifies a task using deterministic heuristics. It is
// deliberately permissive — false positives (flagging a simple task as complex)
// just mean the full Plan state runs, which still works. False negatives
// (missing a complex task) are the real risk.
func assessComplexity(task string, history []failureEntry) ComplexityReport {
	task = strings.TrimSpace(task)
	r := ComplexityReport{
		AnyFailure: len(history) > 0,
	}

	// MultiFile: verb + connector, OR ≥2 distinct file paths mentioned.
	if multiFileVerbRe.MatchString(task) && multiFileConnectorRe.MatchString(task) {
		r.MultiFile = true
	}
	if matches := filePathRe.FindAllString(task, -1); len(matches) >= 2 {
		// Confirm at least 2 distinct paths
		seen := make(map[string]struct{}, len(matches))
		for _, m := range matches {
			seen[m] = struct{}{}
		}
		if len(seen) >= 2 {
			r.MultiFile = true
		}
	}

	// Vague: short task without a concrete reference, OR question without one.
	// A short task like "create hello.txt with content hi" has a file ref and
	// is NOT vague. "fix it" or "why does it fail?" lack any concrete anchor.
	wordCount := len(strings.Fields(task))
	hasConcreteRef := concreteReferenceRe.MatchString(task)
	if wordCount > 0 && wordCount < 8 && !hasConcreteRef {
		r.Vague = true
	}
	if strings.Contains(task, "?") && !hasConcreteRef {
		r.Vague = true
	}

	// ExplicitPlan: user phrasing asks for a plan/steps/decomposition.
	if explicitPlanRe.MatchString(task) {
		r.ExplicitPlan = true
	}

	r.Complex = r.MultiFile || r.Vague || r.ExplicitPlan || r.AnyFailure
	return r
}
