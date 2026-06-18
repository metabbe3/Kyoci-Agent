package builtin

import (
	"context"
	"strings"
	"testing"
)

// =====================================================================================
// Lookup-table skill tests — 15 skills, each returning a static reference table.
//
// Each test verifies:
//   - Match() returns true for the canonical query and false for an unrelated one
//   - Execute() returns a non-empty string that contains the expected
//     reference entries (substring check, not exact match — the table format
//     may evolve without breaking tests).
// =====================================================================================

// ---- ISO country alpha2 ----

func TestISOCountryAlpha2List(t *testing.T) {
	runSkillCases(t, "iso_country_alpha2_list", NewISOCountryAlpha2ListSkill(), []skillCase{
		{"positive: iso country alpha2", "iso country alpha2", true, "US United States", false},
		{"positive: list countries alpha2", "list countries alpha2", true, "GB United Kingdom", false},
		{"positive: alpha2 country codes", "alpha2 country codes", true, "FR France", false},
		{"negative: alpha3", "iso country alpha3", false, "", false},
		{"negative: unrelated", "sha256 of hello", false, "", false},
	})
}

// ---- ISO country alpha3 ----

func TestISOCountryAlpha3List(t *testing.T) {
	runSkillCases(t, "iso_country_alpha3_list", NewISOCountryAlpha3ListSkill(), []skillCase{
		{"positive: iso country alpha3", "iso country alpha3", true, "USA United States", false},
		{"positive: list countries alpha3", "list countries alpha3", true, "GBR United Kingdom", false},
		{"positive: alpha3 country codes", "alpha3 country codes", true, "FRA France", false},
		{"negative: alpha2", "iso country alpha2", false, "", false},
		{"negative: unrelated", "uuid v4", false, "", false},
	})
}

// ---- ISO currency ----

func TestISOCurrencyList(t *testing.T) {
	runSkillCases(t, "iso_currency_list", NewISOCurrencyListSkill(), []skillCase{
		{"positive: iso currency", "iso currency list", true, "USD", false},
		{"positive: currency list", "currency list", true, "EUR", false},
		{"positive: list currencies", "list currencies", true, "JPY", false},
		{"positive: currency codes", "currency codes", true, "GBP", false},
		{"negative: unrelated", "encode base64: hi", false, "", false},
	})
}

// ---- ISO language alpha2 ----

func TestISOLanguageAlpha2List(t *testing.T) {
	runSkillCases(t, "iso_language_alpha2_list", NewISOLanguageAlpha2ListSkill(), []skillCase{
		{"positive: iso language", "iso language list", true, "en    English", false},
		{"positive: language alpha2", "language alpha2", true, "fr    French", false},
		{"positive: language codes", "language codes", true, "es    Spanish", false},
		{"positive: list languages", "list languages", true, "de    German", false},
		{"negative: currency", "iso currency list", false, "", false},
	})
}

// ---- HTTP status all ----

func TestHTTPStatusAll(t *testing.T) {
	skill := NewHTTPStatusAllSkill()
	ctx := context.Background()

	cases := []skillCase{
		{"positive: http status all", "http status all", true, "404   Not Found", false},
		{"positive: all http status", "all http status", true, "200   OK", false},
		{"positive: http status list", "http status list", true, "500   Internal Server Error", false},
		{"positive: list http status", "list http status", true, "418", false},
		{"negative: single status", "http status 200", false, "", false},
		{"negative: unrelated", "encode base64: hi", false, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := skill.Match(tc.query)
			if got != tc.shouldMatch {
				t.Errorf("Match(%q) = %v, want %v", tc.query, got, tc.shouldMatch)
				return
			}
			if !tc.shouldMatch {
				return
			}
			out, err := skill.Execute(ctx, tc.query)
			if err != nil {
				t.Errorf("Execute(%q): %v", tc.query, err)
				return
			}
			if tc.want != "" && !strings.Contains(out, tc.want) {
				t.Errorf("Execute(%q) = %q, want containing %q", tc.query, out, tc.want)
			}
			// Sanity: a full HTTP table should cover every class.
			for _, want := range []string{"100", "200", "301", "404", "500"} {
				if !strings.Contains(out, want) {
					t.Errorf("expected table to contain status %s", want)
				}
			}
		})
	}
}

