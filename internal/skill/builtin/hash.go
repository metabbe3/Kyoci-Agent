package builtin

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// HashSkill handles hash calculations.
type HashSkill struct {
	*kyoci.BaseSkill
	hashPattern *regexp.Regexp
}

// NewHashSkill creates a new hash skill.
func NewHashSkill() *HashSkill {
	skill := &HashSkill{
		BaseSkill: kyoci.NewBaseSkill(
			"hash",
			"Calculates MD5, SHA1, or SHA256 hash of a string",
			[]string{"hash", "md5", "sha1", "sha256", "hash this"},
		),
	}
	skill.hashPattern = regexp.MustCompile(`(?i)(hash|md5|sha1|sha256)\s+(?:this|of|the)?\s*(.+)`)
	return skill
}

// Match checks if the query is asking for a hash.
func (s *HashSkill) Match(query string) bool {
	queryLower := strings.ToLower(query)
	for _, keyword := range []string{"hash", "md5", "sha1", "sha256"} {
		if strings.Contains(queryLower, keyword) {
			return true
		}
	}
	return false
}

// Execute calculates the requested hash.
func (s *HashSkill) Execute(ctx context.Context, query string) (string, error) {
	queryLower := strings.ToLower(query)
	query = strings.TrimSpace(query)

	// Determine hash type and extract text
	var hashType string = "md5" // default
	var text string

	if match := s.hashPattern.FindStringSubmatch(query); match != nil {
		hashType = strings.ToLower(match[1])
		// Normalize "hash" to "md5"
		if hashType == "hash" {
			hashType = "md5"
		}
		text = strings.TrimSpace(match[2])
	} else {
		// Try to extract after keywords
		if strings.Contains(queryLower, "md5") {
			hashType = "md5"
			text = strings.TrimSpace(strings.ReplaceAll(queryLower, "md5", ""))
		} else if strings.Contains(queryLower, "sha1") {
			hashType = "sha1"
			text = strings.TrimSpace(strings.ReplaceAll(queryLower, "sha1", ""))
		} else if strings.Contains(queryLower, "sha256") {
			hashType = "sha256"
			text = strings.TrimSpace(strings.ReplaceAll(queryLower, "sha256", ""))
		} else {
			// Default to md5, remove common words
			text = strings.TrimSpace(strings.ReplaceAll(queryLower, "hash", ""))
			text = strings.TrimSpace(strings.ReplaceAll(text, "this", ""))
			text = strings.TrimSpace(strings.ReplaceAll(text, "of", ""))
		}
	}

	// Clean up text
	text = strings.TrimSpace(text)
	if text == "" {
		return "", fmt.Errorf("no text provided to hash")
	}

	// Calculate hash
	switch hashType {
	case "md5":
		hash := md5.Sum([]byte(text))
		return fmt.Sprintf("MD5: %s\nText: %s", hex.EncodeToString(hash[:]), text), nil
	case "sha1":
		hash := sha1.Sum([]byte(text))
		return fmt.Sprintf("SHA1: %s\nText: %s", hex.EncodeToString(hash[:]), text), nil
	case "sha256":
		hash := sha256.Sum256([]byte(text))
		return fmt.Sprintf("SHA256: %s\nText: %s", hex.EncodeToString(hash[:]), text), nil
	default:
		return "", fmt.Errorf("unsupported hash type: %s (supported: md5, sha1, sha256)", hashType)
	}
}