package builtin

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// RegexSkill tests and explains a regular expression against input text.
type RegexSkill struct {
	*kyoci.BaseSkill
	slashPattern *regexp.Regexp
}

// NewRegexSkill creates a new regex skill.
func NewRegexSkill() *RegexSkill {
	return &RegexSkill{
		BaseSkill: kyoci.NewBaseSkill(
			"regex",
			"Test and explain a regular expression against input text",
			[]string{"regex", "regular expression", "pattern match", "regexp"},
		),
		slashPattern: regexp.MustCompile(`(?s)/(.+)/`),
	}
}

// Match checks if the query references regex.
func (s *RegexSkill) Match(query string) bool {
	queryLower := strings.ToLower(query)
	for _, keyword := range []string{"regex", "regular expression", "regexp"} {
		if strings.Contains(queryLower, keyword) {
			return true
		}
	}
	return false
}

// Execute parses a pattern from the query and tests it against the remaining text.
func (s *RegexSkill) Execute(ctx context.Context, query string) (string, error) {
	query = strings.TrimSpace(query)

	var pattern, text string

	// Prefer /pattern/ form.
	if m := s.slashPattern.FindStringSubmatch(query); m != nil {
		pattern = m[1]
		// text is whatever follows the closing slash
		loc := s.slashPattern.FindStringIndex(query)
		if loc != nil && loc[1] < len(query) {
			text = strings.TrimSpace(query[loc[1]:])
		}
	} else {
		// Fall back to "regex <pattern>" / "pattern <pattern>" followed by text.
		stripRE := regexp.MustCompile(`(?i)^(?:regex|regular expression|regexp|pattern)\s+`)
		rest := strings.TrimSpace(stripRE.ReplaceAllString(query, ""))
		if rest == "" {
			rest = strings.TrimSpace(query)
		}
		// First whitespace-separated token is treated as the pattern, rest as text.
		if idx := strings.IndexAny(rest, " \t\n"); idx > 0 {
			pattern = rest[:idx]
			text = strings.TrimSpace(rest[idx:])
		} else {
			pattern = rest
			text = ""
		}
	}

	pattern = strings.Trim(pattern, `"`)
	if pattern == "" {
		return "", fmt.Errorf("no regular expression found in query (use /pattern/ or 'regex <pattern> <text>')")
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", fmt.Errorf("invalid regex %q: %w", pattern, err)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Pattern: %s\n", pattern)
	if text == "" {
		sb.WriteString("Status: pattern compiled successfully (no input text provided to test against)\n")
		return sb.String(), nil
	}

	fmt.Fprintf(&sb, "Input:   %s\n", text)
	matches := re.FindAllString(text, -1)
	if len(matches) == 0 {
		sb.WriteString("Result:  no matches\n")
		return sb.String(), nil
	}

	fmt.Fprintf(&sb, "Result:  %d match(es)\n", len(matches))
	for _, m := range matches {
		sb.WriteString("  - ")
		sb.WriteString(m)
		sb.WriteByte('\n')
	}
	return strings.TrimRight(sb.String(), "\n"), nil
}
