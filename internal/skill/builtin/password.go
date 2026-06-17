package builtin

import (
	"context"
	"crypto/rand"
	"fmt"
	"math"
	"regexp"
	"strings"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// passwordLenPattern extracts an explicit length argument from a query.
var passwordLenPattern = regexp.MustCompile(`\d+`)

const (
	passwordDefaultLen = 16
	passwordCharset    = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789!@#$%^&*()-_=+"
)

// PasswordSkill generates a secure random password.
type PasswordSkill struct {
	*kyoci.BaseSkill
}

// NewPasswordSkill creates a new password skill.
func NewPasswordSkill() *PasswordSkill {
	return &PasswordSkill{
		BaseSkill: kyoci.NewBaseSkill(
			"password",
			"Generate a secure random password",
			[]string{"password", "generate password", "random password", "passphrase"},
		),
	}
}

// Match returns true if the query mentions password.
func (s *PasswordSkill) Match(query string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(query)), "password")
}

// Execute generates and returns a random password with an entropy estimate.
func (s *PasswordSkill) Execute(ctx context.Context, query string) (string, error) {
	length := passwordDefaultLen
	if m := passwordLenPattern.FindString(strings.TrimSpace(query)); m != "" {
		var n int
		if _, err := fmt.Sscanf(m, "%d", &n); err == nil && n > 0 {
			length = n
		}
	}

	pw, err := generatePassword(length)
	if err != nil {
		return "", fmt.Errorf("failed to generate password: %w", err)
	}

	entropy := float64(length) * math.Log2(float64(len(passwordCharset)))
	return fmt.Sprintf("Password: %s\nLength: %d\nEntropy: ~%.0f bits", pw, length, entropy), nil
}

// generatePassword returns a cryptographically secure random string of the
// given length drawn from passwordCharset.
func generatePassword(length int) (string, error) {
	if length <= 0 {
		length = passwordDefaultLen
	}
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, length)
	for i, b := range buf {
		out[i] = passwordCharset[int(b)%len(passwordCharset)]
	}
	return string(out), nil
}
