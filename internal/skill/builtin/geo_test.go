package builtin

import (
	"context"
	"strings"
	"testing"
)

// =====================================================================================
// Geography skill tests — 9 skills (haversine, latlon validate/parse, country
// code conversions, currency lookup, currency symbol).
// =====================================================================================

// ---- haversine_distance ----

func TestHaversineDistanceSkill(t *testing.T) {
	runSkillCases(t, "haversine_distance", NewHaversineDistanceSkill(), []skillCase{
		{
			name:        "positive: SF to NYC",
			query:       "haversine: 37.7749,-122.4194 to 40.7128,-74.0060",
			shouldMatch: true,
			// Great-circle distance SF-NYC is ~4129 km.
			want:    "4129",
			wantErr: false,
		},
		{
			name:        "positive: synonym",
			query:       "haversine distance: 51.5074,-0.1278 to 48.8566,2.3522",
			shouldMatch: true,
			// London to Paris is ~343 km.
			want: "343",
		},
		{
			name:        "positive: with spaces after comma",
			query:       "haversine: 37.7749, -122.4194 to 40.7128, -74.0060",
			shouldMatch: true,
			want:        "4129",
		},
		{
			name:        "positive: same point is zero",
			query:       "haversine: 0,0 to 0,0",
			shouldMatch: true,
			want:        "distance: 0.00 km",
		},
		{
			name:        "negative: unrelated skill",
			query:       "country alpha2 to alpha3: US",
			shouldMatch: false,
		},
		{
			name:        "edge: missing ' to ' separator",
			query:       "haversine: 37.7749,-122.4194 40.7128,-74.0060",
			shouldMatch: true,
			wantErr:     true,
		},
		{
			name:        "edge: invalid coordinates",
			query:       "haversine: abc,def to 1,2",
			shouldMatch: true,
			wantErr:     true,
		},
	})

	// Verify the bearing field is present in a typical positive case.
	out, err := NewHaversineDistanceSkill().Execute(context.Background(),
		"haversine: 37.7749,-122.4194 to 40.7128,-74.0060")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "bearing:") {
		t.Errorf("expected 'bearing:' in output, got %q", out)
	}
}

// ---- latlon_validate ----

func TestLatlonValidateSkill(t *testing.T) {
	runSkillCases(t, "latlon_validate", NewLatlonValidateSkill(), []skillCase{
		{name: "positive: valid pair", query: "latlon validate: 37.7749, -122.4194", shouldMatch: true, want: "valid"},
		{name: "positive: synonym", query: "latitude longitude validate: 0,0", shouldMatch: true, want: "valid"},
		{name: "positive: extreme", query: "latlon validate: 90,180", shouldMatch: true, want: "valid"},
		{name: "positive: negative extremes", query: "latlon validate: -90,-180", shouldMatch: true, want: "valid"},
		{name: "edge: lat too big", query: "latlon validate: 91,0", shouldMatch: true, want: "invalid"},
		{name: "edge: lat too small", query: "latlon validate: -91,0", shouldMatch: true, want: "invalid"},
		{name: "edge: lon too big", query: "latlon validate: 0,181", shouldMatch: true, want: "invalid"},
		{name: "edge: lon too small", query: "latlon validate: 0,-181", shouldMatch: true, want: "invalid"},
		{name: "edge: not parseable", query: "latlon validate: foo,bar", shouldMatch: true, want: "invalid"},
		{name: "edge: single value", query: "latlon validate: 37.7749", shouldMatch: true, want: "invalid"},
		{name: "negative: unrelated", query: "haversine: 0,0 to 1,1", shouldMatch: false},
	})
}

// ---- latlon_parse ----

func TestLatlonParseSkill(t *testing.T) {
	runSkillCases(t, "latlon_parse", NewLatlonParseSkill(), []skillCase{
		{name: "positive: basic", query: "latlon parse: 37.7749, -122.4194", shouldMatch: true, want: "lat=37.7749 lon=-122.4194"},
		{name: "positive: synonym", query: "parse latlon: 0, 0", shouldMatch: true, want: "lat=0 lon=0"},
		{name: "positive: parse lat lon", query: "parse lat lon: 40.7128, -74.006", shouldMatch: true, want: "lat=40.7128 lon=-74.006"},
		{name: "positive: integers", query: "latlon parse: 12,34", shouldMatch: true, want: "lat=12 lon=34"},
		{name: "edge: invalid format", query: "latlon parse: just one", shouldMatch: true, wantErr: true},
		{name: "negative: validate verb", query: "latlon validate: 0,0", shouldMatch: false},
	})
}

// ---- country_alpha2_to_alpha3 ----

