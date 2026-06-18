package builtin

import (
	"bytes"
	"context"
	"encoding/ascii85"
	"encoding/base64"
	"fmt"
	"math/big"
	"mime/quotedprintable"
	"strings"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// =====================================================================================
// Extended encoding skills — base58, base62, base85 (ascii85), punycode,
// quoted-printable, URL-safe base64.
//
// All pure-Go, stdlib only, no LLM/network/side effects. Each encode/decode
// pair detects intent from the query verb ("encode" vs "decode") to keep the
// Match() keywords unambiguous and avoid collision with the base64/base32
// skills in encoding.go.
// =====================================================================================

// Bitcoin-style base58 alphabet — no 0, O, I, l.
const base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

// Base62 alphabet: 0-9 A-Z a-z.
const base62Alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// ---- helpers shared by base58 / base62 ----

// encodeBigBase encodes a byte slice as a big-endian big-integer in the given
// alphabet, prepending one alphabet[0] for every leading zero byte (per the
// Bitcoin/Satoshi base58 convention). Returns "" for empty input.
func encodeBigBase(input []byte, alphabet string) string {
	if len(input) == 0 {
		return ""
	}
	// Count leading zero bytes — each becomes alphabet[0].
	zeros := 0
	for zeros < len(input) && input[zeros] == 0 {
		zeros++
	}
	// Convert to a big integer so we can repeatedly mod by the base.
	num := new(big.Int).SetBytes(input)
	base := big.NewInt(int64(len(alphabet)))
	mod := new(big.Int)
	// We build digits in reverse, then prepend the zero markers.
	var digits []byte
	for num.Sign() > 0 {
		num.DivMod(num, base, mod)
		digits = append(digits, alphabet[mod.Int64()])
	}
	// Prepend zero markers (alphabet[0]) for each leading zero byte.
	out := make([]byte, 0, zeros+len(digits))
	for i := 0; i < zeros; i++ {
		out = append(out, alphabet[0])
	}
	// digits are little-endian; reverse them.
	for i := len(digits) - 1; i >= 0; i-- {
		out = append(out, digits[i])
	}
	return string(out)
}

// decodeBigBase decodes s using the given alphabet into the original bytes,
// including the leading-zero-byte reconstruction. Returns an error if any
// character is outside the alphabet.
func decodeBigBase(s, alphabet string) ([]byte, error) {
	if s == "" {
		return []byte{}, nil
	}
	// Build a reverse lookup table from rune -> index.
	dec := make([]int, 256)
	for i := range dec {
		dec[i] = -1
	}
	for i := 0; i < len(alphabet); i++ {
		dec[alphabet[i]] = i
	}
	// Count leading zero markers (alphabet[0]).
	zeros := 0
	for zeros < len(s) && s[zeros] == alphabet[0] {
		zeros++
	}
	num := new(big.Int)
	base := big.NewInt(int64(len(alphabet)))
	mul := new(big.Int)
	for i := 0; i < len(s); i++ {
		c := s[i]
		idx := dec[c]
		if idx < 0 {
			return nil, fmt.Errorf("invalid character %q in input", string(rune(c)))
		}
		num.Mul(num, base)
		mul.SetInt64(int64(idx))
		num.Add(num, mul)
	}
	// big.Int.Bytes() drops leading zero bytes, so we reconstruct them.
	encoded := num.Bytes()
	out := make([]byte, 0, zeros+len(encoded))
	for i := 0; i < zeros; i++ {
		out = append(out, 0)
	}
	out = append(out, encoded...)
	return out, nil
}

// =====================================================================================
// ---- base58 ----
// =====================================================================================

type Base58EncodeSkill struct{ *kyoci.BaseSkill }

func NewBase58EncodeSkill() *Base58EncodeSkill {
	return &Base58EncodeSkill{BaseSkill: kyoci.NewBaseSkill(
		"base58_encode", "Encode text/bytes to Bitcoin-style base58",
		[]string{"base58 encode", "encode base58", "base58"},
	)}
}
func (s *Base58EncodeSkill) Match(q string) bool {
	l := strings.ToLower(q)
	if !strings.Contains(l, "base58") {
		return false
	}
	// Match if there's no "decode" verb (or an explicit "encode" verb).
	return !strings.Contains(l, "decode") || strings.Contains(l, "encode")
}
func (s *Base58EncodeSkill) Execute(_ context.Context, q string) (string, error) {
	t := quoteStripped(extractPayload(q))
	if t == "" {
		return "", fmt.Errorf("no text to encode")
	}
	return encodeBigBase([]byte(t), base58Alphabet), nil
}

type Base58DecodeSkill struct{ *kyoci.BaseSkill }

func NewBase58DecodeSkill() *Base58DecodeSkill {
	return &Base58DecodeSkill{BaseSkill: kyoci.NewBaseSkill(
		"base58_decode", "Decode Bitcoin-style base58 to text",
		[]string{"base58 decode", "decode base58"},
	)}
}
func (s *Base58DecodeSkill) Match(q string) bool {
	l := strings.ToLower(q)
	if !strings.Contains(l, "base58") {
		return false
	}
	return strings.Contains(l, "decode")
}
func (s *Base58DecodeSkill) Execute(_ context.Context, q string) (string, error) {
	t := strings.TrimSpace(quoteStripped(extractPayload(q)))
	if t == "" {
		return "", fmt.Errorf("no base58 to decode")
	}
	out, err := decodeBigBase(t, base58Alphabet)
	if err != nil {
		return "", fmt.Errorf("invalid base58: %w", err)
	}
	return string(out), nil
}

// =====================================================================================
// ---- base62 ----
// =====================================================================================

type Base62EncodeSkill struct{ *kyoci.BaseSkill }

func NewBase62EncodeSkill() *Base62EncodeSkill {
	return &Base62EncodeSkill{BaseSkill: kyoci.NewBaseSkill(
		"base62_encode", "Encode text/bytes to base62 (0-9 A-Z a-z)",
		[]string{"base62 encode", "encode base62", "base62"},
	)}
}
func (s *Base62EncodeSkill) Match(q string) bool {
	l := strings.ToLower(q)
	if !strings.Contains(l, "base62") {
		return false
	}
	return !strings.Contains(l, "decode") || strings.Contains(l, "encode")
}
func (s *Base62EncodeSkill) Execute(_ context.Context, q string) (string, error) {
	t := quoteStripped(extractPayload(q))
	if t == "" {
		return "", fmt.Errorf("no text to encode")
	}
	return encodeBigBase([]byte(t), base62Alphabet), nil
}

type Base62DecodeSkill struct{ *kyoci.BaseSkill }

func NewBase62DecodeSkill() *Base62DecodeSkill {
	return &Base62DecodeSkill{BaseSkill: kyoci.NewBaseSkill(
		"base62_decode", "Decode base62 to text",
		[]string{"base62 decode", "decode base62"},
	)}
}
func (s *Base62DecodeSkill) Match(q string) bool {
	l := strings.ToLower(q)
	if !strings.Contains(l, "base62") {
		return false
	}
	return strings.Contains(l, "decode")
}
func (s *Base62DecodeSkill) Execute(_ context.Context, q string) (string, error) {
	t := strings.TrimSpace(quoteStripped(extractPayload(q)))
	if t == "" {
		return "", fmt.Errorf("no base62 to decode")
	}
	out, err := decodeBigBase(t, base62Alphabet)
	if err != nil {
		return "", fmt.Errorf("invalid base62: %w", err)
	}
	return string(out), nil
}

// =====================================================================================
// ---- base85 / ascii85 ----
// =====================================================================================

type Base85EncodeSkill struct{ *kyoci.BaseSkill }

func NewBase85EncodeSkill() *Base85EncodeSkill {
	return &Base85EncodeSkill{BaseSkill: kyoci.NewBaseSkill(
		"base85_encode", "Encode text to Ascii85 (btoa-style)",
		[]string{"base85 encode", "encode base85", "ascii85 encode", "base85"},
	)}
}
func (s *Base85EncodeSkill) Match(q string) bool {
	l := strings.ToLower(q)
	if !strings.Contains(l, "base85") && !strings.Contains(l, "ascii85") {
		return false
	}
	return !strings.Contains(l, "decode") || strings.Contains(l, "encode")
}
func (s *Base85EncodeSkill) Execute(_ context.Context, q string) (string, error) {
	t := quoteStripped(extractPayload(q))
	if t == "" {
		return "", fmt.Errorf("no text to encode")
	}
	src := []byte(t)
	// Ascii85 max expansion is src/4*5 + 1; allocate enough room.
	dst := make([]byte, (len(src)+3)/4*5+1)
	n := ascii85.Encode(dst, src)
	return string(dst[:n]), nil
}

type Base85DecodeSkill struct{ *kyoci.BaseSkill }

func NewBase85DecodeSkill() *Base85DecodeSkill {
	return &Base85DecodeSkill{BaseSkill: kyoci.NewBaseSkill(
		"base85_decode", "Decode Ascii85 to text",
		[]string{"base85 decode", "decode base85", "ascii85 decode"},
	)}
}
func (s *Base85DecodeSkill) Match(q string) bool {
	l := strings.ToLower(q)
	if !strings.Contains(l, "base85") && !strings.Contains(l, "ascii85") {
		return false
	}
	return strings.Contains(l, "decode")
}
func (s *Base85DecodeSkill) Execute(_ context.Context, q string) (string, error) {
	t := strings.TrimSpace(quoteStripped(extractPayload(q)))
	if t == "" {
		return "", fmt.Errorf("no base85 to decode")
	}
	// ascii85.Decode writes up to (len(src)/5+1)*4 bytes; allocate generously.
	src := []byte(t)
	dst := make([]byte, (len(src)/5+1)*4)
	n, _, err := ascii85.Decode(dst, src, true)
	if err != nil {
		return "", fmt.Errorf("invalid ascii85: %w", err)
	}
	return string(dst[:n]), nil
}

// =====================================================================================
// ---- punycode (ASCII passthrough) ----
//
// The full IDNA/punycode algorithm lives in golang.org/x/net/idna, which is
// available as an indirect dependency but not imported by this package. To
// keep the skills stdlib-only (no go.mod edits) we implement a simplified
// version: ASCII-only labels are returned unchanged; any label containing
// non-ASCII characters is flagged as unsupported. This covers the common
// round-trip case (ASCII domain names) without pulling in a new import.
// =====================================================================================

type PunycodeEncodeSkill struct{ *kyoci.BaseSkill }

func NewPunycodeEncodeSkill() *PunycodeEncodeSkill {
	return &PunycodeEncodeSkill{BaseSkill: kyoci.NewBaseSkill(
		"punycode_encode", "Encode a label/domain to punycode (ASCII passthrough)",
		[]string{"punycode encode", "encode punycode", "punycode"},
	)}
}
func (s *PunycodeEncodeSkill) Match(q string) bool {
	l := strings.ToLower(q)
	if !strings.Contains(l, "punycode") {
		return false
	}
	return !strings.Contains(l, "decode") || strings.Contains(l, "encode")
}
func (s *PunycodeEncodeSkill) Execute(_ context.Context, q string) (string, error) {
	t := strings.TrimSpace(quoteStripped(extractPayload(q)))
	if t == "" {
		return "", fmt.Errorf("no text to punycode-encode")
	}
	// Walk labels split on '.'; reject any non-ASCII label.
	var out []string
	for _, label := range strings.Split(t, ".") {
		for _, r := range label {
			if r > 127 {
				return "", fmt.Errorf("non-ASCII label %q not supported by the simplified punycode encoder (requires golang.org/x/net/idna)", label)
			}
		}
		out = append(out, label)
	}
	encoded := strings.Join(out, ".")
	// Note the limitation in the output so callers know non-ASCII was a no-op.
	return encoded + "\n# note: ASCII-only punycode encoder; non-ASCII labels would be rejected", nil
}

type PunycodeDecodeSkill struct{ *kyoci.BaseSkill }

func NewPunycodeDecodeSkill() *PunycodeDecodeSkill {
	return &PunycodeDecodeSkill{BaseSkill: kyoci.NewBaseSkill(
		"punycode_decode", "Decode a punycode label/domain (xn--... or ASCII passthrough)",
		[]string{"punycode decode", "decode punycode"},
	)}
}
func (s *PunycodeDecodeSkill) Match(q string) bool {
	l := strings.ToLower(q)
	if !strings.Contains(l, "punycode") {
		return false
	}
	return strings.Contains(l, "decode")
}
func (s *PunycodeDecodeSkill) Execute(_ context.Context, q string) (string, error) {
	t := strings.TrimSpace(quoteStripped(extractPayload(q)))
	if t == "" {
		return "", fmt.Errorf("no punycode to decode")
	}
	// ASCII passthrough; xn-- labels are flagged since the full decoder
	// lives in golang.org/x/net/idna.
	var out []string
	for _, label := range strings.Split(t, ".") {
		for _, r := range label {
			if r > 127 {
				return "", fmt.Errorf("non-ASCII label %q not supported by the simplified punycode decoder", label)
			}
		}
		if strings.HasPrefix(strings.ToLower(label), "xn--") {
			return "", fmt.Errorf("xn-- label %q requires the full IDNA decoder (golang.org/x/net/idna); not supported by the simplified punycode decoder", label)
		}
		out = append(out, label)
	}
	return strings.Join(out, "."), nil
}

// =====================================================================================
// ---- quoted-printable ----
// =====================================================================================

type QuotedPrintableEncodeSkill struct{ *kyoci.BaseSkill }

func NewQuotedPrintableEncodeSkill() *QuotedPrintableEncodeSkill {
	return &QuotedPrintableEncodeSkill{BaseSkill: kyoci.NewBaseSkill(
		"quoted_printable_encode", "Encode text to quoted-printable (RFC 2045)",
		[]string{"quoted printable encode", "quoted-printable encode", "qprintable encode", "quoted printable"},
	)}
}
func (s *QuotedPrintableEncodeSkill) Match(q string) bool {
	l := strings.ToLower(q)
	if !isQuotedPrintableQuery(l) {
		return false
	}
	return !strings.Contains(l, "decode") || strings.Contains(l, "encode")
}
func (s *QuotedPrintableEncodeSkill) Execute(_ context.Context, q string) (string, error) {
	t := quoteStripped(extractPayload(q))
	if t == "" {
		return "", fmt.Errorf("no text to encode")
	}
	var buf bytes.Buffer
	w := quotedprintable.NewWriter(&buf)
	if _, err := w.Write([]byte(t)); err != nil {
		return "", fmt.Errorf("quoted-printable encode failed: %w", err)
	}
	if err := w.Close(); err != nil {
		return "", fmt.Errorf("quoted-printable close failed: %w", err)
	}
	return buf.String(), nil
}

type QuotedPrintableDecodeSkill struct{ *kyoci.BaseSkill }

func NewQuotedPrintableDecodeSkill() *QuotedPrintableDecodeSkill {
	return &QuotedPrintableDecodeSkill{BaseSkill: kyoci.NewBaseSkill(
		"quoted_printable_decode", "Decode quoted-printable text (RFC 2045)",
		[]string{"quoted printable decode", "quoted-printable decode", "qprintable decode"},
	)}
}
func (s *QuotedPrintableDecodeSkill) Match(q string) bool {
	l := strings.ToLower(q)
	if !isQuotedPrintableQuery(l) {
		return false
	}
	return strings.Contains(l, "decode")
}
func (s *QuotedPrintableDecodeSkill) Execute(_ context.Context, q string) (string, error) {
	t := quoteStripped(extractPayload(q))
	if t == "" {
		return "", fmt.Errorf("no text to decode")
	}
	r := quotedprintable.NewReader(strings.NewReader(t))
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		return "", fmt.Errorf("quoted-printable decode failed: %w", err)
	}
	return buf.String(), nil
}

