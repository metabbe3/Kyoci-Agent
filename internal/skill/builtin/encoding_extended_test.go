package builtin

import (
	"context"
	"encoding/ascii85"
	"strings"
	"testing"
)

// =====================================================================================
// Tests for the extended encoding skills (encoding_extended.go).
// Each test uses runSkillCases (defined in encoding_test.go) plus a couple of
// dedicated round-trip tests for the encode/decode pairs.
// =====================================================================================

// ---- base58 ----

func TestBase58EncodeSkill(t *testing.T) {
	runSkillCases(t, "base58_encode", NewBase58EncodeSkill(), []skillCase{
		{"positive: base58 encode", "base58 encode: hello", true, "Cn8eVZg", false},
		{"positive: encode base58", "encode base58: Hi", true, "6Wc", false},
		{"positive: just base58", "base58: hello", true, "Cn8eVZg", false},
		{"negative: decode query", "base58 decode: Cn8eVZg", false, "", false},
		{"negative: base64 collision", "base64 encode: hello", false, "", false},
		{"negative: unrelated", "sha256 of hello", false, "", false},
	})
}

func TestBase58DecodeSkill(t *testing.T) {
	runSkillCases(t, "base58_decode", NewBase58DecodeSkill(), []skillCase{
		{"positive: base58 decode", "base58 decode: Cn8eVZg", true, "hello", false},
		{"positive: decode base58", "decode base58: 6Wc", true, "Hi", false},
		{"negative: encode query", "base58 encode: hello", false, "", false},
		{"negative: base64 collision", "base64 decode: aGVsbG8=", false, "", false},
		{"edge: invalid char", "base58 decode: 0OIl", true, "", true},
	})
}

func TestBase58RoundTrip(t *testing.T) {
	// Round-trip several inputs to confirm encode/decode are inverses.
	// Use the colon-delimited form (the canonical query shape); none of
	// these inputs contain a colon.
	cases := []string{
		"hello",
		"The quick brown fox",
		"1234567890",
		"\x00\x00lead-zero-bytes", // exercises leading-zero handling
	}
	skill := NewBase58DecodeSkill()
	encSkill := NewBase58EncodeSkill()
	ctx := context.Background()
	for _, in := range cases {
		enc, err := encSkill.Execute(ctx, "base58 encode: "+in)
		if err != nil {
			t.Fatalf("encode %q: %v", in, err)
		}
		out, err := skill.Execute(ctx, "base58 decode: "+enc)
		if err != nil {
			t.Fatalf("decode %q: %v", in, err)
		}
		if out != in {
			t.Errorf("round-trip %q: got %q", in, out)
		}
	}
}

// ---- base62 ----

func TestBase62EncodeSkill(t *testing.T) {
	runSkillCases(t, "base62_encode", NewBase62EncodeSkill(), []skillCase{
		{"positive: base62 encode", "base62 encode: hello", true, "7tQLFHz", false},
		{"positive: encode base62", "encode base62: Hi", true, "4oz", false},
		{"positive: just base62", "base62: hello", true, "7tQLFHz", false},
		{"negative: decode query", "base62 decode: 7tQLFHz", false, "", false},
		{"negative: base58 collision", "base58 encode: hello", false, "", false},
	})
}

func TestBase62DecodeSkill(t *testing.T) {
	runSkillCases(t, "base62_decode", NewBase62DecodeSkill(), []skillCase{
		{"positive: base62 decode", "base62 decode: 7tQLFHz", true, "hello", false},
		{"positive: decode base62", "decode base62: 4oz", true, "Hi", false},
		{"negative: encode query", "base62 encode: hello", false, "", false},
		{"negative: base58 collision", "base58 decode: hello", false, "", false},
		{"edge: invalid char", "base62 decode: @@@", true, "", true},
	})
}

