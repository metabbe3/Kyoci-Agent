package builtin

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// MathSkill handles mathematical calculations.
type MathSkill struct {
	*kyoci.BaseSkill
	exprPattern *regexp.Regexp
}

// NewMathSkill creates a new math skill.
func NewMathSkill() *MathSkill {
	skill := &MathSkill{
		BaseSkill: kyoci.NewBaseSkill(
			"math",
			"Performs mathematical calculations (addition, subtraction, multiplication, division, percentage, square root, exponent)",
			[]string{"calculate", "math", "what is", "compute", "+", "-", "*", "/", "%", "sqrt", "^"},
		),
	}
	// Pattern to match math expressions: "calculate ...", sqrt <num>, a binary
	// op like "2+2" / "2^3", or a percentage "10% of 50". Note: a bare "what is
	// X" is intentionally NOT matched — it fires on almost any factual question
	// ("what is the capital of france") and hijacks unrelated tasks. "what is
	// 2+2" still matches via the binary-op alternative below.
	skill.exprPattern = regexp.MustCompile(`(?i)calculate\s+(.+)|sqrt\s+(\d+(?:\.\d+)?)|(\d+(?:\.\d+)?)\s*([\+\-\*\/\^])\s*(\d+(?:\.\d+)?)(?:\s*([\+\-\*\/\^])\s*(\d+(?:\.\d+)?))?|(\d+(?:\.\d+)?)\s*%\s*(?:of\s+)?(\d+(?:\.\d+)?)`)
	return skill
}

// Match checks if the query is a math calculation.
func (s *MathSkill) Match(query string) bool {
	queryLower := strings.ToLower(query)
	// Check for keywords. "what is" and bare "^" were intentionally removed:
	// they are substrings that fire on huge numbers of unrelated queries
	// ("what is the capital of france") and would hijack them via the skill
	// fast path. Real math intent is carried by "calculate"/"math"/"sqrt"/"% of"
	// or by an expression matched by exprPattern below.
	for _, keyword := range []string{"calculate", "sqrt", "% of", "math"} {
		if strings.Contains(queryLower, keyword) {
			return true
		}
	}
	// Check for math operators with numbers
	if s.exprPattern.MatchString(query) {
		return true
	}
	return false
}

// Execute performs the math calculation.
func (s *MathSkill) Execute(ctx context.Context, query string) (string, error) {
	query = strings.TrimSpace(query)
	queryLower := strings.ToLower(query)

	// Handle square root
	if match := regexp.MustCompile(`sqrt\s+(\d+(?:\.\d+)?)`).FindStringSubmatch(queryLower); match != nil {
		num, err := strconv.ParseFloat(match[1], 64)
		if err != nil {
			return "", fmt.Errorf("invalid number for sqrt: %s", match[1])
		}
		if num < 0 {
			return "", fmt.Errorf("cannot calculate square root of negative number")
		}
		result := math.Sqrt(num)
		return fmt.Sprintf("sqrt(%s) = %.2f", match[1], result), nil
	}

	// Handle percentage: "X% of Y"
	if match := regexp.MustCompile(`(\d+(?:\.\d+)?)\s*%\s*(?:of\s+)?(\d+(?:\.\d+)?)`).FindStringSubmatch(queryLower); match != nil {
		percent, err1 := strconv.ParseFloat(match[1], 64)
		total, err2 := strconv.ParseFloat(match[2], 64)
		if err1 != nil || err2 != nil {
			return "", fmt.Errorf("invalid numbers in percentage calculation")
		}
		result := (percent / 100.0) * total
		return fmt.Sprintf("%.2f%% of %.2f = %.2f", percent, total, result), nil
	}

	// Handle exponent: "X ^ Y"
	if match := regexp.MustCompile(`(\d+(?:\.\d+)?)\s*\^\s*(\d+(?:\.\d+)?)`).FindStringSubmatch(query); match != nil {
		base, err1 := strconv.ParseFloat(match[1], 64)
		exp, err2 := strconv.ParseFloat(match[2], 64)
		if err1 != nil || err2 != nil {
			return "", fmt.Errorf("invalid numbers in exponent calculation")
		}
		result := math.Pow(base, exp)
		return fmt.Sprintf("%s ^ %s = %.2f", match[1], match[2], result), nil
	}

	// Extract expression after "calculate" or "what is"
	var expr string
	if strings.HasPrefix(queryLower, "calculate ") {
		expr = strings.TrimSpace(query[10:])
	} else if strings.HasPrefix(queryLower, "what is ") {
		expr = strings.TrimSpace(query[8:])
	} else {
		expr = query
	}

	// Evaluate basic math expression safely
	result, err := s.evaluateExpression(expr)
	if err != nil {
		return "", fmt.Errorf("failed to evaluate expression: %w", err)
	}

	return fmt.Sprintf("%s = %.2f", expr, result), nil
}