// isQuotedPrintableQuery returns true if the (lowercased) query mentions any of
// the quoted-printable trigger phrases.
func isQuotedPrintableQuery(l string) bool {
	return strings.Contains(l, "quoted printable") ||
		strings.Contains(l, "quoted-printable") ||
		strings.Contains(l, "qprintable")
}

// =====================================================================================
// ---- URL-safe base64 ----
// =====================================================================================

type URLSafeB64EncodeSkill struct{ *kyoci.BaseSkill }

func NewURLSafeB64EncodeSkill() *URLSafeB64EncodeSkill {
	return &URLSafeB64EncodeSkill{BaseSkill: kyoci.NewBaseSkill(
		"url_safe_b64_encode", "Encode text to URL-safe base64 (RFC 4648, no padding)",
		[]string{"url safe base64 encode", "url-safe base64 encode", "urlsafe base64 encode", "urlsafe_b64 encode", "url safe b64", "urlsafe base64"},
	)}
}
func (s *URLSafeB64EncodeSkill) Match(q string) bool {
	l := strings.ToLower(q)
	if !isURLSafeB64Query(l) {
		return false
	}
	return !strings.Contains(l, "decode") || strings.Contains(l, "encode")
}
func (s *URLSafeB64EncodeSkill) Execute(_ context.Context, q string) (string, error) {
	t := quoteStripped(extractPayload(q))
	if t == "" {
		return "", fmt.Errorf("no text to encode")
	}
	return base64.RawURLEncoding.EncodeToString([]byte(t)), nil
}

