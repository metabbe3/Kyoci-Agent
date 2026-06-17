package builtin

import (
	"context"
	"strings"
	"testing"
)

// =====================================================================================
// Hashing skill tests — 13 skills (md5/sha1/sha256/sha512/sha3/crc32/crc64/
// hmac_sha256/hmac_sha512/bcrypt_hash/bcrypt_verify/aes_encrypt/aes_decrypt).
// =====================================================================================

// knownHashes are reference outputs for "hello" — used across multiple hash skills.
const helloMD5 = "5d41402abc4b2a76b9719d911017c592"
const helloSHA1 = "aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d"
const helloSHA256 = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"

func TestMD5Skill(t *testing.T) {
	runSkillCases(t, "md5", NewMD5Skill(), []skillCase{
		{"positive: md5 prefix", "md5 of hello", true, helloMD5, false},
		{"positive: md5 colon", "md5: hello", true, helloMD5, false},
		{"positive: compute md5", "compute md5 hello", true, helloMD5, false},
		{"negative: sha256 query", "sha256 of hello", false, "", false},
	})
}

func TestSHA1Skill(t *testing.T) {
	runSkillCases(t, "sha1", NewSHA1Skill(), []skillCase{
		{"positive", "sha1 of hello", true, helloSHA1, false},
		{"negative: md5 query", "md5 of hello", false, "", false},
	})
}

func TestSHA256Skill(t *testing.T) {
	runSkillCases(t, "sha256", NewSHA256Skill(), []skillCase{
		{"positive", "sha256 of hello", true, helloSHA256, false},
		{"negative: md5 query", "md5 of hello", false, "", false},
	})
}

func TestSHA512Skill(t *testing.T) {
	skill := NewSHA512Skill()
	// SHA-512 of "hello" is 128 hex chars.
	if !skill.Match("sha512 of hello") {
		t.Error("expected match for sha512 query")
	}
	out, err := skill.Execute(context.Background(), "sha512 of hello")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// Strip any non-hex chars and check length.
	hexOnly := stripNonHex(out)
	if len(hexOnly) != 128 {
		t.Errorf("expected 128 hex chars for sha512, got %d (output: %q)", len(hexOnly), out)
	}
}

func TestSHA3Skill(t *testing.T) {
	skill := NewSHA3Skill()
	if !skill.Match("sha3 of hello") {
		t.Error("expected match for sha3 query")
	}
	out, err := skill.Execute(context.Background(), "sha3 of hello")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// SHA3-256 produces 64 hex chars.
	hexOnly := stripNonHex(out)
	if len(hexOnly) != 64 {
		t.Errorf("expected 64 hex chars for sha3-256, got %d (output: %q)", len(hexOnly), out)
	}
	// Negative: sha256 query should not match.
	if skill.Match("sha256 of hello") {
		t.Error("sha3 should not match sha256 query")
	}
}

func TestCRC32Skill(t *testing.T) {
	skill := NewCRC32Skill()
	if !skill.Match("crc32 of hello") {
		t.Error("expected match for crc32 query")
	}
	out, err := skill.Execute(context.Background(), "crc32 of hello")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// CRC-32 of "hello" = 0x3610a686 = "3610a686"
	if !strings.Contains(out, "3610a686") {
		t.Errorf("crc32(hello) should be 3610a686, got %q", out)
	}
}

func TestCRC64Skill(t *testing.T) {
	skill := NewCRC64Skill()
	if !skill.Match("crc64 of hello") {
		t.Error("expected match for crc64 query")
	}
	out, err := skill.Execute(context.Background(), "crc64 of hello")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	hexOnly := stripNonHex(out)
	if len(hexOnly) != 16 {
		t.Errorf("expected 16 hex chars for crc64, got %d (output: %q)", len(hexOnly), out)
	}
}

