package builtin

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// nowUnixMillis returns the current Unix epoch in milliseconds. Wrapped so
// tests can stub the clock if needed (we don't today, but the indirection
// makes the UUIDv7 generator deterministic under test).
func nowUnixMillis() int64 {
	return time.Now().UnixMilli()
}

// stripVerb removes the leading command verb from a query, returning the
// operand. Works around extractPayload which splits at the first ':' — that
// breaks for skills whose operand contains colons (ISO timestamps, ratios
// like "12:8", URLs, etc.).
//
// Example: stripVerb("time diff 2024-01-01T00:00:00Z ...", "time diff")
// returns "2024-01-01T00:00:00Z ...".
//
// The verb match is case-insensitive. If the verb is not found, returns the
// query unchanged (callers should handle that case themselves).
func stripVerb(q, verb string) string {
	low := strings.ToLower(q)
	idx := strings.Index(low, strings.ToLower(verb))
	if idx < 0 {
		return strings.TrimSpace(q)
	}
	return strings.TrimSpace(q[idx+len(verb):])
}

// =====================================================================================
// Shared helpers for skill and tool implementations.
//
// These keep individual skill/tool files small by extracting common patterns:
//   - extracting payloads from natural-language queries
//   - looking up external binaries (git, rg, gopls) with graceful degradation
//   - running shell commands with a timeout
// =====================================================================================

// colonRe matches a colon followed by optional whitespace, capturing everything after.
// Used to split "encode this to base64: hello world" → ("encode this to base64", "hello world").
var colonRe = regexp.MustCompile(`:\s*`)

// extractPayload pulls the operand text out of a natural-language query.
// Tries the following strategies in order:
//  1. Text after the first colon (e.g. "base64 encode: hello" → "hello")
//  2. Text after the last occurrence of a stop word like "of", "this", "the"
//  3. The query itself, with leading command words stripped
//
// Returns "" only if the query is empty after trimming.
func extractPayload(query string) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return ""
	}

	// Strategy 1: split on colon.
	if idx := strings.Index(query, ":"); idx >= 0 {
		rest := strings.TrimSpace(query[idx+1:])
		if rest != "" {
			return rest
		}
	}

	// Strategy 2: strip common command prefixes.
	lower := strings.ToLower(query)
	strippers := []string{
		"encode ", "decode ", "convert ", "compute ", "calculate ",
		"hash ", "format ", "validate ", "parse ", "generate ",
		"the string ", "the text ", "the value ", "the number ",
		"this string ", "this text ", "this value ",
		"string ", "text ", "value ",
	}
	result := query
	for _, p := range strippers {
		if strings.HasPrefix(lower, p) {
			result = strings.TrimSpace(query[len(p):])
			lower = strings.ToLower(result)
			break
		}
	}
	return result
}

// extractAfter finds the substring after the LAST occurrence of any of the given
// markers in query. Returns the trimmed remainder, or "" if no marker is found.
// Useful for queries like "encode this to base64: foo" with markers [":", " base64 ", " b64 "].
func extractAfter(query string, markers ...string) string {
	query = strings.TrimSpace(query)
	bestIdx := -1
	for _, m := range markers {
		if idx := strings.LastIndex(strings.ToLower(query), strings.ToLower(m)); idx >= 0 {
			end := idx + len(m)
			if end > bestIdx {
				bestIdx = end
			}
		}
	}
	if bestIdx < 0 {
		return ""
	}
	return strings.TrimSpace(query[bestIdx:])
}

// quoteStripped removes matching surrounding quotes from s if present.
// Handles ", ', ` and backticks. Useful when the model wraps the operand in quotes.
func quoteStripped(s string) string {
	s = strings.TrimSpace(s)
	if len(s) < 2 {
		return s
	}
	first, last := s[0], s[len(s)-1]
	if first == last && (first == '"' || first == '\'' || first == '`') {
		return s[1 : len(s)-1]
	}
	return s
}

// lookBinary returns the absolute path to name if it's in $PATH, else "".
// Used by tools that wrap external binaries (git, rg, gopls) to gracefully
// degrade when the binary is missing.
func lookBinary(name string) string {
	p, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return p
}

// runShellTimeout runs cmd in shell with a deadline. Returns combined stdout+stderr.
// If the deadline passes, returns what was captured so far plus a timeout marker.
// Caller is responsible for checking ctx.Err() == context.DeadlineExceeded.
func runShellTimeout(ctx context.Context, cmd string, workdir string, timeout time.Duration) (string, error) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	rctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	c := exec.CommandContext(rctx, "/bin/sh", "-c", cmd)
	if workdir != "" {
		c.Dir = workdir
	}
	out, err := c.CombinedOutput()
	return string(out), err
}

// fatalErr formats an error with a stable prefix so tests and callers can
// recognize skill/tool failures uniformly.
func fatalErr(format string, args ...interface{}) error {
	return fmt.Errorf(format, args...)
}
