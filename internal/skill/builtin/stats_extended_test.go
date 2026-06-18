package builtin

import (
	"context"
	"strings"
	"testing"
)

// =====================================================================================
// Extended statistics skill tests — variance, stddev_sample, percentile, quartile.
//
// Test vectors (documented in stats_extended.go):
//   - variance [1..5]            → 2            (population)
//   - sample stddev [1..5]       → 1.5811       (Bessel's, n-1)
//   - P90 of [1..10]             → 9.1          (linear interpolation)
//   - quartiles of [1..10]       → Q1 3.25, Q2 5.5, Q3 7.75  (Type 7 / NumPy default)
// =====================================================================================

// ---- variance ----

func TestVarianceSkill(t *testing.T) {
	runSkillCases(t, "variance", NewVarianceSkill(), []skillCase{
		{"positive: variance colon", "variance: 1 2 3 4 5", true, "2", false},
		{"positive: variance of", "variance of 1 2 3 4 5", true, "2", false},
		{"positive: comma-separated", "variance: 1, 2, 3, 4, 5", true, "2", false},
		{"positive: tab-separated", "variance:\t1\t2\t3\t4\t5", true, "2", false},
		{"positive: population variance label", "population variance: 1 2 3 4 5", true, "2", false},
		{"positive: larger set", "variance: 2 4 4 4 5 5 7 9", true, "4", false},
		{"edge: single value (variance 0)", "variance: 7", true, "0", false},
		{"edge: constant list (variance 0)", "variance: 5 5 5 5", true, "0", false},
		{"negative: no numbers", "variance:", true, "", true},
		{"negative: unrelated query", "encode base64: hello", false, "", false},
	})
}

// ---- stddev_sample ----

func TestStddevSampleSkill(t *testing.T) {
	runSkillCases(t, "stddev_sample", NewStddevSampleSkill(), []skillCase{
		{"positive: stddev sample colon", "stddev sample: 1 2 3 4 5", true, "1.5811", false},
		{"positive: sample stddev phrasing", "sample stddev: 1 2 3 4 5", true, "1.5811", false},
		{"positive: standard deviation sample", "standard deviation sample: 1 2 3 4 5", true, "1.5811", false},
		{"positive: sample standard deviation", "sample standard deviation: 1 2 3 4 5", true, "1.5811", false},
		{"positive: comma-separated", "stddev sample: 1, 2, 3, 4, 5", true, "1.5811", false},
		{"positive: differs from population (1.4142...)", "stddev sample: 1 2 3 4 5", true, "1.5811", false},
		{"edge: two identical values (0)", "stddev sample: 4 4", true, "0", false},
		{"edge: single value errors", "stddev sample: 5", true, "", true},
		{"negative: bare stddev does not match", "stddev: 1 2 3 4 5", false, "", false},
		{"negative: standard deviation alone does not match", "standard deviation: 1 2 3 4 5", false, "", false},
		{"negative: unrelated query", "encode base64: hello", false, "", false},
	})

	// Explicit check: population stddev of [1..5] is sqrt(2) ≈ 1.4142, so the
	// sample value 1.5811 must NOT appear as 1.4142.
	ctx := context.Background()
	out, err := NewStddevSampleSkill().Execute(ctx, "stddev sample: 1 2 3 4 5")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if strings.Contains(out, "1.4142") {
		t.Errorf("sample stddev returned population value %q (should be 1.5811...)", out)
	}
}

// ---- percentile ----

func TestPercentileSkill(t *testing.T) {
	runSkillCases(t, "percentile", NewPercentileSkill(), []skillCase{
		{"positive: P90 of 1..10", "percentile 90: 1 2 3 4 5 6 7 8 9 10", true, "9.1", false},
		{"positive: P50 == median", "percentile 50: 1 2 3 4 5 6 7 8 9 10", true, "5.5", false},
		{"positive: P0 == min", "percentile 0: 1 2 3 4 5 6 7 8 9 10", true, "1", false},
		{"positive: P100 == max", "percentile 100: 1 2 3 4 5 6 7 8 9 10", true, "10", false},
		{"positive: P25 == Q1", "percentile 25: 1 2 3 4 5 6 7 8 9 10", true, "3.25", false},
		{"positive: P75 == Q3", "percentile 75: 1 2 3 4 5 6 7 8 9 10", true, "7.75", false},
		{"positive: comma-separated", "percentile 90: 1, 2, 3, 4, 5, 6, 7, 8, 9, 10", true, "9.1", false},
		{"positive: tab-separated", "percentile 90:\t1\t2\t3\t4\t5\t6\t7\t8\t9\t10", true, "9.1", false},
		{"positive: small list P50", "percentile 50: 1 2 3 4 5", true, "3", false},
		{"positive: decimal percentile", "percentile 99.9: 1 2 3 4 5 6 7 8 9 10", true, "", false},
		{"edge: out of range", "percentile 150: 1 2 3 4 5", true, "", true},
		{"edge: negative percentile", "percentile -10: 1 2 3 4 5", true, "", true},
		{"edge: no numbers", "percentile 90:", true, "", true},
		{"edge: no percentile value", "percentile: 1 2 3 4 5", false, "", false},
		{"negative: unrelated query", "encode base64: hello", false, "", false},
	})
}

// ---- quartile ----

func TestQuartileSkill(t *testing.T) {
	skill := NewQuartileSkill()
	ctx := context.Background()

	// Table-driven match/no-match + Execute-substring checks.
	runSkillCases(t, "quartile", skill, []skillCase{
		{"positive: quartile keyword", "quartile: 1 2 3 4 5 6 7 8 9 10", true, "Q1: 3.25", false},
		{"positive: quartiles keyword", "quartiles: 1 2 3 4 5 6 7 8 9 10", true, "Q2: 5.5", false},
		{"positive: comma-separated", "quartile: 1, 2, 3, 4, 5, 6, 7, 8, 9, 10", true, "Q3: 7.75", false},
		{"positive: tab-separated", "quartile:\t1\t2\t3\t4\t5\t6\t7\t8\t9\t10", true, "5.5", false},
		{"positive: small odd list", "quartile: 1 2 3 4 5", true, "Q2: 3", false},
		{"edge: no numbers", "quartile:", true, "", true},
		{"negative: unrelated query", "encode base64: hello", false, "", false},
	})

	// Full output check: Q1=3.25, Q2=5.5, Q3=7.75 for [1..10].
	out, err := skill.Execute(ctx, "quartile: 1 2 3 4 5 6 7 8 9 10")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	want := "Q1: 3.25\nQ2: 5.5\nQ3: 7.75"
	if out != want {
		t.Errorf("Execute quartile [1..10] = %q, want %q", out, want)
	}
}
