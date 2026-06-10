package skill

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Math handler: evaluates simple arithmetic expressions.
// Pattern: (?i)(hitung|calculate|compute|berapa)\s+([\d+\-*/(). ]+)
func handleMath(ctx context.Context, input string) (string, error) {
	re := regexp.MustCompile(`(?i)(hitung|calculate|compute|berapa)\s+([\d+\-*/(). ]+)`)
	matches := re.FindStringSubmatch(input)
	if len(matches) < 3 {
		return "", fmt.Errorf("no expression found")
	}

	expr := strings.TrimSpace(matches[2])
	result, err := evalExpression(expr)
	if err != nil {
		return "", fmt.Errorf("failed to evaluate expression: %w", err)
	}

	return fmt.Sprintf("Result: %s = %v", expr, result), nil
}

// Time handler: returns current time or date.
// Pattern: (?i)(jam berapa|what time|current time|tanggal|what date)
func handleTime(ctx context.Context, input string) (string, error) {
	re := regexp.MustCompile(`(?i)(jam berapa|what time|current time|tanggal|what date)`)
	matches := re.FindStringSubmatch(input)
	if len(matches) < 1 {
		return "", fmt.Errorf("no time request found")
	}

	query := strings.ToLower(matches[0])
	now := time.Now()

	if strings.Contains(query, "jam") || strings.Contains(query, "time") {
		return fmt.Sprintf("Current time: %s", now.Format("15:04:05")), nil
	}
	return fmt.Sprintf("Current date: %s", now.Format("2006-01-02 Monday")), nil
}

// Hash handler: computes SHA256 or MD5 hash.
// Pattern: (?i)(hash|sha256|md5)\s+(.+)
func handleHash(ctx context.Context, input string) (string, error) {
	re := regexp.MustCompile(`(?i)(hash|sha256|md5)\s+(.+)`)
	matches := re.FindStringSubmatch(input)
	if len(matches) < 3 {
		return "", fmt.Errorf("no hash algorithm or input found")
	}

	algorithm := strings.ToLower(matches[1])
	data := strings.TrimSpace(matches[2])

	switch algorithm {
	case "sha256", "hash":
		h := sha256.Sum256([]byte(data))
		return fmt.Sprintf("SHA256(%s) = %s", data, hex.EncodeToString(h[:])), nil
	case "md5":
		h := md5.Sum([]byte(data))
		return fmt.Sprintf("MD5(%s) = %s", data, hex.EncodeToString(h[:])), nil
	default:
		return "", fmt.Errorf("unsupported hash algorithm: %s", algorithm)
	}
}

// Encode handler: base64 encode or decode.
// Pattern: (?i)(base64 (encode|decode))\s+(.+)
func handleEncode(ctx context.Context, input string) (string, error) {
	re := regexp.MustCompile(`(?i)(base64 (encode|decode))\s+(.+)`)
	matches := re.FindStringSubmatch(input)
	if len(matches) < 4 {
		return "", fmt.Errorf("no base64 action or input found")
	}

	action := strings.ToLower(matches[2])
	data := strings.TrimSpace(matches[3])

	switch action {
	case "encode":
		encoded := base64.StdEncoding.EncodeToString([]byte(data))
		return fmt.Sprintf("Base64(%s) = %s", data, encoded), nil
	case "decode":
		decoded, err := base64.StdEncoding.DecodeString(data)
		if err != nil {
			return "", fmt.Errorf("failed to decode base64: %w", err)
		}
		return fmt.Sprintf("Decoded(%s) = %s", data, string(decoded)), nil
	default:
		return "", fmt.Errorf("unsupported base64 action: %s", action)
	}
}

// UUID handler: generate a random UUID.
// Pattern: (?i)(generate|buat|create)\s+(uuid|id)
func handleUUID(ctx context.Context, input string) (string, error) {
	re := regexp.MustCompile(`(?i)(generate|buat|create)\s+(uuid|id)`)
	if !re.MatchString(input) {
		return "", fmt.Errorf("no uuid request found")
	}

	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate uuid: %w", err)
	}

	// Set version (4) and variant bits
	b[6] = (b[6] & 0x0f) | 0x40 // Version 4
	b[8] = (b[8] & 0x3f) | 0x80 // Variant is 10

	uuid := fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
	return fmt.Sprintf("Generated UUID: %s", uuid), nil
}