func TestBase62RoundTrip(t *testing.T) {
	cases := []string{
		"hello",
		"The quick brown fox",
		"\x00\x00lead-zero-bytes",
	}
	skill := NewBase62DecodeSkill()
	encSkill := NewBase62EncodeSkill()
	ctx := context.Background()
	for _, in := range cases {
		enc, err := encSkill.Execute(ctx, "base62 encode: "+in)
		if err != nil {
			t.Fatalf("encode %q: %v", in, err)
		}
		out, err := skill.Execute(ctx, "base62 decode: "+enc)
		if err != nil {
			t.Fatalf("decode %q: %v", in, err)
		}
		if out != in {
			t.Errorf("round-trip %q: got %q", in, out)
		}
	}
}

// ---- base85 / ascii85 ----

func TestBase85EncodeSkill(t *testing.T) {
	skill := NewBase85EncodeSkill()
	// Compute expected value from the stdlib directly to avoid hand-rolling
	// the ascii85 alphabet in the test.
	want := func(in string) string {
		dst := make([]byte, (len(in)+3)/4*5+1)
		n := ascii85.Encode(dst, []byte(in))
		return string(dst[:n])
	}
	cases := []skillCase{
		{"positive: base85 encode hi", "base85 encode: hi", true, want("hi"), false},
		{"positive: ascii85 encode hello", "ascii85 encode: hello", true, want("hello"), false},
		{"positive: encode base85", "encode base85: man", true, want("man"), false},
		{"negative: decode query", "base85 decode: abc", false, "", false},
		{"negative: base64 collision", "base64 encode: hi", false, "", false},
	}
	runSkillCases(t, "base85_encode", skill, cases)
}

func TestBase85DecodeSkill(t *testing.T) {
	skill := NewBase85DecodeSkill()
	ctx := context.Background()
	// Build a valid ascii85 input using the stdlib encoder so the decode
	// test doesn't depend on a hand-picked alphabet.
	encode := func(in string) string {
		dst := make([]byte, (len(in)+3)/4*5+1)
		n := ascii85.Encode(dst, []byte(in))
		return string(dst[:n])
	}
	encHello := encode("hello")
	encHi := encode("hi")

	t.Run("positive: base85 decode hello", func(t *testing.T) {
		if !skill.Match("base85 decode: " + encHello) {
			t.Fatalf("Match failed")
		}
		out, err := skill.Execute(ctx, "base85 decode: "+encHello)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if out != "hello" {
			t.Errorf("got %q, want hello", out)
		}
	})

	t.Run("positive: ascii85 decode hi", func(t *testing.T) {
		if !skill.Match("ascii85 decode: " + encHi) {
			t.Fatalf("Match failed")
		}
		out, err := skill.Execute(ctx, "ascii85 decode: "+encHi)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if out != "hi" {
			t.Errorf("got %q, want hi", out)
		}
	})

	t.Run("negative: encode query", func(t *testing.T) {
		if skill.Match("base85 encode: hi") {
			t.Errorf("expected no match for encode query")
		}
	})
	t.Run("negative: base64 collision", func(t *testing.T) {
		if skill.Match("base64 decode: aGVsbG8=") {
			t.Errorf("expected no match for base64 query")
		}
	})
	t.Run("edge: invalid ascii85", func(t *testing.T) {
		// '~' (126) is outside the ascii85 range '!' (33) .. 'u' (117).
		_, err := skill.Execute(ctx, "base85 decode: ~~~~~")
		if err == nil {
			t.Error("expected error on invalid ascii85")
		}
	})
}

func TestBase85RoundTrip(t *testing.T) {
	enc := NewBase85EncodeSkill()
	dec := NewBase85DecodeSkill()
	ctx := context.Background()
	cases := []string{"hello", "The quick brown fox", "12345"}
	for _, in := range cases {
		e, err := enc.Execute(ctx, "base85 encode: "+in)
		if err != nil {
			t.Fatalf("encode %q: %v", in, err)
		}
		out, err := dec.Execute(ctx, "base85 decode: "+e)
		if err != nil {
			t.Fatalf("decode %q: %v", in, err)
		}
		if out != in {
			t.Errorf("round-trip %q: got %q", in, out)
		}
	}
}

// ---- punycode ----

