package builtin

import (
	"context"
	"strings"
	"testing"
)

// =====================================================================================
// Encoding skill tests — 12 skills (base64/base32/url/html/hex/unicode).
// =====================================================================================

// runSkillCases is the shared driver for table-driven skill tests.
// Each case asserts Match() then, if shouldMatch, runs Execute and checks output.
type skillCase struct {
	name        string
	query       string
	shouldMatch bool
	want        string // substring expected in Execute output; "" = expect error/empty
	wantErr     bool   // if true, Execute must return an error
}

func runSkillCases(t *testing.T, skillName string, skill interface {
	Match(string) bool
	Execute(context.Context, string) (string, error)
}, cases []skillCase) {
	t.Helper()
	ctx := context.Background()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := skill.Match(tc.query)
			if got != tc.shouldMatch {
				t.Errorf("%s.Match(%q) = %v, want %v", skillName, tc.query, got, tc.shouldMatch)
				return
			}
			if !tc.shouldMatch {
				return
			}
			out, err := skill.Execute(ctx, tc.query)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil (output: %q)", out)
				}
				return
			}
			if err != nil {
				t.Errorf("Execute(%q): %v", tc.query, err)
				return
			}
			if tc.want != "" && !strings.Contains(out, tc.want) {
				t.Errorf("Execute(%q) = %q, want containing %q", tc.query, out, tc.want)
			}
		})
	}
}

// ---- base64 ----

func TestBase64EncodeSkill(t *testing.T) {
	runSkillCases(t, "base64_encode", NewBase64EncodeSkill(), []skillCase{
		{"positive: encode command", "base64 encode: hello", true, "aGVsbG8=", false},
		{"positive: encode base64", "encode base64: hello", true, "aGVsbG8=", false},
		{"positive: to base64", "convert to base64: hi", true, "aGk=", false},
		{"negative: decode query", "base64 decode: aGVsbG8=", false, "", false},
		{"negative: unrelated", "sha256 of hello", false, "", false},
	})
}

func TestBase64DecodeSkill(t *testing.T) {
	runSkillCases(t, "base64_decode", NewBase64DecodeSkill(), []skillCase{
		{"positive: decode command", "base64 decode: aGVsbG8=", true, "hello", false},
		{"positive: from base64", "from base64: aGk=", true, "hi", false},
		{"negative: encode query", "base64 encode: hello", false, "", false},
		{"edge: invalid base64", "base64 decode: @@@notvalid@@@", true, "", true},
	})
}

// ---- base32 ----

func TestBase32EncodeSkill(t *testing.T) {
	runSkillCases(t, "base32_encode", NewBase32EncodeSkill(), []skillCase{
		{"positive", "base32 encode: hello", true, "NBSWY3DP", false},
		{"negative", "base64 encode: hi", false, "", false},
	})
}

func TestBase32DecodeSkill(t *testing.T) {
	runSkillCases(t, "base32_decode", NewBase32DecodeSkill(), []skillCase{
		{"positive", "base32 decode: NBSWY3DP", true, "hello", false},
		{"negative", "base32 encode: hi", false, "", false},
	})
}

// ---- url ----

func TestURLEncodeSkill(t *testing.T) {
	runSkillCases(t, "url_encode", NewURLEncodeSkill(), []skillCase{
		{"positive: space", "url encode: hello world", true, "hello+world", false},
		{"positive: special", "url encode: a&b=c", true, "a%26b%3Dc", false},
		{"negative", "url decode: hello", false, "", false},
	})
}

func TestURLDecodeSkill(t *testing.T) {
	runSkillCases(t, "url_decode", NewURLDecodeSkill(), []skillCase{
		{"positive: plus", "url decode: hello+world", true, "hello world", false},
		{"positive: percent", "url decode: %41", true, "A", false},
		{"negative", "url encode: hello", false, "", false},
	})
}

// ---- html ----

func TestHTMLEscapeSkill(t *testing.T) {
	runSkillCases(t, "html_escape", NewHTMLEscapeSkill(), []skillCase{
		{"positive: tag", "html escape: <b>", true, "&lt;b&gt;", false},
		{"positive: ampersand", "html escape: a & b", true, "a &amp; b", false},
		{"negative", "html unescape: &lt;", false, "", false},
	})
}

func TestHTMLUnescapeSkill(t *testing.T) {
	runSkillCases(t, "html_unescape", NewHTMLUnescapeSkill(), []skillCase{
		{"positive: lt", "html unescape: &lt;b&gt;", true, "<b>", false},
		{"positive: amp", "html unescape: a &amp; b", true, "a & b", false},
		{"negative", "html escape: <", false, "", false},
	})
}

// ---- hex ----

func TestHexEncodeSkill(t *testing.T) {
	runSkillCases(t, "hex_encode", NewHexEncodeSkill(), []skillCase{
		{"positive", "hex encode: A", true, "41", false},
		{"positive: word", "hex encode: hi", true, "6869", false},
		{"negative: from hex", "from hex: 41", false, "", false},
	})
}

func TestHexDecodeSkill(t *testing.T) {
	runSkillCases(t, "hex_decode", NewHexDecodeSkill(), []skillCase{
		{"positive", "hex decode: 41", true, "A", false},
		{"positive: with 0x prefix", "hex decode: 0x4869", true, "Hi", false},
		{"negative", "hex encode: A", false, "", false},
		{"edge: invalid hex", "hex decode: xyz", true, "", true},
	})
}

// ---- unicode ----

func TestUnicodeEscapeSkill(t *testing.T) {
	skill := NewUnicodeEscapeSkill()
	ctx := context.Background()

	// "café" contains 'é' which is U+00E9 — the skill should escape it.
	if !skill.Match("unicode escape: café") {
		t.Error("expected match for 'unicode escape: café'")
	}
	out, err := skill.Execute(ctx, "unicode escape: café")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// ASCII chars pass through; only non-ASCII gets \uXXXX form.
	if !strings.Contains(out, "caf") {
		t.Errorf("expected ASCII prefix to pass through, got %q", out)
	}
	// The literal 6-character escape sequence "é" (backslash, u, 0, 0, e, 9).
	if !strings.Contains(out, "\\u00e9") {
		t.Errorf("expected literal \\u00e9 escape sequence, got %q", out)
	}
}

func TestUnicodeUnescapeSkill(t *testing.T) {
	runSkillCases(t, "unicode_unescape", NewUnicodeUnescapeSkill(), []skillCase{
		{"positive", "unicode unescape: \\u0041\\u0042", true, "AB", false},
		{"positive: A", "unescape unicode: \\u0041", true, "A", false},
		{"negative", "unicode escape: A", false, "", false},
	})
}
