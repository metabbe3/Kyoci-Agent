package security

import (
	"strings"
	"unicode"
)

const (
	maxStringLength = 4096
	maxSessionIDLen = 64
	maxToolNameLen  = 64
)

// Allowed tools whitelist (lowercase, underscore only)
var allowedTools = map[string]bool{
	"browser":       true,
	"terminal":      true,
	"code_exec":     true,
	"http_client":   true,
	"web_scraper":   true,
	"database":      true,
	"email":         true,
	"scheduler":     true,
	"delegation":    true,
	"pdf":           true,
	"image_gen":     true,
	"vision":        true,
	"patch":         true,
	"read_file":     true,
	"write_file":    true,
	"search_files":  true,
}

// SanitizeString removes null bytes, control characters, and limits length
func SanitizeString(input string) string {
	if len(input) == 0 {
		return input
	}

	// Trim to max length
	if len(input) > maxStringLength {
		input = input[:maxStringLength]
	}

	// Remove null bytes and control characters (except newline, tab, carriage return)
	var result strings.Builder
	result.Grow(len(input))

	for _, r := range input {
		// Keep printable ASCII and common whitespace
		if r == '\n' || r == '\t' || r == '\r' {
			result.WriteRune(r)
		} else if r >= 32 && r <= 126 {
			// Printable ASCII
			result.WriteRune(r)
		} else if r > 127 && unicode.IsPrint(r) {
			// Printable Unicode
			result.WriteRune(r)
		}
		// Skip: null bytes, control chars, non-printable
	}

	return result.String()
}

// validateSessionID checks if session ID is valid
// Valid: alphanumeric + dash only, max 64 chars
func validateSessionID(id string) bool {
	if len(id) == 0 || len(id) > maxSessionIDLen {
		return false
	}

	for _, r := range id {
		if !((r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '-') {
			return false
		}
	}

	return true
}

// validateToolName checks if tool name is valid
// Valid: lowercase + underscore only, from whitelist
func validateToolName(name string) bool {
	if len(name) == 0 || len(name) > maxToolNameLen {
		return false
	}

	// Check characters: lowercase letters, digits, underscore only
	for _, r := range name {
		if !((r >= 'a' && r <= 'z') ||
			(r >= '0' && r <= '9') ||
			r == '_') {
			return false
		}
	}

	// Check against whitelist
	return allowedTools[name]
}

// SanitizeInput combines sanitization with validation
// Returns sanitized string and whether it's valid
func SanitizeInput(input string) (string, bool) {
	sanitized := SanitizeString(input)
	if len(sanitized) == 0 {
		return "", false
	}
	return sanitized, true
}

// IsSafePath checks if a path doesn't contain directory traversal
func IsSafePath(path string) bool {
	// Simple check for path traversal patterns
	return !strings.Contains(path, "..") && 
	       !strings.Contains(path, "~") &&
	       !strings.HasPrefix(path, "/")
}

// trimAndValidate trims whitespace and validates non-empty
func trimAndValidate(input string) (string, bool) {
	trimmed := strings.TrimSpace(input)
	if len(trimmed) == 0 {
		return "", false
	}
	return trimmed, true
}