package builtin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// EncodeSkill handles encoding and decoding operations.
type EncodeSkill struct {
	*kyoci.BaseSkill
	pattern *regexp.Regexp
}

// NewEncodeSkill creates a new encode/decode skill.
func NewEncodeSkill() *EncodeSkill {
	skill := &EncodeSkill{
		BaseSkill: kyoci.NewBaseSkill(
			"encode",
			"Handles base64 encode/decode, URL encode/decode, and JSON formatting",
			[]string{"base64", "url encode", "url decode", "json format", "json pretty", "encode", "decode"},
		),
	}
	skill.pattern = regexp.MustCompile(`(?i)(base64|url)\s+(encode|decode)\s+(.+)`)
	return skill
}

// Match checks if the query is asking for encoding/decoding.
func (s *EncodeSkill) Match(query string) bool {
	queryLower := strings.ToLower(query)
	keywords := []string{"base64", "url encode", "url decode", "json format", "json pretty", "encode", "decode"}
	for _, keyword := range keywords {
		if strings.Contains(queryLower, keyword) {
			return true
		}
	}
	return false
}

// Execute performs the encoding/decoding operation.
func (s *EncodeSkill) Execute(ctx context.Context, query string) (string, error) {
	queryLower := strings.ToLower(query)
	query = strings.TrimSpace(query)

	// Handle JSON formatting
	if strings.Contains(queryLower, "json") {
		return s.formatJSON(query)
	}

	// Use pattern matching for encode/decode
	if match := s.pattern.FindStringSubmatch(query); match != nil {
		operation := strings.ToLower(match[1]) // base64 or url
		action := strings.ToLower(match[2])    // encode or decode
		text := strings.TrimSpace(match[3])

		return s.performEncodeDecode(operation, action, text)
	}

	// Try to parse manually
	if strings.Contains(queryLower, "base64 encode") {
		text := strings.TrimSpace(strings.ReplaceAll(queryLower, "base64 encode", ""))
		return s.performEncodeDecode("base64", "encode", text)
	}
	if strings.Contains(queryLower, "base64 decode") {
		text := strings.TrimSpace(strings.ReplaceAll(queryLower, "base64 decode", ""))
		return s.performEncodeDecode("base64", "decode", text)
	}
	if strings.Contains(queryLower, "url encode") {
		text := strings.TrimSpace(strings.ReplaceAll(queryLower, "url encode", ""))
		return s.performEncodeDecode("url", "encode", text)
	}
	if strings.Contains(queryLower, "url decode") {
		text := strings.TrimSpace(strings.ReplaceAll(queryLower, "url decode", ""))
		return s.performEncodeDecode("url", "decode", text)
	}

	return "", fmt.Errorf("unrecognized encode/decode operation: %s", query)
}

// performEncodeDecode performs the actual encoding or decoding.
func (s *EncodeSkill) performEncodeDecode(operation, action, text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", fmt.Errorf("no text provided to %s %s", action, operation)
	}

	switch operation {
	case "base64":
		if action == "encode" {
			encoded := base64.StdEncoding.EncodeToString([]byte(text))
			return fmt.Sprintf("Base64 encoded: %s", encoded), nil
		} else {
			decoded, err := base64.StdEncoding.DecodeString(text)
			if err != nil {
				return "", fmt.Errorf("base64 decode failed: %w", err)
			}
			return fmt.Sprintf("Base64 decoded: %s", string(decoded)), nil
		}
	case "url":
		if action == "encode" {
			encoded := url.QueryEscape(text)
			return fmt.Sprintf("URL encoded: %s", encoded), nil
		} else {
			decoded, err := url.QueryUnescape(text)
			if err != nil {
				return "", fmt.Errorf("URL decode failed: %w", err)
			}
			return fmt.Sprintf("URL decoded: %s", decoded), nil
		}
	default:
		return "", fmt.Errorf("unsupported operation: %s", operation)
	}
}

// formatJSON formats JSON string.
func (s *EncodeSkill) formatJSON(query string) (string, error) {
	queryLower := strings.ToLower(query)

	// Extract JSON from query
	var jsonStr string
	// Try to match patterns like "json format {...}" or "format json {...}"
	if match := regexp.MustCompile(`(?:json\s+(?:format|pretty)?|format\s+json)\s*(\{.+\})`).FindStringSubmatch(queryLower); match != nil {
		jsonStr = match[1]
	} else if match := regexp.MustCompile(`json\s+(.+)`).FindStringSubmatch(queryLower); match != nil {
		jsonStr = strings.TrimSpace(match[1])
		// If the match doesn't start with {, look for { in the string
		if !strings.HasPrefix(jsonStr, "{") {
			if idx := strings.Index(jsonStr, "{"); idx >= 0 {
				jsonStr = jsonStr[idx:]
			}
		}
	} else if match := regexp.MustCompile(`\{.+\}`).FindString(query); match != "" {
		jsonStr = match
	} else {
		return "", fmt.Errorf("no JSON found in query")
	}

	// Parse and format JSON
	var data interface{}
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return "", fmt.Errorf("invalid JSON: %w", err)
	}

	formatted, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to format JSON: %w", err)
	}

	return fmt.Sprintf("Formatted JSON:\n%s", string(formatted)), nil
}