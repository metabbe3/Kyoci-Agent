package builtin

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"strings"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// =====================================================================================
// Compression skills — gzip/zlib/flate compress + decompress.
//
// Each compress skill takes plain text in and returns base64-encoded compressed
// bytes (so the output is printable / pasteable). Each decompress skill takes
// base64-encoded compressed bytes and returns the original text. All three
// formats use the same DEFLATE algorithm under the hood — they differ only in
// the framing/header wrapped around the raw DEFLATE stream:
//   - gzip: 10-byte gzip header + DEFLATE + 8-byte trailer (CRC32 + size)
//   - zlib : 2-byte zlib header + DEFLATE + 4-byte Adler-32 trailer
//   - flate: raw DEFLATE, no framing
// =====================================================================================

// ---- gzip compress / decompress ----

// GzipCompressSkill compresses input text with gzip and returns base64.
type GzipCompressSkill struct{ *kyoci.BaseSkill }

// NewGzipCompressSkill creates a skill that gzip-compresses text to base64.
func NewGzipCompressSkill() *GzipCompressSkill {
	return &GzipCompressSkill{BaseSkill: kyoci.NewBaseSkill(
		"gzip_compress", "Gzip-compress text and return base64. Usage: 'gzip compress: hello world'",
		[]string{"gzip compress", "gzip encode"},
	)}
}

// Match returns true for queries that ask to gzip-compress text. We match the
// distinct phrase "gzip compress" (and the alias "gzip encode") rather than the
// bare word "compress", which would collide with zlib/flate and other skills.
func (s *GzipCompressSkill) Match(q string) bool {
	low := strings.ToLower(q)
	if strings.Contains(low, "gzip compress") || strings.Contains(low, "gzip encode") {
		// Reject queries that also mention a decompress-style verb so the
		// decompress skill wins for those.
		return !containsAny(low, "decompress", "decode", "inflate", "extract", "unzip", "gunzip")
	}
	return false
}

// Execute gzip-compresses the payload and returns base64-encoded bytes.
func (s *GzipCompressSkill) Execute(_ context.Context, q string) (string, error) {
	in := extractPayload(q)
	if in == "" {
		return "", fmt.Errorf("no text to gzip compress")
	}
	compressed, err := gzipBytes([]byte(in))
	if err != nil {
		return "", fmt.Errorf("gzip compress failed: %w", err)
	}
	return base64.StdEncoding.EncodeToString(compressed), nil
}

// GzipDecompressSkill decompresses base64-encoded gzipped bytes to text.
type GzipDecompressSkill struct{ *kyoci.BaseSkill }

// NewGzipDecompressSkill creates a skill that decompresses base64 gzipped data.
func NewGzipDecompressSkill() *GzipDecompressSkill {
	return &GzipDecompressSkill{BaseSkill: kyoci.NewBaseSkill(
		"gzip_decompress", "Decompress base64-encoded gzipped data back to text. Usage: 'gzip decompress: <base64>'",
		[]string{"gzip decompress", "gunzip"},
	)}
}

// Match returns true for queries that ask to gzip-decompress (or gunzip) text.
func (s *GzipDecompressSkill) Match(q string) bool {
	low := strings.ToLower(q)
	return strings.Contains(low, "gzip decompress") || strings.Contains(low, "gunzip") ||
		strings.Contains(low, "gzip decode") || strings.Contains(low, "gzip extract") ||
		strings.Contains(low, "gzip inflate") || strings.Contains(low, "unzip gzip")
}

// Execute base64-decodes then gzip-decompresses the payload.
func (s *GzipDecompressSkill) Execute(_ context.Context, q string) (string, error) {
	in := quoteStripped(extractPayload(q))
	if in == "" {
		return "", fmt.Errorf("no base64 to gzip decompress")
	}
	raw, err := base64.StdEncoding.DecodeString(in)
	if err != nil {
		return "", fmt.Errorf("invalid base64 input: %w", err)
	}
	plain, err := gunzipBytes(raw)
	if err != nil {
		return "", fmt.Errorf("gzip decompress failed: %w", err)
	}
	return string(plain), nil
}

