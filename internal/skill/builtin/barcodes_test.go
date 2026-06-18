package builtin

import (
	"context"
	"testing"
)

// =====================================================================================
// Barcode / identifier validation skill tests — EAN-13, EAN-8, UPC-A, ISSN, VIN,
// SWIFT/BIC. 6 skills, table-driven via runSkillCases.
// =====================================================================================

// ---- EAN-13 ----

func TestEAN13ValidateSkill(t *testing.T) {
	runSkillCases(t, "ean13_validate", NewEAN13ValidateSkill(), []skillCase{
		{"positive: canonical valid", "ean13 validate: 5901234123457", true, "valid", false},
		{"positive: with spaces", "ean13 validate: 590 1234 1234 57", true, "valid", false},
		{"positive: with hyphens", "ean13 validate: 590-1234-1234-57", true, "valid", false},
		{"positive: ean 13 spaced keyword", "ean 13 validate: 5901234123457", true, "valid", false},
		{"positive: ean13 keyword no validate", "ean13 5901234123457", true, "valid", false},
		{"negative: wrong check digit", "ean13 validate: 5901234123458", true, "invalid: check digit mismatch (expected 7, got 8)", false},
		{"negative: too short", "ean13 validate: 590123412345", true, "invalid: EAN-13 must be 13 digits", false},
		{"negative: too long", "ean13 validate: 59012341234570", true, "invalid: EAN-13 must be 13 digits", false},
		{"negative: non-digit", "ean13 validate: 590123412345a", true, "invalid: non-digit characters", false},
		{"positive: quoted operand", `ean13 validate: "5901234123457"`, true, "valid", false},
		{"negative: unrelated ean8", "ean8 validate: 73513537", false, "", false},
		{"negative: unrelated text", "slugify hello world", false, "", false},
	})
}

// ---- EAN-8 ----

func TestEAN8ValidateSkill(t *testing.T) {
	runSkillCases(t, "ean8_validate", NewEAN8ValidateSkill(), []skillCase{
		{"positive: canonical valid", "ean8 validate: 73513537", true, "valid", false},
		{"positive: with spaces", "ean8 validate: 7351 3537", true, "valid", false},
		{"positive: with hyphens", "ean8 validate: 7351-3537", true, "valid", false},
		{"positive: ean 8 spaced keyword", "ean 8 validate: 73513537", true, "valid", false},
		{"positive: ean8 keyword no validate", "ean8 73513537", true, "valid", false},
		{"negative: wrong check digit", "ean8 validate: 73513530", true, "invalid: check digit mismatch", false},
		{"negative: too short", "ean8 validate: 7351353", true, "invalid: EAN-8 must be 8 digits", false},
		{"negative: too long", "ean8 validate: 735135371", true, "invalid: EAN-8 must be 8 digits", false},
		{"negative: non-digit", "ean8 validate: 7351353a", true, "invalid: non-digit characters", false},
		{"negative: unrelated ean13", "ean13 validate: 5901234123457", false, "", false},
		{"negative: unrelated text", "base64 encode: hello", false, "", false},
	})
}

// ---- UPC-A ----

func TestUPCAValidateSkill(t *testing.T) {
	runSkillCases(t, "upc_a_validate", NewUPCAValidateSkill(), []skillCase{
		{"positive: canonical valid", "upc-a validate: 036000291452", true, "valid", false},
		{"positive: upc keyword", "upc validate: 036000291452", true, "valid", false},
		{"positive: with hyphens", "upc-a validate: 0-36000-29145-2", true, "valid", false},
		{"positive: with spaces", "upc-a validate: 0 36000 29145 2", true, "valid", false},
		{"positive: upca no separator", "upca 036000291452", true, "valid", false},
		{"negative: wrong check digit", "upc-a validate: 036000291453", true, "invalid: check digit mismatch (expected 2, got 3)", false},
		{"negative: too short", "upc-a validate: 03600029145", true, "invalid: UPC-A must be 12 digits", false},
		{"negative: too long", "upc-a validate: 0360002914523", true, "invalid: UPC-A must be 12 digits", false},
		{"negative: non-digit", "upc-a validate: 03600029145a", true, "invalid: non-digit characters", false},
		{"negative: unrelated ean13", "ean13 validate: 5901234123457", false, "", false},
		{"negative: unrelated text", "reverse text: hello", false, "", false},
	})
}

// ---- ISSN ----

