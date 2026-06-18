package builtin

import (
	"context"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash"
	"strconv"
	"strings"
	"time"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// =====================================================================================
// OTP / HMAC crypto skills — TOTP, HOTP, secret generation, generic HMAC,
// constant-time compare, and HMAC-MD5.
//
// Distinct from hashing.go (which covers md5/sha*/HMAC-SHA256/HMAC-SHA512/bcrypt):
// these skills add RFC 6238/4226 OTP support, an algorithm-pluggable generic HMAC
// entry point, HMAC-MD5 (missing from hashing.go), and a constant-time comparison
// helper suitable for token verification.
// =====================================================================================

// nowFunc is the time source used by the TOTP skill. Indirected at package level
// so tests can substitute a fixed clock for deterministic verification against
// the RFC 6238 test vectors. Production callers should not reassign this.
var nowFunc = time.Now

// ---- totp_generate (RFC 6238) ----

type TOTPGenerateSkill struct{ *kyoci.BaseSkill }

func NewTOTPGenerateSkill() *TOTPGenerateSkill {
	return &TOTPGenerateSkill{BaseSkill: kyoci.NewBaseSkill(
		"totp_generate", "Generate a TOTP code (RFC 6238) for a base32 secret. Default 6 digits, 30s step, SHA-1",
		[]string{"totp", "totp generate", "generate totp"},
	)}
}
func (s *TOTPGenerateSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "totp")
}
func (s *TOTPGenerateSkill) Execute(_ context.Context, q string) (string, error) {
	secret := stripVerb(q, "totp")
	secret = quoteStripped(secret)
	// Strip a leading "for " if present ("totp for SECRET").
	secret = strings.TrimSpace(stripLeadingWord(secret, "for"))
	secret = strings.TrimLeft(secret, ": \t")
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return "", fmt.Errorf("no TOTP secret provided")
	}
	step := uint64(nowFunc().Unix() / 30)
	code, err := computeOTP(secret, step)
	if err != nil {
		return "", err
	}
	return code, nil
}

// ---- hotp_generate (RFC 4226) ----

type HOTPGenerateSkill struct{ *kyoci.BaseSkill }

func NewHOTPGenerateSkill() *HOTPGenerateSkill {
	return &HOTPGenerateSkill{BaseSkill: kyoci.NewBaseSkill(
		"hotp_generate", "Generate an HOTP code (RFC 4226) for a base32 secret + counter. Usage: 'hotp: SECRET counter N'",
		[]string{"hotp", "hotp generate", "generate hotp"},
	)}
}
func (s *HOTPGenerateSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "hotp")
}
func (s *HOTPGenerateSkill) Execute(_ context.Context, q string) (string, error) {
	payload := stripVerb(q, "hotp")
	payload = quoteStripped(payload)
	payload = strings.TrimSpace(stripLeadingWord(payload, "for"))
	payload = strings.TrimLeft(payload, ": \t")
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return "", fmt.Errorf("no HOTP secret provided")
	}
	// Parse "SECRET counter N" or "SECRET|N". Default counter 0.
	var (
		counter uint64
		secret  string
	)
	parts := strings.SplitN(payload, "|", 2)
	if len(parts) == 2 {
		secret = strings.TrimSpace(parts[0])
		n, err := strconv.ParseUint(strings.TrimSpace(parts[1]), 10, 64)
		if err != nil {
			return "", fmt.Errorf("invalid counter: %w", err)
		}
		counter = n
	} else {
		// Try "SECRET counter N" form. Match "counter" as a whole word so a
		// bare "counter 5" (no secret) is detected as the missing-secret case.
		lowPayload := strings.ToLower(payload)
		counterIdx := wordIndex(lowPayload, "counter")
		if counterIdx >= 0 {
			secret = strings.TrimSpace(payload[:counterIdx])
			tail := strings.TrimSpace(payload[counterIdx+len("counter"):])
			n, err := strconv.ParseUint(tail, 10, 64)
			if err != nil {
				return "", fmt.Errorf("invalid counter: %w", err)
			}
			counter = n
		} else {
			secret = strings.TrimSpace(payload)
		}
	}
	if secret == "" {
		return "", fmt.Errorf("no HOTP secret provided")
	}
	code, err := computeOTP(secret, counter)
	if err != nil {
		return "", err
	}
	return code, nil
}

// stripLeadingWord removes a leading word (and any trailing space) from s if it
// matches w (case-insensitive). Used to peel off filler words like "for" that
// appear between the skill verb and the operand.
func stripLeadingWord(s, w string) string {
	low := strings.ToLower(s)
	if low == w || strings.HasPrefix(low, w+" ") {
		return s[len(w):]
	}
	return s
}