// ---- zlib compress / decompress ----

// ZlibCompressSkill compresses input text with zlib and returns base64.
type ZlibCompressSkill struct{ *kyoci.BaseSkill }

// NewZlibCompressSkill creates a skill that zlib-compresses text to base64.
func NewZlibCompressSkill() *ZlibCompressSkill {
	return &ZlibCompressSkill{BaseSkill: kyoci.NewBaseSkill(
		"zlib_compress", "Zlib-compress text and return base64. Usage: 'zlib compress: hello world'",
		[]string{"zlib compress", "zlib encode"},
	)}
}

// Match returns true for queries that ask to zlib-compress text.
func (s *ZlibCompressSkill) Match(q string) bool {
	low := strings.ToLower(q)
	if strings.Contains(low, "zlib compress") || strings.Contains(low, "zlib encode") {
		return !containsAny(low, "decompress", "decode", "inflate", "extract")
	}
	return false
}

// Execute zlib-compresses the payload and returns base64-encoded bytes.
func (s *ZlibCompressSkill) Execute(_ context.Context, q string) (string, error) {
	in := extractPayload(q)
	if in == "" {
		return "", fmt.Errorf("no text to zlib compress")
	}
	compressed, err := zlibBytes([]byte(in))
	if err != nil {
		return "", fmt.Errorf("zlib compress failed: %w", err)
	}
	return base64.StdEncoding.EncodeToString(compressed), nil
}

// ZlibDecompressSkill decompresses base64-encoded zlib bytes to text.
type ZlibDecompressSkill struct{ *kyoci.BaseSkill }

// NewZlibDecompressSkill creates a skill that decompresses base64 zlib data.
func NewZlibDecompressSkill() *ZlibDecompressSkill {
	return &ZlibDecompressSkill{BaseSkill: kyoci.NewBaseSkill(
		"zlib_decompress", "Decompress base64-encoded zlib data back to text. Usage: 'zlib decompress: <base64>'",
		[]string{"zlib decompress"},
	)}
}

// Match returns true for queries that ask to zlib-decompress text.
func (s *ZlibDecompressSkill) Match(q string) bool {
	low := strings.ToLower(q)
	return strings.Contains(low, "zlib decompress") || strings.Contains(low, "zlib decode") ||
		strings.Contains(low, "zlib extract") || strings.Contains(low, "zlib inflate")
}

// Execute base64-decodes then zlib-decompresses the payload.
func (s *ZlibDecompressSkill) Execute(_ context.Context, q string) (string, error) {
	in := quoteStripped(extractPayload(q))
	if in == "" {
		return "", fmt.Errorf("no base64 to zlib decompress")
	}
	raw, err := base64.StdEncoding.DecodeString(in)
	if err != nil {
		return "", fmt.Errorf("invalid base64 input: %w", err)
	}
	plain, err := unzlibBytes(raw)
	if err != nil {
		return "", fmt.Errorf("zlib decompress failed: %w", err)
	}
	return string(plain), nil
}

// ---- flate compress / decompress ----

// FlateCompressSkill compresses input text with raw DEFLATE and returns base64.
type FlateCompressSkill struct{ *kyoci.BaseSkill }

// NewFlateCompressSkill creates a skill that DEFLATE-compresses text to base64.
func NewFlateCompressSkill() *FlateCompressSkill {
	return &FlateCompressSkill{BaseSkill: kyoci.NewBaseSkill(
		"flate_compress", "Raw DEFLATE-compress text and return base64. Usage: 'flate compress: hello' or 'deflate compress: hello'",
		[]string{"flate compress", "deflate compress"},
	)}
}