func TestISSNValidateSkill(t *testing.T) {
	runSkillCases(t, "issn_validate", NewISSNValidateSkill(), []skillCase{
		{"positive: canonical valid X", "issn validate: 0317847X", true, "valid", false},
		{"positive: with hyphen", "issn validate: 0317-847X", true, "valid", false},
		{"positive: lowercased x", "issn validate: 0317847x", true, "valid", false},
		{"positive: issn keyword no validate", "issn 0317847X", true, "valid", false},
		{"positive: all-digit check", "issn validate: 00249319", true, "valid", false}, // check digit 9
		{"negative: wrong check digit", "issn validate: 03178470", true, "invalid: check digit mismatch", false},
		{"negative: too short", "issn validate: 0317847", true, "invalid: ISSN must be 8 characters", false},
		{"negative: too long", "issn validate: 0317847XX", true, "invalid: ISSN must be 8 characters", false},
		{"negative: bad check char", "issn validate: 0317847Y", true, "invalid: check character must be a digit or X", false},
		{"negative: letters in payload", "issn validate: 0A17847X", true, "invalid: first 7 characters must be digits", false},
		{"negative: unrelated text", "uuid v4 generate", false, "", false},
	})
}

// ---- VIN ----

func TestVINValidateSkill(t *testing.T) {
	runSkillCases(t, "vin_validate", NewVINValidateSkill(), []skillCase{
		{"positive: canonical valid", "vin validate: 1M8GDM9AXKP042788", true, "valid", false},
		{"positive: lowercase input", "vin validate: 1m8gdm9axkp042788", true, "valid", false},
		{"positive: with spaces", "vin validate: 1M8 GDM9A XKP 042788", true, "valid", false},
		{"positive: vehicle identification number", "validate vehicle identification number 1M8GDM9AXKP042788", true, "valid", false},
		{"positive: vin keyword no validate", "vin 1M8GDM9AXKP042788", true, "valid", false},
		{"negative: wrong check digit", "vin validate: 1M8GDM9A1KP042788", true, "invalid: check digit mismatch", false},
		{"negative: too short", "vin validate: 1M8GDM9AXKP04278", true, "invalid: VIN must be 17 characters", false},
		{"negative: too long", "vin validate: 1M8GDM9AXKP0427888", true, "invalid: VIN must be 17 characters", false},
		{"negative: illegal char I", "vin validate: 1M8GDM9AXKP04278I", true, "invalid: illegal character", false},
		{"negative: illegal char O", "vin validate: 1M8GDM9AOXP042788", true, "invalid: illegal character", false},
		{"negative: illegal char Q", "vin validate: 1M8GDM9AQXP042788", true, "invalid: illegal character", false},
		{"negative: unrelated text", "ean13 validate: 5901234123457", false, "", false},
	})
}

// ---- SWIFT / BIC ----

func TestSWIFTBICValidateSkill(t *testing.T) {
	runSkillCases(t, "swift_bic_validate", NewSWIFTBICValidateSkill(), []skillCase{
		{"positive: 8-char valid", "swift validate: DEUTDEFF", true, "valid", false},
		{"positive: 11-char valid", "swift validate: DEUTDEFF500", true, "valid", false},
		{"positive: bic keyword", "bic validate: DEUTDEFF", true, "valid", false},
		{"positive: swift bic keyword", "swift bic validate: DEUTDEFF", true, "valid", false},
		{"positive: with spaces", "swift validate: DEUT DE FF 500", true, "valid", false},
		{"positive: lowercase input", "swift validate: deutdeff500", true, "valid", false},
		{"positive: US bank", "swift validate: BOFAUS3N", true, "valid", false},
		{"positive: GB bank", "swift validate: BARCGB22", true, "valid", false},
		{"negative: too short", "swift validate: DEUTDEF", true, "invalid: SWIFT/BIC must be 8 or 11 characters", false},
		{"negative: too long", "swift validate: DEUTDEFF5001", true, "invalid: SWIFT/BIC must be 8 or 11 characters", false},
		{"negative: bad country", "swift validate: DEUTXXFF", true, "invalid: country code", false},
		{"negative: digits in bank", "swift validate: DE1TDEFF", true, "invalid: SWIFT/BIC must be uppercase letters", false},
		{"negative: empty", "swift validate:", true, "invalid: empty input", false},
		{"negative: unrelated text", "uuid v4 generate", false, "", false},
	})

	// Spot-check the SWIFT skill directly with the well-known DEUTDEFF500.
	s := NewSWIFTBICValidateSkill()
	out, err := s.Execute(context.Background(), "swift validate: DEUTDEFF500")
	if err != nil {
		t.Fatalf("Execute DEUTDEFF500: %v", err)
	}
	if out != "valid" {
		t.Errorf("DEUTDEFF500: got %q, want valid", out)
	}
}
