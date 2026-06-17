package builtin

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"hash"
	"hash/crc32"
	"hash/crc64"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/pbkdf2"
	"golang.org/x/crypto/sha3"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// =====================================================================================
// Hashing / Crypto skills — message digests, HMAC, password hashing, AES.
// =====================================================================================

// ---- individual hashes (split from existing `hash` skill) ----

func hashSkill(name, desc string, kw []string, hashFn func([]byte) string) *simpleHashSkill {
	return &simpleHashSkill{
		BaseSkill: kyoci.NewBaseSkill(name, desc, kw),
		hashFn:    hashFn,
	}
}

type simpleHashSkill struct {
	*kyoci.BaseSkill
	hashFn func([]byte) string
}

func (s *simpleHashSkill) Execute(_ context.Context, q string) (string, error) {
	t := stripHashVerb(q, s.Name())
	t = quoteStripped(t)
	if t == "" {
		return "", fmt.Errorf("no text to hash")
	}
	return s.hashFn([]byte(t)), nil
}

// stripHashVerb removes the hash-command prefix from a query. The user's
// query is typically "md5 of hello", "sha256: hello", "compute crc32 foo".
// extractPayload doesn't strip these names because they're not in its
// stopword list, so we do it here per-skill using the skill's own Name().
func stripHashVerb(q, verb string) string {
	low := strings.ToLower(q)
	// Find the verb (e.g. "md5") and skip past it + optional "of/for/:" prefix.
	idx := strings.Index(low, strings.ToLower(verb))
	if idx < 0 {
		return strings.TrimSpace(q)
	}
	rest := q[idx+len(verb):]
	// Skip a colon or " of " / " for " after the verb.
	rest = strings.TrimLeft(rest, ": \t")
	for _, p := range []string{"of ", "for ", "this ", "the "} {
		if strings.HasPrefix(strings.ToLower(rest), p) {
			rest = rest[len(p):]
			break
		}
	}
	return strings.TrimSpace(rest)
}

func NewMD5Skill() *simpleHashSkill {
	return hashSkill("md5", "MD5 digest (128-bit, hex)",
		[]string{"md5 hash", "md5 of", "md5:", "compute md5"},
		func(b []byte) string {
			h := md5.Sum(b)
			return hex.EncodeToString(h[:])
		})
}
func (s *simpleHashSkill) MatchMD5(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "md5 ") || strings.Contains(q, "md5:") || strings.Contains(q, "md5 hash") || strings.Contains(q, "md5 of")
}

func NewSHA1Skill() *simpleHashSkill {
	return hashSkill("sha1", "SHA-1 digest (160-bit, hex)",
		[]string{"sha1 hash", "sha1 of", "sha1:", "compute sha1"},
		func(b []byte) string {
			h := sha1.Sum(b)
			return hex.EncodeToString(h[:])
		})
}
func (s *simpleHashSkill) MatchSHA1(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "sha1 ") || strings.Contains(q, "sha1:") || strings.Contains(q, "sha1 hash") || strings.Contains(q, "sha1 of")
}

func NewSHA256Skill() *simpleHashSkill {
	return hashSkill("sha256", "SHA-256 digest (256-bit, hex)",
		[]string{"sha256 hash", "sha256 of", "sha256:", "compute sha256"},
		func(b []byte) string {
			h := sha256.Sum256(b)
			return hex.EncodeToString(h[:])
		})
}
func (s *simpleHashSkill) MatchSHA256(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "sha256 ") || strings.Contains(q, "sha256:") || strings.Contains(q, "sha256 hash") || strings.Contains(q, "sha256 of")
}

func NewSHA512Skill() *simpleHashSkill {
	return hashSkill("sha512", "SHA-512 digest (512-bit, hex)",
		[]string{"sha512 hash", "sha512 of", "sha512:", "compute sha512"},
		func(b []byte) string {
			h := sha512.Sum512(b)
			return hex.EncodeToString(h[:])
		})
}
func (s *simpleHashSkill) MatchSHA512(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "sha512 ") || strings.Contains(q, "sha512:") || strings.Contains(q, "sha512 hash") || strings.Contains(q, "sha512 of")
}

