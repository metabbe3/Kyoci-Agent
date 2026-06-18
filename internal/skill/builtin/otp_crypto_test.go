package builtin

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"
)

// =====================================================================================
// OTP / HMAC crypto skill tests — 7 skills.
//
// TOTP/HOTP use RFC 6238/4226 published test vectors with the clock pinned
// (via nowFunc) to a fixed instant. Random skills verify output format only.
// =====================================================================================

// withFixedNow swaps in a deterministic clock for the duration of fn, then
// restores the real clock. Tests use this to assert exact TOTP codes against
// RFC 6238 Appendix B.
func withFixedNow(t time.Time, fn func()) {
	prev := nowFunc
	nowFunc = func() time.Time { return t }
	defer func() { nowFunc = prev }()
	fn()
}

// ---- totp_generate ----

func TestTOTPGenerateSkill(t *testing.T) {
	// RFC 6238 Appendix B test vector:
	//   secret (ASCII) "12345678901234567890"
	//   base32        "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	//   T=59          8-digit SHA-1 code = 94287082 → 6-digit = 287082
	//   T=1111111109  6-digit SHA-1 code = 081804
	const secretB32 = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"

	skill := NewTOTPGenerateSkill()
	ctx := context.Background()

	t.Run("positive: T=59 → 287082", func(t *testing.T) {
		withFixedNow(time.Unix(59, 0), func() {
			out, err := skill.Execute(ctx, "totp: "+secretB32)
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if out != "287082" {
				t.Errorf("Execute at T=59 = %q, want 287082 (6-digit truncation of RFC 6238 vector)", out)
			}
		})
	})

	t.Run("positive: T=1111111109 → 081804", func(t *testing.T) {
		withFixedNow(time.Unix(1111111109, 0), func() {
			out, err := skill.Execute(ctx, "totp: "+secretB32)
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if out != "081804" {
				t.Errorf("Execute at T=1111111109 = %q, want 081804", out)
			}
		})
	})

	// Match cases.
	runSkillCases(t, "totp_generate", skill, []skillCase{
		{"match: totp", "totp: GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ", true, "", false},
		{"match: generate totp", "generate totp for GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ", true, "", false},
		{"negative: hotp", "hotp: GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ counter 5", false, "", false},
		{"negative: unrelated", "sha256 of hello", false, "", false},
		{"edge: empty secret", "totp:", true, "", true},
		{"edge: invalid base32", "totp: @@@notbase32@@@", true, "", true},
	})
}

// ---- hotp_generate ----

func TestHOTPGenerateSkill(t *testing.T) {
	// RFC 4226 Appendix D test vectors for "12345678901234567890":
	//   count=0 → 755224, count=1 → 287082, count=5 → 254676
	const secretB32 = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"

	ctx := context.Background()

	t.Run("RFC 4226 count=0 → 755224", func(t *testing.T) {
		skill := NewHOTPGenerateSkill()
		out, err := skill.Execute(ctx, "hotp: "+secretB32+" counter 0")
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if out != "755224" {
			t.Errorf("count=0 → %q, want 755224", out)
		}
	})

	t.Run("RFC 4226 count=1 → 287082", func(t *testing.T) {
		skill := NewHOTPGenerateSkill()
		out, err := skill.Execute(ctx, "hotp: "+secretB32+" counter 1")
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if out != "287082" {
			t.Errorf("count=1 → %q, want 287082", out)
		}
	})

	t.Run("RFC 4226 count=5 → 254676", func(t *testing.T) {
		skill := NewHOTPGenerateSkill()
		out, err := skill.Execute(ctx, "hotp: "+secretB32+" counter 5")
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if out != "254676" {
			t.Errorf("count=5 → %q, want 254676", out)
		}
	})

	t.Run("pipe format count=5 → 254676", func(t *testing.T) {
		skill := NewHOTPGenerateSkill()
		out, err := skill.Execute(ctx, "hotp: "+secretB32+" | 5")
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if out != "254676" {
			t.Errorf("pipe count=5 → %q, want 254676", out)
		}
	})

	skill := NewHOTPGenerateSkill()
	runSkillCases(t, "hotp_generate", skill, []skillCase{
		{"match: hotp", "hotp: GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ counter 5", true, "254676", false},
		{"match: generate hotp", "generate hotp GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ counter 5", true, "254676", false},
		{"negative: totp", "totp: GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ", false, "", false},
		{"negative: unrelated", "random hex: 16", false, "", false},
		{"edge: empty secret", "hotp: counter 5", true, "", true},
		{"edge: invalid counter", "hotp: GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ counter abc", true, "", true},
	})
}

