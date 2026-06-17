package builtin

import (
	"context"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"html"
	"net/url"
	"strconv"
	"strings"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// =====================================================================================
// Encoding skills — base64, base32, url, html, hex, unicode.
//
// Each skill encodes OR decodes depending on the query verb. Match() requires
// a specific encoding name + verb to avoid false positives on generic text.
// =====================================================================================

// ---- base64 ----

type Base64EncodeSkill struct{ *kyoci.BaseSkill }

func NewBase64EncodeSkill() *Base64EncodeSkill {
	return &Base64EncodeSkill{BaseSkill: kyoci.NewBaseSkill(
		"base64_encode", "Encode text to base64",
		[]string{"base64 encode", "encode base64", "b64 encode", "encode b64"},
	)}
}
func (s *Base64EncodeSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "base64 encode") || strings.Contains(q, "encode base64") ||
		strings.Contains(q, "b64 encode") || strings.Contains(q, "encode b64") ||
		strings.Contains(q, "to base64") || strings.Contains(q, "base64-encode")
}
func (s *Base64EncodeSkill) Execute(_ context.Context, q string) (string, error) {
	t := quoteStripped(extractPayload(q))
	if t == "" {
		return "", fmt.Errorf("no text to encode")
	}
	return base64.StdEncoding.EncodeToString([]byte(t)), nil
}

type Base64DecodeSkill struct{ *kyoci.BaseSkill }

func NewBase64DecodeSkill() *Base64DecodeSkill {
	return &Base64DecodeSkill{BaseSkill: kyoci.NewBaseSkill(
		"base64_decode", "Decode base64 to text",
		[]string{"base64 decode", "decode base64", "b64 decode", "decode b64"},
	)}
}
func (s *Base64DecodeSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "base64 decode") || strings.Contains(q, "decode base64") ||
		strings.Contains(q, "b64 decode") || strings.Contains(q, "decode b64") ||
		strings.Contains(q, "from base64")
}
func (s *Base64DecodeSkill) Execute(_ context.Context, q string) (string, error) {
	t := quoteStripped(extractPayload(q))
	if t == "" {
		return "", fmt.Errorf("no text to decode")
	}
	out, err := base64.StdEncoding.DecodeString(strings.TrimSpace(t))
	if err != nil {
		// Try URL-safe encoding before giving up.
		out, err = base64.URLEncoding.DecodeString(strings.TrimSpace(t))
		if err != nil {
			return "", fmt.Errorf("invalid base64: %w", err)
		}
	}
	return string(out), nil
}

// ---- base32 ----

type Base32EncodeSkill struct{ *kyoci.BaseSkill }

func NewBase32EncodeSkill() *Base32EncodeSkill {
	return &Base32EncodeSkill{BaseSkill: kyoci.NewBaseSkill(
		"base32_encode", "Encode text to base32",
		[]string{"base32 encode", "encode base32"},
	)}
}
func (s *Base32EncodeSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "base32 encode") || strings.Contains(q, "encode base32") ||
		strings.Contains(q, "to base32")
}
func (s *Base32EncodeSkill) Execute(_ context.Context, q string) (string, error) {
	t := quoteStripped(extractPayload(q))
	if t == "" {
		return "", fmt.Errorf("no text to encode")
	}
	return base32.StdEncoding.EncodeToString([]byte(t)), nil
}

type Base32DecodeSkill struct{ *kyoci.BaseSkill }

func NewBase32DecodeSkill() *Base32DecodeSkill {
	return &Base32DecodeSkill{BaseSkill: kyoci.NewBaseSkill(
		"base32_decode", "Decode base32 to text",
		[]string{"base32 decode", "decode base32"},
	)}
}
func (s *Base32DecodeSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "base32 decode") || strings.Contains(q, "decode base32") ||
		strings.Contains(q, "from base32")
}
func (s *Base32DecodeSkill) Execute(_ context.Context, q string) (string, error) {
	t := quoteStripped(extractPayload(q))
	if t == "" {
		return "", fmt.Errorf("no text to decode")
	}
	out, err := base32.StdEncoding.DecodeString(strings.TrimSpace(t))
	if err != nil {
		return "", fmt.Errorf("invalid base32: %w", err)
	}
	return string(out), nil
}

