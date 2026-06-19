package builtin

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// =====================================================================================
// Barcode / identifier validation skills — EAN-13, EAN-8, UPC-A, ISSN, VIN, SWIFT/BIC.
//
// Pure Go, stdlib only. No LLM/network. Each skill normalizes its payload (strips
// spaces and hyphens), runs the canonical check-digit algorithm, and returns
// "valid" or "invalid: <reason>".
// =====================================================================================

// stripBarcodeNoise removes spaces, hyphens, dots, and tabs from a payload to
// recover the raw barcode. Callers should also strip quotes via quoteStripped.
func stripBarcodeNoise(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Map(func(r rune) rune {
		switch r {
		case ' ', '-', '.', '\t', '\n', '\r':
			return -1
		}
		return r
	}, s)
	return s
}

// isAllDigits reports whether s is non-empty and consists solely of ASCII digits.
func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// mod10CheckDigit computes the GS1/EAN/UPC mod-10 check digit for the payload
// (without check digit). Weights alternate 1,3 starting with weight 3 on the
// rightmost digit. Sum, mod 10, subtract from 10, mod 10 again.
func mod10CheckDigit(payload string) (int, error) {
	if !isAllDigits(payload) {
		return 0, fmt.Errorf("non-digit payload")
	}
	sum := 0
	weight := 3
	for i := len(payload) - 1; i >= 0; i-- {
		d := int(payload[i] - '0')
		sum += d * weight
		if weight == 3 {
			weight = 1
		} else {
			weight = 3
		}
	}
	return (10 - (sum % 10)) % 10, nil
}

// ---- EAN-13 ----

type EAN13ValidateSkill struct{ *kyoci.BaseSkill }

func NewEAN13ValidateSkill() *EAN13ValidateSkill {
	return &EAN13ValidateSkill{BaseSkill: kyoci.NewBaseSkill(
		"ean13_validate", "Validate an EAN-13 barcode (13 digits, mod-10 check digit)",
		[]string{"ean13", "ean 13", "ean13 validate"},
	)}
}

func (s *EAN13ValidateSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "ean13") || strings.Contains(q, "ean 13")
}

func (s *EAN13ValidateSkill) Execute(_ context.Context, q string) (string, error) {
	raw := stripBarcodeNoise(quoteStripped(extractPayload(q)))
	if raw == "" {
		return "invalid: empty input", nil
	}
	if !isAllDigits(raw) {
		return fmt.Sprintf("invalid: non-digit characters in %q", raw), nil
	}
	if len(raw) != 13 {
		return fmt.Sprintf("invalid: EAN-13 must be 13 digits, got %d", len(raw)), nil
	}
	want, err := mod10CheckDigit(raw[:12])
	if err != nil {
		return fmt.Sprintf("invalid: %v", err), nil
	}
	got := int(raw[12] - '0')
	if want != got {
		return fmt.Sprintf("invalid: check digit mismatch (expected %d, got %d)", want, got), nil
	}
	return "valid", nil
}

// ---- EAN-8 ----

type EAN8ValidateSkill struct{ *kyoci.BaseSkill }

func NewEAN8ValidateSkill() *EAN8ValidateSkill {
	return &EAN8ValidateSkill{BaseSkill: kyoci.NewBaseSkill(
		"ean8_validate", "Validate an EAN-8 barcode (8 digits, mod-10 check digit)",
		[]string{"ean8", "ean 8", "ean8 validate"},
	)}
}

func (s *EAN8ValidateSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "ean8") || strings.Contains(q, "ean 8")
}

func (s *EAN8ValidateSkill) Execute(_ context.Context, q string) (string, error) {
	raw := stripBarcodeNoise(quoteStripped(extractPayload(q)))
	if raw == "" {
		return "invalid: empty input", nil
	}
	if !isAllDigits(raw) {
		return fmt.Sprintf("invalid: non-digit characters in %q", raw), nil
	}
	if len(raw) != 8 {
		return fmt.Sprintf("invalid: EAN-8 must be 8 digits, got %d", len(raw)), nil
	}
	want, err := mod10CheckDigit(raw[:7])
	if err != nil {
		return fmt.Sprintf("invalid: %v", err), nil
	}
	got := int(raw[7] - '0')
	if want != got {
		return fmt.Sprintf("invalid: check digit mismatch (expected %d, got %d)", want, got), nil
	}
	return "valid", nil
}