func TestPunycodeEncodeSkill(t *testing.T) {
	runSkillCases(t, "punycode_encode", NewPunycodeEncodeSkill(), []skillCase{
		{"positive: punycode encode", "punycode encode: example.com", true, "example.com", false},
		{"positive: encode punycode", "encode punycode: foo.bar", true, "foo.bar", false},
		{"positive: just punycode", "punycode: example.com", true, "example.com", false},
		{"negative: decode query", "punycode decode: example.com", false, "", false},
	})
}

func TestPunycodeEncodeRejectsNonASCII(t *testing.T) {
	skill := NewPunycodeEncodeSkill()
	ctx := context.Background()
	// Non-ASCII label should be rejected (simplified encoder).
	_, err := skill.Execute(ctx, "punycode encode: café.com")
	if err == nil {
		t.Error("expected error for non-ASCII label, got nil")
	}
}

func TestPunycodeDecodeSkill(t *testing.T) {
	runSkillCases(t, "punycode_decode", NewPunycodeDecodeSkill(), []skillCase{
		{"positive: punycode decode", "punycode decode: example.com", true, "example.com", false},
		{"positive: decode punycode", "decode punycode: foo.bar", true, "foo.bar", false},
		{"negative: encode query", "punycode encode: example.com", false, "", false},
	})
}

func TestPunycodeDecodeRejectsXNLabel(t *testing.T) {
	skill := NewPunycodeDecodeSkill()
	ctx := context.Background()
	// xn-- label requires the full IDNA decoder; simplified version errors.
	_, err := skill.Execute(ctx, "punycode decode: xn--caf-dma.com")
	if err == nil {
		t.Error("expected error for xn-- label, got nil")
	}
}

// ---- quoted-printable ----

func TestQuotedPrintableEncodeSkill(t *testing.T) {
	runSkillCases(t, "quoted_printable_encode", NewQuotedPrintableEncodeSkill(), []skillCase{
		{"positive: quoted printable encode", "quoted printable encode: hello", true, "hello", false},
		{"positive: quoted-printable encode", "quoted-printable encode: hi", true, "hi", false},
		{"positive: qprintable encode", "qprintable encode: hello", true, "hello", false},
		{"negative: decode query", "quoted printable decode: hello", false, "", false},
	})
}

func TestQuotedPrintableEncodeHighBytes(t *testing.T) {
	skill := NewQuotedPrintableEncodeSkill()
	ctx := context.Background()
	// é (0xC3 0xA9 in UTF-8) should become =C3=A9.
	out, err := skill.Execute(ctx, "quoted printable encode: café")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "=C3=A9") {
		t.Errorf("expected =C3=A9 in output, got %q", out)
	}
}

func TestQuotedPrintableDecodeSkill(t *testing.T) {
	runSkillCases(t, "quoted_printable_decode", NewQuotedPrintableDecodeSkill(), []skillCase{
		{"positive: quoted printable decode", "quoted printable decode: hello", true, "hello", false},
		{"positive: quoted-printable decode", "quoted-printable decode: =C3=A9", true, "é", false},
		{"positive: qprintable decode", "qprintable decode: hi", true, "hi", false},
		{"negative: encode query", "quoted printable encode: hello", false, "", false},
	})
}

func TestQuotedPrintableRoundTrip(t *testing.T) {
	enc := NewQuotedPrintableEncodeSkill()
	dec := NewQuotedPrintableDecodeSkill()
	ctx := context.Background()
	// Avoid inputs with colons (extractPayload splits on the first colon).
	// Use simple words; the quoted-printable encoder preserves ASCII bytes
	// verbatim and escapes high bytes, so the round-trip should be exact.
	cases := []string{"hello", "café", "plain text"}
	for _, in := range cases {
		e, err := enc.Execute(ctx, "quoted printable encode: "+in)
		if err != nil {
			t.Fatalf("encode %q: %v", in, err)
		}
		out, err := dec.Execute(ctx, "quoted printable decode: "+e)
		if err != nil {
			t.Fatalf("decode %q: %v", in, err)
		}
		if out != in {
			t.Errorf("round-trip %q: got %q", in, out)
		}
	}
}

// ---- URL-safe base64 ----