// wordIndex returns the byte index of the first whole-word occurrence of w in s
// (case-insensitive), where "whole word" means the match is preceded by either
// the start of the string or a non-alphanumeric byte, and followed by either a
// non-alphanumeric byte or end-of-string. Returns -1 if not found. Used by the
// HOTP parser to find the literal word "counter" without false-matching inside a
// base32 secret that happens to contain those letters.
func wordIndex(s, w string) int {
	low := strings.ToLower(s)
	start := 0
	for {
		idx := strings.Index(low[start:], w)
		if idx < 0 {
			return -1
		}
		idx += start
		end := idx + len(w)
		prevOK := idx == 0 || !isAlphaNum(low[idx-1])
		nextOK := end == len(low) || !isAlphaNum(low[end])
		if prevOK && nextOK {
			return idx
		}
		start = idx + len(w)
	}
}

// isAlphaNum reports whether b is an ASCII letter or digit.
func isAlphaNum(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

// computeOTP implements the shared HOTP/TOTP core: HMAC-SHA1 over the 8-byte
// big-endian counter, dynamic truncation per RFC 4226 §5.3, and mod 10^digits.
// Secret is base32-decoded (padding-insensitive). Defaults: 6 digits, SHA-1.
func computeOTP(secretBase32 string, counter uint64) (string, error) {
	key, err := decodeBase32Secret(secretBase32)
	if err != nil {
		return "", err
	}
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)
	mac := hmac.New(sha1.New, key)
	mac.Write(buf[:])
	h := mac.Sum(nil)
	offset := int(h[len(h)-1] & 0x0F)
	truncated := binary.BigEndian.Uint32(h[offset:offset+4]) & 0x7FFFFFFF
	const digits = 6
	mod := uint32(1)
	for i := 0; i < digits; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", digits, truncated%mod), nil
}

// decodeBase32Secret decodes a base32 secret, tolerating missing padding and
// embedded whitespace. Uppercases first since RFC 6238 secrets are typically
// upper-case and base32 lower-case decoding is non-standard.
func decodeBase32Secret(s string) ([]byte, error) {
	s = strings.ToUpper(strings.Join(strings.Fields(s), ""))
	if _, err := base32.StdEncoding.DecodeString(s); err != nil {
		// Try with padding fixed up.
		padded := s + strings.Repeat("=", (8-len(s)%8)%8)
		dec, err2 := base32.StdEncoding.DecodeString(padded)
		if err2 != nil {
			return nil, fmt.Errorf("invalid base32 secret: %w", err)
		}
		return dec, nil
	}
	return base32.StdEncoding.DecodeString(s)
}

// ---- otp_secret_generate ----

type OTPSecretGenerateSkill struct{ *kyoci.BaseSkill }

func NewOTPSecretGenerateSkill() *OTPSecretGenerateSkill {
	return &OTPSecretGenerateSkill{BaseSkill: kyoci.NewBaseSkill(
		"otp_secret_generate", "Generate a random base32 secret (default 20 bytes). Optional byte count: 'otp secret 32'",
		[]string{"otp secret", "generate otp secret", "generate secret"},
	)}
}
func (s *OTPSecretGenerateSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "otp secret") || strings.Contains(q, "generate secret") ||
		strings.Contains(q, "totp secret") || strings.Contains(q, "hotp secret")
}
func (s *OTPSecretGenerateSkill) Execute(_ context.Context, q string) (string, error) {
	n := 20
	if v := parseIntSuffix(strings.ToLower(q)); v > 0 {
		n = v
	}
	if n <= 0 || n > 1024 {
		return "", fmt.Errorf("invalid byte count: %d", n)
	}
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("rand failed: %w", err)
	}
	// NoPadding variant is the canonical OTP secret encoding used by Google
	// Authenticator / RFC 6238 provisioning URIs.
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b), nil
}

// ---- random_hex ----

// NOTE: The existing RandomBytesSkill also matches "random hex". This skill
// uses "random_hex" / "random hex:" / "generate hex" as its primary match
// phrases; tests target this skill directly so registry overlap is harmless.

type RandomHexSkill struct{ *kyoci.BaseSkill }