// JSONFormat handler: pretty-print JSON.
// Pattern: (?i)(format|pretty)\s+json\s+(.+)
func handleJSONFormat(ctx context.Context, input string) (string, error) {
	re := regexp.MustCompile(`(?i)(format|pretty)\s+json\s+(.+)`)
	matches := re.FindStringSubmatch(input)
	if len(matches) < 3 {
		return "", fmt.Errorf("no json data found")
	}

	jsonStr := strings.TrimSpace(matches[2])

	var v interface{}
	if err := json.Unmarshal([]byte(jsonStr), &v); err != nil {
		return "", fmt.Errorf("invalid json: %w", err)
	}

	formatted, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to format json: %w", err)
	}

	return string(formatted), nil
}

// Health handler: simple ping/status check.
// Pattern: (?i)(ping|health|alive|status)
func handleHealth(ctx context.Context, input string) (string, error) {
	re := regexp.MustCompile(`(?i)(ping|health|alive|status)`)
	if !re.MatchString(input) {
		return "", fmt.Errorf("no health check request found")
	}

	return "OK - System is healthy and responsive", nil
}

// UnitConvert handler: convert between units.
// Pattern: (?i)(convert|konversi)\s+(\d+)\s+(celsius|fahrenheit|km|mi|kg|lb)
func handleUnitConvert(ctx context.Context, input string) (string, error) {
	re := regexp.MustCompile(`(?i)(convert|konversi)\s+(\d+)\s+(celsius|fahrenheit|km|mi|kg|lb)`)
	matches := re.FindStringSubmatch(input)
	if len(matches) < 4 {
		return "", fmt.Errorf("no conversion request found")
	}

	value, err := strconv.ParseFloat(matches[2], 64)
	if err != nil {
		return "", fmt.Errorf("invalid number: %w", err)
	}

	unit := strings.ToLower(matches[3])

	switch unit {
	case "celsius":
		fahrenheit := value*9/5 + 32
		return fmt.Sprintf("%.2f°C = %.2f°F", value, fahrenheit), nil
	case "fahrenheit":
		celsius := (value - 32) * 5 / 9
		return fmt.Sprintf("%.2f°F = %.2f°C", value, celsius), nil
	case "km":
		miles := value * 0.621371
		return fmt.Sprintf("%.2f km = %.2f miles", value, miles), nil
	case "mi":
		km := value / 0.621371
		return fmt.Sprintf("%.2f miles = %.2f km", value, km), nil
	case "kg":
		pounds := value * 2.20462
		return fmt.Sprintf("%.2f kg = %.2f lb", value, pounds), nil
	case "lb":
		kg := value / 2.20462
		return fmt.Sprintf("%.2f lb = %.2f kg", value, kg), nil
	default:
		return "", fmt.Errorf("unsupported unit: %s", unit)
	}
}

// RegisterBuiltinSkills registers all built-in zero-AI skills to a registry.
func RegisterBuiltinSkills(r *Registry) error {
	if err := r.Register("math", `(?i)(hitung|calculate|compute|berapa)\s+([\d+\-*/(). ]+)`, handleMath, "Evaluate arithmetic expressions"); err != nil {
		return err
	}
	if err := r.Register("time", `(?i)(jam berapa|what time|current time|tanggal|what date)`, handleTime, "Get current time or date"); err != nil {
		return err
	}
	if err := r.Register("hash", `(?i)(hash|sha256|md5)\s+(.+)`, handleHash, "Compute SHA256 or MD5 hash"); err != nil {
		return err
	}
	if err := r.Register("encode", `(?i)(base64 (encode|decode))\s+(.+)`, handleEncode, "Base64 encode/decode"); err != nil {
		return err
	}
	if err := r.Register("uuid", `(?i)(generate|buat|create)\s+(uuid|id)`, handleUUID, "Generate random UUID"); err != nil {
		return err
	}
	if err := r.Register("json_format", `(?i)(format|pretty)\s+json\s+(.+)`, handleJSONFormat, "Format/pretty-print JSON"); err != nil {
		return err
	}
	if err := r.Register("health", `(?i)(ping|health|alive|status)`, handleHealth, "Health/ping check"); err != nil {
		return err
	}
	if err := r.Register("unit_convert", `(?i)(convert|konversi)\s+(\d+)\s+(celsius|fahrenheit|km|mi|kg|lb)`, handleUnitConvert, "Convert between units"); err != nil {
		return err
	}

	return nil
}