func TestURLSafeB64EncodeSkill(t *testing.T) {
	runSkillCases(t, "url_safe_b64_encode", NewURLSafeB64EncodeSkill(), []skillCase{
		{"positive: url safe base64 encode", "url safe base64 encode: hi", true, "aGk", false},
		{"positive: url-safe base64 encode", "url-safe base64 encode: hello", true, "aGVsbG8", false},
		{"positive: urlsafe base64 encode", "urlsafe base64 encode: hello", true, "aGVsbG8", false},
		{"positive: urlsafe_b64 encode", "urlsafe_b64 encode: hello", true, "aGVsbG8", false},
		{"positive: just urlsafe_b64", "urlsafe_b64: hello", true, "aGVsbG8", false},
		{"negative: decode query", "url safe base64 decode: aGk", false, "", false},
		{"negative: plain base64", "base64 encode: hello", false, "", false},
	})
}

func TestURLSafeB64DecodeSkill(t *testing.T) {
	runSkillCases(t, "url_safe_b64_decode", NewURLSafeB64DecodeSkill(), []skillCase{
		{"positive: url safe base64 decode", "url safe base64 decode: aGk", true, "hi", false},
		{"positive: url-safe base64 decode", "url-safe base64 decode: aGVsbG8", true, "hello", false},
		{"positive: urlsafe base64 decode", "urlsafe base64 decode: aGVsbG8", true, "hello", false},
		{"positive: urlsafe_b64 decode", "urlsafe_b64 decode: aGVsbG8", true, "hello", false},
		{"negative: encode query", "url safe base64 encode: hi", false, "", false},
		{"negative: plain base64", "base64 decode: aGVsbG8=", false, "", false},
		{"edge: invalid", "url safe base64 decode: @@@", true, "", true},
	})
}

func TestURLSafeB64RoundTrip(t *testing.T) {
	enc := NewURLSafeB64EncodeSkill()
	dec := NewURLSafeB64DecodeSkill()
	ctx := context.Background()
	cases := []string{"hello", "world", "The quick brown fox"}
	for _, in := range cases {
		e, err := enc.Execute(ctx, "url safe base64 encode: "+in)
		if err != nil {
			t.Fatalf("encode %q: %v", in, err)
		}
		out, err := dec.Execute(ctx, "url safe base64 decode: "+e)
		if err != nil {
			t.Fatalf("decode %q: %v", in, err)
		}
		if out != in {
			t.Errorf("round-trip %q: got %q", in, out)
		}
	}
}

// ---- cross-collision guards ----

// TestEncodingExtendedNoFalsePositives confirms that the new skills don't
// shadow the existing base64/base32 skills in encoding.go.
func TestEncodingExtendedNoFalsePositives(t *testing.T) {
	b58e := NewBase58EncodeSkill()
	b62e := NewBase62EncodeSkill()
	b85e := NewBase85EncodeSkill()
	pun := NewPunycodeEncodeSkill()
	qp := NewQuotedPrintableEncodeSkill()
	urlb := NewURLSafeB64EncodeSkill()
	// Each of these is clearly a different skill's territory.
	type otherSkillCase struct{ name, query string }
	others := []otherSkillCase{
		{"base64", "base64 encode: hello"},
		{"base32", "base32 encode: hello"},
		{"url", "url encode: hello"},
		{"hex", "hex encode: hello"},
		{"sha256", "sha256 of hello"},
		{"slugify", "slugify: Hello World"},
		{"uuid", "generate uuid v4"},
	}
	for _, tc := range others {
		if b58e.Match(tc.query) {
			t.Errorf("base58_encode matched %q", tc.query)
		}
		if b62e.Match(tc.query) {
			t.Errorf("base62_encode matched %q", tc.query)
		}
		if b85e.Match(tc.query) {
			t.Errorf("base85_encode matched %q", tc.query)
		}
		if pun.Match(tc.query) {
			t.Errorf("punycode_encode matched %q", tc.query)
		}
		if qp.Match(tc.query) {
			t.Errorf("quoted_printable_encode matched %q", tc.query)
		}
		if urlb.Match(tc.query) {
			t.Errorf("url_safe_b64_encode matched %q", tc.query)
		}
	}
}