// Match returns true for queries that ask to flate/deflate-compress text.
func (s *FlateCompressSkill) Match(q string) bool {
	low := strings.ToLower(q)
	if strings.Contains(low, "flate compress") || strings.Contains(low, "deflate compress") ||
		strings.Contains(low, "flate encode") || strings.Contains(low, "deflate encode") {
		return !containsAny(low, "decompress", "decode", "inflate", "extract")
	}
	return false
}

// Execute DEFLATE-compresses the payload and returns base64-encoded bytes.
func (s *FlateCompressSkill) Execute(_ context.Context, q string) (string, error) {
	in := extractPayload(q)
	if in == "" {
		return "", fmt.Errorf("no text to flate compress")
	}
	compressed, err := flateBytes([]byte(in))
	if err != nil {
		return "", fmt.Errorf("flate compress failed: %w", err)
	}
	return base64.StdEncoding.EncodeToString(compressed), nil
}

// FlateDecompressSkill decompresses base64-encoded raw DEFLATE bytes to text.
type FlateDecompressSkill struct{ *kyoci.BaseSkill }

// NewFlateDecompressSkill creates a skill that decompresses base64 flate data.
func NewFlateDecompressSkill() *FlateDecompressSkill {
	return &FlateDecompressSkill{BaseSkill: kyoci.NewBaseSkill(
		"flate_decompress", "Decompress base64-encoded raw DEFLATE data back to text. Usage: 'flate decompress: <base64>' or 'deflate decompress: <base64>'",
		[]string{"flate decompress", "deflate decompress"},
	)}
}

// Match returns true for queries that ask to flate/deflate-decompress text.
func (s *FlateDecompressSkill) Match(q string) bool {
	low := strings.ToLower(q)
	return strings.Contains(low, "flate decompress") || strings.Contains(low, "deflate decompress") ||
		strings.Contains(low, "flate decode") || strings.Contains(low, "deflate decode") ||
		strings.Contains(low, "flate inflate") || strings.Contains(low, "deflate inflate") ||
		strings.Contains(low, "flate extract") || strings.Contains(low, "deflate extract")
}

// Execute base64-decodes then DEFLATE-decompresses the payload.
func (s *FlateDecompressSkill) Execute(_ context.Context, q string) (string, error) {
	in := quoteStripped(extractPayload(q))
	if in == "" {
		return "", fmt.Errorf("no base64 to flate decompress")
	}
	raw, err := base64.StdEncoding.DecodeString(in)
	if err != nil {
		return "", fmt.Errorf("invalid base64 input: %w", err)
	}
	plain, err := unflateBytes(raw)
	if err != nil {
		return "", fmt.Errorf("flate decompress failed: %w", err)
	}
	return string(plain), nil
}

// =====================================================================================
// Compression helpers — thin wrappers over compress/gzip, compress/zlib,
// compress/flate. Kept package-private; tests call the skills' Execute instead.
// =====================================================================================

// gzipBytes returns the gzip-compressed form of in.
func gzipBytes(in []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(in); err != nil {
		w.Close()
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// gunzipBytes returns the gzip-decompressed form of in.
func gunzipBytes(in []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(in))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// zlibBytes returns the zlib-compressed form of in.
func zlibBytes(in []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	if _, err := w.Write(in); err != nil {
		w.Close()
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// unzlibBytes returns the zlib-decompressed form of in.
func unzlibBytes(in []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(in))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// flateBytes returns the raw DEFLATE-compressed form of in.
func flateBytes(in []byte) ([]byte, error) {
	var buf bytes.Buffer
	// BestCompression for smallest output; speed isn't a concern for skill use.
	w, err := flate.NewWriter(&buf, flate.BestCompression)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(in); err != nil {
		w.Close()
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// unflateBytes returns the raw DEFLATE-decompressed form of in.
func unflateBytes(in []byte) ([]byte, error) {
	r := flate.NewReader(bytes.NewReader(in))
	defer r.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// containsAny reports whether s contains any of the given substrings. Used by
// Match() methods to reject queries that mention a decompress-style verb.
func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
