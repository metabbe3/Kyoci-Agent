package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// JSONFmtSkill formats, minifies, and validates JSON.
type JSONFmtSkill struct {
	*kyoci.BaseSkill
}

// NewJSONFmtSkill creates a new json formatter skill.
func NewJSONFmtSkill() *JSONFmtSkill {
	return &JSONFmtSkill{
		BaseSkill: kyoci.NewBaseSkill(
			"jsonfmt",
			"Format, minify, and validate JSON",
			[]string{"json", "format json", "minify json", "validate json", "pretty json"},
		),
	}
}

// Match checks if the query references JSON.
//
// Defers to the data-format skills (yaml_to_json, json_to_yaml, csv_to_json,
// etc.) when their tighter patterns would fire. The legacy jsonfmt catches
// generic "format this json" / "validate json" phrasings that no specific
// datafmt skill claims.
func (s *JSONFmtSkill) Match(query string) bool {
	q := strings.ToLower(query)
	// Defer to specific datafmt skills.
	deferPhrases := []string{
		"yaml to json", "json to yaml", "yaml → json", "json → yaml",
		"toml to json", "json to toml", "toml → json", "json → toml",
		"csv to json", "json to csv", "csv → json", "json → csv",
		"xml to json", "json to xml", "xml → json", "json → xml",
		"env to json", "json to env",
		"json minify", "minify json", "compact json",
		"json pretty", "pretty json", "beautify json",
	}
	for _, p := range deferPhrases {
		if strings.Contains(q, p) {
			return false
		}
	}
	return strings.Contains(q, "json")
}

// Execute formats, minifies, or validates JSON found in the query.
func (s *JSONFmtSkill) Execute(ctx context.Context, query string) (string, error) {
	queryLower := strings.ToLower(query)
	minify := strings.Contains(queryLower, "minify") || strings.Contains(queryLower, "compact")

	// Strip the leading verb(s) and any whitespace; keep the rest as JSON.
	content := query
	content = strings.TrimSpace(content)
	content = stripLeadingAny(content,
		"format", "minify", "compact", "validate", "pretty", "json",
	)
	content = strings.TrimSpace(content)

	// Allow the user to wrap JSON in quotes or backticks.
	content = trimQuotes(content)

	if content == "" {
		return "", fmt.Errorf("no JSON content found in query")
	}

	var parsed interface{}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return "", fmt.Errorf("invalid JSON: %w", err)
	}

	var out bytes.Buffer
	var enc *json.Encoder
	if minify {
		if err := json.Compact(&out, []byte(content)); err != nil {
			return "", fmt.Errorf("failed to minify JSON: %w", err)
		}
	} else {
		enc = json.NewEncoder(&out)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		if err := enc.Encode(parsed); err != nil {
			return "", fmt.Errorf("failed to format JSON: %w", err)
		}
	}

	header := "Formatted JSON (2-space indent):"
	if minify {
		header = "Minified JSON:"
	}
	return fmt.Sprintf("%s\n\n%s", header, strings.TrimRight(out.String(), "\n")), nil
}

func stripLeadingAny(s string, words ...string) string {
	lowered := strings.ToLower(s)
	for {
		trimmed := false
		for _, w := range words {
			prefix := w + " "
			if strings.HasPrefix(lowered, prefix) {
				s = s[len(prefix):]
				lowered = strings.ToLower(s)
				trimmed = true
				break
			}
			if lowered == w {
				s = ""
				lowered = ""
				trimmed = true
				break
			}
		}
		if !trimmed {
			break
		}
	}
	return s
}

func trimQuotes(s string) string {
	if len(s) >= 2 {
		first, last := s[0], s[len(s)-1]
		if (first == '"' && last == '"') || (first == '`' && last == '`') || (first == '\'' && last == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