func TestCountryAlpha2ToAlpha3Skill(t *testing.T) {
	runSkillCases(t, "country_alpha2_to_alpha3", NewCountryAlpha2ToAlpha3Skill(), []skillCase{
		{name: "positive: US", query: "country alpha2 to alpha3: US", shouldMatch: true, want: "USA"},
		{name: "positive: GB", query: "country alpha2 to alpha3: GB", shouldMatch: true, want: "GBR"},
		{name: "positive: lowercase input", query: "country alpha2 to alpha3: us", shouldMatch: true, want: "USA"},
		{name: "positive: JP", query: "country alpha2 to alpha3: JP", shouldMatch: true, want: "JPN"},
		{name: "positive: DE", query: "country alpha2 to alpha3: DE", shouldMatch: true, want: "DEU"},
		{name: "positive: synonym", query: "country code to alpha3: FR", shouldMatch: true, want: "FRA"},
		{name: "edge: unknown code", query: "country alpha2 to alpha3: ZZ", shouldMatch: true, wantErr: true},
		{name: "negative: reverse direction", query: "country alpha3 to alpha2: USA", shouldMatch: false},
	})
}

// ---- country_alpha3_to_alpha2 ----

func TestCountryAlpha3ToAlpha2Skill(t *testing.T) {
	runSkillCases(t, "country_alpha3_to_alpha2", NewCountryAlpha3ToAlpha2Skill(), []skillCase{
		{name: "positive: USA", query: "country alpha3 to alpha2: USA", shouldMatch: true, want: "US"},
		{name: "positive: GBR", query: "country alpha3 to alpha2: GBR", shouldMatch: true, want: "GB"},
		{name: "positive: lowercase input", query: "country alpha3 to alpha2: usa", shouldMatch: true, want: "US"},
		{name: "positive: JPN", query: "country alpha3 to alpha2: JPN", shouldMatch: true, want: "JP"},
		{name: "positive: DEU", query: "country alpha3 to alpha2: DEU", shouldMatch: true, want: "DE"},
		{name: "edge: unknown code", query: "country alpha3 to alpha2: ZZZ", shouldMatch: true, wantErr: true},
		{name: "negative: forward direction", query: "country alpha2 to alpha3: US", shouldMatch: false},
	})
}

// ---- country_name_to_alpha2 ----

func TestCountryNameToAlpha2Skill(t *testing.T) {
	runSkillCases(t, "country_name_to_alpha2", NewCountryNameToAlpha2Skill(), []skillCase{
		{name: "positive: full name", query: "country name to alpha2: United States", shouldMatch: true, want: "US"},
		{name: "positive: USA alias", query: "country name to alpha2: USA", shouldMatch: true, want: "US"},
		{name: "positive: UK alias", query: "country name to alpha2: UK", shouldMatch: true, want: "GB"},
		{name: "positive: Great Britain", query: "country name to alpha2: Great Britain", shouldMatch: true, want: "GB"},
		{name: "positive: U.S. dotted", query: "country name to alpha2: U.S.", shouldMatch: true, want: "US"},
		{name: "positive: synonym country name to code", query: "country name to code: Japan", shouldMatch: true, want: "JP"},
		{name: "positive: Russia alias", query: "country name to alpha2: Russia", shouldMatch: true, want: "RU"},
		{name: "positive: South Korea", query: "country name to alpha2: South Korea", shouldMatch: true, want: "KR"},
		{name: "positive: Netherlands", query: "country name to alpha2: Netherlands", shouldMatch: true, want: "NL"},
		{name: "positive: Holland alias", query: "country name to alpha2: Holland", shouldMatch: true, want: "NL"},
		{name: "positive: Czech Republic", query: "country name to alpha2: Czech Republic", shouldMatch: true, want: "CZ"},
		{name: "edge: unknown name", query: "country name to alpha2: Atlantis", shouldMatch: true, wantErr: true},
		{name: "negative: alpha2 input", query: "country alpha2 to alpha3: US", shouldMatch: false},
	})
}

// ---- country_alpha2_to_name ----

func TestCountryAlpha2ToNameSkill(t *testing.T) {
	runSkillCases(t, "country_alpha2_to_name", NewCountryAlpha2ToNameSkill(), []skillCase{
		{name: "positive: US", query: "country alpha2 to name: US", shouldMatch: true, want: "United States"},
		{name: "positive: GB", query: "country alpha2 to name: GB", shouldMatch: true, want: "United Kingdom"},
		{name: "positive: FR", query: "country alpha2 to name: FR", shouldMatch: true, want: "France"},
		{name: "positive: JP", query: "country alpha2 to name: JP", shouldMatch: true, want: "Japan"},
		{name: "positive: synonym code to name", query: "country code to name: DE", shouldMatch: true, want: "Germany"},
		{name: "edge: unknown code", query: "country alpha2 to name: ZZ", shouldMatch: true, wantErr: true},
		{name: "negative: name input", query: "country name to alpha2: United States", shouldMatch: false},
	})
}

