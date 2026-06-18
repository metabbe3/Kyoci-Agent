package builtin

import (
	"context"
	"strings"
	"testing"
)

// =====================================================================================
// Compression skill tests — gzip/zlib/flate compress + decompress (6 skills).
//
// Tests use the shared runSkillCases driver from encoding_test.go. Round-trip
// cases (compress then decompress returns the original) are covered separately
// below because they need the output of one Execute as input to another.
// =====================================================================================

// ---- gzip compress ----

func TestGzipCompressSkill(t *testing.T) {
	runSkillCases(t, "gzip_compress", NewGzipCompressSkill(), []skillCase{
		{"positive: gzip compress", "gzip compress: hello world", true, "", false},
		{"positive: gzip encode", "gzip encode: hello world", true, "", false},
		{"negative: decompress", "gzip decompress: H4sIAAAAAAAA", false, "", false},
		{"negative: zlib", "zlib compress: hi", false, "", false},
		{"negative: bare compress", "compress this text", false, "", false},
		{"negative: unrelated", "hash of hello", false, "", false},
	})
	// Verify output is non-empty base64 (no spaces, looks like base64).
	out, err := NewGzipCompressSkill().Execute(context.Background(), "gzip compress: hello world")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out == "" || strings.ContainsAny(out, " \t\n") {
		t.Errorf("expected non-empty base64 output, got %q", out)
	}
}

// ---- gzip decompress ----

func TestGzipDecompressSkill(t *testing.T) {
	// Pre-compute a known-good gzip+base64 string so the match-only cases can
	// reference it without depending on the compress skill itself.
	ctx := context.Background()
	known, err := NewGzipCompressSkill().Execute(ctx, "gzip compress: hello world")
	if err != nil {
		t.Fatalf("setup: gzip compress failed: %v", err)
	}

	runSkillCases(t, "gzip_decompress", NewGzipDecompressSkill(), []skillCase{
		{"positive: gzip decompress", "gzip decompress: " + known, true, "hello world", false},
		{"positive: gunzip alias", "gunzip: " + known, true, "hello world", false},
		{"positive: gzip decode", "gzip decode: " + known, true, "hello world", false},
		{"negative: compress query", "gzip compress: hello", false, "", false},
		{"negative: unrelated", "hash of hello", false, "", false},
		{"edge: invalid base64", "gzip decompress: @@@notbase64@@@", true, "", true},
	})
}

// ---- zlib compress ----

func TestZlibCompressSkill(t *testing.T) {
	runSkillCases(t, "zlib_compress", NewZlibCompressSkill(), []skillCase{
		{"positive: zlib compress", "zlib compress: hello world", true, "", false},
		{"positive: zlib encode", "zlib encode: hello world", true, "", false},
		{"negative: decompress", "zlib decompress: eJz...", false, "", false},
		{"negative: gzip", "gzip compress: hi", false, "", false},
		{"negative: bare compress", "compress this text", false, "", false},
	})
	out, err := NewZlibCompressSkill().Execute(context.Background(), "zlib compress: hello world")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out == "" {
		t.Errorf("expected non-empty base64 output, got %q", out)
	}
}

// ---- zlib decompress ----

func TestZlibDecompressSkill(t *testing.T) {
	ctx := context.Background()
	known, err := NewZlibCompressSkill().Execute(ctx, "zlib compress: hello world")
	if err != nil {
		t.Fatalf("setup: zlib compress failed: %v", err)
	}

	runSkillCases(t, "zlib_decompress", NewZlibDecompressSkill(), []skillCase{
		{"positive: zlib decompress", "zlib decompress: " + known, true, "hello world", false},
		{"positive: zlib decode", "zlib decode: " + known, true, "hello world", false},
		{"negative: compress query", "zlib compress: hello", false, "", false},
		{"negative: unrelated", "hash of hello", false, "", false},
		{"edge: invalid base64", "zlib decompress: @@@notbase64@@@", true, "", true},
	})
}

// ---- flate compress ----

func TestFlateCompressSkill(t *testing.T) {
	skill := NewFlateCompressSkill()
	runSkillCases(t, "flate_compress", skill, []skillCase{
		{"positive: flate compress", "flate compress: hello world", true, "", false},
		{"positive: deflate compress", "deflate compress: hello world", true, "", false},
		{"positive: flate encode", "flate encode: hello world", true, "", false},
		{"negative: decompress", "flate decompress: eJz...", false, "", false},
		{"negative: gzip", "gzip compress: hi", false, "", false},
		{"negative: bare compress", "compress this text", false, "", false},
	})
	out, err := skill.Execute(context.Background(), "flate compress: hello world")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out == "" {
		t.Errorf("expected non-empty base64 output, got %q", out)
	}
}

// ---- flate decompress ----

func TestFlateDecompressSkill(t *testing.T) {
	ctx := context.Background()
	known, err := NewFlateCompressSkill().Execute(ctx, "flate compress: hello world")
	if err != nil {
		t.Fatalf("setup: flate compress failed: %v", err)
	}

	runSkillCases(t, "flate_decompress", NewFlateDecompressSkill(), []skillCase{
		{"positive: flate decompress", "flate decompress: " + known, true, "hello world", false},
		{"positive: deflate decompress", "deflate decompress: " + known, true, "hello world", false},
		{"positive: deflate decode", "deflate decode: " + known, true, "hello world", false},
		{"negative: compress query", "flate compress: hello", false, "", false},
		{"negative: unrelated", "hash of hello", false, "", false},
		{"edge: invalid base64", "flate decompress: @@@notbase64@@@", true, "", true},
	})
}

