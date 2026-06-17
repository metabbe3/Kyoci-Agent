package builtin

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"unicode"

	"github.com/metabbe3/Kyoci-Agent/pkg"
)

// CalculatorTool implements the kyoci.Tool interface for mathematical expression evaluation.
type CalculatorTool struct {
	logger *slog.Logger
}

// NewCalculatorTool creates a new calculator tool instance.
func NewCalculatorTool() *CalculatorTool {
	return &CalculatorTool{
		logger: slog.Default(),
	}
}

// Name returns the tool name.
func (c *CalculatorTool) Name() string {
	return "calculator"
}

// Description returns the tool description.
func (c *CalculatorTool) Description() string {
	return "Evaluate mathematical expressions safely. Supports +, -, *, /, ^, parentheses. No eval() used - parses and evaluates expressions directly."
}

// Parameters returns the tool parameter definition.
func (c *CalculatorTool) Parameters() []kyoci.ToolParameter {
	return []kyoci.ToolParameter{
		{
			Name:        "expression",
			Type:        "string",
			Description: "Mathematical expression to evaluate (e.g., '2 + 3 * 4', '(1 + 2) ^ 3')",
			Required:    true,
		},
	}
}

// Execute evaluates a mathematical expression.
//
// Parameters:
//   - ctx: Context for cancellation
//   - params: Map containing "expression" (required)
//
// Returns:
//   - string: Result of the evaluation
//   - error: Error if expression is invalid or evaluation fails
func (c *CalculatorTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	// Extract expression
	expression, ok := params["expression"].(string)
	if !ok || expression == "" {
		return "", fmt.Errorf("expression parameter is required and must be a string")
	}

	c.logger.Info("evaluating expression", "expression", expression)

	// Validate expression (only allow safe characters)
	if !c.isValidExpression(expression) {
		return "", fmt.Errorf("invalid expression: contains invalid characters")
	}

	// Parse and evaluate expression
	result, err := c.evaluate(expression)
	if err != nil {
		c.logger.Error("expression evaluation failed", "expression", expression, "error", err)
		return "", err
	}

	c.logger.Info("expression evaluated successfully", "expression", expression, "result", result)
	return fmt.Sprintf("%s = %s", expression, formatFloat(result)), nil
}

// isValidExpression checks if the expression contains only valid characters.
func (c *CalculatorTool) isValidExpression(expr string) bool {
	// Remove spaces for validation
	expr = strings.ReplaceAll(expr, " ", "")

	// Must not be empty
	if expr == "" {
		return false
	}

	// Check each character
	for _, ch := range expr {
		if unicode.IsDigit(ch) {
			continue
		}
		switch ch {
		case '+', '-', '*', '/', '^', '(', ')', '.':
			continue
		default:
			return false
		}
	}

	return true
}

// evaluate parses and evaluates a mathematical expression using shunting-yard algorithm.
func (c *CalculatorTool) evaluate(expr string) (float64, error) {
	// Tokenize
	tokens, err := c.tokenize(expr)
	if err != nil {
		return 0, err
	}

	// Convert to Reverse Polish Notation (RPN)
	rpn, err := c.toRPN(tokens)
	if err != nil {
		return 0, err
	}

	// Evaluate RPN
	return c.evaluateRPN(rpn)
}

// tokenize splits the expression into tokens.
func (c *CalculatorTool) tokenize(expr string) ([]string, error) {
	var tokens []string
	var number strings.Builder

	for i, ch := range expr {
		if ch == ' ' {
			continue
		}

		if unicode.IsDigit(ch) || ch == '.' {
			number.WriteRune(ch)
		} else {
			if number.Len() > 0 {
				tokens = append(tokens, number.String())
				number.Reset()
			}

			// Handle negative numbers
			if ch == '-' && (i == 0 || expr[i-1] == '(') {
				number.WriteRune(ch)
				continue
			}

			tokens = append(tokens, string(ch))
		}
	}

	if number.Len() > 0 {
		tokens = append(tokens, number.String())
	}

	return tokens, nil
}

