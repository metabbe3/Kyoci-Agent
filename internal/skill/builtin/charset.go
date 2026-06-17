package builtin

import (
	"context"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// CharsetSkill reports character set / encoding information for a string.
type CharsetSkill struct {
	*kyoci.BaseSkill
}

// NewCharsetSkill creates a new charset skill.
func NewCharsetSkill() *CharsetSkill {
	return &CharsetSkill{
		BaseSkill: kyoci.NewBaseSkill(
			"charset",
			"Detect and report the character set / encoding info for a string",
			[]string{"charset", "encoding", "character set", "utf"},
		),
	}
}

// Match returns true if the query mentions charset/encoding/character set.
func (s *CharsetSkill) Match(query string) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	for _, kw := range []string{"charset", "encoding", "character set"} {
		if strings.Contains(q, kw) {
			return true
		}
	}
	return false
}

// Execute analyzes the input string and reports encoding details.
func (s *CharsetSkill) Execute(ctx context.Context, query string) (string, error) {
	input := extractCharsetInput(query)
	if input == "" {
		return "", fmt.Errorf("no input string provided for charset analysis")
	}

	byteLen := len(input)
	runeCount := utf8.RuneCountInString(input)
	allASCII := utf8.ValidString(input) && isASCII(input)

	nonASCIICount := 0
	var nonASCII []rune
	for _, r := range input {
		if r > unicode.MaxASCII {
			nonASCIICount++
			if len(nonASCII) < 10 {
				nonASCII = append(nonASCII, r)
			}
		}
	}

	var sb strings.Builder
	sb.WriteString("Charset analysis\n")
	sb.WriteString("----------------\n")
	sb.WriteString(fmt.Sprintf("Input: %q\n", input))
	sb.WriteString(fmt.Sprintf("Byte length: %d\n", byteLen))
	sb.WriteString(fmt.Sprintf("Rune count: %d\n", runeCount))
	sb.WriteString("Encoding: UTF-8\n")
	sb.WriteString(fmt.Sprintf("All ASCII: %v\n", allASCII))
	sb.WriteString(fmt.Sprintf("Non-ASCII runes: %d\n", nonASCIICount))
	if len(nonASCII) > 0 {
		sb.WriteString("Sample non-ASCII codepoints:\n")
		for _, r := range nonASCII {
			sb.WriteString(fmt.Sprintf("  U+%04X %q\n", r, r))
		}
	}

	return strings.TrimRight(sb.String(), "\n"), nil
}

// extractCharsetInput strips command words from the query and returns the
// remaining input string to analyze.
func extractCharsetInput(query string) string {
	q := strings.TrimSpace(query)
	lower := strings.ToLower(q)

	prefixes := []string{"character set", "charset", "encoding"}
	for _, p := range prefixes {
		if lower == p {
			return ""
		}
		if strings.HasPrefix(lower, p+" ") {
			return strings.TrimSpace(q[len(p):])
		}
	}
	return q
}

// isASCII reports whether all bytes of s decode to ASCII runes.
func isASCII(s string) bool {
	for _, r := range s {
		if r > unicode.MaxASCII {
			return false
		}
	}
	return true
}