// ---- UPC-A ----

type UPCAValidateSkill struct{ *kyoci.BaseSkill }

func NewUPCAValidateSkill() *UPCAValidateSkill {
	return &UPCAValidateSkill{BaseSkill: kyoci.NewBaseSkill(
		"upc_a_validate", "Validate a UPC-A barcode (12 digits, mod-10 check digit)",
		[]string{"upc", "upc-a", "upc validate"},
	)}
}

func (s *UPCAValidateSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "upc-a") || strings.Contains(q, "upc a") ||
		strings.Contains(q, "upca") || (strings.Contains(q, "upc") && !strings.Contains(q, "upc-e"))
}

func (s *UPCAValidateSkill) Execute(_ context.Context, q string) (string, error) {
	raw := stripBarcodeNoise(quoteStripped(extractPayload(q)))
	if raw == "" {
		return "invalid: empty input", nil
	}
	if !isAllDigits(raw) {
		return fmt.Sprintf("invalid: non-digit characters in %q", raw), nil
	}
	if len(raw) != 12 {
		return fmt.Sprintf("invalid: UPC-A must be 12 digits, got %d", len(raw)), nil
	}
	want, err := mod10CheckDigit(raw[:11])
	if err != nil {
		return fmt.Sprintf("invalid: %v", err), nil
	}
	got := int(raw[11] - '0')
	if want != got {
		return fmt.Sprintf("invalid: check digit mismatch (expected %d, got %d)", want, got), nil
	}
	return "valid", nil
}

// ---- ISSN ----

type ISSNValidateSkill struct{ *kyoci.BaseSkill }

func NewISSNValidateSkill() *ISSNValidateSkill {
	return &ISSNValidateSkill{BaseSkill: kyoci.NewBaseSkill(
		"issn_validate", "Validate an ISSN (8 chars, mod-11 check digit, last may be X)",
		[]string{"issn", "issn validate"},
	)}
}

func (s *ISSNValidateSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "issn")
}

func (s *ISSNValidateSkill) Execute(_ context.Context, q string) (string, error) {
	raw := strings.ToUpper(stripBarcodeNoise(quoteStripped(extractPayload(q))))
	if raw == "" {
		return "invalid: empty input", nil
	}
	if len(raw) != 8 {
		return fmt.Sprintf("invalid: ISSN must be 8 characters, got %d", len(raw)), nil
	}
	payload := raw[:7]
	if !isAllDigits(payload) {
		return fmt.Sprintf("invalid: first 7 characters must be digits in %q", raw), nil
	}
	last := raw[7]
	if !((last >= '0' && last <= '9') || last == 'X') {
		return fmt.Sprintf("invalid: check character must be a digit or X in %q", raw), nil
	}
	// ISSN check digit: weights 8,7,6,5,4,3,2 on the first 7 digits, sum mod 11.
	sum := 0
	for i := 0; i < 7; i++ {
		sum += int(payload[i]-'0') * (8 - i)
	}
	rem := sum % 11
	want := 0
	if rem != 0 {
		want = 11 - rem
	}
	var got int
	if last == 'X' {
		got = 10
	} else {
		got = int(last - '0')
	}
	if want != got {
		wantStr := strconv.Itoa(want)
		if want == 10 {
			wantStr = "X"
		}
		return fmt.Sprintf("invalid: check digit mismatch (expected %s, got %s)", wantStr, string(last)), nil
	}
	return "valid", nil
}

// ---- VIN ----

var vinTranslit = map[byte]int{
	'A': 1, 'B': 2, 'C': 3, 'D': 4, 'E': 5, 'F': 6, 'G': 7, 'H': 8,
	'J': 1, 'K': 2, 'L': 3, 'M': 4, 'N': 5, 'P': 7, 'R': 9,
	'S': 2, 'T': 3, 'U': 4, 'V': 5, 'W': 6, 'X': 7, 'Y': 8, 'Z': 9,
	'0': 0, '1': 1, '2': 2, '3': 3, '4': 4, '5': 5, '6': 6, '7': 7, '8': 8, '9': 9,
}

var vinWeights = [17]int{8, 7, 6, 5, 4, 3, 2, 10, 0, 9, 8, 7, 6, 5, 4, 3, 2}