// ---- MIME types ----

func TestMIMETypeCommon(t *testing.T) {
	skill := NewMIMETypeCommonSkill()
	ctx := context.Background()

	cases := []skillCase{
		{"positive: mime type common", "mime type common", true, "text/html", false},
		{"positive: common mime types", "common mime types", true, "application/json", false},
		{"positive: mime type list", "mime type list", true, "image/png", false},
		{"positive: list mime types", "list mime types", true, "application/pdf", false},
		{"negative: unrelated", "encode base64: hi", false, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := skill.Match(tc.query)
			if got != tc.shouldMatch {
				t.Errorf("Match(%q) = %v, want %v", tc.query, got, tc.shouldMatch)
				return
			}
			if !tc.shouldMatch {
				return
			}
			out, err := skill.Execute(ctx, tc.query)
			if err != nil {
				t.Errorf("Execute(%q): %v", tc.query, err)
				return
			}
			if tc.want != "" && !strings.Contains(out, tc.want) {
				t.Errorf("Execute(%q) = %q, want containing %q", tc.query, out, tc.want)
			}
			// Sanity: must include several core extensions.
			for _, ext := range []string{".txt", ".html", ".json", ".png", ".pdf"} {
				if !strings.Contains(out, ext) {
					t.Errorf("expected table to contain extension %s", ext)
				}
			}
		})
	}
}

// ---- HTML entities ----

func TestHTMLEntityCommon(t *testing.T) {
	skill := NewHTMLEntityCommonSkill()
	ctx := context.Background()

	cases := []skillCase{
		{"positive: html entity list", "html entity list", true, "&amp;", false},
		{"positive: html entity common", "html entity common", true, "&lt;", false},
		{"positive: html entities", "list of common html entities", true, "&copy;", false},
		{"negative: html escape", "html escape: <b>", false, "", false},
		{"negative: unrelated", "uuid v4", false, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := skill.Match(tc.query)
			if got != tc.shouldMatch {
				t.Errorf("Match(%q) = %v, want %v", tc.query, got, tc.shouldMatch)
				return
			}
			if !tc.shouldMatch {
				return
			}
			out, err := skill.Execute(ctx, tc.query)
			if err != nil {
				t.Errorf("Execute(%q): %v", tc.query, err)
				return
			}
			if tc.want != "" && !strings.Contains(out, tc.want) {
				t.Errorf("Execute(%q) = %q, want containing %q", tc.query, out, tc.want)
			}
			for _, want := range []string{"&amp;", "&lt;", "&gt;", "&quot;", "&nbsp;"} {
				if !strings.Contains(out, want) {
					t.Errorf("expected table to contain entity %s", want)
				}
			}
		})
	}
}

// ---- ASCII table ----

func TestASCIITable(t *testing.T) {
	skill := NewASCIITableSkill()
	ctx := context.Background()

	cases := []skillCase{
		{"positive: ascii table", "ascii table", true, "65", false}, // 'A'
		{"positive: ascii chart", "ascii chart", true, "20", false}, // hex of 32
		{"negative: charset", "charset of hello", false, "", false},
		{"negative: bare ascii", "ascii of hello", false, "", false},
		{"negative: unrelated", "uuid v4", false, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := skill.Match(tc.query)
			if got != tc.shouldMatch {
				t.Errorf("Match(%q) = %v, want %v", tc.query, got, tc.shouldMatch)
				return
			}
			if !tc.shouldMatch {
				return
			}
			out, err := skill.Execute(ctx, tc.query)
			if err != nil {
				t.Errorf("Execute(%q): %v", tc.query, err)
				return
			}
			if tc.want != "" && !strings.Contains(out, tc.want) {
				t.Errorf("Execute(%q) = %q, want containing %q", tc.query, out, tc.want)
			}
			// Sanity: ASCII table should cover all printable glyphs A-Z, a-z, 0-9.
			for _, want := range []string{"(space)", "A", "Z", "a", "z", "~"} {
				if !strings.Contains(out, want) {
					t.Errorf("expected ASCII table to contain %q", want)
				}
			}
		})
	}
}