// ---- otp_secret_generate ----

var base32NoPadRe = regexp.MustCompile(`^[A-Z2-7]+$`)

func TestOTPSecretGenerateSkill(t *testing.T) {
	skill := NewOTPSecretGenerateSkill()
	ctx := context.Background()

	t.Run("default 20 bytes → 32 base32 chars", func(t *testing.T) {
		out, err := skill.Execute(ctx, "generate otp secret")
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if len(out) != 32 {
			t.Errorf("default length = %d, want 32 (20 bytes → ceil(20/5)*8)", len(out))
		}
		if !base32NoPadRe.MatchString(out) {
			t.Errorf("output %q is not base32 (no padding)", out)
		}
	})

	t.Run("explicit 10 bytes", func(t *testing.T) {
		out, err := skill.Execute(ctx, "otp secret 10")
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if len(out) != 16 {
			t.Errorf("10-byte length = %d, want 16", len(out))
		}
	})

	t.Run("produces distinct values", func(t *testing.T) {
		a, _ := skill.Execute(ctx, "otp secret")
		b, _ := skill.Execute(ctx, "otp secret")
		if a == b {
			t.Errorf("expected distinct secrets, got %q twice", a)
		}
	})

	runSkillCases(t, "otp_secret_generate", skill, []skillCase{
		{"match: otp secret", "otp secret", true, "", false},
		{"match: generate secret", "generate secret", true, "", false},
		{"match: generate otp secret", "generate otp secret", true, "", false},
		{"negative: totp", "totp: JBSWY3DPEHPK3PXP", false, "", false},
		{"negative: unrelated", "uuid v4", false, "", false},
		{"edge: huge byte count", "otp secret 999999999", true, "", true},
	})
}

// ---- random_hex ----

var hexRe = regexp.MustCompile(`^[0-9a-f]+$`)

func TestRandomHexSkill(t *testing.T) {
	skill := NewRandomHexSkill()
	ctx := context.Background()

	t.Run("default 16 bytes → 32 hex chars", func(t *testing.T) {
		out, err := skill.Execute(ctx, "random hex")
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if len(out) != 32 {
			t.Errorf("default length = %d, want 32", len(out))
		}
		if !hexRe.MatchString(out) {
			t.Errorf("output %q is not hex", out)
		}
	})

	t.Run("explicit 16 bytes via colon", func(t *testing.T) {
		out, err := skill.Execute(ctx, "random hex: 16")
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if len(out) != 32 {
			t.Errorf("16-byte length = %d, want 32", len(out))
		}
	})

	t.Run("explicit 8 bytes via suffix", func(t *testing.T) {
		out, err := skill.Execute(ctx, "random_hex 8")
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if len(out) != 16 {
			t.Errorf("8-byte length = %d, want 16", len(out))
		}
	})

	t.Run("distinct values", func(t *testing.T) {
		a, _ := skill.Execute(ctx, "random hex: 32")
		b, _ := skill.Execute(ctx, "random hex: 32")
		if a == b {
			t.Errorf("expected distinct hex, got %q twice", a)
		}
	})

	runSkillCases(t, "random_hex", skill, []skillCase{
		{"match: random hex", "random hex: 16", true, "", false},
		{"match: generate hex", "generate hex 16", true, "", false},
		{"match: random_hex underscore", "random_hex 16", true, "", false},
		{"negative: uuid", "uuid v4", false, "", false},
		{"negative: sha256", "sha256 of hello", false, "", false},
	})
}