// evaluateExpression safely evaluates a simple math expression.
// Supports: +, -, *, /, and parentheses.
func (s *MathSkill) evaluateExpression(expr string) (float64, error) {
	expr = strings.TrimSpace(expr)
	expr = strings.ReplaceAll(expr, " ", "")

	if expr == "" {
		return 0, fmt.Errorf("empty expression")
	}

	// Simple recursive descent parser
	return s.parseAddSub(expr)
}

// parseAddSub handles addition and subtraction.
func (s *MathSkill) parseAddSub(expr string) (float64, error) {
	left, rest, err := s.parseMulDiv(expr)
	if err != nil {
		return 0, err
	}

	for len(rest) > 0 {
		if rest[0] == '+' {
			right, newRest, err := s.parseMulDiv(rest[1:])
			if err != nil {
				return 0, err
			}
			left += right
			rest = newRest
		} else if rest[0] == '-' {
			right, newRest, err := s.parseMulDiv(rest[1:])
			if err != nil {
				return 0, err
			}
			left -= right
			rest = newRest
		} else {
			break
		}
	}

	return left, nil
}

// parseMulDiv handles multiplication and division.
func (s *MathSkill) parseMulDiv(expr string) (float64, string, error) {
	left, rest, err := s.parsePrimary(expr)
	if err != nil {
		return 0, "", err
	}

	for len(rest) > 0 {
		if rest[0] == '*' {
			right, newRest, err := s.parsePrimary(rest[1:])
			if err != nil {
				return 0, "", err
			}
			left *= right
			rest = newRest
		} else if rest[0] == '/' {
			right, newRest, err := s.parsePrimary(rest[1:])
			if err != nil {
				return 0, "", err
			}
			if right == 0 {
				return 0, "", fmt.Errorf("division by zero")
			}
			left /= right
			rest = newRest
		} else {
			break
		}
	}

	return left, rest, nil
}

// parsePrimary handles numbers and parentheses.
func (s *MathSkill) parsePrimary(expr string) (float64, string, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return 0, "", fmt.Errorf("unexpected end of expression")
	}

	// Handle parentheses
	if expr[0] == '(' {
		// Find matching closing parenthesis
		depth := 1
		closePos := -1
		for i := 1; i < len(expr); i++ {
			if expr[i] == '(' {
				depth++
			} else if expr[i] == ')' {
				depth--
				if depth == 0 {
					closePos = i
					break
				}
			}
		}
		if closePos == -1 {
			return 0, "", fmt.Errorf("unclosed parenthesis")
		}

		value, err := s.evaluateExpression(expr[1:closePos])
		if err != nil {
			return 0, "", err
		}
		return value, expr[closePos+1:], nil
	}

	// Handle negative numbers
	if expr[0] == '-' {
		value, rest, err := s.parsePrimary(expr[1:])
		if err != nil {
			return 0, "", err
		}
		return -value, rest, nil
	}

	// Parse number
	numStr := ""
	for i := 0; i < len(expr); i++ {
		c := expr[i]
		if (c >= '0' && c <= '9') || c == '.' {
			numStr += string(c)
		} else {
			break
		}
	}

	if numStr == "" {
		return 0, "", fmt.Errorf("expected number, got '%s'", expr)
	}

	value, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0, "", fmt.Errorf("invalid number: %s", numStr)
	}

	return value, expr[len(numStr):], nil
}