func NewRandomHexSkill() *RandomHexSkill {
	return &RandomHexSkill{BaseSkill: kyoci.NewBaseSkill(
		"random_hex", "Generate N random hex bytes. Usage: 'random hex: 16' → 32-char hex",
		[]string{"random_hex", "random hex", "generate hex"},
	)}
}
func (s *RandomHexSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "random_hex") || strings.Contains(q, "generate hex") ||
		strings.Contains(q, "random hex")
}
func (s *RandomHexSkill) Execute(_ context.Context, q string) (string, error) {
	n := 16
	// Look for a length either via "random hex: N" or trailing integer.
	if payload := extractPayload(q); payload != "" {
		if v, err := strconv.Atoi(strings.TrimSpace(payload)); err == nil && v > 0 {
			n = v
		}
	}
	if v := parseIntSuffix(strings.ToLower(q)); v > 0 && n == 16 {
		n = v
	}
	if n <= 0 || n > 1<<20 {
		return "", fmt.Errorf("invalid byte count: %d", n)
	}
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("rand failed: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// ---- hmac_for_algorithm ----

// hmacForAlgos maps user-supplied algorithm names to hash constructors.
var hmacForAlgos = map[string]func() hash.Hash{
	"md5":    md5.New,
	"sha1":   sha1.New,
	"sha256": sha256.New,
	"sha512": sha512.New,
}

type HMACForAlgorithmSkill struct{ *kyoci.BaseSkill }

func NewHMACForAlgorithmSkill() *HMACForAlgorithmSkill {
	return &HMACForAlgorithmSkill{BaseSkill: kyoci.NewBaseSkill(
		"hmac_for_algorithm", "Generic HMAC over data with selectable algorithm (md5/sha1/sha256/sha512). Usage: 'hmac sha256: key | data' or 'hmac: sha256 key data'",
		[]string{"hmac for algorithm", "generic hmac", "hmac for"},
	)}
}
func (s *HMACForAlgorithmSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "hmac for algorithm") || strings.Contains(q, "generic hmac") ||
		strings.Contains(q, "hmac for")
}
func (s *HMACForAlgorithmSkill) Execute(_ context.Context, q string) (string, error) {
	alg, key, data, err := parseGenericHMACQuery(q)
	if err != nil {
		return "", err
	}
	hashFn, ok := hmacForAlgos[alg]
	if !ok {
		return "", fmt.Errorf("unsupported algorithm: %s (want md5/sha1/sha256/sha512)", alg)
	}
	mac := hmac.New(hashFn, []byte(key))
	mac.Write([]byte(data))
	return hex.EncodeToString(mac.Sum(nil)), nil
}

// parseGenericHMACQuery accepts three shapes:
//  1. "hmac for algorithm sha256: key | data"  (filler words "for"/"algorithm" skipped)
//  2. "generic hmac sha256: key | data"        (filler "generic"/"hmac" skipped)
//  3. "hmac: sha256 key data"                  (algorithm is first token after colon)
//
// Returns (alg, key, data, err). Algorithm is lower-cased. Tokens are scanned
// left-to-right; the first recognized algorithm name is taken as the alg, any
// tokens before it are treated as filler.
func parseGenericHMACQuery(q string) (alg, key, data string, err error) {
	low := strings.ToLower(q)
	hi := strings.Index(low, "hmac")
	if hi < 0 {
		return "", "", "", fmt.Errorf("expected 'hmac <alg>: key | data' or 'hmac: <alg> key data'")
	}
	// Operate on a token stream so filler words ("for", "algorithm", "generic")
	// and the separator colon can all be skipped uniformly. Strip the colon
	// that often follows "hmac" first so it doesn't pollute the first token.
	rest := q[hi+len("hmac"):]
	rest = strings.TrimLeft(rest, " :_\t-")
	tokens := strings.Fields(rest)
	if len(tokens) == 0 {
		return "", "", "", fmt.Errorf("expected 'hmac <alg>: key | data'")
	}

	// Find the first token that names a known algorithm; everything before it
	// is filler. The algorithm may be attached to a trailing colon ("sha256:").
	algIdx := -1
	for i, tok := range tokens {
		clean := strings.ToLower(strings.TrimRight(tok, ":"))
		if _, ok := hmacForAlgos[clean]; ok {
			alg = clean
			algIdx = i
			break
		}
	}
	if algIdx < 0 {
		return "", "", "", fmt.Errorf("could not identify algorithm (want md5/sha1/sha256/sha512)")
	}
	// Remaining tokens after the algorithm are key/data. They may still contain
	// a leading colon on the first one.
	remaining := tokens[algIdx+1:]
	if len(remaining) == 0 {
		return "", "", "", fmt.Errorf("expected 'key | data' separator")
	}
	remaining[0] = strings.TrimLeft(remaining[0], ":")
	joined := strings.Join(remaining, " ")
	joined = strings.TrimSpace(joined)

	// Split on '|' (preferred) or fall back to whitespace.
	if idx := strings.Index(joined, "|"); idx > 0 {
		return alg, strings.TrimSpace(joined[:idx]), strings.TrimSpace(joined[idx+1:]), nil
	}
	toks := strings.Fields(joined)
	if len(toks) >= 2 {
		return alg, toks[0], strings.Join(toks[1:], " "), nil
	}
	return "", "", "", fmt.Errorf("expected 'key | data' separator")
}

// ---- timing_safe_compare ----

type TimingSafeCompareSkill struct{ *kyoci.BaseSkill }

func NewTimingSafeCompareSkill() *TimingSafeCompareSkill {
	return &TimingSafeCompareSkill{BaseSkill: kyoci.NewBaseSkill(
		"timing_safe_compare", "Constant-time compare of two strings. Usage: 'timing safe compare: a | b' → match/mismatch",
		[]string{"timing safe compare", "safe compare", "constant time compare"},
	)}
}
func (s *TimingSafeCompareSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "timing safe compare") || strings.Contains(q, "constant time compare") ||
		strings.Contains(q, "safe compare")
}
func (s *TimingSafeCompareSkill) Execute(_ context.Context, q string) (string, error) {
	payload := extractPayload(q)
	payload = quoteStripped(payload)
	if payload == "" {
		return "", fmt.Errorf("expected two values separated by '|'")
	}
	var a, b string
	parts := strings.SplitN(payload, "|", 2)
	if len(parts) == 2 {
		a = strings.TrimSpace(parts[0])
		b = strings.TrimSpace(parts[1])
	} else {
		// Fall back to whitespace-separated.
		tokens := strings.Fields(payload)
		if len(tokens) != 2 {
			return "", fmt.Errorf("expected two values separated by '|' or whitespace")
		}
		a, b = tokens[0], tokens[1]
	}
	// Length-mismatch short-circuit is intentionally avoided to keep the
	// comparison time independent of where the strings differ; subtle's
	// ConstantTimeCompare already returns 0 on length mismatch.
	if subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 {
		return "match", nil
	}
	return "mismatch", nil
}