// ---- round-trip tests ----

// TestGzipRoundTrip verifies that gzip_decompress(gzip_compress(x)) == x.
// This is the core invariant of any symmetric codec.
func TestGzipRoundTrip(t *testing.T) {
	ctx := context.Background()
	// Note: inputs must not begin or end with whitespace — extractPayload
	// trims surrounding whitespace, which would break the round-trip equality.
	cases := []string{
		"hello",
		"hello world",
		"The quick brown fox jumps over the lazy dog.",
		strings.TrimSpace(strings.Repeat("lorem ipsum dolor sit amet, ", 50)), // compressible body
		"unicode: café naïve résumé — 日本語 — 🎉",
	}
	compress := NewGzipCompressSkill()
	decompress := NewGzipDecompressSkill()
	for i, in := range cases {
		compressed, err := compress.Execute(ctx, "gzip compress: "+in)
		if err != nil {
			t.Errorf("case %d: compress(%q): %v", i, in, err)
			continue
		}
		got, err := decompress.Execute(ctx, "gzip decompress: "+compressed)
		if err != nil {
			t.Errorf("case %d: decompress failed: %v", i, err)
			continue
		}
		if got != in {
			t.Errorf("case %d: round-trip mismatch\n  in : %q\n  out: %q", i, in, got)
		}
	}
}

// TestZlibRoundTrip verifies that zlib_decompress(zlib_compress(x)) == x.
func TestZlibRoundTrip(t *testing.T) {
	ctx := context.Background()
	cases := []string{
		"hello",
		"hello world",
		"The quick brown fox jumps over the lazy dog.",
		strings.TrimSpace(strings.Repeat("lorem ipsum dolor sit amet, ", 50)),
		"unicode: café naïve résumé — 日本語 — 🎉",
	}
	compress := NewZlibCompressSkill()
	decompress := NewZlibDecompressSkill()
	for i, in := range cases {
		compressed, err := compress.Execute(ctx, "zlib compress: "+in)
		if err != nil {
			t.Errorf("case %d: compress(%q): %v", i, in, err)
			continue
		}
		got, err := decompress.Execute(ctx, "zlib decompress: "+compressed)
		if err != nil {
			t.Errorf("case %d: decompress failed: %v", i, err)
			continue
		}
		if got != in {
			t.Errorf("case %d: round-trip mismatch\n  in : %q\n  out: %q", i, in, got)
		}
	}
}

// TestFlateRoundTrip verifies that flate_decompress(flate_compress(x)) == x.
func TestFlateRoundTrip(t *testing.T) {
	ctx := context.Background()
	cases := []string{
		"hello",
		"hello world",
		"The quick brown fox jumps over the lazy dog.",
		strings.TrimSpace(strings.Repeat("lorem ipsum dolor sit amet, ", 50)),
		"unicode: café naïve résumé — 日本語 — 🎉",
	}
	compress := NewFlateCompressSkill()
	decompress := NewFlateDecompressSkill()
	for i, in := range cases {
		compressed, err := compress.Execute(ctx, "flate compress: "+in)
		if err != nil {
			t.Errorf("case %d: compress(%q): %v", i, in, err)
			continue
		}
		got, err := decompress.Execute(ctx, "flate decompress: "+compressed)
		if err != nil {
			t.Errorf("case %d: decompress failed: %v", i, err)
			continue
		}
		if got != in {
			t.Errorf("case %d: round-trip mismatch\n  in : %q\n  out: %q", i, in, got)
		}
	}
}

// ---- cross-format sanity (gzip output must NOT decompress with zlib) ----

// TestCompressionFormatsAreDistinct guards against the mistake of wiring the
// wrong decompressor behind a skill. A gzip stream should fail under zlib, etc.
func TestCompressionFormatsAreDistinct(t *testing.T) {
	ctx := context.Background()
	gz, _ := NewGzipCompressSkill().Execute(ctx, "gzip compress: hello")
	zl, _ := NewZlibCompressSkill().Execute(ctx, "zlib compress: hello")
	fl, _ := NewFlateCompressSkill().Execute(ctx, "flate compress: hello")

	// gzip output should not decompress with zlib or flate.
	if _, err := NewZlibDecompressSkill().Execute(ctx, "zlib decompress: "+gz); err == nil {
		t.Error("gzip stream unexpectedly decoded by zlib decompressor")
	}
	if _, err := NewFlateDecompressSkill().Execute(ctx, "flate decompress: "+gz); err == nil {
		t.Error("gzip stream unexpectedly decoded by flate decompressor")
	}
	// zlib output should not decompress with flate.
	if _, err := NewFlateDecompressSkill().Execute(ctx, "flate decompress: "+zl); err == nil {
		t.Error("zlib stream unexpectedly decoded by flate decompressor")
	}
	// flate output should not decompress with gzip or zlib.
	if _, err := NewGzipDecompressSkill().Execute(ctx, "gzip decompress: "+fl); err == nil {
		t.Error("flate stream unexpectedly decoded by gzip decompressor")
	}
	if _, err := NewZlibDecompressSkill().Execute(ctx, "zlib decompress: "+fl); err == nil {
		t.Error("flate stream unexpectedly decoded by zlib decompressor")
	}
}
