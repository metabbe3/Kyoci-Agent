package builtin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// jwtPattern matches a compact JWT (header.payload.signature).
var jwtPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$`)

// JWTSkill decodes a JWT and shows its header and payload without signature verification.
type JWTSkill struct {
	*kyoci.BaseSkill
}

// NewJWTSkill creates a new JWT skill.
func NewJWTSkill() *JWTSkill {
	return &JWTSkill{
		BaseSkill: kyoci.NewBaseSkill(
			"jwt",
			"Decode a JWT and show its header and payload (no signature verification)",
			[]string{"jwt", "json web token", "decode jwt", "token decode"},
		),
	}
}

// Match returns true if the query mentions jwt or looks like a JWT.
func (s *JWTSkill) Match(query string) bool {
	queryLower := strings.ToLower(strings.TrimSpace(query))
	if queryLower == "" {
		return false
	}
	if strings.Contains(queryLower, "jwt") {
		return true
	}
	// Try the whole trimmed query, then each whitespace-stripped token group.
	if jwtPattern.MatchString(strings.TrimSpace(query)) {
		return true
	}
	for _, field := range strings.Fields(query) {
		if jwtPattern.MatchString(field) {
			return true
		}
	}
	return false
}

// Execute decodes the JWT and pretty-prints header and payload.
func (s *JWTSkill) Execute(ctx context.Context, query string) (string, error) {
	token := extractJWT(query)
	if token == "" {
		return "", fmt.Errorf("no JWT token found in query")
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid JWT: expected 3 parts separated by '.', got %d", len(parts))
	}

	headerJSON, err := decodeSegment(parts[0])
	if err != nil {
		return "", fmt.Errorf("failed to decode JWT header: %w", err)
	}
	payloadJSON, err := decodeSegment(parts[1])
	if err != nil {
		return "", fmt.Errorf("failed to decode JWT payload: %w", err)
	}

	headerPretty := prettyJSON(headerJSON)
	payloadPretty := prettyJSON(payloadJSON)

	sig := parts[2]
	sigDisplay := sig
	if len(sigDisplay) > 12 {
		sigDisplay = sigDisplay[:12] + "..."
	}

	return fmt.Sprintf("Header: %s\nPayload: %s\nSignature: %s", headerPretty, payloadPretty, sigDisplay), nil
}

// extractJWT strips common prefixes and returns the first JWT-looking token.
func extractJWT(query string) string {
	q := strings.TrimSpace(query)
	lower := strings.ToLower(q)

	// Strip leading command words.
	prefixes := []string{"decode jwt", "jwt decode", "decode", "jwt"}
	for _, p := range prefixes {
		if strings.HasPrefix(lower, p) {
			q = strings.TrimSpace(q[len(p):])
			lower = strings.ToLower(q)
		}
	}

	for _, field := range strings.Fields(q) {
		if jwtPattern.MatchString(field) {
			return field
		}
	}
	if jwtPattern.MatchString(q) {
		return q
	}
	return ""
}

// decodeSegment base64-url-decodes a JWT segment and returns the raw bytes.
func decodeSegment(seg string) ([]byte, error) {
	if len(seg)%4 == 2 {
		seg += "=="
	} else if len(seg)%4 == 3 {
		seg += "="
	}
	return base64.URLEncoding.DecodeString(seg)
}

// prettyJSON re-indents JSON bytes; falls back to the raw string on error.
func prettyJSON(raw []byte) string {
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return string(raw)
	}
	return string(out)
}