func TestHMACSHA256Skill(t *testing.T) {
	runSkillCases(t, "hmac_sha256", NewHMACSHA256Skill(), []skillCase{
		{"positive", "hmac-sha256 key:hello", true, "", false},
		{"negative: hmac_sha512 query", "hmac-sha512 key:hello", false, "", false},
	})
	// Verify output is deterministic + correct length.
	skill := NewHMACSHA256Skill()
	out, _ := skill.Execute(context.Background(), "hmac-sha256 secret:hello")
	hexOnly := stripNonHex(out)
	if len(hexOnly) != 64 {
		t.Errorf("expected 64 hex chars for hmac-sha256, got %d (output: %q)", len(hexOnly), out)
	}
}

func TestHMACSHA512Skill(t *testing.T) {
	skill := NewHMACSHA512Skill()
	if !skill.Match("hmac-sha512 k:v") {
		t.Error("expected match")
	}
	out, _ := skill.Execute(context.Background(), "hmac-sha512 k:v")
	hexOnly := stripNonHex(out)
	if len(hexOnly) != 128 {
		t.Errorf("expected 128 hex chars for hmac-sha512, got %d (output: %q)", len(hexOnly), out)
	}
}

func TestBcryptHashSkill(t *testing.T) {
	skill := NewBcryptHashSkill()
	if !skill.Match("bcrypt hash: password123") {
		t.Error("expected match")
	}
	out, err := skill.Execute(context.Background(), "bcrypt hash: password123")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// Bcrypt hashes start with $2a$, $2b$, or $2y$.
	if !strings.HasPrefix(out, "$2") {
		t.Errorf("expected bcrypt $2X$ prefix, got %q", out)
	}
}

func TestBcryptVerifySkill(t *testing.T) {
	skill := NewBcryptVerifySkill()
	// First hash a password, then verify it.
	hashSkill := NewBcryptHashSkill()
	hashed, _ := hashSkill.Execute(context.Background(), "bcrypt hash: secret")
	hashed = strings.TrimSpace(hashed)
	query := "bcrypt verify secret:" + hashed
	if !skill.Match(query) {
		t.Error("expected match for bcrypt verify query")
	}
	out, err := skill.Execute(context.Background(), query)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "match") {
		t.Errorf("expected 'match' verdict, got %q", out)
	}

	// Negative: wrong password should mismatch.
	wrongQuery := "bcrypt verify wrong:" + hashed
	out2, _ := skill.Execute(context.Background(), wrongQuery)
	if !strings.Contains(out2, "mismatch") {
		t.Errorf("expected 'mismatch' verdict, got %q", out2)
	}
}

func TestAESEncryptDecryptSkill(t *testing.T) {
	encSkill := NewAESEncryptSkill()
	decSkill := NewAESDecryptSkill()

	if !encSkill.Match("aes encrypt pass:plaintext") {
		t.Error("encrypt: expected match")
	}
	if !decSkill.Match("aes decrypt pass:ciphertext") {
		t.Error("decrypt: expected match")
	}

	// Round-trip: encrypt then decrypt → original plaintext.
	encOut, err := encSkill.Execute(context.Background(), "aes encrypt my-secret-passphrase:Hello, World!")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	encOut = strings.TrimSpace(encOut)
	if len(encOut) < 64 { // salt(16) + nonce(12) + body + tag
		t.Errorf("ciphertext too short: %d chars", len(encOut))
	}

	decQuery := "aes decrypt my-secret-passphrase:" + encOut
	decOut, err := decSkill.Execute(context.Background(), decQuery)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !strings.Contains(decOut, "Hello, World!") {
		t.Errorf("round-trip failed: expected 'Hello, World!', got %q", decOut)
	}

	// Negative: wrong passphrase should fail.
	_, err = decSkill.Execute(context.Background(), "aes decrypt wrong-passphrase:"+encOut)
	if err == nil {
		t.Error("decrypt with wrong passphrase should fail")
	}
}

// stripNonHex returns s with everything except [0-9a-fA-F] removed. Used to
// validate hash output lengths regardless of label prefix/suffix.
func stripNonHex(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F') {
			b.WriteRune(r)
		}
	}
	return b.String()
}