// ---- hmac_md5 ----

type HMACMD5Skill struct{ *kyoci.BaseSkill }

func NewHMACMD5Skill() *HMACMD5Skill {
	return &HMACMD5Skill{BaseSkill: kyoci.NewBaseSkill(
		"hmac_md5", "HMAC-MD5 (keyed hash). Usage: 'hmac md5: key | data' → hex digest",
		[]string{"hmac md5", "hmac-md5", "hmac_md5"},
	)}
}
func (s *HMACMD5Skill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "hmac md5") || strings.Contains(q, "hmac-md5") ||
		strings.Contains(q, "hmac_md5")
}
func (s *HMACMD5Skill) Execute(_ context.Context, q string) (string, error) {
	key, data, err := parseHMACKeyData(q, "md5")
	if err != nil {
		return "", err
	}
	mac := hmac.New(md5.New, []byte(key))
	mac.Write([]byte(data))
	return hex.EncodeToString(mac.Sum(nil)), nil
}

// parseHMACKeyData extracts (key, data) from a single-algorithm HMAC query.
// Accepts "hmac <verb>: key | data", "hmac-<verb>: key | data",
// "hmac_<verb> key | data", or "hmac <verb>: key:data". The verb (e.g. "md5")
// is located case-insensitively and skipped along with any separators.
func parseHMACKeyData(q, verb string) (key, data string, err error) {
	low := strings.ToLower(q)
	hi := strings.Index(low, "hmac")
	if hi < 0 {
		return "", "", fmt.Errorf("expected 'hmac %s: key | data'", verb)
	}
	rest := q[hi+len("hmac"):]
	// Find the verb (case-insensitive) and skip past it.
	rl := strings.ToLower(rest)
	vi := strings.Index(rl, verb)
	if vi < 0 {
		return "", "", fmt.Errorf("expected 'hmac %s: key | data'", verb)
	}
	rest = rest[vi+len(verb):]
	rest = strings.TrimSpace(strings.TrimLeft(rest, " :_\t-"))
	// Prefer '|' separator; fall back to ':'.
	if idx := strings.Index(rest, "|"); idx > 0 {
		return strings.TrimSpace(rest[:idx]), strings.TrimSpace(rest[idx+1:]), nil
	}
	if idx := strings.Index(rest, ":"); idx > 0 {
		return strings.TrimSpace(rest[:idx]), strings.TrimSpace(rest[idx+1:]), nil
	}
	tokens := strings.Fields(rest)
	if len(tokens) >= 2 {
		return tokens[0], strings.Join(tokens[1:], " "), nil
	}
	return "", "", fmt.Errorf("expected 'hmac %s: key | data'", verb)
}
