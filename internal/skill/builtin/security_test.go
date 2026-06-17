package builtin

import (
	"context"
	"strings"
	"testing"
)

// =====================================================================================
// Security skill tests — 4 skills (password_strength, secret_redact,
// hash_identify, cve_parse).
// =====================================================================================

func TestPasswordStrengthSkill(t *testing.T) {
	skill := NewPasswordStrengthSkill()
	if !skill.Match("password strength: abc") {
		t.Error("expected match for password strength query")
	}
	if skill.Match("encode this to base64") {
		t.Error("should not match unrelated query")
	}

	ctx := context.Background()
	// Weak password should get a low score + tips.
	out, err := skill.Execute(ctx, "password strength: abc")
	if err != nil {
		t.Fatalf("Execute weak: %v", err)
	}
	if !strings.Contains(out, "weak") && !strings.Contains(out, "fair") {
		t.Errorf("weak password expected weak/fair verdict, got %q", out)
	}

	// Strong password should get a high score.
	out2, _ := skill.Execute(ctx, "password strength: Sup3r$ecret!Passphrase99")
	if !strings.Contains(out2, "strong") && !strings.Contains(out2, "good") {
		t.Errorf("strong password expected strong/good verdict, got %q", out2)
	}
}

func TestSecretRedactSkill(t *testing.T) {
	skill := NewSecretRedactSkill()
	if !skill.Match("redact secrets in this text") {
		t.Error("expected match")
	}
	if skill.Match("compute sha256") {
		t.Error("should not match sha query")
	}

	// Input with known AWS key pattern.
	input := "redact secrets: AKIAIOSFODNN7EXAMPLE was leaked"
	out, err := skill.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "AWS Access Key") {
		t.Errorf("expected AWS Access Key finding, got %q", out)
	}
	// Output should contain "[REDACTED]" — never the original key.
	if strings.Contains(out, "AKIAIOSFODNN7EXAMPLE") {
		t.Errorf("redaction failed — original key still in output: %q", out)
	}
	if !strings.Contains(out, "[REDACTED]") {
		t.Errorf("expected [REDACTED] marker, got %q", out)
	}
}

func TestHashIdentifySkill(t *testing.T) {
	skill := NewHashIdentifySkill()
	if !skill.Match("identify hash 5d41402abc4b2a76b9719d911017c592") {
		t.Error("expected match for identify hash query")
	}

	ctx := context.Background()
	cases := []struct {
		hash  string
		want  string
	}{
		{"5d41402abc4b2a76b9719d911017c592", "MD5"},          // md5("hello")
		{"aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d", "SHA-1"}, // sha1("hello")
		{"2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824", "SHA-256"}, // sha256("hello")
		{"$2a$12$0InOksCFtFm/mVHdpb06eOvfYjNRsROYEUpLXo0nQqsUk4BfCwTbm", "bcrypt"},
		{"$argon2id$v=19$m=65536,t=3,p=4$abc", "argon2"},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			out, err := skill.Execute(ctx, "identify hash: "+tc.hash)
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if !strings.Contains(out, tc.want) {
				t.Errorf("expected %q in identification, got %q", tc.want, out)
			}
		})
	}
}

func TestCVEParseSkill(t *testing.T) {
	skill := NewCVEParseSkill()
	if !skill.Match("parse cve CVE-2024-12345") {
		t.Error("expected match for explicit parse query")
	}
	if !skill.Match("tell me about CVE-2021-44228") {
		t.Error("expected match for embedded CVE ID")
	}

	out, err := skill.Execute(context.Background(), "parse cve CVE-2021-44228")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "year: 2021") {
		t.Errorf("expected year field, got %q", out)
	}
	if !strings.Contains(out, "sequence: 44228") {
		t.Errorf("expected sequence field, got %q", out)
	}
	if !strings.Contains(out, "https://nvd.nist.gov/vuln/detail/CVE-2021-44228") {
		t.Errorf("expected NVD URL, got %q", out)
	}
}