// ---- url ----

type URLEncodeSkill struct{ *kyoci.BaseSkill }

func NewURLEncodeSkill() *URLEncodeSkill {
	return &URLEncodeSkill{BaseSkill: kyoci.NewBaseSkill(
		"url_encode", "URL-encode text (percent-encoding)",
		[]string{"url encode", "urlencode", "percent encode", "encode url"},
	)}
}
func (s *URLEncodeSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "url encode") || strings.Contains(q, "url-encode") ||
		strings.Contains(q, "urlencode") || strings.Contains(q, "percent encode") ||
		strings.Contains(q, "percent-encode")
}
func (s *URLEncodeSkill) Execute(_ context.Context, q string) (string, error) {
	t := quoteStripped(extractPayload(q))
	if t == "" {
		return "", fmt.Errorf("no text to encode")
	}
	return url.QueryEscape(t), nil
}

type URLDecodeSkill struct{ *kyoci.BaseSkill }

func NewURLDecodeSkill() *URLDecodeSkill {
	return &URLDecodeSkill{BaseSkill: kyoci.NewBaseSkill(
		"url_decode", "URL-decode percent-encoded text",
		[]string{"url decode", "urldecode", "decode url"},
	)}
}
func (s *URLDecodeSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "url decode") || strings.Contains(q, "url-decode") ||
		strings.Contains(q, "urldecode")
}
func (s *URLDecodeSkill) Execute(_ context.Context, q string) (string, error) {
	t := quoteStripped(extractPayload(q))
	if t == "" {
		return "", fmt.Errorf("no text to decode")
	}
	out, err := url.QueryUnescape(t)
	if err != nil {
		return "", fmt.Errorf("invalid url encoding: %w", err)
	}
	return out, nil
}

// ---- html ----

type HTMLEscapeSkill struct{ *kyoci.BaseSkill }

func NewHTMLEscapeSkill() *HTMLEscapeSkill {
	return &HTMLEscapeSkill{BaseSkill: kyoci.NewBaseSkill(
		"html_escape", "Escape HTML special characters (<, >, &, \", ')",
		[]string{"html escape", "escape html", "htmlencode"},
	)}
}
func (s *HTMLEscapeSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "html escape") || strings.Contains(q, "escape html") ||
		strings.Contains(q, "html-escape")
}
func (s *HTMLEscapeSkill) Execute(_ context.Context, q string) (string, error) {
	t := quoteStripped(extractPayload(q))
	if t == "" {
		return "", fmt.Errorf("no text to escape")
	}
	return html.EscapeString(t), nil
}

type HTMLUnescapeSkill struct{ *kyoci.BaseSkill }

func NewHTMLUnescapeSkill() *HTMLUnescapeSkill {
	return &HTMLUnescapeSkill{BaseSkill: kyoci.NewBaseSkill(
		"html_unescape", "Unescape HTML entities (&lt; &gt; &amp; etc.)",
		[]string{"html unescape", "unescape html", "htmldecode"},
	)}
}
func (s *HTMLUnescapeSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "html unescape") || strings.Contains(q, "unescape html") ||
		strings.Contains(q, "html-unescape")
}
func (s *HTMLUnescapeSkill) Execute(_ context.Context, q string) (string, error) {
	t := quoteStripped(extractPayload(q))
	if t == "" {
		return "", fmt.Errorf("no text to unescape")
	}
	return html.UnescapeString(t), nil
}

// ---- hex ----

type HexEncodeSkill struct{ *kyoci.BaseSkill }