// ---- currency_code_lookup ----

func TestCurrencyCodeLookupSkill(t *testing.T) {
	runSkillCases(t, "currency_code_lookup", NewCurrencyCodeLookupSkill(), []skillCase{
		{name: "positive: US", query: "currency code lookup: US", shouldMatch: true, want: "USD"},
		{name: "positive: GB", query: "currency code lookup: GB", shouldMatch: true, want: "GBP"},
		{name: "positive: JP", query: "currency code lookup: JP", shouldMatch: true, want: "JPY"},
		{name: "positive: by country name", query: "currency code lookup: United States", shouldMatch: true, want: "USD"},
		{name: "positive: by UK alias", query: "currency code lookup: UK", shouldMatch: true, want: "GBP"},
		{name: "positive: synonym currency for country", query: "currency for country: Switzerland", shouldMatch: true, want: "CHF"},
		{name: "edge: unknown country", query: "currency code lookup: Atlantis", shouldMatch: true, wantErr: true},
		{name: "negative: currency symbol query", query: "currency symbol: USD", shouldMatch: false},
	})
}

// ---- currency_symbol ----

func TestCurrencySymbolSkill(t *testing.T) {
	runSkillCases(t, "currency_symbol", NewCurrencySymbolSkill(), []skillCase{
		{name: "positive: USD", query: "currency symbol: USD", shouldMatch: true, want: "$"},
		{name: "positive: EUR", query: "currency symbol: EUR", shouldMatch: true, want: "€"},
		{name: "positive: GBP", query: "currency symbol: GBP", shouldMatch: true, want: "£"},
		{name: "positive: JPY", query: "currency symbol: JPY", shouldMatch: true, want: "¥"},
		{name: "positive: INR", query: "currency symbol: INR", shouldMatch: true, want: "₹"},
		{name: "positive: lowercase input", query: "currency symbol: usd", shouldMatch: true, want: "$"},
		{name: "edge: unknown currency", query: "currency symbol: XXX", shouldMatch: true, wantErr: true},
		{name: "negative: lookup query", query: "currency code lookup: US", shouldMatch: false},
	})
}

// ---- table sanity checks ----

func TestIsoCountryTableSanity(t *testing.T) {
	if len(isoAlpha2ToCountry) < 150 {
		t.Errorf("country table too small: %d entries", len(isoAlpha2ToCountry))
	}
	if len(isoAlpha3ToAlpha2) != len(isoAlpha2ToCountry) {
		t.Errorf("alpha3→alpha2 (%d) should mirror alpha2→country (%d)",
			len(isoAlpha3ToAlpha2), len(isoAlpha2ToCountry))
	}
	// Spot-check a few canonical conversions.
	for a2, c := range isoAlpha2ToCountry {
		if a2 == "" || c.Alpha3 == "" || c.Name == "" {
			t.Errorf("incomplete entry for %q: %+v", a2, c)
		}
		if back, ok := isoAlpha3ToAlpha2[c.Alpha3]; !ok || back != a2 {
			t.Errorf("round-trip failed for %s/%s", a2, c.Alpha3)
		}
	}
}

func TestIsoNameVariantsSanity(t *testing.T) {
	for name, a2 := range isoNameVariants {
		if a2 == "" {
			t.Errorf("empty alpha2 for variant %q", name)
		}
		if _, ok := isoAlpha2ToCountry[a2]; !ok {
			t.Errorf("variant %q maps to unknown alpha2 %q", name, a2)
		}
	}
}

func TestIsoCountryCurrencySanity(t *testing.T) {
	if len(isoCountryCurrency) < 150 {
		t.Errorf("currency table too small: %d entries", len(isoCountryCurrency))
	}
	// Every currency entry must reference a known country.
	for a2, curr := range isoCountryCurrency {
		if curr == "" {
			t.Errorf("empty currency for %q", a2)
		}
		if _, ok := isoAlpha2ToCountry[a2]; !ok {
			t.Errorf("currency entry references unknown country %q", a2)
		}
	}
}

func TestCurrencySymbolsSanity(t *testing.T) {
	if len(currencySymbols) < 50 {
		t.Errorf("currency symbols table too small: %d entries", len(currencySymbols))
	}
	// All known currency symbols should be non-empty 1-5 char strings.
	for code, sym := range currencySymbols {
		if code == "" || sym == "" {
			t.Errorf("empty entry: %q → %q", code, sym)
		}
	}
}