// ---- UUID namespaces ----

func TestUUIDNamespaceDNS(t *testing.T) {
	runSkillCases(t, "uuid_namespace_dns", NewUUIDNamespaceDNSSkill(), []skillCase{
		{"positive: uuid namespace dns", "uuid namespace dns", true, "6ba7b810-9dad-11d1-80b4-00c04fd430c8", false},
		{"positive: namespace dns uuid", "namespace dns uuid", true, "6ba7b810", false},
		{"negative: url namespace", "uuid namespace url", false, "", false},
		{"negative: uuid v4", "uuid v4", false, "", false},
	})
}

func TestUUIDNamespaceURL(t *testing.T) {
	runSkillCases(t, "uuid_namespace_url", NewUUIDNamespaceURLSkill(), []skillCase{
		{"positive: uuid namespace url", "uuid namespace url", true, "6ba7b811-9dad-11d1-80b4-00c04fd430c8", false},
		{"positive: namespace url uuid", "namespace url uuid", true, "6ba7b811", false},
		{"negative: dns namespace", "uuid namespace dns", false, "", false},
		{"negative: uuid v4", "uuid v4", false, "", false},
	})
}

func TestUUIDNamespaceOID(t *testing.T) {
	runSkillCases(t, "uuid_namespace_oid", NewUUIDNamespaceOIDSkill(), []skillCase{
		{"positive: uuid namespace oid", "uuid namespace oid", true, "6ba7b812-9dad-11d1-80b4-00c04fd430c8", false},
		{"positive: namespace oid uuid", "namespace oid uuid", true, "6ba7b812", false},
		{"negative: dns namespace", "uuid namespace dns", false, "", false},
		{"negative: uuid v4", "uuid v4", false, "", false},
	})
}

func TestUUIDNamespaceX500(t *testing.T) {
	runSkillCases(t, "uuid_namespace_x500", NewUUIDNamespaceX500Skill(), []skillCase{
		{"positive: uuid namespace x500", "uuid namespace x500", true, "6ba7b814-9dad-11d1-80b4-00c04fd430c8", false},
		{"positive: namespace x500 uuid", "namespace x500 uuid", true, "6ba7b814", false},
		{"negative: dns namespace", "uuid namespace dns", false, "", false},
		{"negative: uuid v4", "uuid v4", false, "", false},
	})
}

// ---- Unix signals ----

func TestUnixSignalList(t *testing.T) {
	skill := NewUnixSignalListSkill()
	ctx := context.Background()

	cases := []skillCase{
		{"positive: unix signal", "unix signal list", true, "SIGTERM", false},
		{"positive: signal list", "signal list", true, "SIGKILL", false},
		{"positive: posix signals", "posix signals", true, "SIGINT", false},
		{"positive: list signals", "list signals", true, "SIGHUP", false},
		{"negative: unrelated", "uuid v4", false, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := skill.Match(tc.query)
			if got != tc.shouldMatch {
				t.Errorf("Match(%q) = %v, want %v", tc.query, got, tc.shouldMatch)
				return
			}
			if !tc.shouldMatch {
				return
			}
			out, err := skill.Execute(ctx, tc.query)
			if err != nil {
				t.Errorf("Execute(%q): %v", tc.query, err)
				return
			}
			if tc.want != "" && !strings.Contains(out, tc.want) {
				t.Errorf("Execute(%q) = %q, want containing %q", tc.query, out, tc.want)
			}
			// Sanity: must include both the numeric (1-31) range and the
			// most-cited signal names.
			for _, want := range []string{"1", "9", "15", "SIGINT", "SIGKILL", "SIGTERM", "SIGSEGV"} {
				if !strings.Contains(out, want) {
					t.Errorf("expected signal table to contain %q", want)
				}
			}
		})
	}
}