func NewHexEncodeSkill() *HexEncodeSkill {
	return &HexEncodeSkill{BaseSkill: kyoci.NewBaseSkill(
		"hex_encode", "Encode text to hexadecimal",
		[]string{"hex encode", "encode hex", "to hex", "to hexadecimal"},
	)}
}
func (s *HexEncodeSkill) Match(q string) bool {
	q = strings.ToLower(q)
	// Require the "encode" verb (or "to hex" with an explicit encode prefix)
	// so we don't match "rgb to hex" / "dec to hex" (which belong to
	// rgb_to_hex and base_convert respectively).
	if strings.Contains(q, "hex encode") || strings.Contains(q, "encode hex") ||
		strings.Contains(q, "to hexadecimal") || strings.Contains(q, "hex_encode") {
		return true
	}
	return false
}
func (s *HexEncodeSkill) Execute(_ context.Context, q string) (string, error) {
	t := quoteStripped(extractPayload(q))
	if t == "" {
		return "", fmt.Errorf("no text to encode")
	}
	return hex.EncodeToString([]byte(t)), nil
}

type HexDecodeSkill struct{ *kyoci.BaseSkill }

func NewHexDecodeSkill() *HexDecodeSkill {
	return &HexDecodeSkill{BaseSkill: kyoci.NewBaseSkill(
		"hex_decode", "Decode hexadecimal to text",
		[]string{"hex decode", "decode hex", "from hex"},
	)}
}
func (s *HexDecodeSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "hex decode") || strings.Contains(q, "decode hex") ||
		strings.Contains(q, "from hex") || strings.Contains(q, "from hexadecimal")
}
func (s *HexDecodeSkill) Execute(_ context.Context, q string) (string, error) {
	t := strings.TrimSpace(quoteStripped(extractPayload(q)))
	// Strip 0x prefix if present.
	t = strings.TrimPrefix(t, "0x")
	if t == "" {
		return "", fmt.Errorf("no hex to decode")
	}
	out, err := hex.DecodeString(t)
	if err != nil {
		return "", fmt.Errorf("invalid hex: %w", err)
	}
	return string(out), nil
}

// ---- unicode ----

type UnicodeEscapeSkill struct{ *kyoci.BaseSkill }

func NewUnicodeEscapeSkill() *UnicodeEscapeSkill {
	return &UnicodeEscapeSkill{BaseSkill: kyoci.NewBaseSkill(
		"unicode_escape", "Escape non-ASCII to \\uXXXX form",
		[]string{"unicode escape", "escape unicode", "to unicode escapes"},
	)}
}
func (s *UnicodeEscapeSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "unicode escape") || strings.Contains(q, "escape unicode") ||
		strings.Contains(q, "to \\u")
}
func (s *UnicodeEscapeSkill) Execute(_ context.Context, q string) (string, error) {
	t := quoteStripped(extractPayload(q))
	if t == "" {
		return "", fmt.Errorf("no text to escape")
	}
	var b strings.Builder
	for _, r := range t {
		if r < 128 {
			b.WriteRune(r)
		} else {
			b.WriteString(fmt.Sprintf("\\u%04x", r))
		}
	}
	return b.String(), nil
}

type UnicodeUnescapeSkill struct{ *kyoci.BaseSkill }

func NewUnicodeUnescapeSkill() *UnicodeUnescapeSkill {
	return &UnicodeUnescapeSkill{BaseSkill: kyoci.NewBaseSkill(
		"unicode_unescape", "Decode \\uXXXX escapes to characters",
		[]string{"unicode unescape", "unescape unicode", "decode unicode"},
	)}
}
func (s *UnicodeUnescapeSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "unicode unescape") || strings.Contains(q, "unescape unicode") ||
		strings.Contains(q, "decode \\u") || strings.Contains(q, "decode unicode escapes")
}
func (s *UnicodeUnescapeSkill) Execute(_ context.Context, q string) (string, error) {
	t := quoteStripped(extractPayload(q))
	if t == "" {
		return "", fmt.Errorf("no text to unescape")
	}
	var b strings.Builder
	i := 0
	for i < len(t) {
		if i+6 <= len(t) && t[i] == '\\' && (t[i+1] == 'u' || t[i+1] == 'U') {
			hexStr := t[i+2 : i+6]
			if v, err := strconv.ParseInt(hexStr, 16, 32); err == nil {
				b.WriteRune(rune(v))
				i += 6
				continue
			}
		}
		b.WriteByte(t[i])
		i++
	}
	return b.String(), nil
}
