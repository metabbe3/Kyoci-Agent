package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// CalculatorTool evaluates mathematical expressions
type CalculatorTool struct{}

func NewCalculatorTool() *CalculatorTool { return &CalculatorTool{} }

func (t *CalculatorTool) Name() string { return "calculator" }

func (t *CalculatorTool) Description() string {
	return "Evaluate mathematical expressions. Supports +, -, *, /, ^, sqrt, sin, cos, tan, log, ln, pi, e."
}

func (t *CalculatorTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"expression": map[string]interface{}{
				"type":        "string",
				"description": "Mathematical expression to evaluate (e.g., '2 + 3 * 4', 'sqrt(144)', 'sin(pi/2)')",
			},
		},
		"required": []string{"expression"},
	}
}

func (t *CalculatorTool) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var params struct {
		Expression string `json:"expression"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}

	expr := strings.TrimSpace(params.Expression)
	if expr == "" {
		return "", fmt.Errorf("empty expression")
	}

	// Safe math evaluation — support basic operations
	result, err := evalExpr(expr)
	if err != nil {
		return "", fmt.Errorf("evaluation error: %w", err)
	}

	// Format result nicely
	if result == math.Trunc(result) {
		return fmt.Sprintf("%s = %.0f", expr, result), nil
	}
	return fmt.Sprintf("%s = %.6g", expr, result), nil
}

// evalExpr performs safe math evaluation
func evalExpr(expr string) (float64, error) {
	// Replace math constants and functions
	expr = strings.ReplaceAll(expr, "pi", fmt.Sprintf("%g", math.Pi))
	expr = strings.ReplaceAll(expr, "e", fmt.Sprintf("%g", math.E))

	// Tokenize and parse using a simple recursive descent parser
	p := &parser{input: expr}
	result, err := p.parseExpression()
	if err != nil {
		return 0, err
	}
	return result, nil
}

// Simple recursive descent math parser
type parser struct {
	input string
	pos   int
}

func (p *parser) peek() byte {
	if p.pos >= len(p.input) {
		return 0
	}
	return p.input[p.pos]
}

func (p *parser) consume() byte {
	ch := p.peek()
	p.pos++
	return ch
}

func (p *parser) skipSpace() {
	for p.peek() == ' ' || p.peek() == '\t' {
		p.consume()
	}
}

func (p *parser) parseExpression() (float64, error) {
	return p.parseAddSub()
}

func (p *parser) parseAddSub() (float64, error) {
	left, err := p.parseMulDiv()
	if err != nil {
		return 0, err
	}
	for {
		p.skipSpace()
		ch := p.peek()
		if ch == '+' || ch == '-' {
			p.consume()
			right, err := p.parseMulDiv()
			if err != nil {
				return 0, err
			}
			if ch == '+' {
				left += right
			} else {
				left -= right
			}
		} else {
			break
		}
	}
	return left, nil
}

func (p *parser) parseMulDiv() (float64, error) {
	left, err := p.parsePower()
	if err != nil {
		return 0, err
	}
	for {
		p.skipSpace()
		ch := p.peek()
		if ch == '*' || ch == '/' || ch == '%' {
			p.consume()
			right, err := p.parsePower()
			if err != nil {
				return 0, err
			}
			switch ch {
			case '*':
				left *= right
			case '/':
				if right == 0 {
					return 0, fmt.Errorf("division by zero")
				}
				left /= right
			case '%':
				left = math.Mod(left, right)
			}
		} else {
			break
		}
	}
	return left, nil
}

func (p *parser) parsePower() (float64, error) {
	base, err := p.parseUnary()
	if err != nil {
		return 0, err
	}
	p.skipSpace()
	if p.peek() == '^' {
		p.consume()
		exp, err := p.parsePower() // right-associative
		if err != nil {
			return 0, err
		}
		return math.Pow(base, exp), nil
	}
	return base, nil
}

func (p *parser) parseUnary() (float64, error) {
	p.skipSpace()
	if p.peek() == '-' {
		p.consume()
		val, err := p.parseUnary()
		return -val, err
	}
	if p.peek() == '+' {
		p.consume()
		return p.parseUnary()
	}
	return p.parsePrimary()
}

func (p *parser) parsePrimary() (float64, error) {
	p.skipSpace()

	// Parentheses
	if p.peek() == '(' {
		p.consume()
		val, err := p.parseExpression()
		if err != nil {
			return 0, err
		}
		p.skipSpace()
		if p.peek() != ')' {
			return 0, fmt.Errorf("expected ')'")
		}
		p.consume()
		return val, nil
	}

	// Functions
	letters := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	if p.pos < len(p.input) && strings.ContainsRune(letters, rune(p.input[p.pos])) {
		return p.parseFunctionOrNumber()
	}

	// Number
	return p.parseNumber()
}

func (p *parser) parseFunctionOrNumber() (float64, error) {
	start := p.pos
	for p.pos < len(p.input) && (strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ.", rune(p.input[p.pos]))) {
		p.pos++
	}
	word := p.input[start:p.pos]

	p.skipSpace()
	if p.peek() == '(' {
		p.consume()
		arg, err := p.parseExpression()
		if err != nil {
			return 0, err
		}
		p.skipSpace()
		if p.peek() != ')' {
			return 0, fmt.Errorf("expected ')' after function argument")
		}
		p.consume()

		switch word {
		case "sqrt":
			return math.Sqrt(arg), nil
		case "sin":
			return math.Sin(arg), nil
		case "cos":
			return math.Cos(arg), nil
		case "tan":
			return math.Tan(arg), nil
		case "log":
			return math.Log10(arg), nil
		case "ln":
			return math.Log(arg), nil
		case "abs":
			return math.Abs(arg), nil
		case "ceil":
			return math.Ceil(arg), nil
		case "floor":
			return math.Floor(arg), nil
		default:
			return 0, fmt.Errorf("unknown function: %s", word)
		}
	}

	// It's a number that was parsed as text (shouldn't happen often)
	var val float64
	_, err := fmt.Sscanf(word, "%g", &val)
	if err != nil {
		return 0, fmt.Errorf("unexpected token: %s", word)
	}
	return val, nil
}

func (p *parser) parseNumber() (float64, error) {
	p.skipSpace()
	start := p.pos
	for p.pos < len(p.input) && (strings.ContainsRune("0123456789.", rune(p.input[p.pos]))) {
		p.pos++
	}
	if start == p.pos {
		return 0, fmt.Errorf("expected number at position %d", p.pos)
	}
	var val float64
	_, err := fmt.Sscanf(p.input[start:p.pos], "%g", &val)
	return val, err
}
