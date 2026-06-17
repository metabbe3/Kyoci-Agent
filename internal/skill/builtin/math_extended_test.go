package builtin

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// =====================================================================================
// Math skill tests — 12 skills (stats, gcd, lcm, is_prime, prime_factors,
// factorial, base_convert, round_sig, units_convert, currency_format,
// percentage, ratio_simplify).
// =====================================================================================

func TestStatsSkill(t *testing.T) {
	skill := NewStatsSkill()
	if !skill.Match("stats for 1 2 3 4 5") {
		t.Error("expected match")
	}
	out, err := skill.Execute(context.Background(), "stats for 1 2 3 4 5")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// mean of 1-5 = 3, median = 3, max = 5, min = 1
	for _, want := range []string{"count:", "mean:", "median:", "stddev:"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %s field, got %q", want, out)
		}
	}
	if !strings.Contains(out, "3") {
		t.Errorf("expected mean=3 represented somewhere, got %q", out)
	}
}

func TestGCDSkill(t *testing.T) {
	skill := NewGCDSkill()
	if !skill.Match("gcd of 12 18") {
		t.Error("expected match")
	}
	out, err := skill.Execute(context.Background(), "gcd of 12 18")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "6") {
		t.Errorf("gcd(12,18) should be 6, got %q", out)
	}
}

func TestLCMSkill(t *testing.T) {
	skill := NewLCMSkill()
	if !skill.Match("lcm of 4 6") {
		t.Error("expected match")
	}
	out, err := skill.Execute(context.Background(), "lcm of 4 6")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "12") {
		t.Errorf("lcm(4,6) should be 12, got %q", out)
	}
}

func TestIsPrimeSkill(t *testing.T) {
	skill := NewIsPrimeSkill()
	if !skill.Match("is prime 17") {
		t.Error("expected match")
	}
	cases := []struct {
		n    int
		want string
	}{
		{2, "prime"}, {3, "prime"}, {7, "prime"}, {17, "prime"},
		{4, "composite"}, {9, "composite"}, {15, "composite"}, {1, "composite"},
	}
	for _, tc := range cases {
		t.Run("", func(t *testing.T) {
			out, err := skill.Execute(context.Background(), fmt.Sprintf("is prime %d", tc.n))
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if !strings.Contains(out, tc.want) {
				t.Errorf("is_prime(%d) should be %s, got %q", tc.n, tc.want, out)
			}
		})
	}
}

func TestPrimeFactorsSkill(t *testing.T) {
	skill := NewPrimeFactorsSkill()
	if !skill.Match("prime factors of 60") {
		t.Error("expected match")
	}
	out, err := skill.Execute(context.Background(), "prime factors of 60")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// 60 = 2 × 2 × 3 × 5
	for _, want := range []string{"2", "3", "5"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected factor %s, got %q", want, out)
		}
	}
}

func TestFactorialSkill(t *testing.T) {
	skill := NewFactorialSkill()
	if !skill.Match("factorial of 5") {
		t.Error("expected match")
	}
	out, err := skill.Execute(context.Background(), "factorial of 5")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "120") {
		t.Errorf("5! should be 120, got %q", out)
	}
}

func TestBaseConvertSkill(t *testing.T) {
	skill := NewBaseConvertSkill()
	if !skill.Match("base_convert ff hex dec") {
		t.Error("expected match")
	}
	cases := []struct {
		query string
		want  string
	}{
		{"bin to dec: 1010", "10"},
		{"dec to hex: 255", "ff"},
		{"hex to dec: ff", "255"},
		{"dec to bin: 8", "1000"},
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			out, err := skill.Execute(context.Background(), tc.query)
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if !strings.Contains(strings.ToLower(out), tc.want) {
				t.Errorf("expected %q, got %q", tc.want, out)
			}
		})
	}
}

func TestRoundSigSkill(t *testing.T) {
	skill := NewRoundSigSkill()
	if !skill.Match("round_sig 3.14159 3") {
		t.Error("expected match")
	}
	out, err := skill.Execute(context.Background(), "round_sig 3.14159 3")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "3.14") {
		t.Errorf("expected 3.14 (3 sig figs), got %q", out)
	}
}

func TestUnitsConvertSkill(t *testing.T) {
	skill := NewUnitsConvertSkill()
	if !skill.Match("units convert c to f: 100") {
		t.Error("expected match")
	}
	cases := []struct {
		query string
		want  string
	}{
		{"c to f: 0", "32"},
		{"c to f: 100", "212"},
		{"f to c: 32", "0"},
		{"b to kb: 1024", "1"},
		{"kb to mb: 2048", "2"},
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			out, err := skill.Execute(context.Background(), "units convert "+tc.query)
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if !strings.Contains(out, tc.want) {
				t.Errorf("expected %q in output, got %q", tc.want, out)
			}
		})
	}
}

func TestCurrencyFormatSkill(t *testing.T) {
	skill := NewCurrencyFormatSkill()
	if !skill.Match("currency format usd 1234.5") {
		t.Error("expected match")
	}
	out, err := skill.Execute(context.Background(), "currency format usd 1234.5")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "$") {
		t.Errorf("expected $ symbol for USD, got %q", out)
	}
}

func TestPercentageSkill(t *testing.T) {
	skill := NewPercentageSkill()
	if !skill.Match("percentage of 20 from 80") {
		t.Error("expected match for 'percentage of ... from ...'")
	}
	// 20 (in 0-100 range) is treated as "20% of 80" = 16.
	out, err := skill.Execute(context.Background(), "percentage of 20 from 80")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "16") {
		t.Errorf("20%% of 80 = 16, got %q", out)
	}
	// Larger number triggers "X is what % of Y" path: 200 of 50 → 400%.
	out2, _ := skill.Execute(context.Background(), "percentage of 200 from 50")
	if !strings.Contains(out2, "400") {
		t.Errorf("200 is 400%% of 50, got %q", out2)
	}
}

func TestRatioSimplifySkill(t *testing.T) {
	skill := NewRatioSimplifySkill()
	if !skill.Match("simplify ratio 12:8") {
		t.Error("expected match")
	}
	out, err := skill.Execute(context.Background(), "simplify ratio 12:8")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "3:2") {
		t.Errorf("12:8 simplified = 3:2, got %q", out)
	}
}