func NewSHA3Skill() *simpleHashSkill {
	return hashSkill("sha3_256", "SHA3-256 digest (Keccak, 256-bit, hex)",
		[]string{"sha3 hash", "sha3-256", "sha3 of", "keccak"},
		func(b []byte) string {
			h := sha3.Sum256(b)
			return hex.EncodeToString(h[:])
		})
}
func (s *simpleHashSkill) MatchSHA3(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "sha3 ") || strings.Contains(q, "sha3-256") || strings.Contains(q, "sha3 hash") || strings.Contains(q, "keccak")
}

// Override Match() on simpleHashSkill — dispatches to the per-variant helper
// for SHA-family (which has tight per-name patterns), and falls back to
// BaseSkill.Match for CRC variants (whose keywords "crc32" / "crc64" are
// already unambiguous substring matches).
func (s *simpleHashSkill) Match(q string) bool {
	switch s.Name() {
	case "md5":
		return s.MatchMD5(q)
	case "sha1":
		return s.MatchSHA1(q)
	case "sha256":
		return s.MatchSHA256(q)
	case "sha512":
		return s.MatchSHA512(q)
	case "sha3_256":
		return s.MatchSHA3(q)
	}
	// CRC variants — defer to the BaseSkill keyword matcher.
	return s.BaseSkill.Match(q)
}

// ---- CRC ----

func NewCRC32Skill() *simpleHashSkill {
	return hashSkill("crc32", "CRC-32 checksum (IEEE polynomial, hex)",
		[]string{"crc32", "crc-32", "crc 32"},
		func(b []byte) string {
			return fmt.Sprintf("%08x", crc32.ChecksumIEEE(b))
		})
}

func NewCRC64Skill() *simpleHashSkill {
	return hashSkill("crc64", "CRC-64 checksum (ISO polynomial, hex)",
		[]string{"crc64", "crc-64", "crc 64"},
		func(b []byte) string {
			return fmt.Sprintf("%016x", crc64.Checksum(b, crc64.MakeTable(crc64.ISO)))
		})
}

// CRC variants don't need per-name Match overrides since "crc32" and "crc64"
// are distinct enough that the BaseSkill substring match is unambiguous.
// Override the simpleHashSkill.Match to fall back to BaseSkill.Match for these.
func crcMatch(s *simpleHashSkill, q string) bool {
	return s.BaseSkill.Match(q)
}

// ---- HMAC ----

type HMACSkill struct {
	*kyoci.BaseSkill
	bits int // 256 or 512
}

func NewHMACSHA256Skill() *HMACSkill {
	return &HMACSkill{BaseSkill: kyoci.NewBaseSkill(
		"hmac_sha256", "HMAC-SHA-256 (keyed hash), input format: 'key:value'",
		[]string{"hmac-sha256", "hmac sha256", "hmac_sha256"},
	), bits: 256}
}
func NewHMACSHA512Skill() *HMACSkill {
	return &HMACSkill{BaseSkill: kyoci.NewBaseSkill(
		"hmac_sha512", "HMAC-SHA-512 (keyed hash), input format: 'key:value'",
		[]string{"hmac-sha512", "hmac sha512", "hmac_sha512"},
	), bits: 512}
}
func (s *HMACSkill) Match(q string) bool {
	q = strings.ToLower(q)
	if s.bits == 256 {
		return strings.Contains(q, "hmac-sha256") || strings.Contains(q, "hmac sha256") || strings.Contains(q, "hmac_sha256")
	}
	return strings.Contains(q, "hmac-sha512") || strings.Contains(q, "hmac sha512") || strings.Contains(q, "hmac_sha512")
}
func (s *HMACSkill) Execute(_ context.Context, q string) (string, error) {
	// extractPayload splits at the first ':' but we want the FULL "key:message"
	// pair after the verb. Operate on the full query.
	low := strings.ToLower(q)
	verb := "sha256 "
	if s.bits == 512 {
		verb = "sha512 "
	}
	verbEnd := strings.Index(low, verb)
	if verbEnd < 0 {
		// Fall back to looking for "hmac" then the next space.
		verbEnd = strings.Index(low, "hmac")
		if verbEnd < 0 {
			return "", fmt.Errorf("expected 'hmac-%s key:message'", strings.Trim(verb, " "))
		}
		// Skip "hmac-<verb>" plus one space.
		rest := q[verbEnd:]
		spaceIdx := strings.Index(rest, " ")
		if spaceIdx < 0 {
			return "", fmt.Errorf("expected 'hmac-%s key:message'", strings.Trim(verb, " "))
		}
		verbEnd = verbEnd + spaceIdx + 1
	} else {
		verbEnd = verbEnd + len(verb)
	}
	rest := strings.TrimSpace(q[verbEnd:])
	idx := strings.Index(rest, ":")
	if idx <= 0 {
		return "", fmt.Errorf("expected 'key:message' format")
	}
	key := strings.TrimSpace(rest[:idx])
	msg := strings.TrimSpace(rest[idx+1:])
	var mac hash.Hash
	if s.bits == 256 {
		mac = hmac.New(sha256.New, []byte(key))
	} else {
		mac = hmac.New(sha512.New, []byte(key))
	}
	mac.Write([]byte(msg))
	return hex.EncodeToString(mac.Sum(nil)), nil
}