// ---- File signatures ----

func TestFileSignatureList(t *testing.T) {
	skill := NewFileSignatureListSkill()
	ctx := context.Background()

	cases := []skillCase{
		{"positive: file signature", "file signature list", true, "89504E47", false},
		{"positive: magic bytes", "magic bytes list", true, "FFD8FF", false},
		{"positive: file magic", "file magic", true, "25504446", false},
		{"positive: magic numbers", "magic numbers", true, "504B0304", false},
		{"negative: unrelated", "uuid v4", false, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := skill.Match(tc.query)
			if got != tc.shouldMatch {
				t.Errorf("Match(%q) = %v, want %v", tc.query, got, tc.shouldMatch)
				return
			}
			if !tc.shouldMatch {
				return
			}
			out, err := skill.Execute(ctx, tc.query)
			if err != nil {
				t.Errorf("Execute(%q): %v", tc.query, err)
				return
			}
			if tc.want != "" && !strings.Contains(out, tc.want) {
				t.Errorf("Execute(%q) = %q, want containing %q", tc.query, out, tc.want)
			}
			// Sanity: must include the most-cited signatures.
			for _, want := range []string{"89504E47", "FFD8FF", "25504446", "504B0304", "47494638"} {
				if !strings.Contains(out, want) {
					t.Errorf("expected signature table to contain %q", want)
				}
			}
		})
	}
}

// ---- Emoji shortcodes ----

func TestEmojiShortcodeList(t *testing.T) {
	skill := NewEmojiShortcodeListSkill()
	ctx := context.Background()

	cases := []skillCase{
		{"positive: emoji shortcode", "emoji shortcode list", true, ":smile:", false},
		{"positive: shortcode list", "shortcode list", true, ":heart:", false},
		{"positive: list shortcodes", "list shortcodes", true, ":thumbsup:", false},
		{"positive: emoji shortcodes", "list common emoji shortcodes", true, ":fire:", false},
		{"negative: emoji info", "emoji info", false, "", false},
		{"negative: unrelated", "uuid v4", false, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := skill.Match(tc.query)
			if got != tc.shouldMatch {
				t.Errorf("Match(%q) = %v, want %v", tc.query, got, tc.shouldMatch)
				return
			}
			if !tc.shouldMatch {
				return
			}
			out, err := skill.Execute(ctx, tc.query)
			if err != nil {
				t.Errorf("Execute(%q): %v", tc.query, err)
				return
			}
			if tc.want != "" && !strings.Contains(out, tc.want) {
				t.Errorf("Execute(%q) = %q, want containing %q", tc.query, out, tc.want)
			}
			// Sanity: must include the canonical shortcodes.
			for _, want := range []string{":smile:", ":heart:", ":thumbsup:", ":100:"} {
				if !strings.Contains(out, want) {
					t.Errorf("expected shortcode table to contain %q", want)
				}
			}
		})
	}
}

// ---- Sanity: lookupTableSkillNames covers all 15 constructors ----

func TestLookupTableSkillNames(t *testing.T) {
	names := lookupTableSkillNames()
	wantCount := 15
	if len(names) != wantCount {
		t.Fatalf("lookupTableSkillNames() has %d entries, want %d", len(names), wantCount)
	}
	// Spot-check that a few of the expected names are present.
	wantNames := map[string]bool{
		"NewISOCountryAlpha2ListSkill": true,
		"NewHTTPStatusAllSkill":        true,
		"NewASCIITableSkill":           true,
		"NewUUIDNamespaceDNSSkill":     true,
		"NewUnixSignalListSkill":       true,
		"NewFileSignatureListSkill":    true,
		"NewEmojiShortcodeListSkill":   true,
	}
	for _, n := range names {
		if !wantNames[n] {
			continue
		}
		delete(wantNames, n)
	}
	if len(wantNames) > 0 {
		t.Errorf("missing expected names: %v", wantNames)
	}
}