// toRPN converts infix notation to Reverse Polish Notation (shunting-yard algorithm).
func (c *CalculatorTool) toRPN(tokens []string) ([]string, error) {
	var output []string
	var stack []string

	precedence := map[string]int{
		"^": 4,
		"*": 3,
		"/": 3,
		"+": 2,
		"-": 2,
	}

	associativity := map[string]string{
		"^": "right",
		"*": "left",
		"/": "left",
		"+": "left",
		"-": "left",
	}

	for _, token := range tokens {
		if c.isNumber(token) {
			output = append(output, token)
		} else if token == "(" {
			stack = append(stack, token)
		} else if token == ")" {
			for len(stack) > 0 && stack[len(stack)-1] != "(" {
				output = append(output, stack[len(stack)-1])
				stack = stack[:len(stack)-1]
			}
			if len(stack) == 0 {
				return nil, fmt.Errorf("mismatched parentheses")
			}
			stack = stack[:len(stack)-1] // Pop "("
		} else if c.isOperator(token) {
			for len(stack) > 0 && c.isOperator(stack[len(stack)-1]) {
				top := stack[len(stack)-1]
				if (associativity[token] == "left" && precedence[token] <= precedence[top]) ||
					(associativity[token] == "right" && precedence[token] < precedence[top]) {
					output = append(output, top)
					stack = stack[:len(stack)-1]
				} else {
					break
				}
			}
			stack = append(stack, token)
		} else {
			return nil, fmt.Errorf("invalid token: %s", token)
		}
	}

	for len(stack) > 0 {
		top := stack[len(stack)-1]
		if top == "(" || top == ")" {
			return nil, fmt.Errorf("mismatched parentheses")
		}
		output = append(output, top)
		stack = stack[:len(stack)-1]
	}

	return output, nil
}

// evaluateRPN evaluates an expression in Reverse Polish Notation.
func (c *CalculatorTool) evaluateRPN(tokens []string) (float64, error) {
	var stack []float64

	for _, token := range tokens {
		if c.isNumber(token) {
			num, err := strconv.ParseFloat(token, 64)
			if err != nil {
				return 0, fmt.Errorf("invalid number: %s", token)
			}
			stack = append(stack, num)
		} else if c.isOperator(token) {
			if len(stack) < 2 {
				return 0, fmt.Errorf("invalid expression: not enough operands")
			}

			b := stack[len(stack)-1]
			a := stack[len(stack)-2]
			stack = stack[:len(stack)-2]

			var result float64
			var err error

			switch token {
			case "+":
				result = a + b
			case "-":
				result = a - b
			case "*":
				result = a * b
			case "/":
				if b == 0 {
					return 0, fmt.Errorf("division by zero")
				}
				result = a / b
			case "^":
				// Exponentiation using power
				result = c.power(a, b)
			default:
				return 0, fmt.Errorf("unknown operator: %s", token)
			}

			if err != nil {
				return 0, err
			}

			stack = append(stack, result)
		}
	}

	if len(stack) != 1 {
		return 0, fmt.Errorf("invalid expression: too many operands")
	}

	return stack[0], nil
}

// isNumber checks if a token is a number.
func (c *CalculatorTool) isNumber(token string) bool {
	_, err := strconv.ParseFloat(token, 64)
	return err == nil
}

// isOperator checks if a token is an operator.
func (c *CalculatorTool) isOperator(token string) bool {
	switch token {
	case "+", "-", "*", "/", "^":
		return true
	default:
		return false
	}
}

// power calculates a to the power of b (a^b).
func (c *CalculatorTool) power(a, b float64) float64 {
	result := 1.0
	for i := 0; i < int(b); i++ {
		result *= a
	}
	return result
}

// formatFloat formats a float value for display, removing unnecessary decimal places.
func formatFloat(f float64) string {
	if f == float64(int(f)) {
		return fmt.Sprintf("%.0f", f)
	}
	return fmt.Sprintf("%.6f", f)
}