type URLSafeB64DecodeSkill struct{ *kyoci.BaseSkill }

func NewURLSafeB64DecodeSkill() *URLSafeB64DecodeSkill {
	return &URLSafeB64DecodeSkill{BaseSkill: kyoci.NewBaseSkill(
		"url_safe_b64_decode", "Decode URL-safe base64 to text",
		[]string{"url safe base64 decode", "url-safe base64 decode", "urlsafe base64 decode", "urlsafe_b64 decode"},
	)}
}
func (s *URLSafeB64DecodeSkill) Match(q string) bool {
	l := strings.ToLower(q)
	if !isURLSafeB64Query(l) {
		return false
	}
	return strings.Contains(l, "decode")
}
func (s *URLSafeB64DecodeSkill) Execute(_ context.Context, q string) (string, error) {
	t := strings.TrimSpace(quoteStripped(extractPayload(q)))
	if t == "" {
		return "", fmt.Errorf("no base64 to decode")
	}
	out, err := base64.RawURLEncoding.DecodeString(t)
	if err != nil {
		// Try padded URL-safe encoding before giving up.
		out, err = base64.URLEncoding.DecodeString(t)
		if err != nil {
			return "", fmt.Errorf("invalid URL-safe base64: %w", err)
		}
	}
	return string(out), nil
}

// isURLSafeB64Query returns true if the (lowercased) query mentions any of the
// URL-safe base64 trigger phrases.
func isURLSafeB64Query(l string) bool {
	return strings.Contains(l, "url safe base64") ||
		strings.Contains(l, "url-safe base64") ||
		strings.Contains(l, "urlsafe base64") ||
		strings.Contains(l, "urlsafe_b64")
}