type VINValidateSkill struct{ *kyoci.BaseSkill }

func NewVINValidateSkill() *VINValidateSkill {
	return &VINValidateSkill{BaseSkill: kyoci.NewBaseSkill(
		"vin_validate", "Validate a VIN (17 chars, ISO 3779 mod-11 check digit)",
		[]string{"vin", "vin validate", "vehicle identification number"},
	)}
}

func (s *VINValidateSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "vin ") || strings.Contains(q, "vin:") ||
		strings.Contains(q, "vin validate") || strings.Contains(q, "vehicle identification number") ||
		strings.HasSuffix(q, "vin")
}

func (s *VINValidateSkill) Execute(_ context.Context, q string) (string, error) {
	raw := strings.ToUpper(stripBarcodeNoise(quoteStripped(extractPayload(q))))
	if raw == "" {
		return "invalid: empty input", nil
	}
	if len(raw) != 17 {
		return fmt.Sprintf("invalid: VIN must be 17 characters, got %d", len(raw)), nil
	}
	sum := 0
	for i := 0; i < 17; i++ {
		c := raw[i]
		v, ok := vinTranslit[c]
		if !ok {
			return fmt.Sprintf("invalid: illegal character %q at position %d", string(c), i+1), nil
		}
		sum += v * vinWeights[i]
	}
	checkChar := raw[8]
	var got int
	if checkChar >= '0' && checkChar <= '9' {
		got = int(checkChar - '0')
	} else if checkChar == 'X' {
		got = 10
	} else {
		return fmt.Sprintf("invalid: check digit at position 9 must be 0-9 or X, got %q", string(checkChar)), nil
	}
	want := sum % 11
	if want == 10 {
		if got != 10 {
			return fmt.Sprintf("invalid: check digit mismatch (expected X, got %s)", string(checkChar)), nil
		}
		return "valid", nil
	}
	if want != got {
		return fmt.Sprintf("invalid: check digit mismatch (expected %d, got %s)", want, string(checkChar)), nil
	}
	return "valid", nil
}

// ---- SWIFT / BIC ----

// validISOAlpha2 is a complete list of ISO 3166-1 alpha-2 country codes.
// Inline (no external dep) keeps the skill stdlib-only.
var validISOAlpha2 = map[string]bool{
	"AD": true, "AE": true, "AF": true, "AG": true, "AI": true, "AL": true, "AM": true, "AO": true,
	"AQ": true, "AR": true, "AS": true, "AT": true, "AU": true, "AW": true, "AX": true, "AZ": true,
	"BA": true, "BB": true, "BD": true, "BE": true, "BF": true, "BG": true, "BH": true, "BI": true,
	"BJ": true, "BL": true, "BM": true, "BN": true, "BO": true, "BQ": true, "BR": true, "BS": true,
	"BT": true, "BV": true, "BW": true, "BY": true, "BZ": true, "CA": true, "CC": true, "CD": true,
	"CF": true, "CG": true, "CH": true, "CI": true, "CK": true, "CL": true, "CM": true, "CN": true,
	"CO": true, "CR": true, "CU": true, "CV": true, "CW": true, "CX": true, "CY": true, "CZ": true,
	"DE": true, "DJ": true, "DK": true, "DM": true, "DO": true, "DZ": true, "EC": true, "EE": true,
	"EG": true, "EH": true, "ER": true, "ES": true, "ET": true, "FI": true, "FJ": true, "FK": true,
	"FM": true, "FO": true, "FR": true, "GA": true, "GB": true, "GD": true, "GE": true, "GF": true,
	"GG": true, "GH": true, "GI": true, "GL": true, "GM": true, "GN": true, "GP": true, "GQ": true,
	"GR": true, "GS": true, "GT": true, "GU": true, "GW": true, "GY": true, "HK": true, "HM": true,
	"HN": true, "HR": true, "HT": true, "HU": true, "ID": true, "IE": true, "IL": true, "IM": true,
	"IN": true, "IO": true, "IQ": true, "IR": true, "IS": true, "IT": true, "JE": true, "JM": true,
	"JO": true, "JP": true, "KE": true, "KG": true, "KH": true, "KI": true, "KM": true, "KN": true,
	"KP": true, "KR": true, "KW": true, "KY": true, "KZ": true, "LA": true, "LB": true, "LC": true,
	"LI": true, "LK": true, "LR": true, "LS": true, "LT": true, "LU": true, "LV": true, "LY": true,
	"MA": true, "MC": true, "MD": true, "ME": true, "MF": true, "MG": true, "MH": true, "MK": true,
	"ML": true, "MM": true, "MN": true, "MO": true, "MP": true, "MQ": true, "MR": true, "MS": true,
	"MT": true, "MU": true, "MV": true, "MW": true, "MX": true, "MY": true, "MZ": true, "NA": true,
	"NC": true, "NE": true, "NF": true, "NG": true, "NI": true, "NL": true, "NO": true, "NP": true,
	"NR": true, "NU": true, "NZ": true, "OM": true, "PA": true, "PE": true, "PF": true, "PG": true,
	"PH": true, "PK": true, "PL": true, "PM": true, "PN": true, "PR": true, "PS": true, "PT": true,
	"PW": true, "PY": true, "QA": true, "RE": true, "RO": true, "RS": true, "RU": true, "RW": true,
	"SA": true, "SB": true, "SC": true, "SD": true, "SE": true, "SG": true, "SH": true, "SI": true,
	"SJ": true, "SK": true, "SL": true, "SM": true, "SN": true, "SO": true, "SR": true, "SS": true,
	"ST": true, "SV": true, "SX": true, "SY": true, "SZ": true, "TC": true, "TD": true, "TF": true,
	"TG": true, "TH": true, "TJ": true, "TK": true, "TL": true, "TM": true, "TN": true, "TO": true,
	"TR": true, "TT": true, "TV": true, "TW": true, "TZ": true, "UA": true, "UG": true, "UM": true,
	"US": true, "UY": true, "UZ": true, "VA": true, "VC": true, "VE": true, "VG": true, "VI": true,
	"VN": true, "VU": true, "WF": true, "WS": true, "YE": true, "YT": true, "ZA": true, "ZM": true,
	"ZW": true,
}