// ---- hmac_for_algorithm ----

func TestHMACForAlgorithmSkill(t *testing.T) {
	// Reference values (computed with Python hmac):
	//   hmac-md5(key="key", data="hello")    = 04130747afca4d79e32e87cf2104f087
	//   hmac-sha1(key="key", data="hello")   = b34ceac4516ff23a143e61d79d0fa7a4fbe5f266
	//   hmac-sha256(key="key", data="hello") = 9307b3b915efb5171ff14d8cb55fbcc798c6c0ef1456d66ded1a6aa723a58b7b
	//   hmac-sha512(key="key", data="hello") = ff06ab36...c0c92 (full digest checked)
	ctx := context.Background()

	t.Run("sha256 via colon form", func(t *testing.T) {
		skill := NewHMACForAlgorithmSkill()
		out, err := skill.Execute(ctx, "hmac for algorithm sha256: key | hello")
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		want := "9307b3b915efb5171ff14d8cb55fbcc798c6c0ef1456d66ded1a6aa723a58b7b"
		if out != want {
			t.Errorf("HMAC-SHA256 = %q, want %q", out, want)
		}
	})

	t.Run("sha1 via colon form", func(t *testing.T) {
		skill := NewHMACForAlgorithmSkill()
		out, err := skill.Execute(ctx, "generic hmac sha1: key | hello")
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		want := "b34ceac4516ff23a143e61d79d0fa7a4fbe5f266"
		if out != want {
			t.Errorf("HMAC-SHA1 = %q, want %q", out, want)
		}
	})

	t.Run("md5 via colon form", func(t *testing.T) {
		skill := NewHMACForAlgorithmSkill()
		out, err := skill.Execute(ctx, "hmac for algorithm md5: key | hello")
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		want := "04130747afca4d79e32e87cf2104f087"
		if out != want {
			t.Errorf("HMAC-MD5 = %q, want %q", out, want)
		}
	})

	t.Run("sha512 full digest", func(t *testing.T) {
		skill := NewHMACForAlgorithmSkill()
		out, err := skill.Execute(ctx, "hmac for algorithm sha512: key | hello")
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		want := "ff06ab36757777815c008d32c8e14a705b4e7bf310351a06a23b612dc4c7433e7757d20525a5593b71020ea2ee162d2311b247e9855862b270122419652c0c92"
		if out != want {
			t.Errorf("HMAC-SHA512 = %q, want %q", out, want)
		}
	})

	t.Run("three-token form 'hmac: alg key data'", func(t *testing.T) {
		skill := NewHMACForAlgorithmSkill()
		out, err := skill.Execute(ctx, "hmac: sha256 key hello")
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		want := "9307b3b915efb5171ff14d8cb55fbcc798c6c0ef1456d66ded1a6aa723a58b7b"
		if out != want {
			t.Errorf("three-token HMAC-SHA256 = %q, want %q", out, want)
		}
	})

	t.Run("uppercase algorithm normalized", func(t *testing.T) {
		skill := NewHMACForAlgorithmSkill()
		out, err := skill.Execute(ctx, "hmac for algorithm SHA256: key | hello")
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if !strings.HasPrefix(out, "9307b3b9") {
			t.Errorf("uppercase-alg HMAC = %q, expected SHA256 prefix", out)
		}
	})

	skill := NewHMACForAlgorithmSkill()
	runSkillCases(t, "hmac_for_algorithm", skill, []skillCase{
		{"match: hmac for algorithm", "hmac for algorithm sha256: key | hello", true, "9307b3b9", false},
		{"match: generic hmac", "generic hmac sha256: key | hello", true, "9307b3b9", false},
		{"match: hmac for", "hmac for sha256 key hello", true, "9307b3b9", false},
		{"negative: hmac md5 only", "hmac md5: key | hello", false, "", false},
		{"negative: unrelated", "sha256 of hello", false, "", false},
		{"edge: unsupported algorithm", "hmac for algorithm ripemd160: key | hello", true, "", true},
	})
}