// splitKeyMessage splits "key:message" on the first colon. Returns ok=false if
// no colon is present. Both halves are trimmed of surrounding whitespace.
func splitKeyMessage(s string) (key, msg string, ok bool) {
	idx := strings.Index(s, ":")
	if idx < 0 {
		return "", "", false
	}
	return strings.TrimSpace(s[:idx]), strings.TrimSpace(s[idx+1:]), true
}

// ---- bcrypt ----

type BcryptHashSkill struct{ *kyoci.BaseSkill }

func NewBcryptHashSkill() *BcryptHashSkill {
	return &BcryptHashSkill{BaseSkill: kyoci.NewBaseSkill(
		"bcrypt_hash", "Bcrypt-hash a password (cost 12)",
		[]string{"bcrypt hash", "hash with bcrypt", "bcrypt password", "bcrypt:"},
	)}
}
func (s *BcryptHashSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "bcrypt hash") || strings.Contains(q, "hash with bcrypt") ||
		strings.Contains(q, "bcrypt:") || strings.Contains(q, "bcrypt password")
}
func (s *BcryptHashSkill) Execute(_ context.Context, q string) (string, error) {
	pw := quoteStripped(extractPayload(q))
	if pw == "" {
		return "", fmt.Errorf("no password to hash")
	}
	h, err := bcrypt.GenerateFromPassword([]byte(pw), 12)
	if err != nil {
		return "", fmt.Errorf("bcrypt hash failed: %w", err)
	}
	return string(h), nil
}

type BcryptVerifySkill struct{ *kyoci.BaseSkill }

func NewBcryptVerifySkill() *BcryptVerifySkill {
	return &BcryptVerifySkill{BaseSkill: kyoci.NewBaseSkill(
		"bcrypt_verify", "Verify a password against a bcrypt hash. Input: 'password:hash'",
		[]string{"bcrypt verify", "verify bcrypt", "check bcrypt"},
	)}
}
func (s *BcryptVerifySkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "bcrypt verify") || strings.Contains(q, "verify bcrypt") ||
		strings.Contains(q, "check bcrypt")
}
func (s *BcryptVerifySkill) Execute(_ context.Context, q string) (string, error) {
	// extractPayload splits at the first ':' but the bcrypt hash may contain
	// no ':' itself, so the password gets dropped. Operate on the full query.
	low := strings.ToLower(q)
	verbEnd := strings.Index(low, "verify ")
	if verbEnd < 0 {
		return "", fmt.Errorf("expected 'bcrypt verify password:hash'")
	}
	rest := strings.TrimSpace(q[verbEnd+len("verify "):])
	idx := strings.Index(rest, ":")
	if idx <= 0 {
		return "", fmt.Errorf("expected 'password:hash' format")
	}
	pw := strings.TrimSpace(rest[:idx])
	hash := strings.TrimSpace(rest[idx+1:])
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)); err != nil {
		return "mismatch", nil
	}
	return "match", nil
}

