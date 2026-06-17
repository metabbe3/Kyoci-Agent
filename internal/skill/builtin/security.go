package builtin

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// =====================================================================================
// Security skills — password strength, secret redaction, hash identification, CVE parsing.
// =====================================================================================

// ---- password strength ----

type PasswordStrengthSkill struct{ *kyoci.BaseSkill }

func NewPasswordStrengthSkill() *PasswordStrengthSkill {
	return &PasswordStrengthSkill{BaseSkill: kyoci.NewBaseSkill(
		"password_strength", "Score password strength (0-100) with improvement tips",
		[]string{"password strength", "strength of password", "how strong is this password", "password score"},
	)}
}
func (s *PasswordStrengthSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "password strength") || strings.Contains(q, "strength of password") ||
		strings.Contains(q, "how strong") || strings.Contains(q, "password score") ||
		strings.Contains(q, "password quality")
}
func (s *PasswordStrengthSkill) Execute(_ context.Context, q string) (string, error) {
	pw := quoteStripped(extractPayload(q))
	if pw == "" {
		return "", fmt.Errorf("no password to score")
	}
	score, tips := scorePassword(pw)
	verdict := "weak"
	switch {
	case score >= 80:
		verdict = "strong"
	case score >= 60:
		verdict = "good"
	case score >= 40:
		verdict = "fair"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "score: %d/100 (%s)\n", score, verdict)
	fmt.Fprintf(&b, "length: %d\n", len(pw))
	if len(tips) == 0 {
		b.WriteString("tips: none — solid password\n")
	} else {
		b.WriteString("tips:\n")
		for _, t := range tips {
			fmt.Fprintf(&b, "  - %s\n", t)
		}
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// scorePassword is a simple length+variety scoring heuristic. Not as rigorous
// as zxcvbn but zero-dep and fast. Returns 0-100 score and improvement tips.
func scorePassword(pw string) (int, []string) {
	var (
		hasLower, hasUpper, hasDigit, hasSymbol bool
	)
	for _, r := range pw {
		switch {
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsDigit(r):
			hasDigit = true
		case unicode.IsPunct(r) || unicode.IsSymbol(r):
			hasSymbol = true
		}
	}
	score := 0
	tips := []string{}

	// Length scoring (up to 50)
	switch {
	case len(pw) >= 16:
		score += 50
	case len(pw) >= 12:
		score += 40
	case len(pw) >= 8:
		score += 25
		tips = append(tips, "use 12+ characters for a stronger password")
	default:
		score += 10
		tips = append(tips, "use at least 8 characters (12+ recommended)")
	}

	// Variety scoring (up to 50)
	classes := 0
	if hasLower {
		classes++
	} else {
		tips = append(tips, "add lowercase letters")
	}
	if hasUpper {
		classes++
	} else {
		tips = append(tips, "add uppercase letters")
	}
	if hasDigit {
		classes++
	} else {
		tips = append(tips, "add digits")
	}
	if hasSymbol {
		classes++
	} else {
		tips = append(tips, "add symbols (!@#$...)")
	}
	score += classes * 12

	// Penalize common patterns.
	lower := strings.ToLower(pw)
	for _, bad := range []string{"password", "123456", "qwerty", "abc123", "letmein", "admin"} {
		if strings.Contains(lower, bad) {
			score -= 20
			tips = append(tips, fmt.Sprintf("avoid the common pattern %q", bad))
			break
		}
	}

	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return score, tips
}

// ---- secret redaction ----

type SecretRedactSkill struct{ *kyoci.BaseSkill }

// Common secret patterns. Order matters — more specific patterns first.
var secretPatterns = []struct {
	name   string
	pattern *regexp.Regexp
}{
	{"AWS Access Key", regexp.MustCompile(`AKIA[0-9A-Z]{16}`)},
	{"AWS Secret Key", regexp.MustCompile(`(?m)aws_secret_access_key\s*=\s*[A-Za-z0-9/+=]{40}`)},
	{"GitHub Token (ghp_)", regexp.MustCompile(`ghp_[A-Za-z0-9]{36}`)},
	{"GitHub Token (github_pat_)", regexp.MustCompile(`github_pat_[A-Za-z0-9_]{82}`)},
	{"Google API Key", regexp.MustCompile(`AIza[0-9A-Za-z\-_]{35}`)},
	{"Slack Token", regexp.MustCompile(`xox[baprs]-[A-Za-z0-9-]{10,}`)},
	{"Stripe Key (sk_live_)", regexp.MustCompile(`sk_live_[A-Za-z0-9]{24,}`)},
	{"Stripe Key (rk_live_)", regexp.MustCompile(`rk_live_[A-Za-z0-9]{24,}`)},
	{"JWT", regexp.MustCompile(`eyJ[A-Za-z0-9_-]{10,}\.eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}`)},
	{"Generic Bearer Token", regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9_\-\.=]{20,}`)},
	{"Private Key Block", regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)},
}

func NewSecretRedactSkill() *SecretRedactSkill {
	return &SecretRedactSkill{BaseSkill: kyoci.NewBaseSkill(
		"secret_redact", "Find and mask API keys / tokens / private keys in text",
		[]string{"redact secrets", "find secrets", "mask api keys", "scan for secrets"},
	)}
}
func (s *SecretRedactSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "redact secrets") || strings.Contains(q, "mask secrets") ||
		strings.Contains(q, "find secrets") || strings.Contains(q, "find api keys") ||
		strings.Contains(q, "mask api keys") || strings.Contains(q, "scan for secrets")
}
func (s *SecretRedactSkill) Execute(_ context.Context, q string) (string, error) {
	text := extractPayload(q)
	if text == "" {
		text = q
	}
	findings := []string{}
	redacted := text
	for _, p := range secretPatterns {
		matches := p.pattern.FindAllString(redacted, -1)
		if len(matches) > 0 {
			findings = append(findings, fmt.Sprintf("- %s: %d occurrence(s)", p.name, len(matches)))
		}
		redacted = p.pattern.ReplaceAllStringFunc(redacted, func(m string) string {
			if len(m) < 8 {
				return "[REDACTED]"
			}
			return m[:4] + "..." + m[len(m)-4:] + "  [REDACTED]"
		})
	}
	var b strings.Builder
	if len(findings) == 0 {
		return "no secrets found", nil
	}
	b.WriteString(fmt.Sprintf("found %d secret type(s):\n", len(findings)))
	for _, f := range findings {
		b.WriteString(f)
		b.WriteString("\n")
	}
	b.WriteString("\nredacted text:\n")
	b.WriteString(redacted)
	return strings.TrimRight(b.String(), "\n"), nil
}

// ---- hash identification ----

type HashIdentifySkill struct{ *kyoci.BaseSkill }

func NewHashIdentifySkill() *HashIdentifySkill {
	return &HashIdentifySkill{BaseSkill: kyoci.NewBaseSkill(
		"hash_identify", "Identify hash type from its format (md5, sha1, sha256, sha512, bcrypt, argon2)",
		[]string{"identify hash", "what hash is this", "hash type", "what kind of hash"},
	)}
}
func (s *HashIdentifySkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "identify hash") || strings.Contains(q, "what hash") ||
		strings.Contains(q, "hash type") || strings.Contains(q, "kind of hash") ||
		strings.Contains(q, "what kind of hash")
}
func (s *HashIdentifySkill) Execute(_ context.Context, q string) (string, error) {
	h := strings.TrimSpace(quoteStripped(extractPayload(q)))
	if h == "" {
		return "", fmt.Errorf("no hash to identify")
	}
	candidates := []string{}

	// bcrypt / argon2 — prefix-based
	if strings.HasPrefix(h, "$2a$") || strings.HasPrefix(h, "$2b$") || strings.HasPrefix(h, "$2y$") {
		candidates = append(candidates, "bcrypt (60 chars, $2X$ format)")
	}
	if strings.HasPrefix(h, "$argon2i") || strings.HasPrefix(h, "$argon2id") {
		candidates = append(candidates, "argon2 (PHC string format)")
	}
	if strings.HasPrefix(h, "$1$") {
		candidates = append(candidates, "MD5 Crypt (Unix $1$ format)")
	}
	if strings.HasPrefix(h, "$5$") {
		candidates = append(candidates, "SHA-256 Crypt (Unix $5$ format)")
	}
	if strings.HasPrefix(h, "$6$") {
		candidates = append(candidates, "SHA-512 Crypt (Unix $6$ format)")
	}

	// Hex-based — length only (this is best-effort; multiple algorithms can share lengths)
	hexOnly := regexp.MustCompile(`^[a-fA-F0-9]+$`).MatchString(h)
	if hexOnly {
		switch len(h) {
		case 32:
			candidates = append(candidates, "MD5 (32 hex chars)")
		case 40:
			candidates = append(candidates, "SHA-1 (40 hex chars), also RIPEMD-160")
		case 56:
			candidates = append(candidates, "SHA-224 (56 hex chars)")
		case 64:
			candidates = append(candidates, "SHA-256 (64 hex chars), also SHA3-256")
		case 96:
			candidates = append(candidates, "SHA-384 (96 hex chars)")
		case 128:
			candidates = append(candidates, "SHA-512 (128 hex chars), also SHA3-512")
		}
	}

	if len(candidates) == 0 {
		return fmt.Sprintf("unknown hash format (length=%d, looks-like-hex=%v)", len(h), hexOnly), nil
	}
	return strings.Join(candidates, "\n"), nil
}

// ---- CVE parse ----

type CVEParseSkill struct{ *kyoci.BaseSkill }

var cveRe = regexp.MustCompile(`CVE-(\d{4})-(\d{4,})`)

func NewCVEParseSkill() *CVEParseSkill {
	return &CVEParseSkill{BaseSkill: kyoci.NewBaseSkill(
		"cve_parse", "Parse CVE-YYYY-NNNN identifier into year + sequence number",
		[]string{"parse cve", "cve parse", "cve id", "cve-"},
	)}
}
func (s *CVEParseSkill) Match(q string) bool {
	q = strings.ToLower(q)
	if strings.Contains(q, "parse cve") || strings.Contains(q, "cve parse") || strings.Contains(q, "cve id") {
		return true
	}
	// Direct CVE-YYYY-NNNN in the query also triggers.
	return cveRe.MatchString(q)
}
func (s *CVEParseSkill) Execute(_ context.Context, q string) (string, error) {
	m := cveRe.FindStringSubmatch(q)
	if m == nil {
		return "", fmt.Errorf("no CVE-YYYY-NNNN identifier found")
	}
	year := m[1]
	seq := m[2]
	id := m[0]
	return fmt.Sprintf(
		"id: %s\nyear: %s\nsequence: %s\nurl: https://nvd.nist.gov/vuln/detail/%s",
		id, year, seq, id,
	), nil
}