// ---- timing_safe_compare ----

func TestTimingSafeCompareSkill(t *testing.T) {
	skill := NewTimingSafeCompareSkill()
	ctx := context.Background()

	t.Run("match via pipe", func(t *testing.T) {
		out, err := skill.Execute(ctx, "timing safe compare: abc | abc")
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if out != "match" {
			t.Errorf("= %q, want match", out)
		}
	})

	t.Run("mismatch via pipe", func(t *testing.T) {
		out, err := skill.Execute(ctx, "timing safe compare: abc | xyz")
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if out != "mismatch" {
			t.Errorf("= %q, want mismatch", out)
		}
	})

	t.Run("length mismatch → mismatch", func(t *testing.T) {
		out, err := skill.Execute(ctx, "timing safe compare: a | ab")
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if out != "mismatch" {
			t.Errorf("= %q, want mismatch", out)
		}
	})

	t.Run("token-verify use case", func(t *testing.T) {
		token := "supersecrettoken123"
		out, err := skill.Execute(ctx, "constant time compare: "+token+" | "+token)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if out != "match" {
			t.Errorf("= %q, want match", out)
		}
	})

	runSkillCases(t, "timing_safe_compare", skill, []skillCase{
		{"match: timing safe compare", "timing safe compare: abc | abc", true, "match", false},
		{"match: safe compare", "safe compare abc | abc", true, "match", false},
		{"match: constant time compare", "constant time compare: abc | abc", true, "match", false},
		{"negative: hmac md5", "hmac md5: key | data", false, "", false},
		{"negative: unrelated", "base64 encode: hi", false, "", false},
		{"edge: single value", "timing safe compare: onlyone", true, "", true},
	})
}

// ---- hmac_md5 ----

func TestHMACMD5Skill(t *testing.T) {
	skill := NewHMACMD5Skill()
	ctx := context.Background()

	t.Run("pipe form", func(t *testing.T) {
		out, err := skill.Execute(ctx, "hmac md5: key | hello")
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		want := "04130747afca4d79e32e87cf2104f087"
		if out != want {
			t.Errorf("HMAC-MD5 = %q, want %q", out, want)
		}
	})

	t.Run("colon form", func(t *testing.T) {
		out, err := skill.Execute(ctx, "hmac md5: key:hello")
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		want := "04130747afca4d79e32e87cf2104f087"
		if out != want {
			t.Errorf("HMAC-MD5 = %q, want %q", out, want)
		}
	})

	t.Run("dashed form", func(t *testing.T) {
		out, err := skill.Execute(ctx, "hmac-md5: key | hello")
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		want := "04130747afca4d79e32e87cf2104f087"
		if out != want {
			t.Errorf("HMAC-MD5 = %q, want %q", out, want)
		}
	})

	runSkillCases(t, "hmac_md5", skill, []skillCase{
		{"match: hmac md5", "hmac md5: key | hello", true, "04130747afca4d79e32e87cf2104f087", false},
		{"match: hmac-md5", "hmac-md5: key | hello", true, "04130747afca4d79e32e87cf2104f087", false},
		{"match: hmac_md5", "hmac_md5 key | hello", true, "04130747afca4d79e32e87cf2104f087", false},
		{"negative: hmac sha256", "hmac sha256: key | hello", false, "", false},
		{"negative: generic hmac", "hmac for algorithm md5: key | hello", false, "", false},
		{"edge: missing separator", "hmac md5: onlykey", true, "", true},
	})
}