// ---- AES ----

type AESEncryptSkill struct{ *kyoci.BaseSkill }

func NewAESEncryptSkill() *AESEncryptSkill {
	return &AESEncryptSkill{BaseSkill: kyoci.NewBaseSkill(
		"aes_encrypt", "AES-256-GCM encrypt. Input: 'passkey:plaintext'. Output: hex 'salt:ciphertext'",
		[]string{"aes encrypt", "encrypt aes", "aes-256 encrypt", "encrypt with aes"},
	)}
}
func (s *AESEncryptSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "aes encrypt") || strings.Contains(q, "encrypt aes") ||
		strings.Contains(q, "encrypt with aes") || strings.Contains(q, "aes-encrypt")
}
func (s *AESEncryptSkill) Execute(_ context.Context, q string) (string, error) {
	// extractPayload splits at the first ':' which is INSIDE the
	// "passkey:plaintext" payload. Operate on the full query and find the
	// split colon ourselves (skipping the leading "aes encrypt " verb).
	low := strings.ToLower(q)
	verbEnd := strings.Index(low, "encrypt ")
	if verbEnd < 0 {
		return "", fmt.Errorf("expected 'aes encrypt passkey:plaintext'")
	}
	rest := strings.TrimSpace(q[verbEnd+len("encrypt "):])
	// Find the first ':' — that separates the passkey from the plaintext.
	idx := strings.Index(rest, ":")
	if idx <= 0 {
		return "", fmt.Errorf("expected 'passkey:plaintext' format")
	}
	passkey := strings.TrimSpace(rest[:idx])
	plaintext := strings.TrimSpace(rest[idx+1:])
	if plaintext == "" {
		return "", fmt.Errorf("plaintext is empty")
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("salt generation failed: %w", err)
	}
	key := pbkdf2.Key([]byte(passkey), salt, 100_000, 32, sha256.New)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("cipher init: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("gcm init: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("nonce generation failed: %w", err)
	}
	ct := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	// Pack salt + nonce + ciphertext together.
	combined := append(salt, nonce...)
	combined = append(combined, ct...)
	return hex.EncodeToString(combined), nil
}

type AESDecryptSkill struct{ *kyoci.BaseSkill }

func NewAESDecryptSkill() *AESDecryptSkill {
	return &AESDecryptSkill{BaseSkill: kyoci.NewBaseSkill(
		"aes_decrypt", "AES-256-GCM decrypt. Input: 'passkey:hex' where hex is the output of aes_encrypt",
		[]string{"aes decrypt", "decrypt aes", "decrypt with aes"},
	)}
}
func (s *AESDecryptSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "aes decrypt") || strings.Contains(q, "decrypt aes") ||
		strings.Contains(q, "decrypt with aes")
}
func (s *AESDecryptSkill) Execute(_ context.Context, q string) (string, error) {
	// Same extractPayload workaround as AESEncryptSkill — operate on the full
	// query, find the split colon after the verb.
	low := strings.ToLower(q)
	verbEnd := strings.Index(low, "decrypt ")
	if verbEnd < 0 {
		return "", fmt.Errorf("expected 'aes decrypt passkey:hex'")
	}
	rest := strings.TrimSpace(q[verbEnd+len("decrypt "):])
	idx := strings.Index(rest, ":")
	if idx <= 0 {
		return "", fmt.Errorf("expected 'passkey:hex' format")
	}
	passkey := strings.TrimSpace(rest[:idx])
	hexCombined := strings.TrimSpace(rest[idx+1:])
	combined, err := hex.DecodeString(strings.TrimSpace(hexCombined))
	if err != nil {
		return "", fmt.Errorf("invalid hex: %w", err)
	}
	if len(combined) < 16+12 {
		return "", fmt.Errorf("ciphertext too short")
	}
	salt := combined[:16]
	nonce := combined[16 : 16+12]
	ct := combined[16+12:]
	key := pbkdf2.Key([]byte(passkey), salt, 100_000, 32, sha256.New)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("cipher init: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("gcm init: %w", err)
	}
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt failed (wrong key?): %w", err)
	}
	return string(pt), nil
}