// evalExpression evaluates a simple arithmetic expression.
// Supports: +, -, *, /, and parentheses.
func evalExpression(expr string) (float64, error) {
	// Simple evaluation using Go's eval is not available directly.
	// For safety, we'll parse and evaluate step by step.

	// Remove all whitespace
	expr = strings.ReplaceAll(expr, " ", "")

	// Evaluate parentheses first
	for {
		start := strings.LastIndex(expr, "(")
		if start == -1 {
			break
		}
		end := strings.Index(expr[start:], ")")
		if end == -1 {
			return 0, fmt.Errorf("unmatched parentheses")
		}
		end += start

		subExpr := expr[start+1 : end]
		result, err := evalSimple(subExpr)
		if err != nil {
			return 0, err
		}

		expr = expr[:start] + fmt.Sprintf("%v", result) + expr[end+1:]
	}

	return evalSimple(expr)
}

// evalSimple evaluates an expression without parentheses.
func evalSimple(expr string) (float64, error) {
	// Handle multiplication and division first
	for {
		mulIdx := strings.IndexAny(expr, "*/")
		if mulIdx == -1 {
			break
		}

		// Find the left operand
		left := findLeftOperand(expr[:mulIdx])
		leftVal, err := strconv.ParseFloat(left, 64)
		if err != nil {
			return 0, err
		}

		// Find the right operand
		right := findRightOperand(expr[mulIdx+1:])
		rightVal, err := strconv.ParseFloat(right, 64)
		if err != nil {
			return 0, err
		}

		var result float64
		switch expr[mulIdx] {
		case '*':
			result = leftVal * rightVal
		case '/':
			if rightVal == 0 {
				return 0, fmt.Errorf("division by zero")
			}
			result = leftVal / rightVal
		}

		leftStart := strings.LastIndex(expr[:mulIdx], "+")
		if leftStart == -1 {
			leftStart = strings.LastIndex(expr[:mulIdx], "-")
		}
		if leftStart == -1 {
			leftStart = 0
		} else {
			leftStart++
		}

		rightEnd := mulIdx + 1 + len(right)

		expr = expr[:leftStart] + fmt.Sprintf("%v", result) + expr[rightEnd:]
	}

	// Handle addition and subtraction
	parts := strings.FieldsFunc(expr, func(r rune) bool {
		return r == '+' || r == '-'
	})

	if len(parts) == 0 {
		return 0, fmt.Errorf("invalid expression")
	}

	result, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0, err
	}

	offset := len(parts[0])
	for i := 1; i < len(parts); i++ {
		op := expr[offset]
		val, err := strconv.ParseFloat(parts[i], 64)
		if err != nil {
			return 0, err
		}

		switch op {
		case '+':
			result += val
		case '-':
			result -= val
		}

		offset += 1 + len(parts[i])
	}

	return result, nil
}

// findLeftOperand finds the numeric left operand.
func findLeftOperand(s string) string {
	i := len(s) - 1
	for i >= 0 && (s[i] >= '0' && s[i] <= '9' || s[i] == '.') {
		i--
	}
	return s[i+1:]
}

// findRightOperand finds the numeric right operand.
func findRightOperand(s string) string {
	i := 0
	for i < len(s) && (s[i] >= '0' && s[i] <= '9' || s[i] == '.') {
		i++
	}
	return s[:i]
}