type SWIFTBICValidateSkill struct{ *kyoci.BaseSkill }

func NewSWIFTBICValidateSkill() *SWIFTBICValidateSkill {
	return &SWIFTBICValidateSkill{BaseSkill: kyoci.NewBaseSkill(
		"swift_bic_validate", "Validate a SWIFT/BIC code (8 or 11 chars, ISO 9362)",
		[]string{"swift", "bic", "swift bic", "swift validate"},
	)}
}

func (s *SWIFTBICValidateSkill) Match(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "swift") || strings.Contains(q, "bic")
}

func (s *SWIFTBICValidateSkill) Execute(_ context.Context, q string) (string, error) {
	raw := strings.ToUpper(stripBarcodeNoise(quoteStripped(extractPayload(q))))
	if raw == "" {
		return "invalid: empty input", nil
	}
	if len(raw) != 8 && len(raw) != 11 {
		return fmt.Sprintf("invalid: SWIFT/BIC must be 8 or 11 characters, got %d", len(raw)), nil
	}
	// Per ISO 9362: bank code (4) is uppercase letters only; country (2) is
	// ISO 3166-1 alpha-2; location (2) and optional branch (3) are alphanumeric.
	bank := raw[:4]
	country := raw[4:6]
	location := raw[6:8]
	if !isAllUpperAlpha(bank) {
		return fmt.Sprintf("invalid: SWIFT/BIC must be uppercase letters, got %q", bank), nil
	}
	if !validISOAlpha2[country] {
		return fmt.Sprintf("invalid: country code %q is not a valid ISO 3166-1 alpha-2 code", country), nil
	}
	if !isAllUpperAlnum(location) {
		return fmt.Sprintf("invalid: location code (chars 5-6) must be alphanumeric, got %q", location), nil
	}
	if len(raw) == 11 {
		branch := raw[8:11]
		if !isAllUpperAlnum(branch) {
			return fmt.Sprintf("invalid: branch code (chars 9-11) must be alphanumeric, got %q", branch), nil
		}
	}
	return "valid", nil
}

// isAllUpperAlpha reports whether s is non-empty and contains only A-Z.
func isAllUpperAlpha(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < 'A' || c > 'Z' {
			return false
		}
	}
	return true
}

// isAllUpperAlnum reports whether s is non-empty and contains only A-Z or 0-9.
func isAllUpperAlnum(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
			return false
		}
	}
	return true
}
